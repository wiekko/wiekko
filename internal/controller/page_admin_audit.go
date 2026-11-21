// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	admin "nexmap.net/web/templates/pages/admin"
)

// Audit renders GET /admin/audit: the whole audit trail, filterable, with the
// deleted products whose history is still kept.
func (c *PageAdminController) Audit(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	ctx := r.Context()
	filter := auditFilterFromQuery(r)

	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	props := admin.AuditProps{
		Meta:        templates.PageMeta{Title: "Audit Trail", CSRFToken: middleware.CSRFTokenFromContext(ctx)},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Filter: admin.AuditFilterValues{
			ProductID: filter.ProductID, EntityType: filter.EntityType, UserID: filter.UserID,
			Action: filter.Action, From: filter.From, To: filter.To,
		},
		Page: page,
	}
	if n := r.URL.Query().Get("purged"); n != "" {
		props.Notice = fmt.Sprintf("Purged %s audit entries of %s.", n, r.URL.Query().Get("purged_name"))
	}

	products, _ := c.product.List(ctx, user)
	deleted, _ := c.audit.AdminDeletedProducts(ctx, user)
	users, _ := c.user.List(ctx, store.UserFilter{})

	productNames := make(map[string]string, len(products)+len(deleted))
	live := make(map[string]bool, len(products))
	props.ProductOptions = []templates.SelectOption{{Value: "", Label: "All products"}}
	for _, p := range products {
		productNames[p.ID] = p.Name
		live[p.ID] = true
		props.ProductOptions = append(props.ProductOptions, templates.SelectOption{Value: p.ID, Label: p.Name})
	}
	for _, p := range deleted {
		label := deletedProductLabel(p)
		productNames[p.ID] = label
		props.ProductOptions = append(props.ProductOptions, templates.SelectOption{Value: p.ID, Label: label})
		props.Deleted = append(props.Deleted, admin.DeletedProductRow{
			ID: p.ID, Name: label, DeletedAt: formatTimestamp(p.DeletedAt), Entries: p.Entries,
			PurgeURL:  "/admin/audit/products/" + p.ID,
			ExportURL: "/admin/audit/export?product_id=" + url.QueryEscape(p.ID),
			FilterURL: "/admin/audit?product_id=" + url.QueryEscape(p.ID),
		})
	}

	names := make(map[string]string, len(users))
	props.UserOptions = []templates.SelectOption{{Value: "", Label: "All users"}}
	for _, u := range users {
		label := u.DisplayName
		if label == "" {
			label = u.Username
		}
		names[u.ID] = label
		props.UserOptions = append(props.UserOptions, templates.SelectOption{Value: u.ID, Label: label})
	}
	props.EntityTypeOptions = []templates.SelectOption{{Value: "", Label: "All types"}}
	for _, t := range auditEntityTypes {
		props.EntityTypeOptions = append(props.EntityTypeOptions, templates.SelectOption{Value: t, Label: auditTypeLabel(t)})
	}
	props.ActionOptions = []templates.SelectOption{{Value: "", Label: "All actions"}}
	for _, a := range auditActions {
		props.ActionOptions = append(props.ActionOptions, templates.SelectOption{Value: a, Label: a})
	}

	entries, total, err := c.audit.AdminList(ctx, user, filter, auditPageSize, (page-1)*auditPageSize)
	if err != nil {
		props.Error = errorText(err)
	}
	props.Total = total
	props.PageCount = (total + auditPageSize - 1) / auditPageSize

	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.EntityID)
	}
	gone, _ := c.audit.AdminDeletedEntities(ctx, user, ids)
	for _, e := range entries {
		row := admin.AuditRow{
			When:       formatTimestamp(e.CreatedAt),
			EntityType: auditTypeLabel(e.EntityType),
			EntityID:   e.EntityID,
			UserName:   userName(names, e.UserID),
			Action:     e.Action,
			Field:      e.FieldName,
			OldValue:   e.OldValue,
			NewValue:   e.NewValue,
		}
		if e.ProductID != "" {
			row.Product = productNames[e.ProductID]
			if row.Product == "" {
				row.Product = "Deleted product"
			}
			if live[e.ProductID] {
				row.ProductURL = "/products/" + e.ProductID
				if seg, ok := auditEntitySegments[e.EntityType]; ok && !gone[e.EntityID] {
					row.EntityURL = "/products/" + e.ProductID + "/" + seg + "/" + e.EntityID
				}
			}
		}
		props.Entries = append(props.Entries, row)
	}

	q := r.URL.Query()
	q.Del("page")
	q.Del("purged")
	q.Del("purged_name")
	base := "/admin/audit"
	pageURL := func(p int) string {
		v := url.Values{}
		for k, vs := range q {
			if len(vs) > 0 && vs[0] != "" {
				v.Set(k, vs[0])
			}
		}
		if p > 1 {
			v.Set("page", strconv.Itoa(p))
		}
		if enc := v.Encode(); enc != "" {
			return base + "?" + enc
		}
		return base
	}
	if page > 1 {
		props.PrevURL = pageURL(page - 1)
	}
	if page < props.PageCount {
		props.NextURL = pageURL(page + 1)
	}
	exp := url.Values{}
	for k, vs := range q {
		if len(vs) > 0 && vs[0] != "" {
			exp.Set(k, vs[0])
		}
	}
	exp.Set("format", "jsonl")
	props.ExportJSONL = "/admin/audit/export?" + exp.Encode()
	exp.Set("format", "csv")
	props.ExportCSV = "/admin/audit/export?" + exp.Encode()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = admin.AuditPage(props).Render(ctx, w)
}

// AuditExport handles GET /admin/audit/export: the filtered trail as a file.
func (c *PageAdminController) AuditExport(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	writeAuditExport(w, r, c.audit, user, func(err error) {
		http.Error(w, errorText(err), http.StatusBadRequest)
	})
}

// PurgeAudit handles DELETE /admin/audit/products/{productId}: it removes the
// kept history of a deleted product and sends the browser back to the page.
func (c *PageAdminController) PurgeAudit(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")
	name := productID
	if deleted, err := c.audit.AdminDeletedProducts(r.Context(), user); err == nil {
		for _, p := range deleted {
			if p.ID == productID {
				name = deletedProductLabel(p)
			}
		}
	}
	n, err := c.audit.AdminPurgeProduct(r.Context(), user, productID)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, service.ErrProductExists) {
			status = http.StatusConflict
		}
		http.Error(w, errorText(err), status)
		return
	}
	logAdminAction(r, "purge audit trail", "", "product_id", productID, "entries", n)
	w.Header().Set("HX-Redirect", "/admin/audit?purged="+strconv.FormatInt(n, 10)+"&purged_name="+url.QueryEscape(name))
	w.WriteHeader(http.StatusOK)
}

func deletedProductLabel(p *model.DeletedProduct) string {
	if p.Name == "" {
		return "Deleted product"
	}
	return p.Name + " (deleted)"
}

func auditTypeLabel(t string) string {
	switch t {
	case "test_case":
		return "test case"
	case "testing_campaign":
		return "campaign"
	case "pyramid_level":
		return "pyramid level"
	case "runtime_environment":
		return "environment"
	}
	return t
}
