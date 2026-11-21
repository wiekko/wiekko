package ops

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"nexmap.net/internal/client"
	"nexmap.net/internal/ingest"
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

// TestSync describes which test cases SyncTestCases creates and where it
// files them (FR-119).
type TestSync struct {
	// Identifiers are the test identifiers of a run. The MCP tool may give a
	// Report instead, which is parsed like `nexmap test sync` input.
	Identifiers     []string `json:"test_identifiers,omitempty" jsonschema:"test identifiers from a test run, e.g. example.com/app.TestAdd; give these or report"`
	Report          string   `json:"report,omitempty" jsonschema:"a test report to read the identifiers from: go test -json or -v output, or JUnit XML"`
	IdentifierStyle string   `json:"identifier_style,omitempty" jsonschema:"package (default: pkg/path.TestName) or name (TestName); must match how the CI uploads results"`
	Capability      string   `json:"capability,omitempty" jsonschema:"capability (name or ID) to file the new test cases under"`
	Component       string   `json:"component,omitempty" jsonschema:"component (name or ID) to file the new test cases under"`
	PyramidLevel    string   `json:"pyramid_level,omitempty" jsonschema:"pyramid level (name or ID) of the new test cases, e.g. unit"`
	Requirement     string   `json:"requirement,omitempty" jsonschema:"requirement the new test cases verify, e.g. FR-12"`
	Constraints     []string `json:"constraints,omitempty" jsonschema:"constraints the new test cases verify, e.g. SC-30"`
	DryRun          bool     `json:"dry_run,omitempty" jsonschema:"only list the test cases that would be created"`
}

// SyncTestCases creates an automatic test case for every identifier that has
// none yet (developer role), so a repository can bootstrap its test catalogue
// from a test run (FR-119). Subtests roll up into their parent and get no
// test case of their own. Every option is resolved before the first test case
// is created, so a bad name creates nothing.
func (s *Session) SyncTestCases(ctx context.Context, in TestSync) (*SyncResult, error) {
	identifiers := in.Identifiers
	if strings.TrimSpace(in.Report) != "" {
		if len(identifiers) > 0 {
			return nil, errors.New("give test identifiers or a report, not both")
		}
		style := cmp.Or(in.IdentifierStyle, ingest.StylePackage)
		if style != ingest.StylePackage && style != ingest.StyleName {
			return nil, errors.New(`identifier_style must be "package" or "name"`)
		}
		rep, _, err := ingest.Parse(strings.NewReader(in.Report), ingest.Options{Format: ingest.FormatAuto, IdentifierStyle: style})
		if err != nil {
			return nil, fmt.Errorf("read the report: %w", err)
		}
		for _, r := range rep.Results {
			identifiers = append(identifiers, r.Identifier)
		}
	}
	if len(identifiers) == 0 {
		return nil, errors.New("the report contains no tests")
	}

	base := client.TestCaseCreate{ExecutionMode: "automatic"}
	if in.Capability != "" {
		c, err := s.findCapability(ctx, in.Capability)
		if err != nil {
			return nil, err
		}
		base.CapabilityID = c.ID
	}
	if in.Component != "" {
		c, err := s.findComponent(ctx, in.Component)
		if err != nil {
			return nil, err
		}
		base.ComponentID = c.ID
	}
	if in.PyramidLevel != "" {
		levels, err := s.pyramidLevels(ctx)
		if err != nil {
			return nil, err
		}
		l, err := match("pyramid level", in.PyramidLevel, levels,
			func(l client.PyramidLevel) string { return l.ID },
			func(l client.PyramidLevel) string { return l.Name },
			func(l client.PyramidLevel) bool { return strings.EqualFold(l.Name, strings.TrimSpace(in.PyramidLevel)) })
		if err != nil {
			return nil, err
		}
		base.PyramidLevelID = l.ID
	}
	reqID := ""
	if in.Requirement != "" {
		r, err := s.requirement(ctx, in.Requirement, "requirement")
		if err != nil {
			return nil, err
		}
		reqID = r.ID
	}
	constraintIDs := make([]string, 0, len(in.Constraints))
	for _, ref := range in.Constraints {
		c, err := s.constraint(ctx, ref)
		if err != nil {
			return nil, err
		}
		constraintIDs = append(constraintIDs, c.ID)
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

	res := &SyncResult{DryRun: in.DryRun, Created: []TestCaseSummary{}}
	reported := map[string]bool{}
	for _, id := range identifiers {
		reported[strings.TrimSpace(id)] = true
	}
	seen := map[string]bool{}
	var missing []string
	for _, id := range identifiers {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] || isSubtest(id, reported) {
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
		body := base
		body.Title, body.TestIdentifier = testTitle(id), id
		if in.DryRun {
			res.Created = append(res.Created, TestCaseSummary{Title: body.Title, Mode: body.ExecutionMode, TestIdentifier: id})
			continue
		}
		tc, err := s.C.CreateTestCase(ctx, s.ProductID, body)
		if err != nil {
			return res, err
		}
		if reqID != "" {
			if err := s.C.LinkTestCaseRequirement(ctx, s.ProductID, tc.ID, reqID); err != nil {
				return res, fmt.Errorf("test case %s was created, but linking it to %s failed: %w", id, in.Requirement, err)
			}
		}
		for i, conID := range constraintIDs {
			if err := s.C.LinkTestCaseConstraint(ctx, s.ProductID, tc.ID, conID); err != nil {
				return res, fmt.Errorf("test case %s was created, but linking it to %s failed: %w", id, in.Constraints[i], err)
			}
		}
		sums, err := s.testCaseSummaries(ctx, []client.TestCase{*tc})
		if err != nil {
			return res, err
		}
		res.Created = append(res.Created, sums[0])
	}
	return res, nil
}

// isSubtest reports whether id is a subtest of another reported test. Go
// reports a subtest as "<parent>/<case>" next to its parent; package
// identifiers contain slashes of their own (example.com/app.TestX), so an
// identifier only counts as a subtest when its parent was reported too.
func isSubtest(id string, reported map[string]bool) bool {
	for p := id; strings.Contains(p, "/"); {
		p = p[:strings.LastIndex(p, "/")]
		if reported[p] {
			return true
		}
	}
	return false
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
