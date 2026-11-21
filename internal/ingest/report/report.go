// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Package report defines what every test report parser produces.
package report

// Result statuses, matching campaign result statuses.
const (
	Passed  = "passed"
	Failed  = "failed"
	Skipped = "skipped"
)

// Result is one test as reported by a runner.
type Result struct {
	Identifier string // matched against test cases' test_identifier
	Status     string // Passed, Failed or Skipped
	Output     string // captured output; only kept for failures
	DurationMS int64
}

// Report is everything parsed from one input.
type Report struct {
	Results []Result
	// Failures outside any test: packages that failed to build or whose
	// tests crashed before reporting, suites that errored as a whole. They
	// match no test case but make the run fail.
	Errors []string
}

// Failed reports whether any test or the run itself failed.
func (r *Report) Failed() bool {
	if len(r.Errors) > 0 {
		return true
	}
	for _, res := range r.Results {
		if res.Status == Failed {
			return true
		}
	}
	return false
}

// Counts returns the number of results per status.
func (r *Report) Counts() map[string]int {
	m := map[string]int{Passed: 0, Failed: 0, Skipped: 0}
	for _, res := range r.Results {
		m[res.Status]++
	}
	return m
}
