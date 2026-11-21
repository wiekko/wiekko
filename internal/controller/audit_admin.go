// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// auditPageSize is the number of entries per page of the admin audit view.
const auditPageSize = 50

// auditFilterFromQuery reads the admin audit filter from a query string; the
// web page and the API share the parameter names.
func auditFilterFromQuery(r *http.Request) service.AuditFilterInput {
	q := r.URL.Query()
	return service.AuditFilterInput{
		ProductID:  q.Get("product_id"),
		EntityType: q.Get("entity_type"),
		UserID:     q.Get("user_id"),
		Action:     q.Get("action"),
		From:       q.Get("from"),
		To:         q.Get("to"),
	}
}

// auditEntitySegments maps audit entity types that have a page of their own
// to the URL segment under /products/{id}.
var auditEntitySegments = map[string]string{
	"requirement":      "requirements",
	"task":             "tasks",
	"constraint":       "constraints",
	"test_case":        "test-cases",
	"component":        "components",
	"capability":       "capabilities",
	"milestone":        "milestones",
	"sprint":           "sprints",
	"defect":           "defects",
	"document":         "documents",
	"testing_campaign": "campaigns",
}

// auditEntityTypes lists the entity types an administrator can filter by.
var auditEntityTypes = []string{
	"product", "component", "capability", "requirement", "task", "constraint",
	"document", "defect", "test_case", "testing_campaign", "milestone", "sprint",
	"category", "status", "pyramid_level", "runtime_environment", "comment", "attachment",
}

// auditActions lists the actions the trail records, for the filter.
var auditActions = []string{
	"create", "update", "delete", "link", "unlink", "restore", "start", "close", "reopen",
	"split_statuses", "purge_audit",
}

// writeAuditExport streams the entries matching the request's filter as
// JSON Lines (the default) or CSV. Once the first byte is out the status is
// fixed, so a failure part way is only logged and cuts the file short.
func writeAuditExport(w http.ResponseWriter, r *http.Request, audit *service.AuditService, user *model.User, onError func(error)) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "jsonl"
	}
	if format != "jsonl" && format != "csv" {
		onError(fmt.Errorf("format must be jsonl or csv"))
		return
	}
	filter := auditFilterFromQuery(r)
	scope := "all"
	if filter.ProductID != "" {
		scope = filter.ProductID
	}
	name := fmt.Sprintf("audit-%s-%s.%s", scope, time.Now().UTC().Format("20060102"), format)

	started := false
	start := func() {
		if started {
			return
		}
		started = true
		if format == "csv" {
			w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
		}
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
	}

	var cw *csv.Writer
	enc := json.NewEncoder(w)
	err := audit.AdminExport(r.Context(), user, filter, func(e *model.AuditEntry) error {
		if !started {
			start()
			if format == "csv" {
				cw = csv.NewWriter(w)
				if err := cw.Write(auditCSVHeader); err != nil {
					return err
				}
			}
		}
		if format == "csv" {
			return cw.Write(auditCSVRow(e))
		}
		return enc.Encode(auditEntryToJSON(e))
	})
	if cw != nil {
		cw.Flush()
		if err == nil {
			err = cw.Error()
		}
	}
	if err != nil {
		if !started {
			onError(err)
			return
		}
		slog.Error("audit export cut short", "err", err)
		return
	}
	if !started {
		// Nothing matched: still an (empty) file, with the CSV header.
		start()
		if format == "csv" {
			cw = csv.NewWriter(w)
			_ = cw.Write(auditCSVHeader)
			cw.Flush()
		}
	}
}

var auditCSVHeader = []string{"id", "product_id", "entity_type", "entity_id", "user_id", "action", "field_name", "old_value", "new_value", "created_at"}

func auditCSVRow(e *model.AuditEntry) []string {
	return []string{e.ID, e.ProductID, e.EntityType, e.EntityID, e.UserID, e.Action, e.FieldName, csvSafe(e.OldValue), csvSafe(e.NewValue), e.CreatedAt}
}

// csvSafe defuses values a spreadsheet would run as a formula: field values
// are user input, and an archive is likely to be opened in one.
func csvSafe(v string) string {
	if v != "" && strings.ContainsAny(v[:1], "=+-@\t\r") {
		return "'" + v
	}
	return v
}
