package ops

import "testing"

func TestIsSubtest(t *testing.T) {
	reported := map[string]bool{
		"example.com/app.TestA":        true,
		"example.com/app.TestA/case_1": true,
		"example.com/app.TestB/x":      true, // parent not reported
		"TestC":                        true,
		"TestC/deep/er":                true,
	}
	cases := map[string]bool{
		"example.com/app.TestA":        false, // the module path's slash is no subtest
		"example.com/app.TestA/case_1": true,
		"example.com/app.TestB/x":      false, // without its parent it stands alone
		"TestC":                        false,
		"TestC/deep/er":                true,
	}
	for id, want := range cases {
		if got := isSubtest(id, reported); got != want {
			t.Errorf("isSubtest(%q) = %v, want %v", id, got, want)
		}
	}
}
