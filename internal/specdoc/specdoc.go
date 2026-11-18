// Package specdoc renders a product specification as Markdown (FR-137): the
// product structure, constraints, requirements and, on request, tasks, in
// the style of docs/spec/requirements.md.
//
// Rendering is a pure function of a Document. The caller decides what goes
// in and in which order; Render only formats, and formats identically for
// identical input, so an export committed to a repository diffs cleanly.
package specdoc

import (
	"strconv"
	"strings"
)

// Document is everything a specification export contains, already ordered.
type Document struct {
	Product     string
	Description string // Markdown

	// Components with their capabilities, and the capabilities that belong
	// to no component.
	Components   []Component
	Capabilities []Capability

	// Documents is nil when documents were not exported (FR-139), and
	// rendered only then. Their bodies are exported as files of their own.
	Documents []DocumentEntry

	Constraints  []Section
	Requirements []Section
	// Tasks is nil when tasks were not requested, and rendered only then.
	Tasks []Section
}

// Component is a structural part of the product.
type Component struct {
	Name         string
	Description  string // Markdown
	Capabilities []Capability
}

// Capability is a testable feature area.
type Capability struct {
	Name        string
	Description string // Markdown
}

// Section is one category of items, e.g. Functionality (FR).
type Section struct {
	Category string
	Items    []Item
}

// Item is a requirement, task or constraint.
type Item struct {
	Identifier       string // e.g. FR-12
	Title            string
	Description      string // Markdown
	DefinitionOfDone string // Markdown; requirements only
	// Refines names the parent of an item that is not nested below it,
	// because the parent belongs to another category or kind.
	Refines      string
	Capabilities []string
	// Documents are the identifiers of the documents linked to the item.
	Documents []string
	Children  []Item
}

// DocumentEntry lists one document of the specification (FR-139).
type DocumentEntry struct {
	Identifier string // e.g. DOC-3
	Title      string
	Format     string
	State      string // draft or current
	Revision   int
	File       string // the file holding the body, e.g. DOC-3.md
	// Identifiers of the requirements and constraints, and names of the test
	// cases, the document is linked to.
	Requirements, Constraints, TestCases []string
}

// header explains where the document comes from. It carries no timestamp
// on purpose: two exports of an unchanged product must be identical.
const header = "<!-- Exported from Nexmap. Change the items in Nexmap rather than here; " +
	"identifiers such as FR-12 name them in the CLI and REST API. -->"

// Render formats the document as Markdown, ending with a single newline.
func Render(d Document) string {
	w := &writer{}
	w.block("# " + oneLine(d.Product) + " Specification")
	w.block(normalize(d.Description))
	w.block(header)

	w.block("## Product Structure")
	if len(d.Components) == 0 && len(d.Capabilities) == 0 {
		w.block("_None._")
	}
	if len(d.Components) > 0 {
		w.block("### Components")
		for _, c := range d.Components {
			w.block("#### " + oneLine(c.Name))
			w.block(normalize(c.Description))
			w.capabilities(c.Capabilities)
		}
	}
	if len(d.Capabilities) > 0 {
		heading := "### Capabilities"
		if len(d.Components) > 0 {
			heading = "### Capabilities Without a Component"
		}
		w.block(heading)
		w.capabilities(d.Capabilities)
	}

	if d.Documents != nil {
		w.documents(d.Documents)
	}
	w.sections("Constraints", d.Constraints)
	w.sections("Requirements", d.Requirements)
	if d.Tasks != nil {
		w.sections("Tasks", d.Tasks)
	}
	return w.String()
}

// writer collects blocks separated by one blank line.
type writer struct {
	blocks []string
}

// block appends a block; empty blocks are dropped.
func (w *writer) block(s string) {
	if s = strings.TrimRight(s, "\n"); s != "" {
		w.blocks = append(w.blocks, s)
	}
}

func (w *writer) String() string { return strings.Join(w.blocks, "\n\n") + "\n" }

func (w *writer) capabilities(caps []Capability) {
	var b strings.Builder
	for i, c := range caps {
		if i > 0 && (multiLine(caps[i-1].Description) || multiLine(c.Description)) {
			b.WriteByte('\n') // a multi-paragraph capability gets air around it
		}
		b.WriteString("* **" + oneLine(c.Name) + "**")
		switch desc := normalize(c.Description); {
		case desc == "":
			b.WriteByte('\n')
		case !strings.Contains(desc, "\n"):
			b.WriteString(": " + desc + "\n") // like the entries of docs/spec/requirements.md
		default:
			b.WriteString("\n\n" + indent(desc, 2) + "\n")
		}
	}
	w.block(b.String())
}

// documents lists the documents; each body lives in the file named.
func (w *writer) documents(docs []DocumentEntry) {
	w.block("## Documents")
	if len(docs) == 0 {
		w.block("_None._")
		return
	}
	var b strings.Builder
	for i, d := range docs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("* **" + d.Identifier + "**: " + oneLine(d.Title) + "\n")
		meta := []string{"File: `" + d.File + "`.", "Format: " + d.Format + ".", "Revision " + strconv.Itoa(d.Revision) + "."}
		if d.State != "current" {
			meta = append(meta, "State: "+d.State+".")
		}
		b.WriteString("\n" + indent(strings.Join(meta, " "), 2) + "\n")
		var links []string
		if len(d.Requirements) > 0 {
			links = append(links, "Requirements: "+strings.Join(d.Requirements, ", ")+".")
		}
		if len(d.Constraints) > 0 {
			links = append(links, "Constraints: "+strings.Join(d.Constraints, ", ")+".")
		}
		if len(d.TestCases) > 0 {
			links = append(links, "Test cases: "+strings.Join(oneLines(d.TestCases), "; ")+".")
		}
		if len(links) > 0 {
			b.WriteString("\n" + indent(strings.Join(links, " "), 2) + "\n")
		}
	}
	w.block(b.String())
}

func (w *writer) sections(title string, sections []Section) {
	w.block("## " + title)
	if len(sections) == 0 {
		w.block("_None._")
		return
	}
	for _, s := range sections {
		w.block("### " + oneLine(s.Category))
		var b strings.Builder
		writeItems(&b, s.Items, 0)
		w.block(b.String())
	}
}

// writeItems writes items as a bullet list indented by depth*2 spaces.
// Items with nothing but a title stay on one line, like the entries of
// docs/spec/requirements.md; anything more follows as indented paragraphs.
func writeItems(b *strings.Builder, items []Item, depth int) {
	pad := strings.Repeat(" ", depth*2)
	for i, it := range items {
		if i > 0 && (hasBody(items[i-1]) || hasBody(it)) {
			b.WriteByte('\n')
		}
		b.WriteString(pad + "* **" + it.Identifier + "**: " + oneLine(it.Title) + "\n")
		var parts []string
		if desc := normalize(it.Description); desc != "" {
			parts = append(parts, desc)
		}
		if dod := normalize(it.DefinitionOfDone); dod != "" {
			parts = append(parts, "Definition of done:\n\n"+dod)
		}
		var meta []string
		if it.Refines != "" {
			meta = append(meta, "Refines "+it.Refines+".")
		}
		if len(it.Capabilities) > 0 {
			meta = append(meta, "Capabilities: "+strings.Join(oneLines(it.Capabilities), ", ")+".")
		}
		if len(it.Documents) > 0 {
			meta = append(meta, "Documents: "+strings.Join(it.Documents, ", ")+".")
		}
		if len(meta) > 0 {
			parts = append(parts, strings.Join(meta, " "))
		}
		for _, p := range parts {
			b.WriteString("\n" + indent(p, depth*2+2) + "\n")
		}
		if len(it.Children) > 0 {
			b.WriteByte('\n')
			writeItems(b, it.Children, depth+1)
		}
	}
}

func multiLine(s string) bool { return strings.Contains(normalize(s), "\n") }

func hasBody(it Item) bool {
	return normalize(it.Description) != "" || normalize(it.DefinitionOfDone) != "" ||
		it.Refines != "" || len(it.Capabilities) > 0 || len(it.Documents) > 0 || len(it.Children) > 0
}

// normalize makes Markdown text diff-stable: LF line endings, no trailing
// whitespace, no leading or trailing blank lines.
func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// indent prefixes every non-empty line with n spaces, so the text continues
// the list item it belongs to.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}

// oneLine keeps titles and names on their line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func oneLines(list []string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = oneLine(s)
	}
	return out
}
