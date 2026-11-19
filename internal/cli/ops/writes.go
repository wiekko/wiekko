package ops

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"nexmap.net/internal/client"
)

// Write operations. Each resolves names and references client-side, validates
// everything it can before the first write, and returns the same detail view
// the matching show operation returns, so callers see the result of their
// change. Authorization is entirely the server's: these operations never
// check roles themselves.

// StatusChange sets the status of a requirement, task or defect by name.
type StatusChange struct {
	Ref    string `json:"ref" jsonschema:"the item: identifier (FR-12) or ID; for defects the short ID or ID"`
	Status string `json:"status" jsonschema:"status name as shown by the workflow, e.g. In Progress (case-insensitive)"`
	// Kind restricts a requirement status change to requirements or tasks
	// (the CLI's "task status"); not part of the MCP schema.
	Kind string `json:"-"`
}

func (c StatusChange) validate() error {
	if strings.TrimSpace(c.Ref) == "" {
		return errors.New("an item reference is required")
	}
	if strings.TrimSpace(c.Status) == "" {
		return errors.New("a status name is required")
	}
	return nil
}

// SetRequirementStatus moves a requirement or task to another status
// (developer role, FR-116).
func (s *Session) SetRequirementStatus(ctx context.Context, in StatusChange) (*RequirementDetail, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	r, err := s.requirement(ctx, in.Ref, in.Kind)
	if err != nil {
		return nil, err
	}
	statusID, err := s.statusID(ctx, "requirement", strings.TrimSpace(in.Status))
	if err != nil {
		return nil, err
	}
	if _, err := s.C.UpdateRequirement(ctx, s.ProductID, r.ID, client.RequirementPatch{StatusID: &statusID}); err != nil {
		return nil, err
	}
	return s.ShowRequirement(ctx, r.ID)
}

// SetDefectStatus moves a defect to another status (developer role).
func (s *Session) SetDefectStatus(ctx context.Context, in StatusChange) (*DefectDetail, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	d, err := s.findDefect(ctx, in.Ref)
	if err != nil {
		return nil, err
	}
	statusID, err := s.statusID(ctx, "defect", strings.TrimSpace(in.Status))
	if err != nil {
		return nil, err
	}
	if _, err := s.C.UpdateDefect(ctx, s.ProductID, d.ID, client.DefectPatch{StatusID: &statusID}); err != nil {
		return nil, err
	}
	return s.ShowDefect(ctx, d.ID)
}

// initialStatus resolves an optional status name for a new item; empty
// leaves the choice to the server, which applies the workflow's default.
func (s *Session) initialStatus(ctx context.Context, entityType, name string) (string, error) {
	if name = strings.TrimSpace(name); name == "" {
		return "", nil
	}
	return s.statusID(ctx, entityType, name)
}

// requirement fetches a requirement or task by identifier or ID, optionally
// insisting on its kind.
func (s *Session) requirement(ctx context.Context, ref, kind string) (*client.Requirement, error) {
	what := "requirement or task"
	if kind != "" {
		what = kind
	}
	r, err := s.C.Requirement(ctx, s.ProductID, strings.TrimSpace(ref))
	if err != nil {
		if client.IsNotFound(err) {
			return nil, &NotFoundError{Kind: what, Ref: ref}
		}
		return nil, err
	}
	if kind != "" && r.Kind != kind {
		return nil, &NotFoundError{Kind: what, Ref: ref}
	}
	return r, nil
}

// BacklogItemChange changes a requirement or task; nil fields stay as they are.
type BacklogItemChange struct {
	Ref    string  `json:"ref" jsonschema:"the item: identifier (FR-12) or ID"`
	Parent *string `json:"parent,omitempty" jsonschema:"identifier of the new parent requirement or task, e.g. FR-3; empty moves the item to the top level"`
	// Kind restricts the change to requirements or tasks (the CLI's
	// "task update"); not part of a schema.
	Kind string `json:"-"`
}

// UpdateBacklogItem changes a requirement or task (owner role). The server
// checks the new parent as it does on create: same product, no cycle, and a
// requirement never below a task (FR-152).
func (s *Session) UpdateBacklogItem(ctx context.Context, in BacklogItemChange) (*RequirementDetail, error) {
	if strings.TrimSpace(in.Ref) == "" {
		return nil, errors.New("an item reference is required")
	}
	if in.Parent == nil {
		return nil, errors.New("nothing to change: give a parent")
	}
	r, err := s.requirement(ctx, in.Ref, in.Kind)
	if err != nil {
		return nil, err
	}
	var patch client.RequirementPatch
	parentID := ""
	if ref := strings.TrimSpace(*in.Parent); ref != "" {
		p, err := s.requirement(ctx, ref, "")
		if err != nil {
			return nil, err
		}
		parentID = p.ID
	}
	patch.ParentID = &parentID
	if _, err := s.C.UpdateRequirement(ctx, s.ProductID, r.ID, patch); err != nil {
		return nil, err
	}
	return s.ShowRequirement(ctx, r.ID)
}

// NewBacklogItem creates a requirement or task.
type NewBacklogItem struct {
	Kind             string `json:"kind,omitempty" jsonschema:"task (default) or requirement"`
	Title            string `json:"title" jsonschema:"short title"`
	Category         string `json:"category,omitempty" jsonschema:"category prefix (e.g. MT) or name; omit for the product's default category of that kind"`
	Parent           string `json:"parent,omitempty" jsonschema:"identifier of the requirement this item refines, e.g. FR-12"`
	Status           string `json:"status,omitempty" jsonschema:"initial status name; omit for the default"`
	Description      string `json:"description,omitempty" jsonschema:"Markdown description"`
	DefinitionOfDone string `json:"definition_of_done,omitempty" jsonschema:"Markdown definition of done"`
	// Capabilities links a new requirement to capabilities (FR-156).
	Capabilities []string `json:"capabilities,omitempty" jsonschema:"names or IDs of the capabilities a requirement belongs to; not for tasks, which reach capabilities through their parent"`
}

// CreateBacklogItem creates a requirement or task (owner role, FR-81 and
// FR-100). Kind defaults to task: splitting work is the common agent case.
func (s *Session) CreateBacklogItem(ctx context.Context, in NewBacklogItem) (*RequirementDetail, error) {
	kind := strings.ToLower(strings.TrimSpace(in.Kind))
	if kind == "" {
		kind = "task"
	}
	if kind != "task" && kind != "requirement" {
		return nil, fmt.Errorf("kind must be task or requirement, not %q", in.Kind)
	}
	if strings.TrimSpace(in.Title) == "" {
		return nil, errors.New("a title is required")
	}
	body := client.RequirementCreate{
		Kind: kind, Title: strings.TrimSpace(in.Title),
		Description: in.Description, DefinitionOfDone: in.DefinitionOfDone,
	}
	var err error
	if in.Category != "" {
		if body.CategoryID, err = s.categoryID(ctx, strings.TrimSpace(in.Category), kind); err != nil {
			return nil, err
		}
	}
	if body.StatusID, err = s.initialStatus(ctx, "requirement", in.Status); err != nil {
		return nil, err
	}
	if in.Parent != "" {
		p, err := s.requirement(ctx, in.Parent, "")
		if err != nil {
			return nil, err
		}
		body.ParentID = p.ID
	}
	if len(in.Capabilities) > 0 && kind != "requirement" {
		return nil, errors.New("only requirements are linked to capabilities; a task reaches them through its parent")
	}
	for _, ref := range in.Capabilities {
		c, err := s.findCapability(ctx, ref)
		if err != nil {
			return nil, err
		}
		body.CapabilityIDs = append(body.CapabilityIDs, c.ID)
	}
	r, err := s.C.CreateRequirement(ctx, s.ProductID, body)
	if err != nil {
		return nil, err
	}
	return s.ShowRequirement(ctx, r.ID)
}

// NewDefect reports a defect, optionally linked to the test case that shows
// it and the requirement it violates.
type NewDefect struct {
	Title       string `json:"title" jsonschema:"short summary of the bug"`
	Description string `json:"description,omitempty" jsonschema:"Markdown description: steps to reproduce, expected and actual behaviour"`
	Criticality string `json:"criticality,omitempty" jsonschema:"critical, high, medium (default) or low"`
	Status      string `json:"status,omitempty" jsonschema:"initial status name; omit for the default"`
	TestCase    string `json:"test_case,omitempty" jsonschema:"test case that reveals the defect: test identifier, short ID or ID"`
	Requirement string `json:"requirement,omitempty" jsonschema:"requirement or task the defect violates, e.g. FR-12"`
}

var criticalities = []string{"critical", "high", "medium", "low"}

// CreateDefect creates a defect (tester role) and links it. Links are
// resolved before the defect is created, so a bad reference creates nothing.
func (s *Session) CreateDefect(ctx context.Context, in NewDefect) (*DefectDetail, error) {
	if strings.TrimSpace(in.Title) == "" {
		return nil, errors.New("a title is required")
	}
	body := client.DefectCreate{Title: strings.TrimSpace(in.Title), Description: in.Description}
	if c := strings.ToLower(strings.TrimSpace(in.Criticality)); c != "" {
		if !contains(criticalities, c) {
			return nil, fmt.Errorf("criticality must be one of %s, not %q", strings.Join(criticalities, ", "), in.Criticality)
		}
		body.Criticality = c
	}
	var err error
	if body.StatusID, err = s.initialStatus(ctx, "defect", in.Status); err != nil {
		return nil, err
	}
	type link struct{ targetType, id, label string }
	var links []link
	if in.TestCase != "" {
		tc, err := s.findTestCase(ctx, in.TestCase)
		if err != nil {
			return nil, err
		}
		links = append(links, link{"test_case", tc.ID, "test case " + ShortID(tc.ID)})
	}
	if in.Requirement != "" {
		r, err := s.requirement(ctx, in.Requirement, "")
		if err != nil {
			return nil, err
		}
		links = append(links, link{"requirement", r.ID, r.Identifier})
	}

	d, err := s.C.CreateDefect(ctx, s.ProductID, body)
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		if err := s.C.AddDefectLink(ctx, s.ProductID, d.ID, l.targetType, l.id); err != nil {
			return nil, fmt.Errorf("defect %s was created, but linking it to %s failed: %w", d.Identifier, l.label, err)
		}
	}
	return s.ShowDefect(ctx, d.ID)
}

// NewComment adds a comment to an entity.
type NewComment struct {
	Ref  string `json:"ref" jsonschema:"what to comment on: identifier (FR-12, AC-3) or, with type, a short ID, ID or name"`
	Type string `json:"type,omitempty" jsonschema:"requirement, task, constraint, defect, test_case, capability or component; may be omitted for identifiers like FR-12"`
	Body string `json:"body" jsonschema:"Markdown comment text"`
}

// CommentResult is the comment that was added and what it is attached to.
type CommentResult struct {
	OwnerType string      `json:"owner_type"`
	Owner     Ref         `json:"owner"`
	Comment   CommentView `json:"comment"`
}

// commentTypes are the entity types comments can be added to through ops.
var commentTypes = []string{"requirement", "task", "constraint", "defect", "test_case", "capability", "component"}

var identifierPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,5}-[1-9][0-9]{0,9}$`)

// IsIdentifier reports whether s looks like an identifier such as FR-12.
func IsIdentifier(s string) bool { return identifierPattern.MatchString(strings.TrimSpace(s)) }

// AddComment adds a Markdown comment (tester role on defects and test cases,
// developer role elsewhere).
func (s *Session) AddComment(ctx context.Context, in NewComment) (*CommentResult, error) {
	ref := strings.TrimSpace(in.Ref)
	if ref == "" {
		return nil, errors.New("a reference to the commented item is required")
	}
	if strings.TrimSpace(in.Body) == "" {
		return nil, errors.New("the comment body is empty")
	}
	ownerType, owner, err := s.commentOwner(ctx, strings.ToLower(strings.TrimSpace(in.Type)), ref)
	if err != nil {
		return nil, err
	}
	c, err := s.C.CreateComment(ctx, s.ProductID, ownerType, owner.ID, in.Body)
	if err != nil {
		return nil, err
	}
	users, err := s.usernames(ctx)
	if err != nil {
		return nil, err
	}
	return &CommentResult{
		OwnerType: ownerType,
		Owner:     owner,
		Comment:   CommentView{Author: author(users, c.UserID), CreatedAt: c.CreatedAt, Body: c.Body},
	}, nil
}

// commentOwner resolves the commented entity. Without a type, only
// identifiers are accepted: they name a requirement, task, constraint or
// defect.
func (s *Session) commentOwner(ctx context.Context, typ, ref string) (string, Ref, error) {
	switch typ {
	case "":
		if !identifierPattern.MatchString(ref) {
			return "", Ref{}, fmt.Errorf("%q is not an identifier like FR-12; pass a type (%s)", ref, strings.Join(commentTypes, ", "))
		}
		r, err := s.C.Requirement(ctx, s.ProductID, ref)
		if err == nil {
			return r.Kind, Ref{ID: r.ID, Identifier: r.Identifier, Title: r.Title}, nil
		}
		if !client.IsNotFound(err) {
			return "", Ref{}, err
		}
		typ, owner, err := s.commentOwner(ctx, "constraint", ref)
		var nf *NotFoundError
		if !errors.As(err, &nf) {
			return typ, owner, err
		}
		d, err := s.C.Defect(ctx, s.ProductID, ref)
		if err != nil {
			if client.IsNotFound(err) {
				return "", Ref{}, &NotFoundError{Kind: "requirement, task, constraint or defect", Ref: ref}
			}
			return "", Ref{}, err
		}
		return "defect", Ref{ID: d.ID, Identifier: d.Identifier, Title: d.Title}, nil
	case "requirement", "task":
		r, err := s.requirement(ctx, ref, typ)
		if err != nil {
			return "", Ref{}, err
		}
		return r.Kind, Ref{ID: r.ID, Identifier: r.Identifier, Title: r.Title}, nil
	case "constraint":
		c, err := s.C.Constraint(ctx, s.ProductID, ref)
		if err != nil {
			if client.IsNotFound(err) {
				return "", Ref{}, &NotFoundError{Kind: "requirement, task or constraint", Ref: ref}
			}
			return "", Ref{}, err
		}
		return typ, Ref{ID: c.ID, Identifier: c.Identifier, Title: c.Title}, nil
	case "defect":
		d, err := s.findDefect(ctx, ref)
		if err != nil {
			return "", Ref{}, err
		}
		return typ, Ref{ID: d.ID, Identifier: d.Identifier, Title: d.Title}, nil
	case "test_case", "test":
		tc, err := s.findTestCase(ctx, ref)
		if err != nil {
			return "", Ref{}, err
		}
		return "test_case", Ref{ID: tc.ID, Title: tc.Title}, nil
	case "capability":
		c, err := s.findCapability(ctx, ref)
		if err != nil {
			return "", Ref{}, err
		}
		return typ, Ref{ID: c.ID, Title: c.Name}, nil
	case "component":
		list, err := s.components(ctx)
		if err != nil {
			return "", Ref{}, err
		}
		c, err := match("component", ref, list,
			func(c client.Component) string { return c.ID },
			func(c client.Component) string { return c.Name },
			func(c client.Component) bool { return strings.EqualFold(c.Name, ref) })
		if err != nil {
			return "", Ref{}, err
		}
		return typ, Ref{ID: c.ID, Title: c.Name}, nil
	}
	return "", Ref{}, fmt.Errorf("cannot comment on %q; types: %s", typ, strings.Join(commentTypes, ", "))
}
