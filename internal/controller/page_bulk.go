// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"wiekko.net/internal/middleware"
	"wiekko.net/internal/model"
	"wiekko.net/internal/service"
	"wiekko.net/internal/store"
	"wiekko.net/web/templates"
	productpage "wiekko.net/web/templates/pages/product"
	"wiekko.net/web/templates/patterns"
)

// Bulk operations on the requirements, tasks, defects, test cases and
// constraints tables (FR-130, FR-131, FR-169, FR-170). The selection bar asks for a preview, which answers with a
// confirmation naming what will change and what cannot (SC-92, UC-91); the
// confirmation applies the operation to the items it previewed, then
// re-renders the table as it was.

// bulkTable describes one table that offers bulk operations.
type bulkTable struct {
	kind       string // service.BulkRequest.Kind
	collection string // URL path segment
	singular   string
	plural     string
}

var bulkTables = map[string]bulkTable{
	"requirements": {kind: model.RequirementKindRequirement, collection: "requirements", singular: "requirement", plural: "requirements"},
	"tasks":        {kind: model.RequirementKindTask, collection: "tasks", singular: "task", plural: "tasks"},
	"defects":      {kind: "defect", collection: "defects", singular: "defect", plural: "defects"},
	"test-cases":   {kind: service.BulkKindTestCase, collection: "test-cases", singular: "test case", plural: "test cases"},
	"constraints":  {kind: service.BulkKindConstraint, collection: "constraints", singular: "constraint", plural: "constraints"},
}

func bulkTableOf(kind string) bulkTable {
	switch kind {
	case model.RequirementKindTask:
		return bulkTables["tasks"]
	case "defect":
		return bulkTables["defects"]
	case service.BulkKindTestCase:
		return bulkTables["test-cases"]
	case service.BulkKindConstraint:
		return bulkTables["constraints"]
	default:
		return bulkTables["requirements"]
	}
}

func (t bulkTable) defect() bool { return t.kind == "defect" }

func (t bulkTable) testCase() bool { return t.kind == service.BulkKindTestCase }

func (t bulkTable) constraint() bool { return t.kind == service.BulkKindConstraint }

func (t bulkTable) region() string {
	switch {
	case t.defect():
		return "defects-table"
	case t.testCase():
		return "test-cases-table"
	case t.constraint():
		return productpage.ConstraintsTableID
	}
	return productpage.BacklogItemsTableID(t.kind)
}

func (t bulkTable) noun(n int) string {
	if n == 1 {
		return "1 " + t.singular
	}
	return fmt.Sprintf("%d %s", n, t.plural)
}

// bulkBar configures the selection bar of a table for the user's role: the
// operations each role may apply to single items (SC-92). Developers change
// statuses and assign or unassign themselves (UR-171); owners and admins do
// everything; others cannot select.
func bulkBar(t bulkTable, p *model.Product, user *model.User, role string, total int) patterns.BulkBarProps {
	props := patterns.BulkBarProps{
		Region:     t.region(),
		PreviewURL: "/products/" + p.ID + "/" + t.collection + "/bulk/preview",
		Noun:       t.plural,
		Total:      total,
	}
	owner := user.IsAdmin || role == model.RoleOwner
	if !owner && model.RoleRank(role) < model.RoleRank(model.RoleDeveloper) {
		return props
	}
	add := func(action service.BulkAction, label, icon string) {
		props.Actions = append(props.Actions, patterns.BulkAction{Action: string(action), Label: label, Icon: icon})
	}
	add(service.BulkStatus, "Status", "circle-dot")
	if !owner {
		add(service.BulkAssign, "Assign me", "user-plus")
		add(service.BulkUnassign, "Unassign me", "user-minus")
		return props
	}
	if t.defect() {
		add(service.BulkCriticality, "Criticality", "gauge")
	} else {
		add(service.BulkCategory, "Category", "tag")
	}
	add(service.BulkPriority, "Priority", "chevrons-up")
	add(service.BulkMilestone, "Milestone", "flag")
	if p.WorkflowMode == model.WorkflowModeScrum {
		add(service.BulkSprint, "Sprint", "timer")
	}
	add(service.BulkAssign, "Assign", "user-plus")
	add(service.BulkUnassign, "Unassign", "user-minus")
	add(service.BulkArchive, "Archive", "archive")
	add(service.BulkUnarchive, "Unarchive", "archive-restore")
	add(service.BulkDelete, "Delete", "trash-2")
	return props
}

// testCaseBulkBar configures the selection bar of the test cases table:
// developers, owners and admins change test cases (FR-111), so they get every
// operation; the pyramid level only where the product defines levels.
func testCaseBulkBar(p *model.Product, user *model.User, role string, total int, pyramidLevels bool) patterns.BulkBarProps {
	t := bulkTables["test-cases"]
	props := patterns.BulkBarProps{
		Region:     t.region(),
		PreviewURL: "/products/" + p.ID + "/" + t.collection + "/bulk/preview",
		Noun:       t.plural,
		Total:      total,
	}
	if !user.IsAdmin && model.RoleRank(role) < model.RoleRank(model.RoleDeveloper) {
		return props
	}
	add := func(action service.BulkAction, label, icon string) {
		props.Actions = append(props.Actions, patterns.BulkAction{Action: string(action), Label: label, Icon: icon})
	}
	add(service.BulkLinkRequirement, "Link requirement", "link")
	add(service.BulkUnlinkRequirement, "Unlink requirement", "unlink")
	add(service.BulkLinkConstraint, "Link constraint", "shield-check")
	add(service.BulkUnlinkConstraint, "Unlink constraint", "shield-off")
	add(service.BulkComponent, "Component", "boxes")
	add(service.BulkCapability, "Capability", "puzzle")
	if pyramidLevels {
		add(service.BulkPyramidLevel, "Pyramid level", "triangle")
	}
	add(service.BulkDelete, "Delete", "trash-2")
	return props
}

// constraintBulkBar configures the selection bar of the constraints table:
// constraints are the owners' and admins' to change (SC-92).
func constraintBulkBar(p *model.Product, user *model.User, role string, total int) patterns.BulkBarProps {
	t := bulkTables["constraints"]
	props := patterns.BulkBarProps{
		Region:     t.region(),
		PreviewURL: "/products/" + p.ID + "/" + t.collection + "/bulk/preview",
		Noun:       t.plural,
		Total:      total,
	}
	if !user.IsAdmin && role != model.RoleOwner {
		return props
	}
	props.Actions = []patterns.BulkAction{
		{Action: string(service.BulkCompliance), Label: "Compliance", Icon: "shield-check"},
		{Action: string(service.BulkCategory), Label: "Category", Icon: "tag"},
		{Action: string(service.BulkPriority), Label: "Priority", Icon: "chevrons-up"},
		{Action: string(service.BulkDelete), Label: "Delete", Icon: "trash-2"},
	}
	return props
}

// bulkForm is a bulk request as the selection bar or the confirmation sends
// it.
type bulkForm struct {
	action   service.BulkAction
	value    string
	hasValue bool
	children service.ChildPolicy
	ids      []string
	// selection is a confirmation's whole selection when it previews again
	// with another value; ids then are only the items it offered.
	selection []string
	matching  bool   // every item matching state, not ids
	state     string // the table's query string
}

func readBulkForm(r *http.Request) bulkForm {
	_ = r.ParseForm()
	f := bulkForm{
		action:   service.BulkAction(r.PostFormValue("action")),
		value:    r.PostFormValue("value"),
		matching: r.PostFormValue("scope") == "matching",
		state:    strings.TrimPrefix(r.PostFormValue("state"), "?"),
	}
	_, f.hasValue = r.PostForm["value"]
	if r.PostFormValue("children") == "delete" {
		f.children = service.DeleteChildren
	}
	f.ids = splitIDs(r.PostFormValue("ids"))
	f.selection = splitIDs(r.PostFormValue("selection"))
	return f
}

func splitIDs(s string) []string {
	var ids []string
	for _, id := range strings.Split(s, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// stateRequest is r as if it asked for the table's own URL with state, so the
// list parsers read the table's search, filters and sort.
func stateRequest(r *http.Request, state string) *http.Request {
	r2 := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = state
	r2.URL = &u
	return r2
}

// bulkTableFromPath resolves the {collection} of a bulk route.
func bulkTableFromPath(r *http.Request) (bulkTable, bool) {
	t, ok := bulkTables[r.PathValue("collection")]
	return t, ok
}

// matchingIDs lists every item matching the table's search and filters, in
// the table's order, up to one past the cap so an oversized selection shows.
func (c *PageProductController) matchingIDs(r *http.Request, user *model.User, productID string, t bulkTable, state string) ([]string, error) {
	sr := stateRequest(r, state)
	var ids []string
	if t.constraint() {
		all, table, _, _ := c.constraintRows(r, user, productID)
		for _, con := range table.matching(all, table.readState(sr, "")) {
			ids = append(ids, con.ID)
		}
		return ids, nil
	}
	if t.testCase() {
		filter, ls := parseTestCaseListState(sr, "")
		params := listParams(ls)
		params.Limit, params.Offset = service.BulkMaxItems+1, 0
		items, _, err := c.testing.ListTestCasesPage(r.Context(), user, productID, filter, params)
		if err != nil {
			return nil, err
		}
		for _, tc := range items {
			ids = append(ids, tc.ID)
		}
		return ids, nil
	}
	if t.defect() {
		filter, params, _ := parseDefectListState(sr, "")
		params.Limit, params.Offset = service.BulkMaxItems+1, 0
		items, _, err := c.backlog.ListDefectsPage(r.Context(), user, productID, filter, params)
		if err != nil {
			return nil, err
		}
		for _, d := range items {
			ids = append(ids, d.ID)
		}
		return ids, nil
	}
	filter, ls := parseBacklogListState(sr, "", t.kind)
	params := listParams(ls)
	params.Limit, params.Offset = service.BulkMaxItems+1, 0
	items, _, err := c.backlog.ListRequirementsPage(r.Context(), user, productID, filter, params)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		ids = append(ids, it.ID)
	}
	return ids, nil
}

// bulkNamer names the items of one table for a confirmation.
type bulkNamer struct {
	c        *PageProductController
	r        *http.Request
	user     *model.User
	product  string
	table    bulkTable
	prefixes map[string]string
}

func (n *bulkNamer) item(id string) patterns.BulkItem {
	ctx := n.r.Context()
	if n.table.constraint() {
		if n.prefixes == nil {
			n.prefixes = n.c.buildCategoryPrefixMap(n.r, n.user, n.product, "constraint")
		}
		if con, err := n.c.constraint.GetConstraint(ctx, n.user, n.product, id); err == nil {
			return patterns.BulkItem{Identifier: model.EntityIdentifier(requirementPrefixOf(n.prefixes, con.CategoryID), con.SequenceNumber), Title: con.Title}
		}
		return patterns.BulkItem{Title: "(no longer exists)"}
	}
	if n.table.testCase() {
		if tc, err := n.c.testing.GetTestCase(ctx, n.user, n.product, id); err == nil {
			return patterns.BulkItem{Identifier: shortID(tc.ID), Title: tc.Title}
		}
		return patterns.BulkItem{Title: "(no longer exists)"}
	}
	if n.table.defect() {
		if d, err := n.c.backlog.GetDefect(ctx, n.user, n.product, id); err == nil {
			return patterns.BulkItem{Identifier: d.Identifier, Title: d.Title}
		}
		return patterns.BulkItem{Title: "(no longer exists)"}
	}
	if n.prefixes == nil {
		n.prefixes = n.c.buildCategoryPrefixMap(n.r, n.user, n.product, n.table.kind)
	}
	if it, err := n.c.backlog.GetRequirement(ctx, n.user, n.product, id); err == nil {
		return patterns.BulkItem{Identifier: model.EntityIdentifier(requirementPrefixOf(n.prefixes, it.CategoryID), it.SequenceNumber), Title: it.Title}
	}
	return patterns.BulkItem{Title: "(no longer exists)"}
}

// shortID is how test cases are referred to: the first 8 characters of their
// ID.
func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func requirementPrefixOf(prefixes map[string]string, categoryID string) string {
	if p, ok := prefixes[categoryID]; ok {
		return p
	}
	return "?"
}

// bulkReason is why an item cannot be changed, for a person to read.
func bulkReason(err error) string {
	if errors.Is(err, service.ErrNotFound) {
		return "it no longer exists or is not in this list."
	}
	msg := strings.TrimPrefix(errorText(err), service.ErrForbidden.Error()+": ")
	if msg == "" {
		return ""
	}
	msg = strings.ToUpper(msg[:1]) + msg[1:]
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	return msg
}

// bulkChoices are the values an operation offers, and its default.
func (c *PageProductController) bulkChoices(r *http.Request, user *model.User, productID string, t bulkTable, action service.BulkAction) (label string, choices []templates.SelectOption) {
	ctx := r.Context()
	switch action {
	case service.BulkStatus:
		et := model.RequirementKindRequirement // tasks share the requirement statuses
		if t.defect() {
			et = "defect"
		}
		statuses, _ := c.milestone.ListStatuses(ctx, user, productID, et)
		for _, s := range statuses {
			choices = append(choices, templates.SelectOption{Value: s.ID, Label: s.Name})
		}
		return "New status", choices
	case service.BulkCompliance:
		for _, s := range []string{model.ComplianceMet, model.ComplianceViolated, model.ComplianceNotAssessed} {
			choices = append(choices, templates.SelectOption{Value: s, Label: complianceLabels[s]})
		}
		return "New compliance", choices
	case service.BulkCategory:
		cats, _ := c.category.ListCategories(ctx, user, productID, t.kind)
		for _, cat := range cats {
			choices = append(choices, templates.SelectOption{Value: cat.ID, Label: cat.Prefix + " — " + cat.Name})
		}
		return "New category", choices
	case service.BulkCriticality:
		for _, v := range model.DefectCriticalities {
			choices = append(choices, templates.SelectOption{Value: v, Label: strings.ToUpper(v[:1]) + v[1:]})
		}
		return "New criticality", choices
	case service.BulkPriority:
		// The badges tell P1 to P4 apart (P5 and beyond look alike), so
		// those and P5 are offered; the edit form takes any number.
		for p := 1; p <= 5; p++ {
			label := fmt.Sprintf("P%d", p)
			if p == 1 {
				label += " (highest)"
			}
			choices = append(choices, templates.SelectOption{Value: fmt.Sprint(p), Label: label})
		}
		choices = append(choices, templates.SelectOption{Value: "0", Label: "No priority"})
		return "New priority", choices
	case service.BulkMilestone:
		choices = append(choices, templates.SelectOption{Value: "", Label: "No milestone"})
		ms, _ := c.milestone.ListMilestones(ctx, user, productID)
		for _, m := range ms {
			choices = append(choices, templates.SelectOption{Value: m.ID, Label: m.Name})
		}
		return "Milestone", choices
	case service.BulkSprint:
		choices = append(choices, templates.SelectOption{Value: "", Label: "No sprint"})
		sprints, _ := c.milestone.ListSprints(ctx, user, productID)
		for _, s := range sprints {
			if s.Status != model.SprintStatusCompleted {
				choices = append(choices, templates.SelectOption{Value: s.ID, Label: s.Name})
			}
		}
		return "Sprint", choices
	case service.BulkLinkRequirement, service.BulkUnlinkRequirement:
		prefixes := c.buildCategoryPrefixMap(r, user, productID, model.RequirementKindRequirement)
		items, _ := c.backlog.ListRequirements(ctx, user, productID, store.RequirementFilter{Kind: model.RequirementKindRequirement, Archived: store.ArchivedExclude})
		for _, it := range items {
			choices = append(choices, templates.SelectOption{Value: it.ID, Label: model.EntityIdentifier(requirementPrefixOf(prefixes, it.CategoryID), it.SequenceNumber) + " " + it.Title})
		}
		return "Requirement", choices
	case service.BulkLinkConstraint, service.BulkUnlinkConstraint:
		prefixes := c.buildCategoryPrefixMap(r, user, productID, "constraint")
		cons, _ := c.constraint.ListConstraints(ctx, user, productID, store.ConstraintFilter{})
		for _, con := range cons {
			choices = append(choices, templates.SelectOption{Value: con.ID, Label: model.EntityIdentifier(requirementPrefixOf(prefixes, con.CategoryID), con.SequenceNumber) + " " + con.Title})
		}
		return "Constraint", choices
	case service.BulkComponent:
		choices = append(choices, templates.SelectOption{Value: "", Label: "No component"})
		comps, _ := c.component.ListComponents(ctx, user, productID)
		for _, comp := range comps {
			choices = append(choices, templates.SelectOption{Value: comp.ID, Label: comp.Name})
		}
		return "Component", choices
	case service.BulkCapability:
		choices = append(choices, templates.SelectOption{Value: "", Label: "No capability"})
		caps, _ := c.component.ListCapabilities(ctx, user, productID)
		for _, capab := range caps {
			choices = append(choices, templates.SelectOption{Value: capab.ID, Label: capab.Name})
		}
		return "Capability", choices
	case service.BulkPyramidLevel:
		choices = append(choices, templates.SelectOption{Value: "", Label: "No pyramid level"})
		levels, _ := c.milestone.ListPyramidLevels(ctx, user, productID)
		for _, l := range levels {
			choices = append(choices, templates.SelectOption{Value: l.ID, Label: l.Name})
		}
		return "Pyramid level", choices
	case service.BulkAssign, service.BulkUnassign:
		// Developers assign and unassign themselves only (UR-171).
		if _, role, err := c.loadProductWithRole(r, user, productID); err == nil && !user.IsAdmin && role != model.RoleOwner {
			return "Team member", []templates.SelectOption{{Value: user.ID, Label: user.DisplayName}}
		}
		for _, m := range c.buildTeamMembers(r, user, productID) {
			choices = append(choices, templates.SelectOption{Value: m.UserID, Label: m.DisplayName})
		}
		return "Team member", choices
	}
	return "", nil
}

// defaultChoice picks the value a confirmation opens with: the first real
// one, so "no milestone" is a choice to make rather than a default.
func defaultChoice(choices []templates.SelectOption) string {
	for _, ch := range choices {
		if ch.Value != "" {
			return ch.Value
		}
	}
	return ""
}

func choiceLabel(choices []templates.SelectOption, value string) string {
	for _, ch := range choices {
		if ch.Value == value {
			return ch.Label
		}
	}
	return value
}

// bulkWording is the title, question and button of an operation's
// confirmation, and whether it cannot be undone.
func bulkWording(t bulkTable, action service.BulkAction, n int, valueLabel string) (title, question, verb string, danger bool) {
	items := t.noun(n)
	quoted := "“" + valueLabel + "”"
	switch action {
	case service.BulkStatus:
		return "Change status", "Change the status of " + items + " to " + quoted + ":", "Change", false
	case service.BulkCompliance:
		return "Set compliance", "Set the compliance of " + items + " to " + quoted + ":", "Set", false
	case service.BulkCategory:
		return "Change category", "Change the category of " + items + " to " + quoted + ":", "Change", false
	case service.BulkCriticality:
		return "Change criticality", "Set the criticality of " + items + " to " + quoted + ":", "Change", false
	case service.BulkPriority:
		if valueLabel == "No priority" {
			return "Change priority", "Remove the priority of " + items + ":", "Remove", false
		}
		return "Change priority", "Set the priority of " + items + " to " + quoted + ":", "Change", false
	case service.BulkMilestone:
		if valueLabel == "No milestone" {
			return "Set milestone", "Remove " + items + " from their milestone:", "Remove", false
		}
		return "Set milestone", "Move " + items + " to milestone " + quoted + ":", "Move", false
	case service.BulkSprint:
		if valueLabel == "No sprint" {
			return "Set sprint", "Remove " + items + " from their sprint:", "Remove", false
		}
		return "Set sprint", "Plan " + items + " into sprint " + quoted + ":", "Plan", false
	case service.BulkAssign:
		return "Assign", "Assign " + valueLabel + " to " + items + ":", "Assign", false
	case service.BulkUnassign:
		return "Unassign", "Remove " + valueLabel + " from " + items + ":", "Unassign", false
	case service.BulkArchive:
		q := "Archive " + items + ":"
		switch {
		case t.defect():
		case n == 1:
			q = "Archive " + items + ", with the items below it:"
		default:
			q = "Archive " + items + ", with the items below them:"
		}
		return "Archive", q, "Archive", false
	case service.BulkUnarchive:
		return "Unarchive", "Unarchive " + items + ":", "Unarchive", false
	case service.BulkLinkRequirement:
		return "Link requirement", "Link " + items + " to " + quoted + ":", "Link", false
	case service.BulkUnlinkRequirement:
		return "Unlink requirement", "Unlink " + items + " from " + quoted + ":", "Unlink", false
	case service.BulkLinkConstraint:
		return "Link constraint", "Link " + items + " to " + quoted + ":", "Link", false
	case service.BulkUnlinkConstraint:
		return "Unlink constraint", "Unlink " + items + " from " + quoted + ":", "Unlink", false
	case service.BulkComponent:
		return fieldWording("component", items, valueLabel)
	case service.BulkCapability:
		return fieldWording("capability", items, valueLabel)
	case service.BulkPyramidLevel:
		return fieldWording("pyramid level", items, valueLabel)
	case service.BulkDelete:
		return "Delete", "Delete " + items + ". This cannot be undone:", "Delete", true
	}
	return "", "", "", false
}

// BulkPreview renders POST /products/{id}/{collection}/bulk/preview: the
// confirmation of a bulk operation, naming the items it will change and those
// it cannot, with the reason (SC-92). Nothing is changed.
func (c *PageProductController) BulkPreview(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	t, ok := bulkTableFromPath(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f := readBulkForm(r)
	base := "/products/" + productID + "/" + t.collection + "/bulk/"
	props := patterns.BulkConfirmProps{
		PreviewURL: base + "preview",
		ApplyURL:   base + "apply",
		Target:     "#" + t.region(),
		Action:     string(f.action),
		State:      f.state,
	}
	render := func() {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = patterns.BulkConfirm(props).Render(r.Context(), w)
	}

	ids := f.ids
	if len(f.selection) > 0 {
		ids = f.selection
	}
	if f.matching {
		var err error
		if ids, err = c.matchingIDs(r, user, productID, t, f.state); err != nil {
			props.Title, props.Problem = "Bulk change", errorText(err)
			render()
			return
		}
	}
	if len(ids) == 0 {
		props.Title, props.Problem = "Bulk change", "Nothing is selected."
		render()
		return
	}

	props.ChoiceLabel, props.Choices = c.bulkChoices(r, user, productID, t, f.action)
	props.Value = f.value
	if !f.hasValue {
		props.Value = defaultChoice(props.Choices)
	}
	req := service.BulkRequest{ProductID: productID, Kind: t.kind, Action: f.action, Value: props.Value, IDs: ids}
	outcomes, err := c.bulk.Preview(r.Context(), user, req)
	props.Title, _, props.Verb, props.Danger = bulkWording(t, f.action, len(ids), "")
	if err != nil {
		if props.Title == "" {
			props.Title = "Bulk change"
		}
		props.Problem = errorText(err)
		if service.IsBulkInvalid(err) && len(props.Choices) == 0 && needsChoice(f.action) {
			props.Problem = "There is nothing to choose from for this change."
		}
		render()
		return
	}
	props.Selection = strings.Join(ids, ",")
	var allowedIDs []string

	namer := &bulkNamer{c: c, r: r, user: user, product: productID, table: t}
	reasons := map[string]int{}
	for _, o := range outcomes {
		item := namer.item(o.ID)
		if o.Err == nil {
			props.Allowed = append(props.Allowed, item)
			allowedIDs = append(allowedIDs, o.ID)
			if f.action == service.BulkDelete && !t.defect() && !props.Children {
				kids, _ := c.backlog.ListChildRequirements(r.Context(), user, productID, o.ID)
				props.Children = len(kids) > 0
			}
			continue
		}
		reason := bulkReason(o.Err)
		i, seen := reasons[reason]
		if !seen {
			i = len(props.Refused)
			reasons[reason] = i
			props.Refused = append(props.Refused, patterns.BulkRefusal{Reason: reason})
		}
		props.Refused[i].Items = append(props.Refused[i].Items, item)
	}
	props.IDs = strings.Join(allowedIDs, ",")
	props.Title, props.Question, props.Verb, props.Danger = bulkWording(t, f.action, len(props.Allowed), choiceLabel(props.Choices, props.Value))
	render()
}

// fieldWording words setting an optional field of test cases, or clearing it
// with the "No ..." choice.
func fieldWording(field, items, valueLabel string) (title, question, verb string, danger bool) {
	title = "Set " + field
	if valueLabel == "No "+field {
		return title, "Remove the " + field + " of " + items + ":", "Remove", false
	}
	return title, "Set the " + field + " of " + items + " to “" + valueLabel + "”:", "Set", false
}

func needsChoice(a service.BulkAction) bool {
	switch a {
	case service.BulkStatus, service.BulkCompliance, service.BulkCategory, service.BulkPriority, service.BulkAssign, service.BulkUnassign,
		service.BulkLinkRequirement, service.BulkUnlinkRequirement, service.BulkLinkConstraint, service.BulkUnlinkConstraint:
		return true
	}
	return false
}

// bulkPast is what a bulk operation did, for its summary.
func bulkPast(a service.BulkAction) string {
	switch a {
	case service.BulkArchive:
		return "Archived"
	case service.BulkUnarchive:
		return "Unarchived"
	case service.BulkDelete:
		return "Deleted"
	case service.BulkAssign:
		return "Assigned"
	case service.BulkUnassign:
		return "Unassigned"
	case service.BulkLinkRequirement, service.BulkLinkConstraint:
		return "Linked"
	case service.BulkUnlinkRequirement, service.BulkUnlinkConstraint:
		return "Unlinked"
	default:
		return "Changed"
	}
}

// BulkApply handles POST /products/{id}/{collection}/bulk/apply: it applies
// the confirmed operation to the items the confirmation listed, each through
// the individual operation (SC-92), and answers with the table re-rendered as
// it was, a summary toast and the confirmation closed.
func (c *PageProductController) BulkApply(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	t, ok := bulkTableFromPath(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f := readBulkForm(r)
	req := service.BulkRequest{ProductID: productID, Kind: t.kind, Action: f.action, Value: f.value, Children: f.children, IDs: f.ids}
	outcomes, err := c.bulk.Apply(r.Context(), user, req)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, service.ErrForbidden) {
			status = http.StatusForbidden
		}
		http.Error(w, errorText(err), status)
		return
	}

	done, firstReason := 0, ""
	for _, o := range outcomes {
		if o.Err == nil {
			done++
		} else if firstReason == "" {
			firstReason = bulkReason(o.Err)
		}
	}
	failed := len(outcomes) - done
	message, variant := bulkPast(f.action)+" "+t.noun(done)+".", "success"
	if failed > 0 {
		message += fmt.Sprintf(" %s could not be changed: %s", t.noun(failed), firstReason)
		variant = "warning"
		if done == 0 {
			variant = "error"
		}
	}

	table, err := c.bulkTableContent(r, user, productID, t, f.state)
	if err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// bulk-select.js leaves selection mode on it.
	w.Header().Set("HX-Trigger", "bulkApplied")
	_ = table.Render(r.Context(), w)
	_ = patterns.BulkApplied(message, variant).Render(r.Context(), w)
}

// bulkTableContent re-renders a table as its state describes it, after a bulk
// operation changed its items.
func (c *PageProductController) bulkTableContent(r *http.Request, user *model.User, productID string, t bulkTable, state string) (templ.Component, error) {
	sr := stateRequest(r, state)
	if t.constraint() {
		props, err := c.loadConstraintsProps(sr, user, productID)
		if err != nil {
			return nil, err
		}
		props.Fragment = true
		return productpage.ConstraintsTableContent(props), nil
	}
	if t.testCase() {
		props, err := c.loadTestCasesProps(sr, user, productID)
		if err != nil {
			return nil, err
		}
		props.Fragment = true
		return productpage.TestCasesTableContent(props), nil
	}
	if t.defect() {
		props, err := c.loadDefectsProps(sr, user, productID)
		if err != nil {
			return nil, err
		}
		props.Fragment = true
		return productpage.DefectsTableContent(props), nil
	}
	props, err := c.loadBacklogItemsProps(sr, user, productID, t.kind)
	if err != nil {
		return nil, err
	}
	props.Fragment = true
	return productpage.BacklogItemsTableContent(props), nil
}
