package ops

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"

	"nexmap.net/internal/client"
)

// criticalityRank orders defects most severe first.
var criticalityRank = map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}

// DefectQuery filters ListDefects.
type DefectQuery struct {
	Open        bool   `json:"open,omitempty" jsonschema:"only defects whose status is not a closed one"`
	Criticality string `json:"criticality,omitempty" jsonschema:"critical, high, medium or low"`
	Status      string `json:"status,omitempty" jsonschema:"status name (case-insensitive)"`
	Search      string `json:"search,omitempty" jsonschema:"case-insensitive text in title"`
	Limit       int    `json:"limit,omitempty" jsonschema:"return at most this many items; 0 for all"`
}

// ListDefects lists defects, most critical first, then by priority.
func (s *Session) ListDefects(ctx context.Context, q DefectQuery) ([]DefectSummary, error) {
	if q.Criticality != "" {
		if _, ok := criticalityRank[q.Criticality]; !ok {
			return nil, fmt.Errorf("unknown criticality %q (want critical, high, medium or low)", q.Criticality)
		}
	}
	var statusID string
	if q.Status != "" {
		var err error
		if statusID, err = s.statusID(ctx, "defect", q.Status); err != nil {
			return nil, err
		}
	}
	items, err := s.C.Defects(ctx, s.ProductID)
	if err != nil {
		return nil, err
	}
	all, err := s.defectSummaries(ctx, items)
	if err != nil {
		return nil, err
	}
	out := make([]DefectSummary, 0, len(all))
	for i, d := range all {
		if q.Open && d.StatusClosed || q.Criticality != "" && d.Criticality != q.Criticality ||
			statusID != "" && items[i].StatusID != statusID {
			continue
		}
		if q.Search != "" && !containsFold(d.Title, q.Search) {
			continue
		}
		out = append(out, d)
	}
	sortDefects(out)
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// ShowDefect returns one defect by ID or unique ID prefix (at least 4
// characters, e.g. the 8-character short ID).
func (s *Session) ShowDefect(ctx context.Context, ref string) (*DefectDetail, error) {
	d, err := s.findDefect(ctx, ref)
	if err != nil {
		return nil, err
	}
	sums, err := s.defectSummaries(ctx, []client.Defect{*d})
	if err != nil {
		return nil, err
	}
	out := &DefectDetail{DefectSummary: sums[0], Description: d.Description, Links: []LinkView{}}
	if d.MilestoneID != "" {
		ms, err := s.milestoneNames(ctx)
		if err != nil {
			return nil, err
		}
		out.Milestone = ms[d.MilestoneID]
	}
	if d.SprintID != "" {
		sp, err := s.sprintNames(ctx)
		if err != nil {
			return nil, err
		}
		out.Sprint = sp[d.SprintID]
	}

	links, err := s.C.DefectLinks(ctx, s.ProductID, d.ID)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		label, err := s.linkLabel(ctx, l)
		if err != nil {
			return nil, err
		}
		out.Links = append(out.Links, LinkView{Type: l.TargetType, ID: l.TargetID, Label: label})
	}
	if out.Assignees, err = s.assignees(ctx, func() ([]client.Assignment, error) {
		return s.C.DefectAssignees(ctx, s.ProductID, d.ID)
	}); err != nil {
		return nil, err
	}
	if out.Comments, err = s.comments(ctx, "defect", d.ID); err != nil {
		return nil, err
	}
	return out, nil
}

// findTestCase matches a test case by test identifier, short ID or ID.
func (s *Session) findTestCase(ctx context.Context, ref string) (client.TestCase, error) {
	list, err := s.C.TestCases(ctx, s.ProductID, "")
	if err != nil {
		return client.TestCase{}, err
	}
	return match("test case", ref, list,
		func(t client.TestCase) string { return t.ID },
		func(t client.TestCase) string { return ShortID(t.ID) + " " + t.Title },
		func(t client.TestCase) bool {
			return t.TestIdentifier != "" && t.TestIdentifier == strings.TrimSpace(ref)
		})
}

func (s *Session) findDefect(ctx context.Context, ref string) (*client.Defect, error) {
	ref = strings.TrimSpace(ref)
	// An identifier (DF-17) or a full ID is fetched directly, the server
	// resolving the identifier; anything else is matched as an ID prefix.
	if identifierPattern.MatchString(ref) || len(ref) == 36 {
		d, err := s.C.Defect(ctx, s.ProductID, ref)
		if err == nil {
			return d, nil
		}
		if !client.IsNotFound(err) {
			return nil, err
		}
		return nil, &NotFoundError{Kind: "defect", Ref: ref}
	}
	list, err := s.C.Defects(ctx, s.ProductID)
	if err != nil {
		return nil, err
	}
	d, err := match("defect", ref, list,
		func(d client.Defect) string { return d.ID },
		func(d client.Defect) string { return d.Identifier + " " + d.Title }, nil)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// defectIdentifiers maps the product's defect IDs to their identifiers. It
// is best effort: a token without the defects scope gets an empty map, and
// callers fall back to the ID.
func (s *Session) defectIdentifiers(ctx context.Context) map[string]string {
	m := map[string]string{}
	list, err := s.C.Defects(ctx, s.ProductID)
	if err != nil {
		return m
	}
	for _, d := range list {
		m[d.ID] = d.Identifier
	}
	return m
}

func (s *Session) linkLabel(ctx context.Context, l client.DefectLink) (string, error) {
	switch l.TargetType {
	case "requirement":
		r, err := s.C.Requirement(ctx, s.ProductID, l.TargetID)
		if err != nil {
			if client.IsNotFound(err) {
				return "(deleted)", nil
			}
			return "", err
		}
		return strings.TrimSpace(r.Identifier + " " + r.Title), nil
	case "test_case":
		tc, err := s.C.TestCase(ctx, s.ProductID, l.TargetID)
		if err != nil {
			if client.IsNotFound(err) {
				return "(deleted)", nil
			}
			return "", err
		}
		return tc.Title, nil
	case "component":
		m, err := s.componentNames(ctx)
		return m[l.TargetID], err
	case "capability":
		m, err := s.capabilityNames(ctx)
		return m[l.TargetID], err
	}
	return "", nil
}

func (s *Session) defectSummaries(ctx context.Context, items []client.Defect) ([]DefectSummary, error) {
	statuses, err := s.statuses(ctx)
	if err != nil {
		return nil, err
	}
	var envs map[string]string
	out := make([]DefectSummary, 0, len(items))
	for _, d := range items {
		sum := DefectSummary{
			ID: d.ID, Identifier: d.Identifier, Title: d.Title, Criticality: d.Criticality,
			Priority: d.Priority, Active: d.IsActive,
		}
		if st, ok := statuses[d.StatusID]; ok {
			sum.Status, sum.StatusClosed = st.Name, st.IsClosed
		}
		if d.EnvironmentID != "" {
			if envs == nil {
				if envs, err = s.environmentNames(ctx); err != nil {
					return nil, err
				}
			}
			sum.Environment = envs[d.EnvironmentID]
		}
		out = append(out, sum)
	}
	return out, nil
}

func sortDefects(list []DefectSummary) {
	sort.SliceStable(list, func(i, j int) bool {
		ri, rj := rank(list[i].Criticality), rank(list[j].Criticality)
		if ri != rj {
			return ri < rj
		}
		return priorityRank(list[i].Priority) < priorityRank(list[j].Priority)
	})
}

// priorityRank orders priorities lowest number first; 0 means none set and
// ranks last (DF-2), as in the server's list order.
func priorityRank(p int) int {
	if p > 0 {
		return p
	}
	return math.MaxInt32
}

func rank(criticality string) int {
	if r, ok := criticalityRank[criticality]; ok {
		return r
	}
	return len(criticalityRank)
}

// TestCaseQuery filters ListTestCases.
type TestCaseQuery struct {
	Mode       string `json:"mode,omitempty" jsonschema:"manual or automatic; omit for both"`
	Component  string `json:"component,omitempty" jsonschema:"component name or ID"`
	Capability string `json:"capability,omitempty" jsonschema:"capability name or ID"`
	Search     string `json:"search,omitempty" jsonschema:"case-insensitive text in title or test identifier"`
	Limit      int    `json:"limit,omitempty" jsonschema:"return at most this many items; 0 for all"`
}

// ListTestCases lists test cases.
func (s *Session) ListTestCases(ctx context.Context, q TestCaseQuery) ([]TestCaseSummary, error) {
	if q.Mode != "" && q.Mode != "manual" && q.Mode != "automatic" {
		return nil, fmt.Errorf("unknown execution mode %q (want manual or automatic)", q.Mode)
	}
	var compID, capID string
	if q.Component != "" {
		comps, err := s.components(ctx)
		if err != nil {
			return nil, err
		}
		c, err := match("component", q.Component, comps, func(c client.Component) string { return c.ID },
			func(c client.Component) string { return c.Name },
			func(c client.Component) bool { return strings.EqualFold(c.Name, q.Component) })
		if err != nil {
			return nil, err
		}
		compID = c.ID
	}
	if q.Capability != "" {
		c, err := s.findCapability(ctx, q.Capability)
		if err != nil {
			return nil, err
		}
		capID = c.ID
	}
	items, err := s.C.TestCases(ctx, s.ProductID, "")
	if err != nil {
		return nil, err
	}
	var filtered []client.TestCase
	for _, tc := range items {
		if q.Mode != "" && tc.ExecutionMode != q.Mode || compID != "" && tc.ComponentID != compID || capID != "" && tc.CapabilityID != capID {
			continue
		}
		if q.Search != "" && !containsFold(tc.Title, q.Search) && !containsFold(tc.TestIdentifier, q.Search) {
			continue
		}
		filtered = append(filtered, tc)
		if q.Limit > 0 && len(filtered) >= q.Limit {
			break
		}
	}
	return s.testCaseSummaries(ctx, filtered)
}

// ShowTestCase returns one test case by ID, unique ID prefix, or exact test
// identifier (e.g. a Go test function name).
func (s *Session) ShowTestCase(ctx context.Context, ref string) (*TestCaseDetail, error) {
	tc, err := s.findTestCase(ctx, ref)
	if err != nil {
		return nil, err
	}
	sums, err := s.testCaseSummaries(ctx, []client.TestCase{tc})
	if err != nil {
		return nil, err
	}
	d := &TestCaseDetail{TestCaseSummary: sums[0], Description: tc.Description, Requirements: []CoverageRef{}, Environments: []string{}, Results: []ResultView{}, Documents: []Ref{}}
	// Lists carry no links; the single test case names its documents.
	if full, err := s.C.TestCase(ctx, s.ProductID, tc.ID); err == nil {
		d.Documents = documentRefs(full.Documents)
	}

	reqLinks, err := s.C.TestCaseRequirements(ctx, s.ProductID, tc.ID)
	if err != nil {
		return nil, err
	}
	for _, l := range reqLinks {
		ref := Ref{ID: l.RequirementID}
		if r, err := s.C.Requirement(ctx, s.ProductID, l.RequirementID); err == nil {
			ref.Identifier, ref.Title = r.Identifier, r.Title
		} else if !client.IsNotFound(err) {
			return nil, err
		}
		d.Requirements = append(d.Requirements, CoverageRef{Ref: ref, LinkType: l.LinkType})
	}

	envNames, err := s.environmentNames(ctx)
	if err != nil {
		return nil, err
	}
	envIDs, err := s.C.TestCaseEnvironments(ctx, s.ProductID, tc.ID)
	if err != nil {
		return nil, err
	}
	for _, id := range envIDs {
		d.Environments = append(d.Environments, envNames[id])
	}

	results, err := s.C.TestCaseResults(ctx, s.ProductID, tc.ID)
	if err != nil {
		return nil, err
	}
	campaigns, err := s.campaigns(ctx)
	if err != nil {
		return nil, err
	}
	byID := map[string]client.Campaign{}
	for _, c := range campaigns {
		byID[c.ID] = c
	}
	defectIDs := s.defectIdentifiers(ctx)
	for _, r := range results {
		c := byID[r.CampaignID]
		d.Results = append(d.Results, ResultView{
			Campaign: c.Name, CampaignStatus: c.Status, Environment: resultEnvironment(r, envNames),
			Status: r.Status, DefectID: r.DefectID, Defect: defectIDs[r.DefectID], ExecutedAt: r.ExecutedAt,
		})
	}

	defects, err := s.C.TestCaseDefects(ctx, s.ProductID, tc.ID)
	if err != nil {
		return nil, err
	}
	if d.Defects, err = s.defectSummaries(ctx, defects); err != nil {
		return nil, err
	}
	if d.Comments, err = s.comments(ctx, "test_case", tc.ID); err != nil {
		return nil, err
	}
	return d, nil
}

func (s *Session) testCaseSummaries(ctx context.Context, items []client.TestCase) ([]TestCaseSummary, error) {
	out := make([]TestCaseSummary, 0, len(items))
	if len(items) == 0 {
		return out, nil
	}
	comps, err := s.componentNames(ctx)
	if err != nil {
		return nil, err
	}
	caps, err := s.capabilityNames(ctx)
	if err != nil {
		return nil, err
	}
	levels, err := s.pyramidNames(ctx)
	if err != nil {
		return nil, err
	}
	for _, tc := range items {
		out = append(out, TestCaseSummary{
			ID: tc.ID, ShortID: ShortID(tc.ID), Title: tc.Title, Mode: tc.ExecutionMode,
			TestIdentifier: tc.TestIdentifier, Component: comps[tc.ComponentID],
			Capability: caps[tc.CapabilityID], PyramidLevel: levels[tc.PyramidLevelID],
		})
	}
	return out, nil
}

// CampaignStatus reports the active campaign, or the named one (name, ID or ID
// prefix), with result counts, verdict and the results that need attention.
func (s *Session) CampaignStatus(ctx context.Context, ref string) (*CampaignStatusView, error) {
	campaigns, err := s.campaigns(ctx)
	if err != nil {
		return nil, err
	}
	var c client.Campaign
	if ref == "" {
		found := false
		for _, x := range campaigns {
			if x.Status == "active" {
				c, found = x, true
				break
			}
		}
		if !found {
			return &CampaignStatusView{Message: "no testing campaign is active"}, nil
		}
	} else {
		if c, err = match("campaign", ref, campaigns, func(x client.Campaign) string { return x.ID },
			func(x client.Campaign) string { return x.Name },
			func(x client.Campaign) bool { return strings.EqualFold(x.Name, ref) }); err != nil {
			return nil, err
		}
	}

	view, err := s.campaignView(ctx, c)
	if err != nil {
		return nil, err
	}
	results, err := s.C.CampaignResults(ctx, s.ProductID, c.ID)
	if err != nil {
		return nil, err
	}
	var tcByID map[string]client.TestCase
	var defectIDs map[string]string // read once, only if a result names a defect
	envNames, err := s.environmentNames(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range results {
		if r.Status != "failed" && r.Status != "blocked" && r.Status != "retest" {
			continue
		}
		if tcByID == nil {
			list, err := s.C.TestCases(ctx, s.ProductID, "")
			if err != nil {
				return nil, err
			}
			tcByID = map[string]client.TestCase{}
			for _, tc := range list {
				tcByID[tc.ID] = tc
			}
		}
		tc := tcByID[r.TestCaseID]
		if r.DefectID != "" && defectIDs == nil {
			defectIDs = s.defectIdentifiers(ctx)
		}
		title := r.TestTitle
		if r.TestCaseID == "" {
			title += " (deleted)"
		}
		view.Attention = append(view.Attention, ResultView{
			Campaign: c.Name, CampaignStatus: c.Status,
			TestCase:       &Ref{ID: r.TestCaseID, Title: title},
			TestIdentifier: tc.TestIdentifier, Environment: resultEnvironment(r, envNames),
			Status: r.Status, DefectID: r.DefectID, Defect: defectIDs[r.DefectID], ExecutedAt: r.ExecutedAt,
		})
	}
	return &CampaignStatusView{Campaign: view}, nil
}

func (s *Session) campaignView(ctx context.Context, c client.Campaign) (*CampaignView, error) {
	p, err := s.C.CampaignProgress(ctx, s.ProductID, c.ID)
	if err != nil {
		return nil, err
	}
	total := 0
	for _, n := range p.Counts {
		total += n
	}
	if p.Counts == nil {
		p.Counts = map[string]int{}
	}
	return &CampaignView{
		ID: c.ID, Name: c.Name, Status: c.Status, StartsAt: c.StartsAt, EndsAt: c.EndsAt,
		Verdict: p.Verdict, Total: total, Counts: p.Counts,
	}, nil
}

// resultEnvironment labels a campaign result's environment, marking one that
// has been deleted since by the name the result recorded.
func resultEnvironment(r client.CampaignResult, envNames map[string]string) string {
	if name, ok := envNames[r.EnvironmentID]; ok && r.EnvironmentID != "" {
		return name
	}
	if r.EnvironmentName != "" {
		return r.EnvironmentName + " (deleted)"
	}
	return ""
}
