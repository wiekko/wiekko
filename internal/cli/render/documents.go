// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"wiekko.net/internal/cli/ops"
	"wiekko.net/internal/cli/output"
)

var documentHeaders = []string{"ID", "Title", "Format", "State", "Rev"}

// Documents renders a document list.
func Documents(w io.Writer, f output.Format, items []ops.DocumentSummary) error {
	rows := make([][]string, 0, len(items))
	for _, x := range items {
		rows = append(rows, []string{x.Identifier, x.Title, x.Format, x.State, strconv.Itoa(x.Revision)})
	}
	return table(w, f, "No matching documents.", documentHeaders, rows)
}

// Document renders one document. A Markdown body is shown as it is; any
// other format goes into a code block, fenced longer than any backtick run
// in the body so the block cannot be closed early.
func Document(w io.Writer, x *ops.DocumentDetail) error {
	var d doc
	d.h(1, x.Identifier+" "+x.Title)
	d.kv("Format", x.Format)
	d.kv("State", x.State)
	d.kv("Revision", strconv.Itoa(x.Revision))
	d.kv("Superseded by", x.SupersededBy)
	d.kv("Component", x.Component)
	d.kv("Capability", x.Capability)
	d.kv("Milestone", x.Milestone)
	d.kv("File", x.File)
	d.kv("ID", x.ID)
	d.gap()
	d.h(2, "Content")
	if x.Format == "markdown" {
		d.text(x.Body)
	} else {
		d.b.WriteString(codeBlock(x.Format, x.Body))
	}
	for _, l := range []struct {
		title string
		refs  []ops.Ref
	}{{"Requirements", x.Requirements}, {"Constraints", x.Constraints}, {"Test cases", x.TestCases}} {
		d.h(2, l.title)
		d.list(refLines(l.refs))
	}
	comments(&d, x.Comments)
	return d.write(w)
}

func codeBlock(lang, body string) string {
	longest, run := 0, 0
	for _, r := range body {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	body = strings.TrimRight(body, "\n")
	return fence + lang + "\n" + body + "\n" + fence + "\n\n"
}

func refLines(refs []ops.Ref) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, strings.TrimSpace(r.Identifier+" "+r.Title))
	}
	return out
}

// documentsSection lists the documents linked to an item (FR-141).
func documentsSection(d *doc, refs []ops.Ref) {
	d.h(2, "Documents")
	d.list(refLines(refs))
}

// Revisions renders a document's revisions.
func Revisions(w io.Writer, f output.Format, items []ops.RevisionView) error {
	rows := make([][]string, 0, len(items))
	for _, r := range items {
		rows = append(rows, []string{strconv.Itoa(r.Revision), r.Title, r.Format, dash(r.Author), r.CreatedAt})
	}
	return table(w, f, "No revisions.", []string{"Rev", "Title", "Format", "Author", "Created"}, rows)
}

// Diff renders a revision diff in unified format.
func Diff(w io.Writer, x *ops.DocumentDiffView) error {
	var notes []string
	if x.TitleChanged {
		notes = append(notes, "title changed")
	}
	if x.FormatChanged {
		notes = append(notes, "format changed")
	}
	if x.Unified == "" {
		notes = append(notes, "content unchanged")
	}
	if len(notes) > 0 {
		if _, err := fmt.Fprintf(w, "%s revision %d → %d: %s\n", x.Document, x.From, x.To, strings.Join(notes, ", ")); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, x.Unified)
	return err
}
