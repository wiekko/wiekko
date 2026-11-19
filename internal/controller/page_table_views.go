package controller

import (
	"cmp"
	"net/http"

	"github.com/a-h/templ"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	admin "nexmap.net/web/templates/pages/admin"
	productpage "nexmap.net/web/templates/pages/product"
	"nexmap.net/web/templates/patterns"
	"nexmap.net/web/templates/ui"
)

// The table views of UR-145 whose rows are sorted, searched, filtered and
// paged in memory (see rowTable): constraints, components, capabilities,
// milestones, sprints, campaigns, the team and the admin users and
// invitations lists.

// tableView builds the table state of a page from the rows that matched
// (total) and all rows (all).
func tableView(state ui.ListState, total, all int) patterns.TableView {
	return patterns.TableView{State: state, Total: total, ProductTotal: all, TotalPages: totalPages(total)}
}

// renderTableView writes the table fragment for a swap by the table's own
// controls and the full page otherwise; both come from the one URL so the
// address bar always describes the view (DOC-1, "URL = state").
func renderTableView(w http.ResponseWriter, r *http.Request, tableID string, view *patterns.TableView, fragment, page func() templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if tableSwap(r, tableID) {
		view.Fragment = true
		_ = fragment().Render(r.Context(), w)
		return
	}
	_ = page().Render(r.Context(), w)
}

// complianceLabels are the compliance states as the badge shows them.
var complianceLabels = map[string]string{
	model.ComplianceMet:         "Met",
	model.ComplianceViolated:    "Violated",
	model.ComplianceNotAssessed: "Not assessed",
}

// constraintTable sorts violated constraints first, the way defects sort the
// most critical first; categories sort by their configured position.
func constraintTable(categories map[string]*model.Category) rowTable[*model.Constraint] {
	categoryPos := func(id string) int {
		if c, ok := categories[id]; ok {
			return c.Position
		}
		return 1 << 30
	}
	compliance := byRank(model.ComplianceViolated, model.ComplianceNotAssessed, model.ComplianceMet)
	return rowTable[*model.Constraint]{
		sorts: map[string]func(a, b *model.Constraint) int{
			"identifier": func(a, b *model.Constraint) int { return cmp.Compare(a.SequenceNumber, b.SequenceNumber) },
			"title":      func(a, b *model.Constraint) int { return byText(a.Title, b.Title) },
			"category": func(a, b *model.Constraint) int {
				return cmp.Compare(categoryPos(a.CategoryID), categoryPos(b.CategoryID))
			},
			"compliance": func(a, b *model.Constraint) int { return compliance(a.ComplianceState, b.ComplianceState) },
			"priority":   func(a, b *model.Constraint) int { return byPriority(a.Priority, b.Priority) },
			"created":    func(a, b *model.Constraint) int { return cmp.Compare(a.CreatedAt, b.CreatedAt) },
		},
		search: func(c *model.Constraint) []string {
			texts := []string{c.Title, complianceLabels[c.ComplianceState]}
			if cat, ok := categories[c.CategoryID]; ok {
				texts = append(texts, model.EntityIdentifier(cat.Prefix, c.SequenceNumber), cat.Name)
			}
			return texts
		},
		facets: map[string]func(c *model.Constraint, v string) bool{
			"category_id": func(c *model.Constraint, v string) bool { return c.CategoryID == v },
			"compliance":  func(c *model.Constraint, v string) bool { return c.ComplianceState == v },
		},
	}
}

// Constraints renders GET /products/{id}/constraints.
func (c *PageProductController) Constraints(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	all, _ := c.constraint.ListConstraints(r.Context(), user, productID, store.ConstraintFilter{})
	categories, _ := c.category.ListCategories(r.Context(), user, productID, "constraint")
	byID := make(map[string]*model.Category, len(categories))
	prefixes := make(map[string]string, len(categories))
	for _, cat := range categories {
		byID[cat.ID] = cat
		prefixes[cat.ID] = cat.Prefix
	}

	table := constraintTable(byID)
	state := table.readState(r, "/products/"+productID+"/constraints")
	rows, total := table.apply(all, &state)

	props := productpage.ConstraintsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Constraints",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Constraints:      rows,
		CategoryPrefixes: prefixes,
		CanEdit:          c.canEditConstraints(user, role),
		TableView:        tableView(state, total, len(all)),
		CategoryOptions:  categories,
	}
	renderTableView(w, r, productpage.ConstraintsTableID, &props.TableView,
		func() templ.Component { return productpage.ConstraintsTableContent(props) },
		func() templ.Component { return productpage.ConstraintsPage(props) })
}

// namedRef compares optional references by the names they resolve to;
// unset references come last.
func namedRef(names map[string]string) func(a, b string) int {
	return func(a, b string) int {
		na, nb := names[a], names[b]
		switch {
		case na == "" && nb == "":
			return 0
		case na == "":
			return 1
		case nb == "":
			return -1
		}
		return byText(na, nb)
	}
}

// Components renders GET /products/{id}/components.
func (c *PageProductController) Components(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	all, err := c.component.ListComponents(r.Context(), user, productID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
		return
	}
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	milestoneNames := milestoneNameMap(milestones)
	byMilestone := namedRef(milestoneNames)

	table := rowTable[*model.Component]{
		sorts: map[string]func(a, b *model.Component) int{
			"name":      func(a, b *model.Component) int { return byText(a.Name, b.Name) },
			"milestone": func(a, b *model.Component) int { return byMilestone(a.MilestoneID, b.MilestoneID) },
			"created":   func(a, b *model.Component) int { return cmp.Compare(a.CreatedAt, b.CreatedAt) },
		},
		search: func(comp *model.Component) []string { return []string{comp.Name, milestoneNames[comp.MilestoneID]} },
		facets: map[string]func(comp *model.Component, v string) bool{
			"milestone_id": func(comp *model.Component, v string) bool { return optionalRef(comp.MilestoneID, v) },
		},
	}
	state := table.readState(r, "/products/"+productID+"/components")
	rows, total := table.apply(all, &state)

	props := productpage.ComponentsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Components",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Components:       rows,
		Milestones:       milestoneNames,
		IsOwnerOrAdmin:   user.IsAdmin || role == model.RoleOwner,
		TableView:        tableView(state, total, len(all)),
		MilestoneOptions: milestones,
	}
	renderTableView(w, r, productpage.ComponentsTableID, &props.TableView,
		func() templ.Component { return productpage.ComponentsTableContent(props) },
		func() templ.Component { return productpage.ComponentsPage(props) })
}

// Capabilities renders GET /products/{id}/capabilities.
func (c *PageProductController) Capabilities(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	all, err := c.component.ListCapabilities(r.Context(), user, productID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
		return
	}
	components, _ := c.component.ListComponents(r.Context(), user, productID)
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	componentNames := make(map[string]string, len(components))
	for _, comp := range components {
		componentNames[comp.ID] = comp.Name
	}
	milestoneNames := milestoneNameMap(milestones)
	byComponent, byMilestone := namedRef(componentNames), namedRef(milestoneNames)

	table := rowTable[*model.Capability]{
		sorts: map[string]func(a, b *model.Capability) int{
			"name":      func(a, b *model.Capability) int { return byText(a.Name, b.Name) },
			"component": func(a, b *model.Capability) int { return byComponent(a.ComponentID, b.ComponentID) },
			"milestone": func(a, b *model.Capability) int { return byMilestone(a.MilestoneID, b.MilestoneID) },
			"created":   func(a, b *model.Capability) int { return cmp.Compare(a.CreatedAt, b.CreatedAt) },
		},
		search: func(f *model.Capability) []string {
			return []string{f.Name, componentNames[f.ComponentID], milestoneNames[f.MilestoneID]}
		},
		facets: map[string]func(f *model.Capability, v string) bool{
			"component_id": func(f *model.Capability, v string) bool { return optionalRef(f.ComponentID, v) },
			"milestone_id": func(f *model.Capability, v string) bool { return optionalRef(f.MilestoneID, v) },
		},
	}
	state := table.readState(r, "/products/"+productID+"/capabilities")
	rows, total := table.apply(all, &state)

	props := productpage.CapabilitiesProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Capabilities",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Capabilities:     rows,
		Components:       componentNames,
		Milestones:       milestoneNames,
		IsOwnerOrAdmin:   user.IsAdmin || role == model.RoleOwner,
		TableView:        tableView(state, total, len(all)),
		ComponentOptions: components,
		MilestoneOptions: milestones,
	}
	renderTableView(w, r, productpage.CapabilitiesTableID, &props.TableView,
		func() templ.Component { return productpage.CapabilitiesTableContent(props) },
		func() templ.Component { return productpage.CapabilitiesPage(props) })
}

func milestoneNameMap(milestones []*model.Milestone) map[string]string {
	names := make(map[string]string, len(milestones))
	for _, m := range milestones {
		names[m.ID] = m.Name
	}
	return names
}

// milestoneTable sorts milestones without a due date after every dated one.
var milestoneTable = rowTable[*model.Milestone]{
	sorts: map[string]func(a, b *model.Milestone) int{
		"name": func(a, b *model.Milestone) int { return byText(a.Name, b.Name) },
		"type": func(a, b *model.Milestone) int {
			return byRank(model.MilestoneTypePlanning, model.MilestoneTypeRelease, model.MilestoneTypeInternal)(a.Type, b.Type)
		},
		"due":     func(a, b *model.Milestone) int { return byOptional(a.DueDate, b.DueDate) },
		"created": func(a, b *model.Milestone) int { return cmp.Compare(a.CreatedAt, b.CreatedAt) },
	},
	search: func(m *model.Milestone) []string { return []string{m.Name} },
	facets: map[string]func(m *model.Milestone, v string) bool{
		"type": func(m *model.Milestone, v string) bool { return m.Type == v },
	},
}

// Milestones renders GET /products/{id}/milestones: the table, or the
// roadmap (?view=timeline), which always shows every milestone.
func (c *PageProductController) Milestones(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	all, err := c.milestone.ListMilestones(r.Context(), user, productID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
		return
	}
	view := r.URL.Query().Get("view")
	if view != "timeline" {
		view = "table"
	}

	state := milestoneTable.readState(r, "/products/"+productID+"/milestones")
	rows, total := milestoneTable.apply(all, &state)

	props := productpage.MilestonesProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Milestones",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Milestones:     all,
		Rows:           rows,
		IsOwnerOrAdmin: user.IsAdmin || role == model.RoleOwner,
		View:           view,
		TableView:      tableView(state, total, len(all)),
	}
	renderTableView(w, r, productpage.MilestonesTableID, &props.TableView,
		func() templ.Component { return productpage.MilestonesTableContent(props) },
		func() templ.Component { return productpage.MilestonesPage(props) })
}

// sprintTable sorts sprints through their lifecycle and by start date.
var sprintTable = rowTable[productpage.SprintRow]{
	sorts: map[string]func(a, b productpage.SprintRow) int{
		"name": func(a, b productpage.SprintRow) int { return byText(a.Sprint.Name, b.Sprint.Name) },
		"status": func(a, b productpage.SprintRow) int {
			return byRank(model.SprintStatusPlanned, model.SprintStatusActive, model.SprintStatusCompleted)(a.Sprint.Status, b.Sprint.Status)
		},
		"dates": func(a, b productpage.SprintRow) int {
			if c := byOptional(a.Sprint.StartDate, b.Sprint.StartDate); c != 0 {
				return c
			}
			return byOptional(a.Sprint.EndDate, b.Sprint.EndDate)
		},
		"milestone": func(a, b productpage.SprintRow) int { return byOptional(a.MilestoneName, b.MilestoneName) },
		"items":     func(a, b productpage.SprintRow) int { return cmp.Compare(a.ItemCount, b.ItemCount) },
	},
	search: func(s productpage.SprintRow) []string { return []string{s.Sprint.Name, s.MilestoneName} },
	facets: map[string]func(s productpage.SprintRow, v string) bool{
		"status":       func(s productpage.SprintRow, v string) bool { return s.Sprint.Status == v },
		"milestone_id": func(s productpage.SprintRow, v string) bool { return optionalRef(s.Sprint.MilestoneID, v) },
	},
}

// Sprints renders GET /products/{id}/sprints.
func (c *PageProductController) Sprints(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, role, ok := c.loadScrumProduct(w, r, user, productID)
	if !ok {
		return
	}
	sprints, err := c.milestone.ListSprints(r.Context(), user, productID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
		return
	}
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, productID)
	milestoneNames := milestoneNameMap(milestones)
	all := make([]productpage.SprintRow, 0, len(sprints))
	for _, sp := range sprints {
		all = append(all, productpage.SprintRow{
			Sprint:        sp,
			MilestoneName: milestoneNames[sp.MilestoneID],
			ItemCount:     c.countSprintItems(r, user, productID, sp.ID),
		})
	}

	state := sprintTable.readState(r, "/products/"+productID+"/sprints")
	rows, total := sprintTable.apply(all, &state)

	props := productpage.SprintsProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Sprints",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:      user.DisplayName,
		IsAdmin:          user.IsAdmin,
		Product:          p,
		Sprints:          rows,
		IsOwnerOrAdmin:   isOwnerOrAdmin(user, role),
		TableView:        tableView(state, total, len(all)),
		MilestoneOptions: milestones,
	}
	renderTableView(w, r, productpage.SprintsTableID, &props.TableView,
		func() templ.Component { return productpage.SprintsTableContent(props) },
		func() templ.Component { return productpage.SprintsPage(props) })
}

// campaignTable sorts campaigns through their lifecycle, failed outcomes
// first, and by planned start.
var campaignTable = rowTable[productpage.CampaignView]{
	sorts: map[string]func(a, b productpage.CampaignView) int{
		"name": func(a, b productpage.CampaignView) int { return byText(a.Campaign.Name, b.Campaign.Name) },
		"status": func(a, b productpage.CampaignView) int {
			return byRank(model.CampaignStatusDraft, model.CampaignStatusActive, model.CampaignStatusClosed)(a.Campaign.Status, b.Campaign.Status)
		},
		"outcome": func(a, b productpage.CampaignView) int {
			return byRank(model.CampaignVerdictFailed, model.CampaignVerdictIncomplete, model.CampaignVerdictPassed)(a.Verdict, b.Verdict)
		},
		"progress": func(a, b productpage.CampaignView) int { return cmp.Compare(a.Percent, b.Percent) },
		"planned": func(a, b productpage.CampaignView) int {
			if c := byOptional(a.Campaign.StartsAt, b.Campaign.StartsAt); c != 0 {
				return c
			}
			return byOptional(a.Campaign.EndsAt, b.Campaign.EndsAt)
		},
	},
	search: func(v productpage.CampaignView) []string { return []string{v.Campaign.Name} },
	facets: map[string]func(v productpage.CampaignView, value string) bool{
		"status":  func(v productpage.CampaignView, value string) bool { return v.Campaign.Status == value },
		"outcome": func(v productpage.CampaignView, value string) bool { return v.Verdict == value },
	},
}

// Campaigns renders GET /products/{id}/campaigns.
func (c *PageProductController) Campaigns(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	campaigns, err := c.testing.ListCampaigns(r.Context(), user, productID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID, http.StatusSeeOther)
		return
	}
	// One grouped query for the whole product rather than one per campaign.
	counts, _ := c.testing.ListCampaignProgress(r.Context(), user, productID)
	all := make([]productpage.CampaignView, 0, len(campaigns))
	for _, campaign := range campaigns {
		all = append(all, campaignView(campaign, counts[campaign.ID]))
	}

	state := campaignTable.readState(r, "/products/"+productID+"/campaigns")
	rows, total := campaignTable.apply(all, &state)

	props := productpage.CampaignsProps{
		Meta: templates.PageMeta{
			Title:     "Campaigns",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Campaigns:   rows,
		CanEdit:     c.canEditCampaigns(user, role),
		TableView:   tableView(state, total, len(all)),
	}
	renderTableView(w, r, productpage.CampaignsTableID, &props.TableView,
		func() templ.Component { return productpage.CampaignsTableContent(props) },
		func() templ.Component { return productpage.CampaignsPage(props) })
}

// teamTable sorts members by name and by role, owner first.
var teamTable = rowTable[productpage.TeamMemberView]{
	sorts: map[string]func(a, b productpage.TeamMemberView) int{
		"user": func(a, b productpage.TeamMemberView) int { return byText(a.DisplayName, b.DisplayName) },
		"role": func(a, b productpage.TeamMemberView) int {
			return byRank(model.RoleOwner, model.RoleDeveloper, model.RoleTester, model.RoleViewer)(a.Role, b.Role)
		},
	},
	search: func(m productpage.TeamMemberView) []string { return []string{m.DisplayName, m.Username} },
	facets: map[string]func(m productpage.TeamMemberView, v string) bool{
		"role": func(m productpage.TeamMemberView, v string) bool { return m.Role == v },
	},
}

// loadTeamProps assembles the team page for the request's table state.
func (c *PageProductController) loadTeamProps(r *http.Request, user *model.User, productID string) (productpage.TeamProps, error) {
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.TeamProps{}, err
	}
	all := c.buildTeamMembers(r, user, productID)
	state := teamTable.readState(r, "/products/"+productID+"/team")
	rows, total := teamTable.apply(all, &state)
	return productpage.TeamProps{
		Meta: templates.PageMeta{
			Title:     p.Name + " - Team",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName:    user.DisplayName,
		IsAdmin:        user.IsAdmin,
		Product:        p,
		Members:        rows,
		IsOwnerOrAdmin: user.IsAdmin || role == model.RoleOwner,
		MailEnabled:    c.invitation.MailEnabled(),
		TableView:      tableView(state, total, len(all)),
	}, nil
}

// Team renders GET /products/{id}/team.
func (c *PageProductController) Team(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	props, err := c.loadTeamProps(r, user, r.PathValue("id"))
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	renderTableView(w, r, productpage.TeamTableID, &props.TableView,
		func() templ.Component { return productpage.TeamTableContent(props) },
		func() templ.Component { return productpage.TeamPage(props) })
}

// userTable sorts users by account status in the order an admin works
// through them: active, then those waiting for approval, locked, and to be
// deleted.
var userTable = rowTable[*model.User]{
	sorts: map[string]func(a, b *model.User) int{
		"username":     func(a, b *model.User) int { return byText(a.Username, b.Username) },
		"email":        func(a, b *model.User) int { return byText(a.Email, b.Email) },
		"display_name": func(a, b *model.User) int { return byText(a.DisplayName, b.DisplayName) },
		"status": func(a, b *model.User) int {
			return byRank(model.UserStatusActive, model.UserStatusPendingApproval, model.UserStatusLocked, model.UserStatusDeletionRequested)(a.Status, b.Status)
		},
		"created": func(a, b *model.User) int { return cmp.Compare(a.CreatedAt, b.CreatedAt) },
	},
	search: func(u *model.User) []string { return []string{u.Username, u.Email, u.DisplayName} },
	facets: map[string]func(u *model.User, v string) bool{
		"status": func(u *model.User, v string) bool { return u.Status == v },
		"role":   func(u *model.User, v string) bool { return u.IsAdmin == (v == "admin") },
	},
}

// ListUsers renders GET /admin/users. The status facet reads the status
// parameter the former status tabs linked to.
func (c *PageAdminController) ListUsers(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	all, err := c.user.List(r.Context(), store.UserFilter{})
	if err != nil {
		all = nil
	}
	state := userTable.readState(r, "/admin/users")
	rows, total := userTable.apply(all, &state)

	props := admin.UsersProps{
		Meta: templates.PageMeta{
			Title:     "Users",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Users:       rows,
		TableView:   tableView(state, total, len(all)),
	}
	renderTableView(w, r, admin.UsersTableID, &props.TableView,
		func() templ.Component { return admin.UsersTableContent(props) },
		func() templ.Component { return admin.UsersPage(props) })
}

// invitationStatus is the state an invitation's badge shows.
func invitationStatus(inv *model.Invitation) string {
	if inv.AcceptedAt != "" {
		return "accepted"
	}
	return "pending"
}

var invitationTable = rowTable[*model.Invitation]{
	sorts: map[string]func(a, b *model.Invitation) int{
		"email": func(a, b *model.Invitation) int { return byText(a.Email, b.Email) },
		"status": func(a, b *model.Invitation) int {
			return byRank("pending", "accepted")(invitationStatus(a), invitationStatus(b))
		},
		"expires": func(a, b *model.Invitation) int { return cmp.Compare(a.ExpiresAt, b.ExpiresAt) },
		"created": func(a, b *model.Invitation) int { return cmp.Compare(a.CreatedAt, b.CreatedAt) },
	},
	search: func(inv *model.Invitation) []string { return []string{inv.Email} },
	facets: map[string]func(inv *model.Invitation, v string) bool{
		"status": func(inv *model.Invitation, v string) bool { return invitationStatus(inv) == v },
	},
}

// Invitations renders GET /admin/invitations.
func (c *PageAdminController) Invitations(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	all, err := c.invitation.List(r.Context())
	if err != nil {
		all = nil
	}
	state := invitationTable.readState(r, "/admin/invitations")
	rows, total := invitationTable.apply(all, &state)

	props := admin.InvitationsProps{
		Meta: templates.PageMeta{
			Title:     "Invitations",
			CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Invitations: rows,
		MailEnabled: c.invitation.MailEnabled(),
		TableView:   tableView(state, total, len(all)),
	}
	renderTableView(w, r, admin.InvitationsTableID, &props.TableView,
		func() templ.Component { return admin.InvitationsTableContent(props) },
		func() templ.Component { return admin.InvitationsPage(props) })
}
