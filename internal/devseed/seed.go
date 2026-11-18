// Package devseed populates a brand-new database with a coherent demo data set
// so that dev-mode instances have something to look at without clicking every
// entity into existence first.
//
// It runs only when NEXMAP_DEV_MODE is set and the database was just created
// (no users existed before the admin bootstrap). Everything goes through the
// service layer, so the demo data is subject to the same validation as data
// entered through the UI, and this package stops compiling — or fails loudly at
// startup — when a service contract changes.
package devseed

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/auth"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
)

// DemoPassword is the shared password of every seeded demo account. Dev mode
// only — the seeder logs it on startup.
const DemoPassword = "devpassword"

// Seed creates the demo products, their contents and the demo user accounts.
// It is not idempotent: calling it twice produces duplicates, so the caller
// must gate it on a genuinely fresh database.
func Seed(ctx context.Context, svc *service.Services, st *store.Store, provider auth.Provider, admin *model.User) error {
	s := &seeder{ctx: ctx, svc: svc, st: st, provider: provider, admin: admin, users: map[string]*model.User{}}
	// Alias so fixtures can name the admin as an author without knowing the
	// username it was bootstrapped under.
	s.users["admin"] = admin

	s.demoUsers()
	s.seedAviary()
	s.seedFinchTracker()

	return s.err
}

// seeder carries the collaborators every step needs along with a sticky error.
// Once a step fails every later step turns into a no-op, which keeps the data
// definitions below free of error handling without letting a failure cascade
// into a confusing pile of follow-up errors.
type seeder struct {
	ctx      context.Context
	svc      *service.Services
	st       *store.Store
	provider auth.Provider
	admin    *model.User
	users    map[string]*model.User // username -> user
	err      error
}

// fail records the first error and the step that produced it.
func (s *seeder) fail(step string, err error) {
	if s.err == nil && err != nil {
		s.err = fmt.Errorf("%s: %w", step, err)
	}
}

// stopped reports whether an earlier step already failed.
func (s *seeder) stopped() bool { return s.err != nil }

// --- Users ---

// demoUsers creates the three non-admin accounts used to exercise the product
// roles. They bypass UserService.Register, which is gated on the
// self_registration_enabled setting — the seeder has no business flipping a
// security setting to plant fixtures.
func (s *seeder) demoUsers() {
	s.demoUser("ada", "ada@example.test", "Ada Lindqvist")
	s.demoUser("bo", "bo@example.test", "Bo Ferreira")
	s.demoUser("cy", "cy@example.test", "Cy Nakamura")
}

func (s *seeder) demoUser(username, email, displayName string) {
	if s.stopped() {
		return
	}

	hash, err := s.provider.HashPassword(DemoPassword)
	if err != nil {
		s.fail("hash demo password for "+username, err)
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	u := &model.User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		DisplayName:  displayName,
		PasswordHash: hash,
		IsAdmin:      false,
		Status:       model.UserStatusActive,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := s.st.User.Create(s.ctx, u); err != nil {
		s.fail("create demo user "+username, err)
		return
	}
	s.users[username] = u
}

// user returns a seeded demo account by username, falling back to the admin so
// a lookup typo degrades to "authored by admin" rather than a nil dereference.
func (s *seeder) user(username string) *model.User {
	if u, ok := s.users[username]; ok {
		return u
	}
	return s.admin
}

// --- Dates ---

// day returns midnight UTC n days from now, in the RFC 3339 form the date
// inputs produce (see normalizeFormDate in the campaign controller). Seed dates
// are relative to the moment the database was created, so the demo product
// always has a current sprint no matter when it was seeded.
func day(n int) string {
	return time.Now().UTC().AddDate(0, 0, n).Format("2006-01-02") + "T00:00:00Z"
}

// --- Product scaffolding ---

// productSeed is a per-product handle. It resolves the human-readable names
// used in the data definitions (statuses, categories, milestones, …) to the IDs
// the services expect, so the fixtures read like the UI rather than like SQL.
type productSeed struct {
	s  *seeder
	id string

	statuses   map[string]map[string]string // entity type -> name -> id
	categories map[string]map[string]string // entity type -> name -> id
	levels     map[string]string            // pyramid level name -> id
	milestones map[string]string
	sprints    map[string]string
	envs       map[string]string
	components map[string]string
	caps       map[string]string
	reqs       map[string]string // handle -> requirement id
	defects    map[string]string // handle -> defect id
	testCases  map[string]string // handle -> test case id
	cons       map[string]string // title -> constraint id
	// kanban products activate by column, so spec.active is ignored there:
	// the item's status decides (FR-91).
	kanban bool
}

// newProduct creates a product owned by the admin and loads the defaults
// ProductService.Create seeded for it (statuses, categories, pyramid levels).
func (s *seeder) newProduct(name, description, estimation, workflow string) *productSeed {
	p := &productSeed{
		s:          s,
		kanban:     workflow == model.WorkflowModeKanban,
		statuses:   map[string]map[string]string{},
		categories: map[string]map[string]string{},
		levels:     map[string]string{},
		milestones: map[string]string{},
		sprints:    map[string]string{},
		envs:       map[string]string{},
		components: map[string]string{},
		caps:       map[string]string{},
		reqs:       map[string]string{},
		defects:    map[string]string{},
		testCases:  map[string]string{},
		cons:       map[string]string{},
	}
	if s.stopped() {
		return p
	}

	product, err := s.svc.Product.Create(s.ctx, s.admin, service.CreateProductInput{
		Name:           name,
		Description:    description,
		EstimationMode: estimation,
		WorkflowMode:   workflow,
	})
	if err != nil {
		s.fail("create product "+name, err)
		return p
	}
	p.id = product.ID

	for _, entityType := range []string{"requirement", "defect"} {
		statuses, err := s.svc.Milestone.ListStatuses(s.ctx, s.admin, p.id, entityType)
		if err != nil {
			s.fail("list "+entityType+" statuses of "+name, err)
			return p
		}
		byName := map[string]string{}
		for _, st := range statuses {
			byName[st.Name] = st.ID
		}
		p.statuses[entityType] = byName
	}

	for _, entityType := range []string{"requirement", "task", "constraint"} {
		categories, err := s.svc.Category.ListCategories(s.ctx, s.admin, p.id, entityType)
		if err != nil {
			s.fail("list "+entityType+" categories of "+name, err)
			return p
		}
		byName := map[string]string{}
		for _, c := range categories {
			byName[c.Name] = c.ID
		}
		p.categories[entityType] = byName
	}

	levels, err := s.svc.Milestone.ListPyramidLevels(s.ctx, s.admin, p.id)
	if err != nil {
		s.fail("list pyramid levels of "+name, err)
		return p
	}
	for _, l := range levels {
		p.levels[l.Name] = l.ID
	}

	return p
}

// member grants a demo user a role on the product.
func (p *productSeed) member(username, role string) {
	if p.s.stopped() {
		return
	}
	u := p.s.user(username)
	err := p.s.svc.Product.AddMember(p.s.ctx, p.s.admin, p.id, u.ID, role)
	p.s.fail("add "+username+" as "+role, err)
}

// --- Planning ---

func (p *productSeed) milestone(name, description, milestoneType, dueDate string) {
	if p.s.stopped() {
		return
	}
	m, err := p.s.svc.Milestone.CreateMilestone(p.s.ctx, p.s.admin, service.CreateMilestoneInput{
		ProductID:   p.id,
		Name:        name,
		Description: description,
		Type:        milestoneType,
		DueDate:     dueDate,
	})
	if err != nil {
		p.s.fail("create milestone "+name, err)
		return
	}
	p.milestones[name] = m.ID
}

// sprint creates a sprint and moves it to the given status. At most one sprint
// per product may be active, so callers must complete an earlier sprint before
// activating a later one.
func (p *productSeed) sprint(name, milestone, startDate, endDate, status string) {
	if p.s.stopped() {
		return
	}
	sp, err := p.s.svc.Milestone.CreateSprint(p.s.ctx, p.s.admin, service.CreateSprintInput{
		ProductID:   p.id,
		Name:        name,
		MilestoneID: p.milestones[milestone],
		StartDate:   startDate,
		EndDate:     endDate,
	})
	if err != nil {
		p.s.fail("create sprint "+name, err)
		return
	}
	p.sprints[name] = sp.ID

	if status == "" || status == model.SprintStatusPlanned {
		return
	}
	if _, err := p.s.svc.Milestone.SetSprintStatus(p.s.ctx, p.s.admin, p.id, sp.ID, status); err != nil {
		p.s.fail("set sprint "+name+" to "+status, err)
	}
}

func (p *productSeed) environment(name string) {
	if p.s.stopped() {
		return
	}
	env, err := p.s.svc.Milestone.CreateRuntimeEnvironment(p.s.ctx, p.s.admin, p.id, name)
	if err != nil {
		p.s.fail("create runtime environment "+name, err)
		return
	}
	p.envs[name] = env.ID
}

// --- Architecture ---

func (p *productSeed) component(name, description, milestone string) {
	if p.s.stopped() {
		return
	}
	c, err := p.s.svc.Component.CreateComponent(p.s.ctx, p.s.admin, service.CreateComponentInput{
		ProductID:   p.id,
		Name:        name,
		Description: description,
		MilestoneID: p.milestones[milestone],
	})
	if err != nil {
		p.s.fail("create component "+name, err)
		return
	}
	p.components[name] = c.ID
}

func (p *productSeed) capability(component, name, description, milestone string) {
	if p.s.stopped() {
		return
	}
	c, err := p.s.svc.Component.CreateCapability(p.s.ctx, p.s.admin, service.CreateCapabilityInput{
		ProductID:   p.id,
		ComponentID: p.components[component],
		Name:        name,
		Description: description,
		MilestoneID: p.milestones[milestone],
	})
	if err != nil {
		p.s.fail("create capability "+name, err)
		return
	}
	p.caps[name] = c.ID
}

// --- Backlog ---

// reqSpec describes one backlog item. Every reference is by display name and
// resolved against the product's lookup maps.
type reqSpec struct {
	handle      string // key for later references (links, comments); optional
	kind        string // "" means requirement
	parent      string // handle of the parent backlog item
	category    string
	title       string
	description string
	dod         string
	status      string
	milestone   string
	sprint      string
	estimation  string
	priority    int
	coarse      bool
	active      bool
	assignees   []string // demo usernames
	capabilties []string // capability names to link (requirements only)
}

func (p *productSeed) requirement(spec reqSpec) {
	if p.s.stopped() {
		return
	}

	kind := spec.kind
	if kind == "" {
		kind = model.RequirementKindRequirement
	}

	r, err := p.s.svc.Backlog.CreateRequirement(p.s.ctx, p.s.admin, service.CreateRequirementInput{
		ProductID:        p.id,
		Kind:             kind,
		ParentID:         p.reqs[spec.parent],
		CategoryID:       p.categories[kind][spec.category],
		Title:            spec.title,
		Description:      spec.description,
		DefinitionOfDone: spec.dod,
		StatusID:         p.statuses["requirement"][spec.status],
		MilestoneID:      p.milestones[spec.milestone],
		SprintID:         p.sprints[spec.sprint],
		Estimation:       spec.estimation,
		Priority:         spec.priority,
		IsCoarse:         spec.coarse,
	})
	if err != nil {
		p.s.fail("create "+kind+" "+spec.title, err)
		return
	}
	if spec.handle != "" {
		p.reqs[spec.handle] = r.ID
	}

	// IsActive is not part of the create input — activation is a deliberate
	// step in the UI too. Items created in a sprint are already active (FR-91).
	if spec.active && !p.kanban {
		active := true
		_, err := p.s.svc.Backlog.UpdateRequirement(p.s.ctx, p.s.admin, p.id, r.ID, service.UpdateRequirementInput{
			IsActive: &active,
		})
		p.s.fail("activate "+spec.title, err)
	}

	for _, username := range spec.assignees {
		u := p.s.user(username)
		err := p.s.svc.Backlog.AssignRequirement(p.s.ctx, p.s.admin, p.id, r.ID, u.ID)
		p.s.fail("assign "+spec.title+" to "+username, err)
	}

	for _, capability := range spec.capabilties {
		err := p.s.svc.Component.LinkRequirementToCapability(p.s.ctx, p.s.admin, p.id, p.caps[capability], r.ID)
		p.s.fail("link "+spec.title+" to capability "+capability, err)
	}
}

// defectSpec describes one defect and the entities it points at.
type defectSpec struct {
	handle      string
	title       string
	description string
	criticality string
	status      string
	milestone   string
	sprint      string
	environment string
	estimation  string
	priority    int
	active      bool
	assignees   []string
	linkReqs    []string // requirement handles
	linkCaps    []string // capability names
	linkComps   []string // component names
}

func (p *productSeed) defect(spec defectSpec) {
	if p.s.stopped() {
		return
	}

	d, err := p.s.svc.Backlog.CreateDefect(p.s.ctx, p.s.admin, service.CreateDefectInput{
		ProductID:     p.id,
		Title:         spec.title,
		Description:   spec.description,
		Criticality:   spec.criticality,
		StatusID:      p.statuses["defect"][spec.status],
		MilestoneID:   p.milestones[spec.milestone],
		SprintID:      p.sprints[spec.sprint],
		EnvironmentID: p.envs[spec.environment],
		Estimation:    spec.estimation,
		Priority:      spec.priority,
	})
	if err != nil {
		p.s.fail("create defect "+spec.title, err)
		return
	}
	if spec.handle != "" {
		p.defects[spec.handle] = d.ID
	}

	if spec.active && !p.kanban {
		active := true
		_, err := p.s.svc.Backlog.UpdateDefect(p.s.ctx, p.s.admin, p.id, d.ID, service.UpdateDefectInput{
			IsActive: &active,
		})
		p.s.fail("activate defect "+spec.title, err)
	}

	for _, username := range spec.assignees {
		u := p.s.user(username)
		err := p.s.svc.Backlog.AssignDefect(p.s.ctx, p.s.admin, p.id, d.ID, u.ID)
		p.s.fail("assign defect "+spec.title+" to "+username, err)
	}

	p.defectLinks(d.ID, spec.title, model.DefectLinkTargetRequirement, spec.linkReqs, p.reqs)
	p.defectLinks(d.ID, spec.title, model.DefectLinkTargetCapability, spec.linkCaps, p.caps)
	p.defectLinks(d.ID, spec.title, model.DefectLinkTargetComponent, spec.linkComps, p.components)
}

func (p *productSeed) defectLinks(defectID, title, targetType string, keys []string, lookup map[string]string) {
	for _, key := range keys {
		err := p.s.svc.Backlog.AddDefectLink(p.s.ctx, p.s.admin, p.id, defectID, targetType, lookup[key])
		p.s.fail("link defect "+title+" to "+targetType+" "+key, err)
	}
}

// setDefectStatus moves a defect to another status after the fact. Closing a
// defect is what pushes the campaign results it broke into re-test, so the
// order matters and is left to the callers.
func (p *productSeed) setDefectStatus(handle, status string) {
	if p.s.stopped() {
		return
	}
	statusID := p.statuses["defect"][status]
	_, err := p.s.svc.Backlog.UpdateDefect(p.s.ctx, p.s.admin, p.id, p.defects[handle], service.UpdateDefectInput{
		StatusID: &statusID,
	})
	p.s.fail("move defect "+handle+" to "+status, err)
}

// fillerDefectAreas and fillerDefectSymptoms are combined into generated titles.
// Two lists rather than one so titles share words across defects, which is what
// makes a substring search return a partial result rather than everything or
// nothing.
var (
	fillerDefectAreas = []string{
		"Sighting sync", "Species search", "Photo upload", "Map rendering",
		"Offline cache", "CSV export", "Push notification", "Sign-in",
		"Species catalogue", "Field notes", "Checklist", "Location fix",
	}
	fillerDefectSymptoms = []string{
		"drops the last entry", "times out on slow networks",
		"renders stale data", "loses focus after submit",
		"double-counts repeated entries", "ignores the selected filter",
		"crashes when the device rotates", "shows the wrong timestamp",
		"returns results in the wrong order", "silently discards edits",
	}
	fillerDefectCriticalities = []string{
		model.DefectCriticalityCritical,
		model.DefectCriticalityHigh,
		model.DefectCriticalityMedium,
		model.DefectCriticalityLow,
	}
)

// fillerDefects appends n generated defects so the list view has enough rows to
// page, sort and filter against. The curated defects above stay first and
// unchanged; these exist purely to give the table volume and variety.
//
// They are written straight to the store rather than through
// Backlog.CreateDefect, which is the same carve-out the demo accounts use: the
// service stamps its own CreatedAt, so every defect would share one timestamp
// and sorting or paging by creation date would be meaningless. Going through
// the store is what lets the fixture spread them across four months.
//
// Everything cycles on the index rather than on a random source, so the same
// database always comes out the same way and a failing case can be reproduced.
func (p *productSeed) fillerDefects(n int, statuses, envs, sprints []string) {
	if p.s.stopped() {
		return
	}

	for i := range n {
		area := fillerDefectAreas[i%len(fillerDefectAreas)]
		symptom := fillerDefectSymptoms[(i/len(fillerDefectAreas))%len(fillerDefectSymptoms)]

		// Two days apart, oldest first, so created_at is strictly increasing
		// and a date sort has a defined order to produce.
		created := day(-2 * (n - i))

		d := &model.Defect{
			ID:          uuid.NewString(),
			ProductID:   p.id,
			Title:       fmt.Sprintf("%s %s (#%d)", area, symptom, i+1),
			Description: fmt.Sprintf("Reported during triage sweep %d. Reproduced on the affected build; see the linked notes.", i+1),
			Criticality: fillerDefectCriticalities[i%len(fillerDefectCriticalities)],
			StatusID:    p.statuses["defect"][statuses[i%len(statuses)]],
			// Every third defect has no environment and every fourth none of a
			// sprint, so "unset" is represented too — a facet that only ever
			// sees populated rows proves nothing.
			EnvironmentID: p.envs[cycleOrEmpty(envs, i, 3)],
			SprintID:      p.sprints[cycleOrEmpty(sprints, i, 4)],
			Priority:      i%5 + 1,
			IsActive:      i%3 == 0,
			CreatedAt:     created,
			UpdatedAt:     created,
		}
		// Inserted directly to control created_at, so the number has to be
		// drawn here as the service would (FR-49).
		seq, err := p.s.st.Defect.NextSequenceNumber(p.s.ctx, p.id)
		if err != nil {
			p.s.fail(fmt.Sprintf("number filler defect %d", i+1), err)
			return
		}
		d.SequenceNumber = seq
		if err := p.s.st.Defect.Create(p.s.ctx, d); err != nil {
			p.s.fail(fmt.Sprintf("create filler defect %d", i+1), err)
			return
		}
	}
}

// cycleOrEmpty picks the i-th entry of values, or "" on every everyNth index so
// the generated data includes rows with the field unset.
func cycleOrEmpty(values []string, i, everyNth int) string {
	if len(values) == 0 || i%everyNth == 0 {
		return ""
	}
	return values[i%len(values)]
}

// --- Constraints ---

type constraintSpec struct {
	category    string
	title       string
	description string
	compliance  string
	milestone   string
	priority    int
	linkCaps    []string
}

func (p *productSeed) constraint(spec constraintSpec) {
	if p.s.stopped() {
		return
	}
	c, err := p.s.svc.Constraint.CreateConstraint(p.s.ctx, p.s.admin, service.CreateConstraintInput{
		ProductID:       p.id,
		CategoryID:      p.categories["constraint"][spec.category],
		Title:           spec.title,
		Description:     spec.description,
		ComplianceState: spec.compliance,
		MilestoneID:     p.milestones[spec.milestone],
		Priority:        spec.priority,
	})
	if err != nil {
		p.s.fail("create constraint "+spec.title, err)
		return
	}
	p.cons[spec.title] = c.ID

	for _, capability := range spec.linkCaps {
		err := p.s.svc.Constraint.LinkCapability(p.s.ctx, p.s.admin, p.id, c.ID, p.caps[capability])
		p.s.fail("link constraint "+spec.title+" to "+capability, err)
	}
}

// --- Documents ---

// documentSpec seeds a document (FR-50). Revisions after the first are
// written as updates, so the document arrives with a history to diff.
type documentSpec struct {
	title      string
	format     string
	state      string
	component  string
	capability string
	milestone  string
	revisions  []string // bodies, oldest first
	linkReqs   []string // requirement handles
	linkCons   []string // constraint titles
	linkTests  []string // test case handles
}

func (p *productSeed) document(spec documentSpec) {
	if p.s.stopped() {
		return
	}
	d, err := p.s.svc.Document.CreateDocument(p.s.ctx, p.s.admin, service.CreateDocumentInput{
		ProductID:    p.id,
		Title:        spec.title,
		Body:         spec.revisions[0],
		Format:       spec.format,
		State:        spec.state,
		ComponentID:  p.components[spec.component],
		CapabilityID: p.caps[spec.capability],
		MilestoneID:  p.milestones[spec.milestone],
	})
	if err != nil {
		p.s.fail("create document "+spec.title, err)
		return
	}
	for _, body := range spec.revisions[1:] {
		_, err := p.s.svc.Document.UpdateDocument(p.s.ctx, p.s.admin, p.id, d.ID, service.UpdateDocumentInput{Body: &body})
		p.s.fail("revise document "+spec.title, err)
	}
	for _, h := range spec.linkReqs {
		p.s.fail("link document "+spec.title, p.s.svc.Document.LinkDocument(p.s.ctx, p.s.admin, p.id, d.ID, model.DocumentLinkRequirement, p.reqs[h]))
	}
	for _, title := range spec.linkCons {
		p.s.fail("link document "+spec.title, p.s.svc.Document.LinkDocument(p.s.ctx, p.s.admin, p.id, d.ID, model.DocumentLinkConstraint, p.cons[title]))
	}
	for _, h := range spec.linkTests {
		p.s.fail("link document "+spec.title, p.s.svc.Document.LinkDocument(p.s.ctx, p.s.admin, p.id, d.ID, model.DocumentLinkTestCase, p.testCases[h]))
	}
}

// --- Test cases ---

type testCaseSpec struct {
	handle      string
	title       string
	description string
	component   string
	capability  string
	level       string // pyramid level name
	mode        string // execution mode
	identifier  string // test function name for automatic tests
	envs        []string
	// A test case links to a given requirement exactly once, so reqs and
	// dodReqs must not name the same handle.
	reqs    []string // requirement handles, linked as "requirement"
	dodReqs []string // requirement handles, linked as "definition_of_done"
}

func (p *productSeed) testCase(spec testCaseSpec) {
	if p.s.stopped() {
		return
	}

	tc, err := p.s.svc.Testing.CreateTestCase(p.s.ctx, p.s.admin, service.CreateTestCaseInput{
		ProductID:      p.id,
		ComponentID:    p.components[spec.component],
		CapabilityID:   p.caps[spec.capability],
		Title:          spec.title,
		Description:    spec.description,
		ExecutionMode:  spec.mode,
		TestIdentifier: spec.identifier,
		PyramidLevelID: p.levels[spec.level],
	})
	if err != nil {
		p.s.fail("create test case "+spec.title, err)
		return
	}
	if spec.handle != "" {
		p.testCases[spec.handle] = tc.ID
	}

	for _, env := range spec.envs {
		err := p.s.svc.Testing.AddTestCaseEnvironment(p.s.ctx, p.s.admin, p.id, tc.ID, p.envs[env])
		p.s.fail("add environment "+env+" to "+spec.title, err)
	}
	for _, handle := range spec.reqs {
		err := p.s.svc.Testing.LinkRequirement(p.s.ctx, p.s.admin, p.id, tc.ID, p.reqs[handle], "requirement")
		p.s.fail("link "+spec.title+" to requirement "+handle, err)
	}
	for _, handle := range spec.dodReqs {
		err := p.s.svc.Testing.LinkRequirement(p.s.ctx, p.s.admin, p.id, tc.ID, p.reqs[handle], "definition_of_done")
		p.s.fail("link "+spec.title+" to DoD of "+handle, err)
	}
}

// --- Campaigns ---

// verdict names the outcome to record for one test case in a campaign. A
// failing verdict must name a defect handle — the service rejects a failure
// with no defect behind it.
type verdict struct {
	testCase string // test case handle
	status   string
	defect   string // defect handle, required when status is "failed"
}

// campaign creates a campaign over the given test cases, activates it, records
// the verdicts and optionally closes it. Results are only writable while the
// campaign is active, so this ordering is not incidental.
func (p *productSeed) campaign(name, description, startsAt, endsAt string, testCases []string, verdicts []verdict, executedBy string, closeIt bool) {
	if p.s.stopped() {
		return
	}

	ids := make([]string, 0, len(testCases))
	for _, handle := range testCases {
		ids = append(ids, p.testCases[handle])
	}

	c, err := p.s.svc.Testing.CreateCampaign(p.s.ctx, p.s.admin, service.CreateCampaignInput{
		ProductID:   p.id,
		Name:        name,
		Description: description,
		StartsAt:    startsAt,
		EndsAt:      endsAt,
		TestCaseIDs: ids,
	})
	if err != nil {
		p.s.fail("create campaign "+name, err)
		return
	}

	if _, err := p.s.svc.Testing.ActivateCampaign(p.s.ctx, p.s.admin, p.id, c.ID); err != nil {
		p.s.fail("activate campaign "+name, err)
		return
	}

	results, err := p.s.svc.Testing.ListCampaignResults(p.s.ctx, p.s.admin, p.id, c.ID)
	if err != nil {
		p.s.fail("list results of campaign "+name, err)
		return
	}
	// A campaign seeds one result per (test case, environment) pair, so a test
	// case with three environments has three rows to decide.
	resultsByTestCase := map[string][]string{}
	for _, r := range results {
		resultsByTestCase[r.TestCaseID] = append(resultsByTestCase[r.TestCaseID], r.ID)
	}

	tester := p.s.user(executedBy)
	for _, v := range verdicts {
		for _, resultID := range resultsByTestCase[p.testCases[v.testCase]] {
			_, err := p.s.svc.Testing.SetResultStatus(
				p.s.ctx, tester, p.id, c.ID, resultID, v.status, p.defects[v.defect],
			)
			p.s.fail("set result "+v.testCase+" in campaign "+name, err)
		}
	}

	if closeIt {
		// force: the closed demo campaign is allowed to carry undecided rows,
		// which is what the confirmation dialog exists for.
		if _, err := p.s.svc.Testing.CloseCampaign(p.s.ctx, p.s.admin, p.id, c.ID, true); err != nil {
			p.s.fail("close campaign "+name, err)
		}
	}
}

// --- Discussion ---

// comment posts a comment on a requirement, authored by a demo user so the
// discussion threads show more than one name.
func (p *productSeed) comment(username, ownerType, handle, body string) {
	if p.s.stopped() {
		return
	}

	var ownerID string
	switch ownerType {
	case model.OwnerTypeRequirement, model.OwnerTypeTask:
		ownerID = p.reqs[handle]
	case model.OwnerTypeDefect:
		ownerID = p.defects[handle]
	case model.OwnerTypeTestCase:
		ownerID = p.testCases[handle]
	default:
		p.s.fail("comment on "+handle, fmt.Errorf("unsupported comment owner type %q", ownerType))
		return
	}

	_, err := p.s.svc.Discussion.CreateComment(p.s.ctx, p.s.user(username), p.id, ownerType, ownerID, body)
	p.s.fail("comment by "+username+" on "+handle, err)
}
