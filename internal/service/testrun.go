package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// Limits of one automated test run upload (FR-409).
const (
	// MaxTestRunResults caps the number of results in one upload.
	MaxTestRunResults = 50000
	// MaxTestOutput is how much captured output of one failing test is kept
	// in its defect. The tail is kept: that is where assertion failures and
	// panics usually are.
	MaxTestOutput = 64 << 10
)

// Outcomes of one reported test in an ingested run.
const (
	TestRunUpdated             = "updated"              // result status changed
	TestRunUnchanged           = "unchanged"            // result already had this status
	TestRunUnknownTestCase     = "unknown_test_case"    // no automatic test case has this identifier
	TestRunNotInCampaign       = "not_in_campaign"      // test case (or environment) outside the campaign's scope
	TestRunEnvironmentRequired = "environment_required" // test case runs per environment, but the run named none
	TestRunNotRecorded         = "not_recorded"         // no active campaign: nothing is written
)

// Defect handling for a failed result.
const (
	TestRunDefectCreated = "created"
	TestRunDefectReused  = "reused"
)

// TestRunInput is one automated test run, as uploaded by `nexmap utest`.
type TestRunInput struct {
	ProductID   string
	CampaignID  string // optional; default: the product's active campaign
	Source      string // e.g. "go-test", "junit"
	Environment string // optional runtime environment name
	Revision    string
	Branch      string
	DryRun      bool // evaluate and report, write nothing
	Results     []TestRunResult
}

// TestRunResult is one test as reported by a test runner.
type TestRunResult struct {
	TestIdentifier string
	Status         string // passed, failed or skipped
	Output         string // captured output; only kept for failures
	DurationMS     int64
}

// TestRunOutcome reports what happened to one test case of the run. Results
// rolled up into a registered parent (subtests) appear once, under the
// parent's identifier.
type TestRunOutcome struct {
	TestIdentifier string
	Reported       int    // number of reported results merged into this one
	Status         string // passed, failed or skipped, after roll-up
	Outcome        string // one of the TestRun* outcome constants
	TestCaseID     string
	ResultID       string
	PreviousStatus string
	DefectID       string
	// DefectIdentifier is the defect's identifier, e.g. DF-17 (FR-043).
	DefectIdentifier string
	Defect           string // TestRunDefectCreated, TestRunDefectReused or empty
}

// TestRunReport is the result of IngestRun.
type TestRunReport struct {
	Recorded    bool // false when nothing was written (no active campaign, or a dry run)
	DryRun      bool
	Campaign    *model.TestingCampaign // nil without an active campaign
	Environment *model.RuntimeEnvironment
	Outcomes    []TestRunOutcome
}

// TestRunService ingests automated test results into campaigns (FR-018,
// FR-409). It sits next to the testing and backlog services: it writes
// campaign results like a tester does, and files defects for failures
// through the backlog service so they follow the same rules as any other
// defect (FR-502).
type TestRunService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx           store.Transactor
	testCases    store.TestCaseStore
	campaigns    store.TestingCampaignStore
	results      store.CampaignResultStore
	events       store.CampaignResultEventStore
	environments store.RuntimeEnvironmentStore
	defects      store.DefectStore
	statuses     store.StatusStore
	backlog      *BacklogService
	authz        *Authorizer
	owns         *ownership
}

// IngestRun records an automated test run in a campaign. Requires tester role.
//
// Without an explicit campaign the product's active campaign is used; when
// there is none, the run is evaluated and reported but nothing is written —
// no results, events or defects — so CI keeps working between campaigns and
// results only ever change inside an active campaign (FR-505).
//
// Each result is applied in its own transaction — the defect filed for it,
// the defect's link to the test case, the result and its event stand or fall
// together — rather than the whole run in one, which would hold the campaign
// for as long as a large suite takes. Every step is idempotent (unchanged
// statuses are skipped, an open defect on the result is reused), so a run
// that fails half way is completed by sending it again.
func (s *TestRunService) IngestRun(ctx context.Context, user *model.User, in TestRunInput) (*TestRunReport, error) {
	if _, err := s.authz.RequireRole(ctx, user, in.ProductID, model.RoleTester); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, in.ProductID, ref{kindCampaign, in.CampaignID}); err != nil {
		return nil, err
	}
	if err := validateTestRun(in); err != nil {
		return nil, err
	}

	report := &TestRunReport{DryRun: in.DryRun}
	if in.Environment != "" {
		env, err := s.environment(ctx, in.ProductID, in.Environment)
		if err != nil {
			return nil, err
		}
		report.Environment = env
	}

	campaign, err := s.campaign(ctx, in.ProductID, in.CampaignID)
	if err != nil {
		return nil, err
	}
	report.Campaign = campaign

	testCases, err := s.automaticTestCases(ctx, in.ProductID)
	if err != nil {
		return nil, err
	}
	groups := rollUp(in.Results, testCases)

	var rows map[string][]*model.CampaignResult
	if campaign != nil {
		list, err := s.results.ListByCampaignID(ctx, campaign.ID)
		if err != nil {
			return nil, fmt.Errorf("list campaign results: %w", err)
		}
		rows = make(map[string][]*model.CampaignResult)
		for _, r := range list {
			rows[r.TestCaseID] = append(rows[r.TestCaseID], r)
		}
	}

	write := campaign != nil && !in.DryRun
	now := time.Now().UTC().Format(time.RFC3339)
	for _, g := range groups {
		o := TestRunOutcome{TestIdentifier: g.identifier, Reported: g.reported, Status: g.status}
		tc := testCases[g.identifier]
		switch {
		case tc == nil:
			o.Outcome = TestRunUnknownTestCase
		case campaign == nil:
			o.TestCaseID, o.Outcome = tc.ID, TestRunNotRecorded
		default:
			o.TestCaseID = tc.ID
			row, outcome := pickResult(rows[tc.ID], report.Environment)
			if row == nil {
				o.Outcome = outcome
				break
			}
			o.ResultID, o.PreviousStatus = row.ID, row.Status
			if row.Status == g.status {
				o.Outcome, o.DefectID = TestRunUnchanged, row.DefectID
				break
			}
			o.Outcome = TestRunUpdated
			if err := inTx(ctx, s.tx, func(ctx context.Context) error {
				if write {
					if err := s.holdActive(ctx, campaign.ID); err != nil {
						return err
					}
				}
				return s.apply(ctx, user, in, report, campaign, tc, g, row, &o, write, now)
			}); err != nil {
				return nil, fmt.Errorf("record %s: %w", g.identifier, err)
			}
		}
		report.Outcomes = append(report.Outcomes, o)
	}
	report.Recorded = write
	s.fillDefectIdentifiers(ctx, report)
	return report, nil
}

// holdActive takes the campaign's lock for the current transaction and
// checks that the campaign is still active: it may have been closed since the
// run started.
func (s *TestRunService) holdActive(ctx context.Context, campaignID string) error {
	if err := lock(ctx, s.tx, lockCampaign(campaignID)); err != nil {
		return err
	}
	c, err := s.campaigns.GetByID(ctx, campaignID)
	if err != nil {
		return err
	}
	if c.Status != model.CampaignStatusActive {
		return fmt.Errorf("the campaign was %s while the run was recorded", c.Status)
	}
	return nil
}

// fillDefectIdentifiers adds the identifiers of the defects the outcomes name.
// A defect that cannot be read keeps its ID only; the run is recorded anyway.
func (s *TestRunService) fillDefectIdentifiers(ctx context.Context, report *TestRunReport) {
	seen := map[string]string{}
	for i := range report.Outcomes {
		o := &report.Outcomes[i]
		if o.DefectID == "" {
			continue
		}
		ident, ok := seen[o.DefectID]
		if !ok {
			if d, err := s.defects.GetByID(ctx, o.DefectID); err == nil {
				ident = d.Identifier
			}
			seen[o.DefectID] = ident
		}
		o.DefectIdentifier = ident
	}
}

// apply changes one result's status, filing or reusing a defect for a
// failure. Without write it only fills in what would happen.
func (s *TestRunService) apply(ctx context.Context, user *model.User, in TestRunInput, report *TestRunReport,
	campaign *model.TestingCampaign, tc *model.TestCase, g testGroup, row *model.CampaignResult, o *TestRunOutcome, write bool, now string) error {
	defectID := ""
	if g.status == model.CampaignResultStatusFailed {
		open, err := s.openDefect(ctx, row.DefectID)
		if err != nil {
			return err
		}
		switch {
		case open:
			defectID, o.Defect = row.DefectID, TestRunDefectReused
		case !write:
			o.Defect = TestRunDefectCreated
		default:
			d, err := s.fileDefect(ctx, user, in, report, campaign, tc, g)
			if err != nil {
				return err
			}
			defectID, o.Defect = d.ID, TestRunDefectCreated
		}
		o.DefectID = defectID
	}
	if !write {
		return nil
	}

	from := row.Status
	row.Status = g.status
	// Same rule as a tester's verdict: passing or skipping clears the defect.
	row.DefectID = defectID
	row.ExecutedBy = user.ID
	row.ExecutedAt = now
	row.UpdatedAt = now
	if err := s.results.Update(ctx, row); err != nil {
		return fmt.Errorf("update campaign result: %w", err)
	}
	return s.events.Create(ctx, &model.CampaignResultEvent{
		ID:         uuid.New().String(),
		ResultID:   row.ID,
		CampaignID: row.CampaignID,
		UserID:     user.ID,
		FromStatus: from,
		ToStatus:   row.Status,
		DefectID:   row.DefectID,
		Source:     in.Source,
		Revision:   in.Revision,
		Branch:     in.Branch,
		CreatedAt:  now,
	})
}

// fileDefect creates the defect explaining an automated failure and links it
// to the test case (FR-502, FR-023).
func (s *TestRunService) fileDefect(ctx context.Context, user *model.User, in TestRunInput, report *TestRunReport,
	campaign *model.TestingCampaign, tc *model.TestCase, g testGroup) (*model.Defect, error) {
	envID := ""
	if report.Environment != nil {
		envID = report.Environment.ID
	}
	d, err := s.backlog.CreateDefect(ctx, user, CreateDefectInput{
		ProductID:     in.ProductID,
		Title:         "Automated test failed: " + g.identifier,
		Description:   defectDescription(in, report, campaign, g),
		Criticality:   model.DefectCriticalityMedium,
		EnvironmentID: envID,
	})
	if err != nil {
		return nil, fmt.Errorf("create defect: %w", err)
	}
	if err := s.backlog.AddDefectLink(ctx, user, in.ProductID, d.ID, model.DefectLinkTargetTestCase, tc.ID); err != nil {
		return nil, fmt.Errorf("link defect to test case: %w", err)
	}
	return d, nil
}

// openDefect reports whether the defect exists and its status is not closed.
func (s *TestRunService) openDefect(ctx context.Context, id string) (bool, error) {
	if id == "" {
		return false, nil
	}
	d, err := s.defects.GetByID(ctx, id)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if d.StatusID == "" {
		return true, nil
	}
	st, err := s.statuses.GetByID(ctx, d.StatusID)
	if err != nil {
		if isNotFound(err) {
			return true, nil
		}
		return false, err
	}
	return !st.IsClosed, nil
}

func isNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrNotFound)
}

func validateTestRun(in TestRunInput) error {
	if len(in.Results) == 0 {
		return errors.New("a test run needs at least one result")
	}
	if len(in.Results) > MaxTestRunResults {
		return fmt.Errorf("a test run may contain at most %d results, this one has %d", MaxTestRunResults, len(in.Results))
	}
	for i, r := range in.Results {
		if strings.TrimSpace(r.TestIdentifier) == "" {
			return fmt.Errorf("result %d has no test identifier", i+1)
		}
		switch r.Status {
		case model.CampaignResultStatusPassed, model.CampaignResultStatusFailed, model.CampaignResultStatusSkipped:
		default:
			return fmt.Errorf("result %q: status must be passed, failed or skipped, not %q", r.TestIdentifier, r.Status)
		}
	}
	return nil
}

// environment finds a runtime environment by name, ignoring case. An unknown
// name is an error rather than a silent no-op: a typo in a CI job would
// otherwise discard every run of that job.
func (s *TestRunService) environment(ctx context.Context, productID, name string) (*model.RuntimeEnvironment, error) {
	list, err := s.environments.ListByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list runtime environments: %w", err)
	}
	var known []string
	for _, e := range list {
		if strings.EqualFold(e.Name, strings.TrimSpace(name)) {
			return e, nil
		}
		known = append(known, e.Name)
	}
	if len(known) == 0 {
		return nil, fmt.Errorf("unknown runtime environment %q: the product defines none", name)
	}
	return nil, fmt.Errorf("unknown runtime environment %q; known: %s", name, strings.Join(known, ", "))
}

// campaign returns the explicitly named campaign, which must be active, or
// the product's active campaign, or nil when there is none.
func (s *TestRunService) campaign(ctx context.Context, productID, id string) (*model.TestingCampaign, error) {
	if id == "" {
		c, err := s.campaigns.GetActiveByProductID(ctx, productID)
		if err != nil {
			return nil, fmt.Errorf("find active campaign: %w", err)
		}
		return c, nil
	}
	c, err := s.campaigns.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if c.Status != model.CampaignStatusActive {
		return nil, fmt.Errorf("results can only be recorded in an active campaign, %q is %s", c.Name, c.Status)
	}
	return c, nil
}

// automaticTestCases maps test identifiers to the product's automatic test
// cases. Manual test cases are never touched by automation.
func (s *TestRunService) automaticTestCases(ctx context.Context, productID string) (map[string]*model.TestCase, error) {
	list, err := s.testCases.ListByProductID(ctx, productID)
	if err != nil {
		return nil, fmt.Errorf("list test cases: %w", err)
	}
	m := make(map[string]*model.TestCase)
	for _, tc := range list {
		if tc.ExecutionMode != model.ExecutionModeAutomatic || tc.TestIdentifier == "" {
			continue
		}
		if _, dup := m[tc.TestIdentifier]; !dup {
			m[tc.TestIdentifier] = tc
		}
	}
	return m, nil
}

// pickResult selects the campaign result a run updates: the single row of a
// test case without an environment matrix, or the row for the run's
// environment.
func pickResult(rows []*model.CampaignResult, env *model.RuntimeEnvironment) (*model.CampaignResult, string) {
	if len(rows) == 0 {
		return nil, TestRunNotInCampaign
	}
	for _, r := range rows {
		if r.EnvironmentID == "" {
			return r, ""
		}
	}
	if env == nil {
		return nil, TestRunEnvironmentRequired
	}
	for _, r := range rows {
		if r.EnvironmentID == env.ID {
			return r, ""
		}
	}
	return nil, TestRunNotInCampaign
}

// testGroup is one test case's share of a run: a registered identifier with
// everything reported for it and its unregistered subtests.
type testGroup struct {
	identifier string
	reported   int
	status     string
	failures   []TestRunResult // failed members, in report order
}

// statusRank orders statuses for roll-up: any failure fails the parent, any
// pass beats a skip.
var statusRank = map[string]int{
	model.CampaignResultStatusSkipped: 1,
	model.CampaignResultStatusPassed:  2,
	model.CampaignResultStatusFailed:  3,
}

// rollUp groups reported results by the test case they belong to. A subtest
// ("TestX/case_1", "pkg.TestX/a/b") whose own identifier is not registered
// is attributed to its nearest registered ancestor; the group takes the worst
// status. Identifiers that match nothing stay on their own. Report order is
// kept.
func rollUp(results []TestRunResult, testCases map[string]*model.TestCase) []testGroup {
	var order []string
	groups := make(map[string]*testGroup)
	for _, r := range results {
		id := strings.TrimSpace(r.TestIdentifier)
		key := id
		if testCases[id] == nil {
			for p := id; strings.Contains(p, "/"); {
				p = p[:strings.LastIndex(p, "/")]
				if testCases[p] != nil {
					key = p
					break
				}
			}
		}
		g := groups[key]
		if g == nil {
			g = &testGroup{identifier: key}
			groups[key] = g
			order = append(order, key)
		}
		g.reported++
		if statusRank[r.Status] > statusRank[g.status] {
			g.status = r.Status
		}
		if r.Status == model.CampaignResultStatusFailed {
			r.TestIdentifier = id
			g.failures = append(g.failures, r)
		}
	}
	out := make([]testGroup, 0, len(order))
	for _, k := range order {
		out = append(out, *groups[k])
	}
	return out
}

// defectDescription is the Markdown body of an automatically filed defect:
// run metadata, then the captured output of every failing member.
func defectDescription(in TestRunInput, report *TestRunReport, campaign *model.TestingCampaign, g testGroup) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Automated test `%s` failed.\n\n", g.identifier)
	meta := func(label, value string) {
		if value != "" {
			fmt.Fprintf(&b, "- **%s:** %s\n", label, value)
		}
	}
	meta("Source", in.Source)
	if report.Environment != nil {
		meta("Environment", report.Environment.Name)
	}
	meta("Revision", in.Revision)
	meta("Branch", in.Branch)
	meta("Campaign", campaign.Name)

	var out strings.Builder
	for _, f := range g.failures {
		if strings.TrimSpace(f.Output) == "" {
			continue
		}
		if len(g.failures) > 1 {
			fmt.Fprintf(&out, "=== %s\n", f.TestIdentifier)
		}
		out.WriteString(strings.TrimRight(f.Output, "\n"))
		out.WriteString("\n")
	}
	text := out.String()
	if text == "" {
		b.WriteString("\nNo test output was captured.\n")
		return b.String()
	}
	if len(text) > MaxTestOutput {
		total := len(text)
		text = tail(text, MaxTestOutput)
		fmt.Fprintf(&b, "\nOutput (truncated: last %d KiB of %d bytes):\n\n", MaxTestOutput>>10, total)
	} else {
		b.WriteString("\nOutput:\n\n")
	}
	fence := codeFence(text)
	fmt.Fprintf(&b, "%s\n%s%s\n", fence, text, fence)
	return b.String()
}

// tail returns the last n bytes of s, moved forward to a rune boundary.
func tail(s string, n int) string {
	s = s[len(s)-n:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}

// codeFence returns a backtick fence longer than any backtick run in text, so
// test output cannot close the code block early.
func codeFence(text string) string {
	longest, run := 0, 0
	for _, c := range text {
		if c == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return strings.Repeat("`", max(3, longest+1))
}

// Summary counts the run's tests by status ("passed", "failed", "skipped"),
// by outcome (the TestRun* outcome constants) and by defect handling
// ("defects_created", "defects_reused"), plus "total".
func (r *TestRunReport) Summary() map[string]int {
	m := map[string]int{"total": len(r.Outcomes)}
	for _, o := range r.Outcomes {
		m[o.Status]++
		m[o.Outcome]++
		switch o.Defect {
		case TestRunDefectCreated:
			m["defects_created"]++
		case TestRunDefectReused:
			m["defects_reused"]++
		}
	}
	return m
}
