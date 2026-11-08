package service_test

// Automated test run ingestion (FR-409, FR-502, FR-504) through the real
// service layer and SQL, on both engines.

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/auth/builtin"
	"nexmap.net/internal/config"
	"nexmap.net/internal/crypto"
	"nexmap.net/internal/database/dbtest"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
)

type runEnv struct {
	t                     *testing.T
	ctx                   context.Context
	st                    *store.Store
	svc                   *service.Services
	admin, tester, viewer *model.User
	product               string
	linux, windows        *model.RuntimeEnvironment
	tc                    map[string]*model.TestCase // by identifier ("manual" for the manual one)
}

func newRunEnv(t *testing.T, driver string, conn *sql.DB) *runEnv {
	t.Helper()
	ctx := context.Background()
	st := store.New(conn, store.Dialect(driver), crypto.TestKeyRing())
	provider := builtin.New(st.User, 1)
	blobs, err := storage.NewFS(t.TempDir(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	svc := service.New(st, provider, &config.Config{SessionTTL: time.Hour, PBKDF2Iterations: 1}, blobs)
	e := &runEnv{t: t, ctx: ctx, st: st, svc: svc, tc: map[string]*model.TestCase{}}

	user := func(name string, admin bool) *model.User {
		now := time.Now().UTC().Format(time.RFC3339)
		u := &model.User{ID: uuid.NewString(), Username: name, Email: name + "@localhost", DisplayName: name,
			PasswordHash: "x", IsAdmin: admin, Status: model.UserStatusActive, CreatedAt: now, UpdatedAt: now}
		if err := st.User.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		return u
	}
	e.admin, e.tester, e.viewer = user("admin", true), user("ci", false), user("watcher", false)

	p, err := svc.Product.Create(ctx, e.admin, service.CreateProductInput{Name: "Runs"})
	must(t, err)
	e.product = p.ID
	must(t, svc.Product.AddMember(ctx, e.admin, p.ID, e.tester.ID, model.RoleTester))
	must(t, svc.Product.AddMember(ctx, e.admin, p.ID, e.viewer.ID, model.RoleViewer))
	e.linux, err = svc.Milestone.CreateRuntimeEnvironment(ctx, e.admin, p.ID, "linux")
	must(t, err)
	e.windows, err = svc.Milestone.CreateRuntimeEnvironment(ctx, e.admin, p.ID, "windows")
	must(t, err)

	for _, spec := range []struct{ id, mode string }{
		{"TestA", model.ExecutionModeAutomatic},
		{"TestMatrix", model.ExecutionModeAutomatic},
		{"TestParent", model.ExecutionModeAutomatic},
		{"TestOutOfScope", model.ExecutionModeAutomatic},
		{"TestManual", model.ExecutionModeManual},
	} {
		tc, err := svc.Testing.CreateTestCase(ctx, e.admin, service.CreateTestCaseInput{
			ProductID: p.ID, Title: spec.id, ExecutionMode: spec.mode, TestIdentifier: spec.id,
		})
		must(t, err)
		e.tc[spec.id] = tc
	}
	must(t, svc.Testing.AddTestCaseEnvironment(ctx, e.admin, p.ID, e.tc["TestMatrix"].ID, e.linux.ID))
	must(t, svc.Testing.AddTestCaseEnvironment(ctx, e.admin, p.ID, e.tc["TestMatrix"].ID, e.windows.ID))
	return e
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// campaign creates a campaign over every test case except TestOutOfScope,
// activated unless draft is set.
func (e *runEnv) campaign(draft bool) *model.TestingCampaign {
	e.t.Helper()
	var ids []string
	for id, tc := range e.tc {
		if id != "TestOutOfScope" {
			ids = append(ids, tc.ID)
		}
	}
	c, err := e.svc.Testing.CreateCampaign(e.ctx, e.admin, service.CreateCampaignInput{ProductID: e.product, Name: "Release " + uuid.NewString()[:4], TestCaseIDs: ids})
	must(e.t, err)
	if !draft {
		c, err = e.svc.Testing.ActivateCampaign(e.ctx, e.admin, e.product, c.ID)
		must(e.t, err)
	}
	return c
}

func (e *runEnv) ingest(in service.TestRunInput) *service.TestRunReport {
	e.t.Helper()
	in.ProductID = e.product
	if in.Source == "" {
		in.Source = "go-test"
	}
	rep, err := e.svc.TestRun.IngestRun(e.ctx, e.tester, in)
	if err != nil {
		e.t.Fatalf("ingest: %v", err)
	}
	return rep
}

func outcomeOf(t *testing.T, rep *service.TestRunReport, id string) service.TestRunOutcome {
	t.Helper()
	for _, o := range rep.Outcomes {
		if o.TestIdentifier == id {
			return o
		}
	}
	t.Fatalf("no outcome for %s in %+v", id, rep.Outcomes)
	return service.TestRunOutcome{}
}

func (e *runEnv) defects() []*model.Defect {
	e.t.Helper()
	list, err := e.svc.Backlog.ListDefects(e.ctx, e.admin, e.product, store.DefectFilter{})
	must(e.t, err)
	return list
}

func (e *runEnv) result(campaignID, testCase string, env *model.RuntimeEnvironment) *model.CampaignResult {
	e.t.Helper()
	envID := ""
	if env != nil {
		envID = env.ID
	}
	r, err := e.st.CampaignResult.GetByCampaignAndTestCase(e.ctx, campaignID, e.tc[testCase].ID, envID)
	must(e.t, err)
	return r
}

func (e *runEnv) events(campaignID string) []*model.CampaignResultEvent {
	e.t.Helper()
	list, err := e.st.CampaignEvent.ListByCampaignID(e.ctx, campaignID)
	must(e.t, err)
	return list
}

func (e *runEnv) closeDefect(id string) {
	e.t.Helper()
	statuses, err := e.svc.Milestone.ListStatuses(e.ctx, e.admin, e.product, "defect")
	must(e.t, err)
	for _, s := range statuses {
		if s.IsClosed {
			_, err := e.svc.Backlog.UpdateDefect(e.ctx, e.admin, e.product, id, service.UpdateDefectInput{StatusID: &s.ID})
			must(e.t, err)
			return
		}
	}
	e.t.Fatal("no closed defect status")
}

var firstRun = []service.TestRunResult{
	{TestIdentifier: "TestA", Status: "passed"},
	{TestIdentifier: "TestMatrix", Status: "failed", Output: "matrix_test.go:12: got 1, want 2\n"},
	{TestIdentifier: "TestParent/one", Status: "passed"},
	{TestIdentifier: "TestParent/two", Status: "failed", Output: "parent_test.go:40: boom\n"},
	{TestIdentifier: "TestParent", Status: "failed"},
	{TestIdentifier: "TestManual", Status: "passed"},
	{TestIdentifier: "TestOutOfScope", Status: "passed"},
	{TestIdentifier: "TestUnknown", Status: "failed", Output: "nobody cares"},
}

func TestIngestRunWithoutCampaignWritesNothing(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newRunEnv(t, driver, conn)
		draft := e.campaign(true) // a draft campaign is not a target
		rep := e.ingest(service.TestRunInput{Environment: "linux", Results: firstRun})
		if rep.Recorded || rep.Campaign != nil {
			t.Fatalf("recorded=%v campaign=%v, want nothing recorded", rep.Recorded, rep.Campaign)
		}
		if o := outcomeOf(t, rep, "TestMatrix"); o.Outcome != service.TestRunNotRecorded || o.TestCaseID == "" || o.Defect != "" {
			t.Errorf("TestMatrix: %+v", o)
		}
		if o := outcomeOf(t, rep, "TestUnknown"); o.Outcome != service.TestRunUnknownTestCase {
			t.Errorf("TestUnknown: %+v", o)
		}
		if n := len(e.defects()); n != 0 {
			t.Errorf("%d defects created without a campaign", n)
		}
		if ev := e.events(draft.ID); len(ev) != 0 {
			t.Errorf("%d events written", len(ev))
		}
		if r := e.result(draft.ID, "TestA", nil); r.Status != model.CampaignResultStatusPending {
			t.Errorf("draft campaign result changed to %s", r.Status)
		}
	})
}

func TestIngestRun(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newRunEnv(t, driver, conn)
		c := e.campaign(false)
		meta := service.TestRunInput{Environment: "Linux", Revision: "4161949", Branch: "main", Results: firstRun}

		rep := e.ingest(meta)
		if !rep.Recorded || rep.Campaign == nil || rep.Campaign.ID != c.ID || rep.Environment.ID != e.linux.ID {
			t.Fatalf("report: recorded=%v campaign=%v env=%v", rep.Recorded, rep.Campaign, rep.Environment)
		}
		want := map[string]string{
			"TestA": service.TestRunUpdated, "TestMatrix": service.TestRunUpdated, "TestParent": service.TestRunUpdated,
			"TestManual": service.TestRunUnknownTestCase, "TestOutOfScope": service.TestRunNotInCampaign,
			"TestUnknown": service.TestRunUnknownTestCase,
		}
		for id, outcome := range want {
			if o := outcomeOf(t, rep, id); o.Outcome != outcome {
				t.Errorf("%s: outcome %q, want %q", id, o.Outcome, outcome)
			}
		}
		if len(rep.Outcomes) != len(want) {
			t.Errorf("%d outcomes, want %d (subtests roll up into TestParent)", len(rep.Outcomes), len(want))
		}
		parent := outcomeOf(t, rep, "TestParent")
		if parent.Reported != 3 || parent.Status != "failed" || parent.Defect != service.TestRunDefectCreated {
			t.Errorf("TestParent roll-up: %+v", parent)
		}
		sum := rep.Summary()
		if sum["defects_created"] != 2 || sum["updated"] != 3 || sum["total"] != 6 || sum["failed"] != 3 {
			t.Errorf("summary: %v", sum)
		}

		// Results.
		if r := e.result(c.ID, "TestA", nil); r.Status != "passed" || r.ExecutedBy != e.tester.ID {
			t.Errorf("TestA result: %+v", r)
		}
		linux, windows := e.result(c.ID, "TestMatrix", e.linux), e.result(c.ID, "TestMatrix", e.windows)
		if linux.Status != "failed" || linux.DefectID == "" || windows.Status != model.CampaignResultStatusPending {
			t.Errorf("matrix rows: linux %+v, windows %+v", linux, windows)
		}

		// Defects: linked, described, in the environment and default status;
		// none for unknown or out-of-scope tests.
		defects := e.defects()
		if len(defects) != 2 {
			t.Fatalf("%d defects, want 2", len(defects))
		}
		d, err := e.svc.Backlog.GetDefect(e.ctx, e.admin, e.product, linux.DefectID)
		must(t, err)
		if d.Title != "Automated test failed: TestMatrix" || d.EnvironmentID != e.linux.ID || d.StatusID == "" ||
			!strings.Contains(d.Description, "got 1, want 2") || !strings.Contains(d.Description, "4161949") ||
			!strings.Contains(d.Description, "linux") || d.Criticality != model.DefectCriticalityMedium {
			t.Errorf("defect: %+v", d)
		}
		links, err := e.svc.Backlog.ListDefectLinks(e.ctx, e.admin, e.product, d.ID)
		must(t, err)
		if len(links) != 1 || links[0].TargetID != e.tc["TestMatrix"].ID {
			t.Errorf("defect links: %+v", links)
		}
		pd, err := e.svc.Backlog.GetDefect(e.ctx, e.admin, e.product, e.result(c.ID, "TestParent", nil).DefectID)
		must(t, err)
		if !strings.Contains(pd.Description, "boom") || !strings.Contains(pd.Description, "TestParent/two") {
			t.Errorf("parent defect lacks the subtest output: %q", pd.Description)
		}

		// Events carry the run's provenance.
		events := e.events(c.ID)
		if len(events) != 3 {
			t.Fatalf("%d events, want 3", len(events))
		}
		for _, ev := range events {
			if ev.Source != "go-test" || ev.Revision != "4161949" || ev.Branch != "main" || ev.UserID != e.tester.ID {
				t.Errorf("event: %+v", ev)
			}
		}

		// The same run again changes nothing.
		again := e.ingest(meta)
		for _, id := range []string{"TestA", "TestMatrix", "TestParent"} {
			if o := outcomeOf(t, again, id); o.Outcome != service.TestRunUnchanged {
				t.Errorf("repeat %s: %+v", id, o)
			}
		}
		if n := len(e.defects()); n != 2 {
			t.Errorf("repeat created defects: %d", n)
		}
		if n := len(e.events(c.ID)); n != 3 {
			t.Errorf("repeat wrote events: %d", n)
		}

		// A matrix test needs the environment; another environment updates its own row.
		noEnv := e.ingest(service.TestRunInput{Results: []service.TestRunResult{{TestIdentifier: "TestMatrix", Status: "passed"}}})
		if o := outcomeOf(t, noEnv, "TestMatrix"); o.Outcome != service.TestRunEnvironmentRequired {
			t.Errorf("matrix without environment: %+v", o)
		}
		e.ingest(service.TestRunInput{Environment: "windows", Results: []service.TestRunResult{{TestIdentifier: "TestMatrix", Status: "passed"}}})
		if w, l := e.result(c.ID, "TestMatrix", e.windows), e.result(c.ID, "TestMatrix", e.linux); w.Status != "passed" || l.Status != "failed" {
			t.Errorf("windows run: windows %s, linux %s", w.Status, l.Status)
		}
	})
}

// FR-504 with automation: closing the defect sends the result to re-test, a
// green run passes it, and a later failure files a new defect because the old
// one is closed. An open defect on the result is reused.
func TestIngestRunRetestLoop(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newRunEnv(t, driver, conn)
		c := e.campaign(false)
		fail := service.TestRunInput{Results: []service.TestRunResult{{TestIdentifier: "TestA", Status: "failed", Output: "first"}}}
		pass := service.TestRunInput{Results: []service.TestRunResult{{TestIdentifier: "TestA", Status: "passed"}}}

		e.ingest(fail)
		first := e.result(c.ID, "TestA", nil).DefectID
		e.closeDefect(first)
		if r := e.result(c.ID, "TestA", nil); r.Status != model.CampaignResultStatusRetest {
			t.Fatalf("after closing the defect: %s, want retest", r.Status)
		}
		e.ingest(pass)
		if r := e.result(c.ID, "TestA", nil); r.Status != "passed" || r.DefectID != "" {
			t.Fatalf("green run after fix: %+v", r)
		}

		rep := e.ingest(fail)
		second := e.result(c.ID, "TestA", nil).DefectID
		if o := outcomeOf(t, rep, "TestA"); o.Defect != service.TestRunDefectCreated || second == first || second == "" {
			t.Fatalf("regression: %+v, defect %s (first %s)", o, second, first)
		}

		// A tester blocks the result; the defect stays on it and is reused
		// when automation fails it again.
		_, err := e.svc.Testing.SetResultStatus(e.ctx, e.tester, e.product, c.ID, e.result(c.ID, "TestA", nil).ID, model.CampaignResultStatusBlocked, "")
		must(t, err)
		rep = e.ingest(fail)
		if o := outcomeOf(t, rep, "TestA"); o.Defect != service.TestRunDefectReused || o.DefectID != second {
			t.Errorf("open defect not reused: %+v", o)
		}
		if n := len(e.defects()); n != 2 {
			t.Errorf("%d defects, want 2", n)
		}
	})
}

func TestIngestRunDryRunAndValidation(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newRunEnv(t, driver, conn)
		c := e.campaign(false)

		rep := e.ingest(service.TestRunInput{DryRun: true, Environment: "linux", Results: firstRun})
		if rep.Recorded || !rep.DryRun || rep.Campaign == nil {
			t.Fatalf("dry run report: %+v", rep)
		}
		if o := outcomeOf(t, rep, "TestMatrix"); o.Outcome != service.TestRunUpdated || o.Defect != service.TestRunDefectCreated || o.DefectID != "" {
			t.Errorf("dry run TestMatrix: %+v", o)
		}
		if len(e.defects()) != 0 || len(e.events(c.ID)) != 0 || e.result(c.ID, "TestA", nil).Status != model.CampaignResultStatusPending {
			t.Error("dry run wrote something")
		}

		ok := []service.TestRunResult{{TestIdentifier: "TestA", Status: "passed"}}
		draft := e.campaign(true)
		for name, in := range map[string]service.TestRunInput{
			"unknown environment": {Environment: "solaris", Results: ok},
			"bad status":          {Results: []service.TestRunResult{{TestIdentifier: "TestA", Status: "broken"}}},
			"empty identifier":    {Results: []service.TestRunResult{{Status: "passed"}}},
			"no results":          {},
			"draft campaign":      {CampaignID: draft.ID, Results: ok},
		} {
			in.ProductID = e.product
			if _, err := e.svc.TestRun.IngestRun(e.ctx, e.tester, in); err == nil {
				t.Errorf("%s: accepted", name)
			}
		}
		_, err := e.svc.TestRun.IngestRun(e.ctx, e.viewer, service.TestRunInput{ProductID: e.product, Results: ok})
		if !errors.Is(err, service.ErrForbidden) {
			t.Errorf("viewer: err = %v, want forbidden", err)
		}
		// A campaign of another product is not found.
		other, err := e.svc.Product.Create(e.ctx, e.admin, service.CreateProductInput{Name: "Other"})
		must(t, err)
		_, err = e.svc.TestRun.IngestRun(e.ctx, e.admin, service.TestRunInput{ProductID: other.ID, CampaignID: c.ID, Results: ok})
		if !errors.Is(err, service.ErrNotFound) {
			t.Errorf("foreign campaign: err = %v, want not found", err)
		}
	})
}

func TestIngestRunTruncatesOutput(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, driver string, conn *sql.DB) {
		e := newRunEnv(t, driver, conn)
		c := e.campaign(false)
		huge := strings.Repeat("noise ```line\n", 10000) + "THE ASSERTION THAT FAILED\n"
		e.ingest(service.TestRunInput{Results: []service.TestRunResult{{TestIdentifier: "TestA", Status: "failed", Output: huge}}})
		d, err := e.svc.Backlog.GetDefect(e.ctx, e.admin, e.product, e.result(c.ID, "TestA", nil).DefectID)
		must(t, err)
		if len(d.Description) > service.MaxTestOutput+1024 || !strings.Contains(d.Description, "THE ASSERTION THAT FAILED") ||
			!strings.Contains(d.Description, "truncated") || !strings.Contains(d.Description, "\n````\n") {
			t.Errorf("description: %d bytes, tail kept=%v, fence=%v", len(d.Description),
				strings.Contains(d.Description, "THE ASSERTION THAT FAILED"), strings.Contains(d.Description, "````"))
		}
	})
}
