package textdiff

import (
	"strings"
	"testing"
)

// apply rebuilds both sides from a diff; any correct diff must reproduce them.
func apply(lines []Line) (old, new []string) {
	for _, l := range lines {
		if l.Op != Insert {
			old = append(old, l.Text)
		}
		if l.Op != Delete {
			new = append(new, l.Text)
		}
	}
	return old, new
}

func countChanges(lines []Line) int {
	n := 0
	for _, l := range lines {
		if l.Op != Equal {
			n++
		}
	}
	return n
}

func TestLinesRoundTrip(t *testing.T) {
	cases := []struct {
		a, b    string
		changes int
	}{
		{"", "", 0},
		{"a\nb\n", "a\nb\n", 0},
		{"", "a\nb\n", 2},
		{"a\nb\n", "", 2},
		{"a\nb\nc\n", "a\nx\nc\n", 2},
		{"a\nb\nc\nd\n", "b\nc\nd\ne\n", 2},
		{"a\nb\nc\na\nb\nb\na\n", "c\nb\na\nb\na\nc\n", 5}, // Myers' paper example
	}
	for _, c := range cases {
		lines := Lines(c.a, c.b)
		old, new := apply(lines)
		if strings.Join(old, "\n") != strings.Join(Split(c.a), "\n") || strings.Join(new, "\n") != strings.Join(Split(c.b), "\n") {
			t.Errorf("Lines(%q, %q) does not reproduce its inputs: %+v", c.a, c.b, lines)
		}
		if got := countChanges(lines); got != c.changes {
			t.Errorf("Lines(%q, %q) has %d changes, want %d", c.a, c.b, got, c.changes)
		}
	}
}

func TestLineNumbers(t *testing.T) {
	lines := Lines("a\nb\nc\n", "a\nx\nc\n")
	want := []Line{{Equal, "a", 1, 1}, {Delete, "b", 2, 0}, {Insert, "x", 0, 2}, {Equal, "c", 3, 3}}
	if len(lines) != len(want) {
		t.Fatalf("lines = %+v", lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, lines[i], want[i])
		}
	}
}

// TestBoundedFallsBack checks that a diff beyond the bound is still correct,
// just not minimal.
func TestBoundedFallsBack(t *testing.T) {
	a := []string{"1", "2", "3", "4"}
	b := []string{"5", "6", "7", "8"}
	lines := diff(a, b, 2)
	old, new := apply(lines)
	if strings.Join(old, ",") != "1,2,3,4" || strings.Join(new, ",") != "5,6,7,8" {
		t.Fatalf("fallback diff = %+v", lines)
	}
}

func TestUnified(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n"
	b := "1\n2\nthree\n4\n5\n6\n7\n8\n9\n10\n11\ntwelve\n"
	got := Unified("DOC-1 r1", "DOC-1 r2", Lines(a, b))
	want := "--- DOC-1 r1\n+++ DOC-1 r2\n" +
		"@@ -1,6 +1,6 @@\n 1\n 2\n-3\n+three\n 4\n 5\n 6\n" +
		"@@ -9,4 +9,4 @@\n 9\n 10\n 11\n-12\n+twelve\n"
	if got != want {
		t.Errorf("Unified =\n%s\nwant\n%s", got, want)
	}
	if Unified("a", "b", Lines(a, a)) != "" {
		t.Error("identical texts produced a diff")
	}
	if Changed(Lines(a, a)) || !Changed(Lines(a, b)) {
		t.Error("Changed misreports")
	}
}
