// Package render turns ops views into markdown (for agent context windows)
// and plain tables (for people). JSON output needs no renderer: it is the view
// itself. Detail views render as markdown in both text formats, since a
// multi-paragraph description does not fit a table.
package render

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"

	"nexmap.net/internal/cli/ops"
	"nexmap.net/internal/cli/output"
)

// table writes rows as an aligned table, or as a markdown table for md.
func table(w io.Writer, f output.Format, empty string, headers []string, rows [][]string) error {
	if len(rows) == 0 {
		if f == output.Markdown {
			_, err := fmt.Fprintf(w, "_%s_\n", empty)
			return err
		}
		_, err := fmt.Fprintln(w, empty)
		return err
	}
	if f == output.Markdown {
		var b strings.Builder
		b.WriteString("| " + strings.Join(headers, " | ") + " |\n")
		b.WriteString("|" + strings.Repeat(" --- |", len(headers)) + "\n")
		for _, r := range rows {
			cells := make([]string, len(r))
			for i, c := range r {
				cells[i] = mdCell(c)
			}
			b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		}
		_, err := io.WriteString(w, b.String())
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, strings.ToUpper(strings.Join(headers, "\t")))
	for _, r := range rows {
		cells := make([]string, len(r))
		for i, c := range r {
			cells[i] = oneLine(c)
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

func mdCell(s string) string {
	return strings.ReplaceAll(oneLine(s), "|", `\|`)
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func flags(pairs ...any) string {
	var out []string
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1].(bool) {
			out = append(out, pairs[i].(string))
		}
	}
	return strings.Join(out, ",")
}

// date shortens an RFC 3339 timestamp at midnight UTC, which is how the
// server stores plain dates, to the date alone.
func date(s string) string {
	return strings.TrimSuffix(s, "T00:00:00Z")
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// doc accumulates a markdown document.
type doc struct{ b strings.Builder }

func (d *doc) h(level int, s string) { fmt.Fprintf(&d.b, "%s %s\n\n", strings.Repeat("#", level), s) }

func (d *doc) kv(label, value string) {
	if value != "" {
		fmt.Fprintf(&d.b, "- **%s:** %s\n", label, value)
	}
}

func (d *doc) gap() { d.b.WriteString("\n") }

func (d *doc) text(s string) {
	s = strings.TrimSpace(s)
	if s == "" {
		s = "_none_"
	}
	d.b.WriteString(s + "\n\n")
}

func (d *doc) section(level int, title, body string) {
	d.h(level, title)
	d.text(body)
}

func (d *doc) list(items []string) {
	if len(items) == 0 {
		d.b.WriteString("_none_\n\n")
		return
	}
	for _, it := range items {
		d.b.WriteString("- " + it + "\n")
	}
	d.gap()
}

func (d *doc) table(headers []string, rows [][]string) {
	_ = table(&d.b, output.Markdown, "none", headers, rows)
	d.gap()
}

func (d *doc) write(w io.Writer) error {
	_, err := io.WriteString(w, strings.TrimRight(d.b.String(), "\n")+"\n")
	return err
}

// --- Requirements ---

var requirementHeaders = []string{"ID", "Kind", "Title", "Status", "Prio", "Flags"}

func requirementRows(items []ops.RequirementSummary) [][]string {
	rows := make([][]string, 0, len(items))
	for _, r := range items {
		rows = append(rows, []string{dash(r.Identifier), r.Kind, r.Title, dash(r.Status), strconv.Itoa(r.Priority),
			flags("active", r.Active, "coarse", r.Coarse, "closed", r.StatusClosed)})
	}
	return rows
}

// Requirements renders a requirement/task list.
func Requirements(w io.Writer, f output.Format, items []ops.RequirementSummary) error {
	return table(w, f, "No matching requirements or tasks.", requirementHeaders, requirementRows(items))
}

// Requirement renders one requirement or task.
func Requirement(w io.Writer, r *ops.RequirementDetail) error {
	var d doc
	d.h(1, strings.TrimSpace(r.Identifier+" "+r.Title))
	d.kv("Kind", r.Kind)
	d.kv("Category", r.Category)
	d.kv("Status", r.Status)
	d.kv("Priority", strconv.Itoa(r.Priority))
	d.kv("Flags", flags("active", r.Active, "coarse (needs refinement)", r.Coarse, "closed", r.StatusClosed))
	d.kv("Estimation", r.Estimation)
	d.kv("Sprint", r.Sprint)
	d.kv("Milestone", r.Milestone)
	if r.Parent != nil {
		d.kv("Parent", strings.TrimSpace(r.Parent.Identifier+" "+r.Parent.Title))
	}
	d.kv("Assignees", strings.Join(r.Assignees, ", "))
	d.kv("ID", r.ID)
	d.gap()
	d.section(2, "Description", r.Description)
	if r.Kind != "task" {
		d.section(2, "Definition of done", r.DefinitionOfDone)
	}
	if len(r.Children) > 0 {
		d.h(2, "Children")
		d.table(requirementHeaders, requirementRows(r.Children))
	}
	if r.Kind != "task" {
		d.h(2, "Capabilities")
		caps := make([]string, 0, len(r.Capabilities))
		for _, c := range r.Capabilities {
			caps = append(caps, c.Title)
		}
		d.list(caps)
		d.h(2, "Test cases")
		d.table(testCaseHeaders, testCaseRows(r.TestCases))
	}
	d.h(2, "Defects")
	d.table(defectHeaders, defectRows(r.Defects))
	if r.Kind != "task" {
		documentsSection(&d, r.Documents)
	}
	comments(&d, r.Comments)
	return d.write(w)
}

// --- Constraints ---

var constraintHeaders = []string{"ID", "Title", "Category", "Compliance", "Prio"}

func constraintRows(items []ops.ConstraintSummary) [][]string {
	rows := make([][]string, 0, len(items))
	for _, c := range items {
		rows = append(rows, []string{dash(c.Identifier), c.Title, c.Category, c.Compliance, strconv.Itoa(c.Priority)})
	}
	return rows
}

// Constraints renders a constraint list.
func Constraints(w io.Writer, f output.Format, items []ops.ConstraintSummary) error {
	return table(w, f, "No matching constraints.", constraintHeaders, constraintRows(items))
}

// Constraint renders one constraint.
func Constraint(w io.Writer, c *ops.ConstraintDetail) error {
	var d doc
	d.h(1, strings.TrimSpace(c.Identifier+" "+c.Title))
	d.kv("Category", c.Category)
	d.kv("Compliance", c.Compliance)
	d.kv("Priority", strconv.Itoa(c.Priority))
	d.kv("Milestone", c.Milestone)
	d.kv("ID", c.ID)
	d.gap()
	d.section(2, "Description", c.Description)
	d.h(2, "Capabilities")
	caps := make([]string, 0, len(c.Capabilities))
	for _, x := range c.Capabilities {
		caps = append(caps, x.Title)
	}
	d.list(caps)
	documentsSection(&d, c.Documents)
	comments(&d, c.Comments)
	return d.write(w)
}

// --- Defects ---

var defectHeaders = []string{"ID", "Title", "Criticality", "Status", "Prio", "Flags"}

func defectRows(items []ops.DefectSummary) [][]string {
	rows := make([][]string, 0, len(items))
	for _, x := range items {
		rows = append(rows, []string{x.Identifier, x.Title, x.Criticality, dash(x.Status), strconv.Itoa(x.Priority),
			flags("active", x.Active, "closed", x.StatusClosed)})
	}
	return rows
}

// Defects renders a defect list.
func Defects(w io.Writer, f output.Format, items []ops.DefectSummary) error {
	return table(w, f, "No matching defects.", defectHeaders, defectRows(items))
}

// Defect renders one defect.
func Defect(w io.Writer, x *ops.DefectDetail) error {
	var d doc
	d.h(1, "Defect "+x.Identifier+": "+x.Title)
	d.kv("Criticality", x.Criticality)
	d.kv("Status", x.Status)
	d.kv("Priority", strconv.Itoa(x.Priority))
	d.kv("Flags", flags("active", x.Active, "closed", x.StatusClosed))
	d.kv("Environment", x.Environment)
	d.kv("Sprint", x.Sprint)
	d.kv("Milestone", x.Milestone)
	d.kv("Assignees", strings.Join(x.Assignees, ", "))
	d.kv("ID", x.ID)
	d.gap()
	d.section(2, "Description", x.Description)
	d.h(2, "Linked to")
	links := make([]string, 0, len(x.Links))
	for _, l := range x.Links {
		links = append(links, strings.ReplaceAll(l.Type, "_", " ")+": "+l.Label)
	}
	d.list(links)
	comments(&d, x.Comments)
	return d.write(w)
}

// --- Test cases ---

var testCaseHeaders = []string{"ID", "Title", "Mode", "Test identifier", "Capability"}

func testCaseRows(items []ops.TestCaseSummary) [][]string {
	rows := make([][]string, 0, len(items))
	for _, t := range items {
		rows = append(rows, []string{t.ShortID, t.Title, t.Mode, dash(t.TestIdentifier), dash(t.Capability)})
	}
	return rows
}

// TestCases renders a test case list.
func TestCases(w io.Writer, f output.Format, items []ops.TestCaseSummary) error {
	return table(w, f, "No matching test cases.", testCaseHeaders, testCaseRows(items))
}

// TestCase renders one test case.
func TestCase(w io.Writer, t *ops.TestCaseDetail) error {
	var d doc
	d.h(1, "Test case "+t.ShortID+": "+t.Title)
	d.kv("Mode", t.Mode)
	d.kv("Test identifier", t.TestIdentifier)
	d.kv("Component", t.Component)
	d.kv("Capability", t.Capability)
	d.kv("Pyramid level", t.PyramidLevel)
	d.kv("Environments", strings.Join(t.Environments, ", "))
	d.kv("ID", t.ID)
	d.gap()
	d.section(2, "Description", t.Description)
	d.h(2, "Covers")
	reqs := make([]string, 0, len(t.Requirements))
	for _, r := range t.Requirements {
		s := strings.TrimSpace(r.Identifier + " " + r.Title)
		if r.LinkType == "definition_of_done" {
			s += " (definition of done)"
		}
		reqs = append(reqs, s)
	}
	d.list(reqs)
	d.h(2, "Results")
	rows := make([][]string, 0, len(t.Results))
	for _, r := range t.Results {
		rows = append(rows, []string{r.Campaign, r.CampaignStatus, dash(r.Environment), r.Status, dash(r.ExecutedAt)})
	}
	d.table([]string{"Campaign", "Campaign status", "Environment", "Result", "Executed"}, rows)
	d.h(2, "Defects")
	d.table(defectHeaders, defectRows(t.Defects))
	documentsSection(&d, t.Documents)
	comments(&d, t.Comments)
	return d.write(w)
}

// --- Structure ---

// Structure renders the component → capability tree.
func Structure(w io.Writer, f output.Format, s *ops.Structure) error {
	if f != output.Markdown {
		rows := [][]string{}
		for _, c := range s.Components {
			if len(c.Capabilities) == 0 {
				rows = append(rows, []string{c.Name, "-"})
			}
			for _, cp := range c.Capabilities {
				rows = append(rows, []string{c.Name, cp.Name})
			}
		}
		for _, cp := range s.ProductLevel {
			rows = append(rows, []string{"(product level)", cp.Name})
		}
		return table(w, f, "No components or capabilities defined.", []string{"Component", "Capability"}, rows)
	}
	var d doc
	structureMD(&d, s)
	return d.write(w)
}

func structureMD(d *doc, s *ops.Structure) {
	if len(s.Components) == 0 && len(s.ProductLevel) == 0 {
		d.text("_No components or capabilities defined._")
		return
	}
	for _, c := range s.Components {
		line := "**" + c.Name + "**"
		if c.Description != "" {
			line += " — " + oneLine(c.Description)
		}
		d.b.WriteString("- " + line + "\n")
		for _, cp := range c.Capabilities {
			d.b.WriteString("  - " + capabilityLine(cp) + "\n")
		}
	}
	if len(s.ProductLevel) > 0 {
		d.b.WriteString("- **Product level**\n")
		for _, cp := range s.ProductLevel {
			d.b.WriteString("  - " + capabilityLine(cp) + "\n")
		}
	}
	d.gap()
}

func capabilityLine(c ops.CapabilityView) string {
	if c.Description == "" {
		return c.Name
	}
	return c.Name + " — " + oneLine(c.Description)
}

// Capability renders one capability.
func Capability(w io.Writer, c *ops.CapabilityDetail) error {
	var d doc
	d.h(1, "Capability: "+c.Name)
	d.kv("Component", c.Component)
	d.kv("ID", c.ID)
	d.gap()
	d.section(2, "Description", c.Description)
	d.h(2, "Requirements")
	d.table(requirementHeaders, requirementRows(c.Requirements))
	d.h(2, "Constraints")
	d.table(constraintHeaders, constraintRows(c.Constraints))
	d.h(2, "Test cases")
	d.table(testCaseHeaders, testCaseRows(c.TestCases))
	d.h(2, "Defects")
	d.table(defectHeaders, defectRows(c.Defects))
	comments(&d, c.Comments)
	return d.write(w)
}

// --- Campaigns ---

// Campaign renders a campaign status.
func Campaign(w io.Writer, v *ops.CampaignStatusView) error {
	var d doc
	if v.Campaign == nil {
		d.text(v.Message)
		return d.write(w)
	}
	c := v.Campaign
	d.h(1, "Campaign: "+c.Name)
	campaignKV(&d, c)
	d.gap()
	d.h(2, "Needs attention")
	rows := make([][]string, 0, len(c.Attention))
	for _, r := range c.Attention {
		title := ""
		if r.TestCase != nil {
			title = r.TestCase.Title
		}
		defect := ""
		if r.DefectID != "" {
			defect = r.Defect
			if defect == "" { // the token may not read defects
				defect = ops.ShortID(r.DefectID)
			}
		}
		rows = append(rows, []string{title, dash(r.TestIdentifier), dash(r.Environment), r.Status, dash(defect)})
	}
	d.table([]string{"Test case", "Test identifier", "Environment", "Result", "Defect"}, rows)
	return d.write(w)
}

func campaignKV(d *doc, c *ops.CampaignView) {
	d.kv("Status", c.Status)
	d.kv("Verdict", c.Verdict)
	d.kv("Planned", strings.Trim(date(c.StartsAt)+" – "+date(c.EndsAt), " –"))
	order := []string{"passed", "failed", "blocked", "retest", "pending", "skipped"}
	var parts []string
	for _, k := range order {
		if n := c.Counts[k]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k))
		}
	}
	d.kv("Results", fmt.Sprintf("%d total — %s", c.Total, dash(strings.Join(parts, ", "))))
}

// --- Context ---

// Context renders the product briefing.
func Context(w io.Writer, v *ops.ContextView) error {
	var d doc
	d.h(1, "Product: "+v.Product.Name)
	d.text(v.Product.Description)
	d.kv("Workflow mode", v.Product.WorkflowMode)
	d.kv("Estimation", v.Product.EstimationMode)
	d.kv("Statuses (board order)", strings.Join(v.Workflow, " → "))
	d.kv("Product ID", v.Product.ID)
	d.gap()

	d.h(2, "Structure (components → capabilities)")
	structureMD(&d, &v.Structure)

	d.h(2, "Constraints")
	d.table(constraintHeaders, constraintRows(v.Constraints))

	d.h(2, "Current work")
	if v.ActiveSprint != nil {
		d.kv("Active sprint", fmt.Sprintf("%s (%s – %s)", v.ActiveSprint.Name, date(v.ActiveSprint.StartDate), date(v.ActiveSprint.EndDate)))
	}
	d.kv("Open backlog", fmt.Sprintf("%d requirements, %d tasks (%d need refinement)",
		v.Backlog.OpenRequirements, v.Backlog.OpenTasks, v.Backlog.Coarse))
	d.gap()
	d.h(3, "In progress (active, not closed)")
	d.table(requirementHeaders, requirementRows(v.InProgress))

	d.h(2, "Open defects")
	d.table(defectHeaders, defectRows(v.OpenDefects))

	d.h(2, "Testing")
	if v.ActiveCampaign == nil {
		d.text("No testing campaign is active.")
	} else {
		d.kv("Active campaign", v.ActiveCampaign.Name)
		campaignKV(&d, v.ActiveCampaign)
		d.gap()
	}

	d.h(2, "Next steps")
	d.list([]string{
		"`nexmap req show <ID>` — full requirement with definition of done, tests and defects",
		"`nexmap req list --open` / `nexmap defect list --open` — the full lists",
		"`nexmap constraint show <ID>`, `nexmap capability show <name>`, `nexmap test show <test identifier>`",
	})
	return d.write(w)
}

func comments(d *doc, list []ops.CommentView) {
	d.h(2, "Comments")
	if len(list) == 0 {
		d.text("")
		return
	}
	for _, c := range list {
		fmt.Fprintf(&d.b, "**%s** (%s):\n\n%s\n\n", c.Author, c.CreatedAt, strings.TrimSpace(c.Body))
	}
}

// --- Writes ---

// Comment renders a comment that was just added.
func Comment(w io.Writer, c *ops.CommentResult) error {
	var d doc
	owner := c.Owner.Title
	if c.Owner.Identifier != "" {
		owner = c.Owner.Identifier + " " + owner
	}
	d.h(1, "Comment added to "+strings.ReplaceAll(c.OwnerType, "_", " ")+" "+owner)
	d.kv("Author", c.Comment.Author)
	d.kv("Created", c.Comment.CreatedAt)
	d.gap()
	d.text(c.Comment.Body)
	return d.write(w)
}
