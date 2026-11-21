// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli_test

// `nexmap utest` and `nexmap test sync` end to end against the seeded Aviary
// product, whose active campaign "Sprint 13 Verification" contains:
//   - TestPublish_StripsExifLocation      pending
//   - TestSyncQueue_ReplayIdempotent      passed
//   - TestPhotoUpload_PreservesOrientation per environment (iOS 17, Android 14),
//     in re-test after its defect was closed
// TestSpeciesIndex_PartialCommonName exists but is not in that campaign.

import (
	"strings"
	"testing"

	"nexmap.net/internal/cli"
	cliconfig "nexmap.net/internal/cli/config"
	"nexmap.net/internal/cli/ops"
)

const aviaryRun = `=== RUN   TestPublish_StripsExifLocation
--- PASS: TestPublish_StripsExifLocation (0.01s)
=== RUN   TestSyncQueue_ReplayIdempotent
--- PASS: TestSyncQueue_ReplayIdempotent (0.00s)
=== RUN   TestPhotoUpload_PreservesOrientation
    photo_test.go:31: orientation = 6, want 1
--- FAIL: TestPhotoUpload_PreservesOrientation (0.02s)
=== RUN   TestSpeciesIndex_PartialCommonName
    index_test.go:12: no match for "robin"
--- FAIL: TestSpeciesIndex_PartialCommonName (0.00s)
=== RUN   TestBrandNew
--- PASS: TestBrandNew (0.00s)
FAIL
FAIL	example.com/aviary/app	0.031s
`

type utestJSON struct {
	Format      string         `json:"format"`
	Revision    string         `json:"revision"`
	Branch      string         `json:"branch"`
	Parsed      map[string]int `json:"parsed"`
	TestsFailed bool           `json:"tests_failed"`
	Report      *struct {
		Recorded    bool           `json:"recorded"`
		DryRun      bool           `json:"dry_run"`
		Environment string         `json:"environment"`
		Summary     map[string]int `json:"summary"`
		Campaign    *struct {
			Name string `json:"name"`
		} `json:"campaign"`
		Results []struct {
			TestIdentifier string `json:"test_identifier"`
			Outcome        string `json:"outcome"`
			PreviousStatus string `json:"previous_status"`
			DefectID       string `json:"defect_id"`
			Defect         string `json:"defect"`
		} `json:"results"`
	} `json:"report"`
}

func TestE2EUtest(t *testing.T) {
	e := newAviary(t)
	e.env["GITHUB_SHA"] = "0123456789abcdef"
	e.env["GITHUB_REF_NAME"] = "main"
	args := []string{"utest", "--identifier-style", "name", "--env", "android 14"}

	// Role: viewers cannot record results.
	wantForbidden(t, "viewer", e.as("ada", aviaryRun, args...))

	// Dry run first: shows the mapping, records nothing.
	dry := decode[utestJSON](t, e.mustAs("bo", aviaryRun, append(args, "--dry-run", "-o", "json")...).stdout)
	if dry.Report == nil || !dry.Report.DryRun || dry.Report.Recorded || dry.Report.Summary["updated"] != 2 {
		t.Fatalf("dry run: %+v", dry.Report)
	}
	before := len(decode[[]ops.DefectSummary](t, e.mustRun("", "defect", "list", "-o", "json").stdout))

	// The real run, piped: the test log passes through, the summary follows.
	r := e.mustAs("bo", aviaryRun, args...)
	if !strings.HasPrefix(r.stdout, aviaryRun) {
		t.Errorf("stdin was not passed through:\n%s", r.stdout)
	}
	for _, want := range []string{
		`recorded in campaign "Sprint 13 Verification" [Android 14]: 2 updated, 1 unchanged`,
		"PASSED  TestPublish_StripsExifLocation (pending → passed)",
		"FAILED  TestPhotoUpload_PreservesOrientation (retest → failed), defect",
		"FAILED  TestSpeciesIndex_PartialCommonName (not recorded: not in the campaign)",
		"1 other tests not recorded: no automatic test case",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("summary lacks %q:\n%s", want, r.stdout)
		}
	}
	defects := decode[[]ops.DefectSummary](t, e.mustRun("", "defect", "list", "-o", "json").stdout)
	if len(defects) != before+1 {
		t.Fatalf("defects: %d → %d, want one new (the closed photo defect is not reused)", before, len(defects))
	}
	var filed *ops.DefectSummary
	for i := range defects {
		if defects[i].Title == "Automated test failed: TestPhotoUpload_PreservesOrientation" {
			filed = &defects[i]
		}
	}
	if filed == nil {
		t.Fatal("no defect filed for the photo test")
	}
	d := decode[ops.DefectDetail](t, e.mustRun("", "defect", "show", filed.Identifier, "-o", "json").stdout)
	if !strings.Contains(d.Description, "orientation = 6, want 1") || !strings.Contains(d.Description, "0123456789abcdef") ||
		d.Environment != "Android 14" {
		t.Errorf("filed defect: env %q, description:\n%s", d.Environment, d.Description)
	}

	// Again, as JSON: nothing changes, and --exit-code reports the failures.
	r = e.as("bo", aviaryRun, append(args, "-o", "json", "--exit-code")...)
	if r.code != cli.ExitTestsFailed {
		t.Errorf("--exit-code with failures: exit %d, want %d (stderr %q)", r.code, cli.ExitTestsFailed, r.stderr)
	}
	again := decode[utestJSON](t, r.stdout) // JSON output is not mixed with the passed-through log
	if again.Report.Summary["updated"] != 0 || again.Report.Summary["unchanged"] != 3 || again.Revision != "0123456789abcdef" || again.Branch != "main" || !again.TestsFailed {
		t.Errorf("repeat: %+v / %+v", again, again.Report.Summary)
	}

	// The photo test runs per environment: without --env it is not recorded.
	noEnv := decode[utestJSON](t, e.mustAs("bo", aviaryRun, "utest", "--identifier-style", "name", "-o", "json").stdout)
	for _, o := range noEnv.Report.Results {
		if o.TestIdentifier == "TestPhotoUpload_PreservesOrientation" && o.Outcome != "environment_required" {
			t.Errorf("photo test without --env: %s", o.Outcome)
		}
	}

	// Usage errors.
	for _, bad := range [][]string{
		{"utest", "--env", "Solaris"},                     // unknown environment (server)
		{"utest", "--format", "tap"},                      // unknown format
		{"utest", "--campaign", "Release 0.1 Regression"}, // closed campaign (server)
		{"utest", "--identifier-style", "full"},
	} {
		if r := e.as("bo", aviaryRun, bad...); r.code == cli.ExitOK {
			t.Errorf("%v succeeded", bad)
		}
	}
	if r := e.as("bo", "", "utest", "missing-report.xml"); r.code != cli.ExitUsage {
		t.Errorf("missing file: exit %d", r.code)
	}
}

// Without an active campaign the upload succeeds and records nothing.
func TestE2EUtestNoCampaign(t *testing.T) {
	e := newE2E(t)
	e.env[cliconfig.EnvServer] = e.serverURL
	e.env[cliconfig.EnvToken] = e.token
	e.env[cliconfig.EnvProduct] = e.products[0].ID
	r := e.mustRun(aviaryRun, "utest", "--quiet")
	if strings.Contains(r.stdout, "=== RUN") {
		t.Error("--quiet still passed the log through")
	}
	if !strings.Contains(r.stdout, "no active campaign — results not recorded") || !strings.Contains(r.stdout, "5 tests parsed (3 passed, 2 failed, 0 skipped)") {
		t.Errorf("stdout:\n%s", r.stdout)
	}
	if r := e.run(aviaryRun, "", "utest", "--exit-code"); r.code != cli.ExitTestsFailed {
		t.Errorf("--exit-code: exit %d", r.code)
	}
}

func TestE2ETestSync(t *testing.T) {
	e := newAviary(t)
	args := []string{"test", "sync", "--identifier-style", "name", "-o", "json"}

	wantForbidden(t, "tester", e.as("bo", aviaryRun, args...))

	dry := decode[ops.SyncResult](t, e.mustAs("cy", aviaryRun, append(args, "--dry-run")...).stdout)
	if len(dry.Created) != 1 || dry.Created[0].TestIdentifier != "TestBrandNew" || dry.Existing != 4 {
		t.Fatalf("dry run: %+v", dry)
	}
	res := decode[ops.SyncResult](t, e.mustAs("cy", aviaryRun, args...).stdout)
	if len(res.Created) != 1 || res.Created[0].ID == "" || res.Created[0].Mode != "automatic" {
		t.Fatalf("sync: %+v", res)
	}
	show := decode[ops.TestCaseDetail](t, e.mustRun("", "test", "show", "TestBrandNew", "-o", "json").stdout)
	if show.TestIdentifier != "TestBrandNew" {
		t.Errorf("synced test case: %+v", show.TestCaseSummary)
	}
	// Idempotent: nothing left to create.
	if again := decode[ops.SyncResult](t, e.mustAs("cy", aviaryRun, args...).stdout); len(again.Created) != 0 || again.Existing != 5 {
		t.Errorf("second sync: %+v", again)
	}
}
