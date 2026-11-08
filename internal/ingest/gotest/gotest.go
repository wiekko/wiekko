// Package gotest parses `go test -json` events and `go test -v` text.
package gotest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"nexmap.net/internal/ingest/report"
)

// identifier builds "pkg/path.TestName" or "TestName".
func identifier(pkg, test string, qualified bool) string {
	if qualified && pkg != "" {
		return pkg + "." + test
	}
	return test
}

// isFrame reports whether a line is test framing rather than test output.
func isFrame(line string) bool {
	t := strings.TrimLeft(line, " ")
	for _, p := range []string{"=== RUN", "=== PAUSE", "=== CONT", "=== NAME", "--- PASS:", "--- FAIL:", "--- SKIP:"} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// event is one line of `go test -json` (test2json) output.
type event struct {
	Action      string
	Package     string
	ImportPath  string // build events (Go 1.24+)
	Test        string
	Elapsed     float64
	Output      string
	OutputType  string
	FailedBuild string
}

// ParseJSON parses `go test -json` output. Non-JSON lines (a build log mixed
// into the stream, `go vet` noise) are ignored.
func ParseJSON(r io.Reader, qualified bool) (*report.Report, error) {
	rep := &report.Report{}
	type key struct{ pkg, test string }
	output := map[key]*strings.Builder{}
	testsSeen := map[string]bool{} // packages that reported at least one test
	buildOutput := map[string]*strings.Builder{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
	parsed := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var ev event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		parsed++
		switch ev.Action {
		case "build-output":
			b := buildOutput[ev.ImportPath]
			if b == nil {
				b = &strings.Builder{}
				buildOutput[ev.ImportPath] = b
			}
			b.WriteString(ev.Output)
			continue
		case "build-fail":
			continue
		}
		if ev.Test == "" {
			// Package-level events: a package failing without a failed test
			// did not build or crashed outside a test.
			if ev.Action == "fail" && !testsSeen[ev.Package] {
				msg := ev.Package + ": "
				if ev.FailedBuild != "" {
					msg += "build failed"
					if b := buildOutput[ev.FailedBuild]; b != nil {
						msg += "\n" + strings.TrimRight(b.String(), "\n")
					}
				} else {
					msg += "failed outside any test"
				}
				rep.Errors = append(rep.Errors, msg)
			}
			continue
		}
		k := key{ev.Package, ev.Test}
		switch ev.Action {
		case "output":
			if ev.OutputType == "frame" || isFrame(ev.Output) {
				continue
			}
			b := output[k]
			if b == nil {
				b = &strings.Builder{}
				output[k] = b
			}
			b.WriteString(ev.Output)
		case "pass", "fail", "skip":
			testsSeen[ev.Package] = true
			res := report.Result{
				Identifier: identifier(ev.Package, ev.Test, qualified),
				Status:     map[string]string{"pass": report.Passed, "fail": report.Failed, "skip": report.Skipped}[ev.Action],
				DurationMS: int64(ev.Elapsed * 1000),
			}
			if res.Status == report.Failed && output[k] != nil {
				res.Output = output[k].String()
			}
			delete(output, k)
			rep.Results = append(rep.Results, res)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if parsed == 0 {
		return nil, fmt.Errorf("no test2json events found (was the input produced by `go test -json`?)")
	}
	return rep, nil
}

var (
	resultLine  = regexp.MustCompile(`^(\s*)--- (PASS|FAIL|SKIP): (\S+) \(([0-9.]+)s\)`)
	frameLine   = regexp.MustCompile(`^=== (RUN|PAUSE|CONT|NAME)\s+(\S+)`)
	packageLine = regexp.MustCompile(`^(ok|FAIL|\?)\s+(\S+)(\s+.*)?$`)
)

// ParseText parses `go test -v` output (best effort: the format is meant for
// people). Output lines are attributed to the test that last started or
// continued; output after a test's FAIL line (a panic trace) goes to that
// test until the next test starts. Package paths are only known from the
// package's final "ok"/"FAIL" line, so results are buffered per package.
func ParseText(r io.Reader, qualified bool) (*report.Report, error) {
	rep := &report.Report{}
	var pending []report.Result // results of the current package
	output := map[string]*strings.Builder{}
	current, lastFailed := "", -1
	sawTest := false

	flush := func(pkg string) {
		for _, res := range pending {
			res.Identifier = identifier(pkg, res.Identifier, qualified)
			rep.Results = append(rep.Results, res)
		}
		pending, output, current, lastFailed = nil, map[string]*strings.Builder{}, "", -1
	}
	appendOutput := func(test, line string) {
		b := output[test]
		if b == nil {
			b = &strings.Builder{}
			output[test] = b
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if m := frameLine.FindStringSubmatch(line); m != nil {
			sawTest = true
			if m[1] != "PAUSE" {
				current, lastFailed = m[2], -1
			}
			continue
		}
		if m := resultLine.FindStringSubmatch(line); m != nil {
			sawTest = true
			name := m[3]
			secs, _ := strconv.ParseFloat(m[4], 64)
			res := report.Result{Identifier: name, DurationMS: int64(secs * 1000)}
			switch m[2] {
			case "PASS":
				res.Status = report.Passed
			case "SKIP":
				res.Status = report.Skipped
			case "FAIL":
				res.Status = report.Failed
			}
			pending = append(pending, res)
			if res.Status == report.Failed {
				lastFailed = len(pending) - 1
			} else {
				lastFailed = -1
			}
			if current == name {
				current = ""
			}
			continue
		}
		if m := packageLine.FindStringSubmatch(line); m != nil {
			pkg, rest := m[2], m[3]
			if m[1] == "FAIL" && strings.Contains(rest, "[build failed]") {
				rep.Errors = append(rep.Errors, pkg+": build failed")
				flush(pkg)
				continue
			}
			if m[1] == "FAIL" && !hasFailure(pending) {
				rep.Errors = append(rep.Errors, pkg+": failed outside any test")
			}
			// Attach captured output to failures before flushing.
			for i := range pending {
				if pending[i].Status == report.Failed {
					if b := output[pending[i].Identifier]; b != nil {
						pending[i].Output = b.String()
					}
				}
			}
			flush(pkg)
			continue
		}
		if line == "PASS" || line == "FAIL" {
			continue
		}
		switch {
		case current != "":
			appendOutput(current, line)
		case lastFailed >= 0:
			appendOutput(pending[lastFailed].Identifier, line)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// A stream cut short (no final package line) still yields its tests.
	if len(pending) > 0 {
		for i := range pending {
			if b := output[pending[i].Identifier]; b != nil && pending[i].Status == report.Failed {
				pending[i].Output = b.String()
			}
		}
		flush("")
	}
	if !sawTest && len(rep.Errors) == 0 {
		return nil, fmt.Errorf("no test results found (run `go test -v` or `go test -json`)")
	}
	return rep, nil
}

func hasFailure(list []report.Result) bool {
	for _, r := range list {
		if r.Status == report.Failed {
			return true
		}
	}
	return false
}
