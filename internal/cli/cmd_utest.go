package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"nexmap.net/internal/cli/ops"
	"nexmap.net/internal/cli/output"
	"nexmap.net/internal/client"
	"nexmap.net/internal/ingest"
)

// reportFlags are the input options shared by utest and test sync.
type reportFlags struct {
	format string
	style  string
	quiet  bool
}

func (f *reportFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&f.format, "format", ingest.FormatAuto, "report format: auto, go-json, go-text or junit")
	fs.StringVar(&f.style, "identifier-style", ingest.StylePackage,
		`how test identifiers are built: "package" (pkg/path.TestName, classname.name) or "name" (TestName)`)
	fs.BoolVar(&f.quiet, "quiet", false, "do not copy stdin to stdout")
}

// parsed is the merged content of every input.
type parsed struct {
	formats []string
	results []ingest.Result
	errors  []string
}

func (p *parsed) failed() bool {
	if len(p.errors) > 0 {
		return true
	}
	for _, r := range p.results {
		if r.Status == ingest.Failed {
			return true
		}
	}
	return false
}

// readReports parses the given files, or stdin when there are none. Stdin is
// copied to stdout as it is read (unless quiet), so `go test … | nexmap utest`
// keeps the test log visible in the build output.
func (a *app) readReports(files []string, f reportFlags, passthrough bool) (*parsed, error) {
	switch f.format {
	case ingest.FormatAuto, ingest.FormatGoJSON, ingest.FormatGoText, ingest.FormatJUnit:
	default:
		return nil, usageErr(fmt.Errorf("unknown --format %q (use %s)", f.format, strings.Join(ingest.Formats, ", ")))
	}
	if f.style != ingest.StylePackage && f.style != ingest.StyleName {
		return nil, usageErr(fmt.Errorf(`--identifier-style must be "package" or "name"`))
	}
	opts := ingest.Options{Format: f.format, IdentifierStyle: f.style}
	out := &parsed{}
	add := func(r io.Reader, name string) error {
		rep, format, err := ingest.Parse(r, opts)
		if err != nil {
			return usageErr(fmt.Errorf("%s: %w", name, err))
		}
		out.formats = append(out.formats, format)
		out.results = append(out.results, rep.Results...)
		out.errors = append(out.errors, rep.Errors...)
		return nil
	}
	if len(files) == 0 {
		var in io.Reader = a.io.Stdin
		if passthrough && !f.quiet {
			in = io.TeeReader(in, a.io.Stdout)
		}
		return out, add(in, "stdin")
	}
	for _, name := range files {
		p := name
		if !filepath.IsAbs(p) {
			p = filepath.Join(a.io.Cwd, p)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, usageErr(err)
		}
		if err := add(bytes.NewReader(data), name); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// source names the run's format for the server.
func (p *parsed) source() string {
	seen := map[string]bool{}
	var names []string
	for _, f := range p.formats {
		if s := ingest.Source(f); !seen[s] {
			seen[s] = true
			names = append(names, s)
		}
	}
	return strings.Join(names, "+")
}

// CI environment variables for the revision and branch, in order of
// preference. Forgejo and Gitea Actions set the GITHUB_* names as well.
var (
	revisionVars = []string{"GITHUB_SHA", "CI_COMMIT_SHA", "BUILD_SOURCEVERSION", "BITBUCKET_COMMIT", "CIRCLE_SHA1", "GIT_COMMIT"}
	branchVars   = []string{"GITHUB_HEAD_REF", "GITHUB_REF_NAME", "CI_COMMIT_REF_NAME", "BUILD_SOURCEBRANCHNAME", "BITBUCKET_BRANCH", "CIRCLE_BRANCH", "GIT_BRANCH"}
)

func (a *app) firstEnv(names []string) string {
	for _, n := range names {
		if v := strings.TrimSpace(a.io.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

// git runs a read-only git query in the working directory; failures (no git,
// not a repository) yield "".
func (a *app) git(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = a.io.Cwd
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func utestCommand() *command {
	var rf reportFlags
	var env, campaign, revision, branch string
	var dryRun, exitCode bool
	return &command{
		name:    "utest",
		args:    "[report files…]",
		summary: "record automated test results in the active testing campaign (reads stdin without files)",
		flags: func(fs *flag.FlagSet) {
			rf.bind(fs)
			fs.StringVar(&env, "env", "", "runtime environment of this run, e.g. linux-amd64")
			fs.StringVar(&campaign, "campaign", "", "campaign name or ID (default: the active campaign)")
			fs.StringVar(&revision, "revision", "", "VCS revision tested (default: from CI variables or git)")
			fs.StringVar(&branch, "branch", "", "VCS branch tested (default: from CI variables or git)")
			fs.BoolVar(&dryRun, "dry-run", false, "show how tests map to test cases without recording anything")
			fs.BoolVar(&exitCode, "exit-code", false, fmt.Sprintf("exit %d if any test failed", ExitTestsFailed))
		},
		run: func(ctx context.Context, a *app, files []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			p, err := a.readReports(files, rf, a.format != output.JSON)
			if err != nil {
				return err
			}
			run := client.TestRun{Source: p.source(), Environment: env, DryRun: dryRun,
				Revision: revision, Branch: branch, Results: make([]client.TestRunResult, 0, len(p.results))}
			if run.Revision == "" {
				if run.Revision = a.firstEnv(revisionVars); run.Revision == "" {
					run.Revision = a.git("rev-parse", "HEAD")
				}
			}
			if run.Branch == "" {
				if run.Branch = a.firstEnv(branchVars); run.Branch == "" {
					if b := a.git("rev-parse", "--abbrev-ref", "HEAD"); b != "HEAD" {
						run.Branch = b
					}
				}
			}
			if campaign != "" {
				if run.CampaignID, err = s.CampaignID(ctx, campaign); err != nil {
					return err
				}
			}
			for _, r := range p.results {
				run.Results = append(run.Results, client.TestRunResult{
					TestIdentifier: r.Identifier, Status: r.Status, Output: r.Output, DurationMS: r.DurationMS,
				})
			}

			var rep *client.TestRunReport
			if len(run.Results) > 0 {
				if rep, err = s.C.IngestTestRun(ctx, s.ProductID, run); err != nil {
					return err
				}
			}
			if err := a.emitTestRun(p, run, rep); err != nil {
				return err
			}
			if exitCode && p.failed() {
				return silentExit(ExitTestsFailed)
			}
			return nil
		},
	}
}

// testRunView is utest's -o json output.
type testRunView struct {
	Format      string                `json:"format"`
	Revision    string                `json:"revision,omitempty"`
	Branch      string                `json:"branch,omitempty"`
	Parsed      map[string]int        `json:"parsed"`
	Errors      []string              `json:"errors"`
	Report      *client.TestRunReport `json:"report"` // null when nothing was parsed
	TestsFailed bool                  `json:"tests_failed"`
}

func (a *app) emitTestRun(p *parsed, run client.TestRun, rep *client.TestRunReport) error {
	counts := map[string]int{"passed": 0, "failed": 0, "skipped": 0}
	for _, r := range p.results {
		counts[r.Status]++
	}
	if a.format == output.JSON {
		return output.WriteJSON(a.io.Stdout, testRunView{
			Format: strings.Join(p.formats, ","), Revision: run.Revision, Branch: run.Branch,
			Parsed: counts, Errors: append([]string{}, p.errors...), Report: rep, TestsFailed: p.failed(),
		})
	}

	var b strings.Builder
	b.WriteString("\n")
	fmt.Fprintf(&b, "nexmap: %d tests parsed (%d passed, %d failed, %d skipped)", len(p.results), counts["passed"], counts["failed"], counts["skipped"])
	for _, e := range p.errors {
		fmt.Fprintf(&b, "\n  error: %s", firstLine(e))
	}
	b.WriteString("\n")
	switch {
	case rep == nil:
		b.WriteString("nexmap: nothing to upload\n")
	case rep.DryRun:
		writeDryRun(&b, rep)
	case !rep.Recorded:
		b.WriteString("nexmap: no active campaign — results not recorded\n")
		writeUnmatched(&b, rep)
	default:
		writeRecorded(&b, rep)
	}
	_, err := io.WriteString(a.io.Stdout, b.String())
	return err
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func campaignLabel(rep *client.TestRunReport) string {
	label := "campaign"
	if rep.Campaign != nil {
		label = fmt.Sprintf("campaign %q", rep.Campaign.Name)
	}
	if rep.Environment != "" {
		label += " [" + rep.Environment + "]"
	}
	return label
}

func writeRecorded(b *strings.Builder, rep *client.TestRunReport) {
	sum := rep.Summary
	fmt.Fprintf(b, "nexmap: recorded in %s: %d updated, %d unchanged", campaignLabel(rep), sum["updated"], sum["unchanged"])
	if n := sum["defects_created"] + sum["defects_reused"]; n > 0 {
		fmt.Fprintf(b, ", defects: %d created, %d reused", sum["defects_created"], sum["defects_reused"])
	}
	b.WriteString("\n")
	for _, o := range rep.Results {
		if o.Outcome != "updated" {
			continue
		}
		line := fmt.Sprintf("  %-7s %s (%s → %s)", strings.ToUpper(o.Status), o.TestIdentifier, o.PreviousStatus, o.Status)
		if o.Defect != "" {
			line += fmt.Sprintf(", defect %s %s", defectRef(o.DefectIdentifier, o.DefectID), o.Defect)
		}
		b.WriteString(line + "\n")
	}
	writeUnmatched(b, rep)
}

// writeUnmatched reports tests that matched nothing recordable. Failures are
// listed one by one so the build log shows them; passing and skipped ones
// are only counted.
func writeUnmatched(b *strings.Builder, rep *client.TestRunReport) {
	reasons := map[string]string{
		"unknown_test_case":    "no automatic test case",
		"not_in_campaign":      "not in the campaign",
		"environment_required": "needs --env",
	}
	quiet := map[string]int{}
	for _, o := range rep.Results {
		reason, ok := reasons[o.Outcome]
		if !ok {
			continue
		}
		if o.Status == "failed" {
			fmt.Fprintf(b, "  FAILED  %s (not recorded: %s)\n", o.TestIdentifier, reason)
			continue
		}
		quiet[reason]++
	}
	keys := make([]string, 0, len(quiet))
	for k := range quiet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(b, "nexmap: %d other tests not recorded: %s\n", quiet[k], k)
	}
	if len(keys) > 0 {
		b.WriteString("nexmap: `nexmap utest --dry-run` lists them; `nexmap test sync` creates missing test cases\n")
	}
}

func writeDryRun(b *strings.Builder, rep *client.TestRunReport) {
	if rep.Campaign == nil {
		b.WriteString("nexmap: dry run — no active campaign, nothing would be recorded\n")
	} else {
		fmt.Fprintf(b, "nexmap: dry run against %s — nothing recorded\n", campaignLabel(rep))
	}
	rows := make([][]string, 0, len(rep.Results))
	for _, o := range rep.Results {
		effect := o.Outcome
		if o.Outcome == "updated" {
			effect = o.PreviousStatus + " → " + o.Status
		}
		if o.Defect != "" {
			effect += ", defect " + o.Defect
		}
		tc := "-"
		if o.TestCaseID != "" {
			tc = shortID(o.TestCaseID)
		}
		rows = append(rows, []string{o.TestIdentifier, o.Status, tc, effect})
	}
	tw := tabwriter.NewWriter(b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TEST\tSTATUS\tTEST CASE\tEFFECT")
	for _, r := range rows {
		fmt.Fprintln(tw, strings.Join(r, "\t"))
	}
	_ = tw.Flush()
}

// defectRef names a defect by its identifier, or by its short ID if the
// server sent none.
func defectRef(identifier, id string) string {
	if identifier != "" {
		return identifier
	}
	return shortID(id)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func testSyncCommand() *command {
	var rf reportFlags
	var in ops.TestSync
	return &command{
		name:    "test sync",
		args:    "[report files…]",
		summary: "create automatic test cases for tests in a report that have none (developer role)",
		flags: func(fs *flag.FlagSet) {
			rf.bind(fs)
			fs.StringVar(&in.Capability, "capability", "", "capability (name or ID) to file new test cases under")
			fs.StringVar(&in.Component, "component", "", "component (name or ID) to file new test cases under")
			fs.StringVar(&in.PyramidLevel, "pyramid-level", "", "pyramid level (name or ID) of new test cases, e.g. unit")
			fs.StringVar(&in.Requirement, "req", "", "requirement the new test cases verify, e.g. FR-12")
			fs.Var((*listFlag)(&in.Constraints), "constraint", "constraint the new test cases verify, e.g. SC-30 (repeatable or comma-separated)")
			fs.BoolVar(&in.DryRun, "dry-run", false, "list the test cases that would be created")
		},
		run: func(ctx context.Context, a *app, files []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			rf.quiet = true // the report is ours to read, not a pipeline to pass on
			p, err := a.readReports(files, rf, false)
			if err != nil {
				return err
			}
			if len(p.results) == 0 {
				return usageErr(errors.New("the report contains no tests"))
			}
			for _, r := range p.results {
				in.Identifiers = append(in.Identifiers, r.Identifier)
			}
			res, err := s.SyncTestCases(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			if a.format == output.JSON {
				return output.WriteJSON(a.io.Stdout, res)
			}
			verb := "created"
			if in.DryRun {
				verb = "would create"
			}
			fmt.Fprintf(a.io.Stdout, "%d test cases %s, %d already existed\n", len(res.Created), verb, res.Existing)
			for _, tc := range res.Created {
				fmt.Fprintf(a.io.Stdout, "  %s  %s\n", tc.TestIdentifier, tc.Title)
			}
			if len(res.Created) > 0 && !in.DryRun {
				fmt.Fprintln(a.io.Stdout, "New test cases join campaigns created from now on; an active campaign keeps its scope.")
			}
			return nil
		},
	}
}
