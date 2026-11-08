package ops

import (
	"context"
	"sort"
	"strings"

	"nexmap.net/internal/client"
)

// CampaignID resolves a campaign by name, ID or ID prefix.
func (s *Session) CampaignID(ctx context.Context, ref string) (string, error) {
	campaigns, err := s.campaigns(ctx)
	if err != nil {
		return "", err
	}
	c, err := match("campaign", ref, campaigns, func(x client.Campaign) string { return x.ID },
		func(x client.Campaign) string { return x.Name },
		func(x client.Campaign) bool { return strings.EqualFold(x.Name, strings.TrimSpace(ref)) })
	if err != nil {
		return "", err
	}
	return c.ID, nil
}

// SyncResult reports what SyncTestCases created (or would create).
type SyncResult struct {
	DryRun   bool              `json:"dry_run"`
	Created  []TestCaseSummary `json:"created"`
	Existing int               `json:"existing"` // identifiers that already had a test case
}

// SyncTestCases creates an automatic test case for every identifier that has
// none yet (developer role), so a repository can bootstrap its test catalogue
// from a test run. Subtests ("TestX/case") are left out: they roll up into
// their parent. capability optionally files the new test cases under a
// capability (name or ID).
func (s *Session) SyncTestCases(ctx context.Context, identifiers []string, capability string, dryRun bool) (*SyncResult, error) {
	capID := ""
	if capability != "" {
		c, err := s.findCapability(ctx, capability)
		if err != nil {
			return nil, err
		}
		capID = c.ID
	}
	existing, err := s.C.TestCases(ctx, s.ProductID, "")
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	for _, tc := range existing {
		if tc.TestIdentifier != "" {
			known[tc.TestIdentifier] = true
		}
	}

	res := &SyncResult{DryRun: dryRun, Created: []TestCaseSummary{}}
	seen := map[string]bool{}
	var missing []string
	for _, id := range identifiers {
		if strings.Contains(id, "/") || seen[id] {
			continue
		}
		seen[id] = true
		if known[id] {
			res.Existing++
			continue
		}
		missing = append(missing, id)
	}
	sort.Strings(missing)
	for _, id := range missing {
		in := client.TestCaseCreate{Title: testTitle(id), ExecutionMode: "automatic", TestIdentifier: id, CapabilityID: capID}
		if dryRun {
			res.Created = append(res.Created, TestCaseSummary{Title: in.Title, Mode: in.ExecutionMode, TestIdentifier: id})
			continue
		}
		tc, err := s.C.CreateTestCase(ctx, s.ProductID, in)
		if err != nil {
			return res, err
		}
		sums, err := s.testCaseSummaries(ctx, []client.TestCase{*tc})
		if err != nil {
			return res, err
		}
		res.Created = append(res.Created, sums[0])
	}
	return res, nil
}

// testTitle derives a readable title from an identifier:
// "example.com/x/calc.TestAdd" → "TestAdd (calc)".
func testTitle(id string) string {
	dot := strings.LastIndex(id, ".")
	slash := strings.LastIndex(id, "/")
	if dot <= 0 || dot < slash {
		return id
	}
	pkg, name := id[:dot], id[dot+1:]
	if i := strings.LastIndex(pkg, "/"); i >= 0 {
		pkg = pkg[i+1:]
	}
	return name + " (" + pkg + ")"
}
