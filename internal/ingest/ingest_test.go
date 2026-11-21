// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package ingest_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"wiekko.net/internal/ingest"
)

type want struct {
	status string
	output []string // substrings the captured output must contain
}

func parseFile(t *testing.T, path string, opts ingest.Options) (*ingest.Report, string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rep, format, err := ingest.Parse(f, opts)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return rep, format
}

func check(t *testing.T, rep *ingest.Report, expected map[string]want) {
	t.Helper()
	got := map[string]ingest.Result{}
	for _, r := range rep.Results {
		if _, dup := got[r.Identifier]; dup {
			t.Errorf("duplicate result %s", r.Identifier)
		}
		got[r.Identifier] = r
	}
	for id, w := range expected {
		r, ok := got[id]
		if !ok {
			t.Errorf("missing %s (have %v)", id, keys(got))
			continue
		}
		if r.Status != w.status {
			t.Errorf("%s: status %s, want %s", id, r.Status, w.status)
		}
		for _, s := range w.output {
			if !strings.Contains(r.Output, s) {
				t.Errorf("%s: output lacks %q:\n%s", id, s, r.Output)
			}
		}
		if w.status != ingest.Failed && r.Output != "" {
			t.Errorf("%s: output kept for a %s test", id, r.Status)
		}
		if strings.Contains(r.Output, "=== RUN") || strings.Contains(r.Output, "--- FAIL") {
			t.Errorf("%s: framing lines in output:\n%s", id, r.Output)
		}
	}
	if len(got) != len(expected) {
		t.Errorf("%d results, want %d: %v", len(got), len(expected), keys(got))
	}
}

func keys(m map[string]ingest.Result) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Both Go formats come from the same real run (testdata/), so they must agree.
var goExpected = map[string]want{
	"example.com/sample/calc.TestAdd":            {status: ingest.Passed},
	"example.com/sample/calc.TestDivide":         {ingest.Failed, []string{"dividing 1 by 0", "got 0, want 1"}},
	"example.com/sample/calc.TestTable":          {status: ingest.Failed},
	"example.com/sample/calc.TestTable/positive": {status: ingest.Passed},
	"example.com/sample/calc.TestTable/negative": {ingest.Failed, []string{"case negative failed"}},
	"example.com/sample/calc.TestTable/zero":     {status: ingest.Passed},
	"example.com/sample/calc.TestSkipped":        {status: ingest.Skipped},
	"example.com/sample/calc.TestParallelA":      {status: ingest.Passed},
	"example.com/sample/calc.TestParallelB":      {ingest.Failed, []string{"B failed"}},
	"example.com/sample/crash.TestFine":          {status: ingest.Passed},
	"example.com/sample/crash.TestPanics":        {ingest.Failed, []string{"assignment to entry in nil map", "crash_test.go:9"}},
}

func TestGoFormats(t *testing.T) {
	for _, tc := range []struct{ file, format string }{
		{"gotest/testdata/go-json.txt", ingest.FormatGoJSON},
		{"gotest/testdata/go-text.txt", ingest.FormatGoText},
	} {
		t.Run(tc.format, func(t *testing.T) {
			rep, format := parseFile(t, tc.file, ingest.Options{})
			if format != tc.format {
				t.Errorf("detected %s, want %s", format, tc.format)
			}
			check(t, rep, goExpected)
			if len(rep.Errors) != 1 || !strings.Contains(rep.Errors[0], "example.com/sample/broken: build failed") {
				t.Errorf("errors = %q, want the build failure of the broken package", rep.Errors)
			}
			if !rep.Failed() {
				t.Error("report not failed")
			}
			// Parallel output must not leak between tests.
			for _, r := range rep.Results {
				if strings.HasSuffix(r.Identifier, "TestParallelB") && strings.Contains(r.Output, "A done") {
					t.Errorf("TestParallelB got TestParallelA's output: %q", r.Output)
				}
			}
		})
	}
}

func TestGoNameStyle(t *testing.T) {
	rep, _ := parseFile(t, "gotest/testdata/go-json.txt", ingest.Options{IdentifierStyle: ingest.StyleName})
	var ids []string
	for _, r := range rep.Results {
		if strings.Contains(r.Identifier, ".") {
			t.Errorf("name style produced %q", r.Identifier)
		}
		ids = append(ids, r.Identifier)
	}
	if !strings.Contains(strings.Join(ids, " "), "TestTable/negative") {
		t.Errorf("ids = %v", ids)
	}
}

func TestJUnit(t *testing.T) {
	for _, tc := range []struct {
		file     string
		expected map[string]want
	}{
		{"junit/testdata/pytest.xml", map[string]want{
			"tests.test_calc.test_add":                       {status: ingest.Passed},
			"tests.test_calc.test_divide":                    {ingest.Failed, []string{"assert 0 == 1", "tests/test_calc.py:7"}},
			"tests.test_calc.test_skip":                      {status: ingest.Skipped},
			"tests.test_calc.test_db":                        {ingest.Failed, []string{"fixture 'db' not found"}},
			"tests.test_calc.TestClass.test_method[param-1]": {status: ingest.Passed},
		}},
		{"junit/testdata/jest.xml", map[string]want{
			"calc adds numbers":    {status: ingest.Passed},
			"calc divides by zero": {ingest.Failed, []string{"Expected: 1", "calc.test.js:9:20"}},
		}},
		{"junit/testdata/surefire.xml", map[string]want{
			"com.example.CalcTest.testAdd":    {status: ingest.Passed},
			"com.example.CalcTest.testDivide": {ingest.Failed, []string{"expected: <1> but was: <0>", "CalcTest.java:15", "dividing..."}},
			"com.example.CalcTest.testLater":  {status: ingest.Skipped},
		}},
	} {
		t.Run(filepath.Base(tc.file), func(t *testing.T) {
			rep, format := parseFile(t, tc.file, ingest.Options{})
			if format != ingest.FormatJUnit {
				t.Errorf("detected %s", format)
			}
			check(t, rep, tc.expected)
		})
	}
}

func TestDetectAndErrors(t *testing.T) {
	for in, format := range map[string]string{
		"\ufeff  {\"Action\":\"start\"}": ingest.FormatGoJSON,
		"\n<?xml version=\"1.0\"?>":      ingest.FormatJUnit,
		"=== RUN   TestX":                ingest.FormatGoText,
	} {
		if got := ingest.Detect([]byte(in)); got != format {
			t.Errorf("Detect(%q) = %s, want %s", in, got, format)
		}
	}
	for name, tc := range map[string]struct{ in, format string }{
		"empty text":     {"", ""},
		"json garbage":   {"not json at all", ingest.FormatGoJSON},
		"wrong xml root": {"<html></html>", ingest.FormatJUnit},
		"unknown format": {"x", "tap"},
	} {
		if _, _, err := ingest.Parse(strings.NewReader(tc.in), ingest.Options{Format: tc.format}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestTruncateKeepsTail(t *testing.T) {
	s := strings.Repeat("é", ingest.MaxOutput) + "END"
	got := ingest.Truncate(s)
	if len(got) > ingest.MaxOutput || !strings.HasSuffix(got, "END") || !strings.HasPrefix(got, "é") {
		t.Errorf("Truncate: %d bytes, prefix %q", len(got), got[:4])
	}
	if ingest.Truncate("short") != "short" {
		t.Error("short output changed")
	}
	if !reflect.DeepEqual(ingest.Source(ingest.FormatGoText), "go-test") {
		t.Error("source for go-text")
	}
}
