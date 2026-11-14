// Package textdiff computes line-based differences between two texts, for
// comparing document revisions (FR-051). It implements Myers' O(ND)
// algorithm with a bound on the edit distance it explores: two texts that
// differ in more lines than the bound are reported as one replacement rather
// than spending unbounded time and memory on a minimal diff.
package textdiff

import (
	"fmt"
	"strings"
)

// Op says what happened to a line.
type Op int

// Line operations.
const (
	Equal Op = iota
	Delete
	Insert
)

// String returns the unified-diff marker of the operation.
func (o Op) String() string {
	switch o {
	case Delete:
		return "-"
	case Insert:
		return "+"
	}
	return " "
}

// Line is one line of a diff. Old and New are 1-based line numbers in the
// old and new text; the one that does not apply is 0.
type Line struct {
	Op   Op
	Text string
	Old  int
	New  int
}

// MaxEdits bounds the edit distance Lines explores. The trace kept to
// recover the script grows with its square, so this also bounds memory
// (roughly MaxEdits² ints).
const MaxEdits = 1000

// Split cuts text into lines without their line feeds. A final line feed
// does not start another line.
func Split(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// Lines returns the line-by-line difference turning a into b.
func Lines(a, b string) []Line {
	return diff(Split(a), Split(b), MaxEdits)
}

func diff(a, b []string, maxEdits int) []Line {
	// Common prefix and suffix are cheap and shrink the search space.
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	midA, midB := a[pre:len(a)-suf], b[pre:len(b)-suf]

	var out []Line
	oldN, newN := 1, 1
	emit := func(op Op, text string) {
		l := Line{Op: op, Text: text}
		switch op {
		case Equal:
			l.Old, l.New = oldN, newN
			oldN++
			newN++
		case Delete:
			l.Old = oldN
			oldN++
		case Insert:
			l.New = newN
			newN++
		}
		out = append(out, l)
	}
	for _, s := range a[:pre] {
		emit(Equal, s)
	}
	for _, e := range myers(midA, midB, maxEdits) {
		emit(e.op, e.text)
	}
	for _, s := range a[len(a)-suf:] {
		emit(Equal, s)
	}
	return out
}

type edit struct {
	op   Op
	text string
}

// myers returns a shortest edit script from a to b, or a full replacement
// when it would need more than maxEdits edits.
func myers(a, b []string, maxEdits int) []edit {
	n, m := len(a), len(b)
	if n == 0 && m == 0 {
		return nil
	}
	limit := n + m
	if limit > maxEdits {
		limit = maxEdits
	}
	offset := limit + 1
	v := make([]int, 2*limit+3)
	// trace[d] holds v[-d-1 .. d+1] as it was before round d: all the
	// backtrack below reads of it.
	var trace [][]int
	found := -1
	for d := 0; d <= limit && found < 0; d++ {
		snapshot := make([]int, 2*d+3)
		copy(snapshot, v[offset-d-1:offset+d+2])
		trace = append(trace, snapshot)
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}
			v[offset+k] = x
			if x >= n && y >= m {
				found = d
				break
			}
		}
	}
	if found < 0 {
		edits := make([]edit, 0, n+m)
		for _, s := range a {
			edits = append(edits, edit{Delete, s})
		}
		for _, s := range b {
			edits = append(edits, edit{Insert, s})
		}
		return edits
	}

	// Walk the trace backwards to recover the script.
	var rev []edit
	x, y := n, m
	for d := found; d > 0; d-- {
		vd := trace[d]
		at := func(k int) int { return vd[k+d+1] }
		k := x - y
		var prevK int
		if k == -d || (k != d && at(k-1) < at(k+1)) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := at(prevK)
		prevY := prevX - prevK
		for x > prevX && y > prevY {
			x--
			y--
			rev = append(rev, edit{Equal, a[x]})
		}
		if x == prevX {
			y--
			rev = append(rev, edit{Insert, b[y]})
		} else {
			x--
			rev = append(rev, edit{Delete, a[x]})
		}
	}
	for x > 0 && y > 0 {
		x--
		y--
		rev = append(rev, edit{Equal, a[x]})
	}
	edits := make([]edit, len(rev))
	for i, e := range rev {
		edits[len(rev)-1-i] = e
	}
	return edits
}

// Changed reports whether a diff contains any change.
func Changed(lines []Line) bool {
	for _, l := range lines {
		if l.Op != Equal {
			return true
		}
	}
	return false
}

// Hunk is a run of changes with surrounding context.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Lines              []Line
}

// Hunks groups a diff into hunks with context unchanged lines around each
// change, as in a unified diff.
func Hunks(lines []Line, context int) []Hunk {
	var hunks []Hunk
	i := 0
	for i < len(lines) {
		for i < len(lines) && lines[i].Op == Equal {
			i++
		}
		if i == len(lines) {
			break
		}
		start := i - context
		if start < 0 {
			start = 0
		}
		end := i
		// Extend while the next change is within 2*context unchanged lines.
		for end < len(lines) {
			if lines[end].Op != Equal {
				end++
				continue
			}
			run := end
			for run < len(lines) && lines[run].Op == Equal {
				run++
			}
			if run == len(lines) || run-end > 2*context {
				end += min(context, run-end)
				break
			}
			end = run
		}
		h := Hunk{Lines: lines[start:end]}
		for _, l := range h.Lines {
			switch l.Op {
			case Equal:
				h.OldLines++
				h.NewLines++
			case Delete:
				h.OldLines++
			case Insert:
				h.NewLines++
			}
			if h.OldStart == 0 && l.Old != 0 {
				h.OldStart = l.Old
			}
			if h.NewStart == 0 && l.New != 0 {
				h.NewStart = l.New
			}
		}
		hunks = append(hunks, h)
		i = end
	}
	return hunks
}

// Unified renders a diff in unified format with three lines of context.
func Unified(oldName, newName string, lines []Line) string {
	hunks := Hunks(lines, 3)
	if len(hunks) == 0 {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", oldName, newName)
	for _, h := range hunks {
		fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
		for _, l := range h.Lines {
			sb.WriteString(l.Op.String())
			sb.WriteString(l.Text)
			sb.WriteByte('\n')
		}
	}
	return sb.String()
}
