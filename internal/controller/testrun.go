// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"encoding/json"
	"errors"
	"net/http"

	"nexmap.net/internal/httputil"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

// TestRunController handles automated test run uploads (FR-118).
type TestRunController struct {
	runs *service.TestRunService
}

// NewTestRun creates a TestRunController.
func NewTestRun(runs *service.TestRunService) *TestRunController {
	return &TestRunController{runs: runs}
}

type testRunResultJSON struct {
	TestIdentifier string `json:"test_identifier"`
	Status         string `json:"status"`
	Output         string `json:"output,omitempty"`
	DurationMS     int64  `json:"duration_ms,omitempty"`
}

// Ingest handles POST /products/{productId}/test-runs.
func (c *TestRunController) Ingest(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("productId")

	// The body is capped at middleware.MaxTestRunBody by the LimitBody middleware.
	var req struct {
		Source      string              `json:"source"`
		CampaignID  string              `json:"campaign_id"`
		Environment string              `json:"environment"`
		Revision    string              `json:"revision"`
		Branch      string              `json:"branch"`
		DryRun      bool                `json:"dry_run"`
		Results     []testRunResultJSON `json:"results"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			httputil.Error(w, http.StatusRequestEntityTooLarge, "TOO_LARGE", "test run upload exceeds 32 MiB")
			return
		}
		httputil.Error(w, http.StatusBadRequest, "INVALID_JSON", "Invalid JSON request body")
		return
	}

	in := service.TestRunInput{
		ProductID: productID, CampaignID: req.CampaignID, Source: req.Source,
		Environment: req.Environment, Revision: req.Revision, Branch: req.Branch, DryRun: req.DryRun,
		Results: make([]service.TestRunResult, 0, len(req.Results)),
	}
	for _, res := range req.Results {
		in.Results = append(in.Results, service.TestRunResult{
			TestIdentifier: res.TestIdentifier, Status: res.Status, Output: res.Output, DurationMS: res.DurationMS,
		})
	}

	report, err := c.runs.IngestRun(r.Context(), user, in)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	httputil.JSON(w, http.StatusOK, testRunReportToJSON(report))
}

func testRunReportToJSON(rep *service.TestRunReport) map[string]any {
	results := make([]map[string]any, 0, len(rep.Outcomes))
	for _, o := range rep.Outcomes {
		results = append(results, map[string]any{
			"test_identifier":   o.TestIdentifier,
			"reported":          o.Reported,
			"status":            o.Status,
			"outcome":           o.Outcome,
			"test_case_id":      o.TestCaseID,
			"result_id":         o.ResultID,
			"previous_status":   o.PreviousStatus,
			"defect_id":         o.DefectID,
			"defect_identifier": o.DefectIdentifier,
			"defect":            o.Defect,
		})
	}
	var campaign any
	if rep.Campaign != nil {
		campaign = map[string]any{"id": rep.Campaign.ID, "name": rep.Campaign.Name}
	}
	env := ""
	if rep.Environment != nil {
		env = rep.Environment.Name
	}
	return map[string]any{
		"recorded":    rep.Recorded,
		"dry_run":     rep.DryRun,
		"campaign":    campaign,
		"environment": env,
		"summary":     rep.Summary(),
		"results":     results,
	}
}
