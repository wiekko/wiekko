package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
	"nexmap.net/internal/textdiff"
)

// Limits on document input. The body limit keeps a document under the
// server's request body limit (middleware.DefaultMaxBody), both wrapped in a
// JSON request (API, CLI) and in the web form, which is multipart for this
// reason: URL-encoding would triple every non-ASCII byte.
const (
	MaxDocumentTitleLength = 200        // runes
	MaxDocumentBodyBytes   = 512 * 1024 // bytes
)

// DocumentService handles documents, their revisions and links
// (FR-044..FR-052, FR-334).
type DocumentService struct {
	// tx groups multi-step writes into one transaction (nil in unit tests).
	tx           store.Transactor
	documents    store.DocumentStore
	products     store.ProductStore
	requirements store.RequirementStore
	constraints  store.ConstraintStore
	testCases    store.TestCaseStore
	categories   store.CategoryStore
	comments     ownerPurger
	audit        store.AuditStore
	authz        *Authorizer
	owns         *ownership
}

// CreateDocumentInput contains the fields of a new document.
type CreateDocumentInput struct {
	ProductID      string
	Title          string
	Body           string
	Format         string // default markdown
	State          string // default draft
	SupersededByID string
	ComponentID    string
	CapabilityID   string
	MilestoneID    string
}

// UpdateDocumentInput contains the fields that can be changed; nil means
// unchanged.
type UpdateDocumentInput struct {
	// IfUpdatedAt is the updated_at of the version the client edited. When
	// set and the item has changed since, the update fails with ErrConflict.
	IfUpdatedAt    string
	Title          *string
	Body           *string
	Format         *string
	State          *string
	SupersededByID *string
	ComponentID    *string
	CapabilityID   *string
	MilestoneID    *string
}

// DocumentDiff is the difference between two revisions of a document.
type DocumentDiff struct {
	From, To *model.DocumentRevision
	Lines    []textdiff.Line
	// TitleChanged and FormatChanged report changes outside the body.
	TitleChanged, FormatChanged bool
}

// Unified renders the body diff in unified format.
func (d *DocumentDiff) Unified(identifier string) string {
	return textdiff.Unified(
		fmt.Sprintf("%s revision %d", identifier, d.From.Revision),
		fmt.Sprintf("%s revision %d", identifier, d.To.Revision), d.Lines)
}

// cleanDocumentTitle validates a title like every other user-written line
// of text: trimmed, required, valid UTF-8, without control characters.
func cleanDocumentTitle(title string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return "", errors.New("document title is required")
	}
	if !utf8.ValidString(title) {
		return "", errors.New("document title is not valid UTF-8")
	}
	if utf8.RuneCountInString(title) > MaxDocumentTitleLength {
		return "", fmt.Errorf("document title is longer than %d characters", MaxDocumentTitleLength)
	}
	for _, r := range title {
		if unicode.IsControl(r) {
			return "", errors.New("document title must not contain control characters")
		}
	}
	return title, nil
}

// checkDocumentBody validates a body. It is otherwise kept byte for byte
// (FR-046): active content is neutralised where a body is rendered (Markdown
// through the safe renderer, everything else escaped) and served (as an
// attachment of type text/plain), never by rewriting what the author wrote.
func checkDocumentBody(body string) error {
	if len(body) > MaxDocumentBodyBytes {
		return fmt.Errorf("document body is larger than %d KiB", MaxDocumentBodyBytes/1024)
	}
	if !utf8.ValidString(body) {
		return errors.New("document body is not valid UTF-8")
	}
	if strings.ContainsRune(body, 0) {
		return errors.New("document body must not contain NUL characters")
	}
	return nil
}

func checkDocumentFormat(format string) error {
	if !model.ValidDocumentFormat(format) {
		return fmt.Errorf("unknown document format %q (one of %s)", format, strings.Join(model.DocumentFormats, ", "))
	}
	return nil
}

func checkDocumentState(state string) error {
	if !model.ValidDocumentState(state) {
		return fmt.Errorf("unknown document state %q (one of %s)", state, strings.Join(model.DocumentStates, ", "))
	}
	return nil
}

// checkPlacement enforces that a document sits under a component or a
// capability, not both (FR-047), and that only a superseded document names
// a successor other than itself (FR-049).
func checkPlacement(d *model.Document) error {
	if d.ComponentID != "" && d.CapabilityID != "" {
		return errors.New("a document belongs to a component or a capability, not both")
	}
	if d.SupersededByID != "" {
		if d.State != model.DocumentStateSuperseded {
			return errors.New("only a superseded document can name the document replacing it")
		}
		if d.SupersededByID == d.ID {
			return errors.New("a document cannot supersede itself")
		}
	}
	return nil
}

// CreateDocument creates a document with its first revision. Requires owner
// role (FR-334).
func (s *DocumentService) CreateDocument(ctx context.Context, user *model.User, input CreateDocumentInput) (*model.Document, error) {
	if _, err := s.authz.RequireRole(ctx, user, input.ProductID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, input.ProductID, ref{kindDocument, input.SupersededByID},
		ref{kindComponent, input.ComponentID}, ref{kindCapability, input.CapabilityID},
		ref{kindMilestone, input.MilestoneID}); err != nil {
		return nil, err
	}
	title, err := cleanDocumentTitle(input.Title)
	if err != nil {
		return nil, err
	}
	if err := checkDocumentBody(input.Body); err != nil {
		return nil, err
	}
	if input.Format == "" {
		input.Format = model.DocumentFormatMarkdown
	}
	if err := checkDocumentFormat(input.Format); err != nil {
		return nil, err
	}
	if input.State == "" {
		input.State = model.DocumentStateDraft
	}
	if err := checkDocumentState(input.State); err != nil {
		return nil, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	d := &model.Document{
		ID: uuid.New().String(), ProductID: input.ProductID, Title: title, Body: input.Body,
		Format: input.Format, State: input.State, SupersededByID: input.SupersededByID,
		ComponentID: input.ComponentID, CapabilityID: input.CapabilityID, MilestoneID: input.MilestoneID,
		Revision: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := checkPlacement(d); err != nil {
		return nil, err
	}
	rev := s.snapshot(d, user, now)
	// The sequence lock holds off concurrent creates between drawing the next
	// number and inserting it.
	err = inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := lock(ctx, s.tx, lockSequence("document", d.ProductID)); err != nil {
			return err
		}
		seq, err := s.documents.NextSequenceNumber(ctx, d.ProductID)
		if err != nil {
			return fmt.Errorf("next document number: %w", err)
		}
		d.SequenceNumber = seq
		if err := s.documents.Create(ctx, d, rev); err != nil {
			return fmt.Errorf("create document: %w", err)
		}
		return s.record(ctx, user, d.ID, "create", "", "", "", now)
	})
	if err != nil {
		return nil, err
	}
	d.Identifier = model.EntityIdentifier(s.documentPrefix(ctx, d.ProductID), d.SequenceNumber)
	return d, nil
}

func (s *DocumentService) snapshot(d *model.Document, user *model.User, now string) *model.DocumentRevision {
	return &model.DocumentRevision{
		ID: uuid.New().String(), DocumentID: d.ID, Revision: d.Revision,
		Title: d.Title, Body: d.Body, Format: d.Format, UserID: user.ID, CreatedAt: now,
	}
}

func (s *DocumentService) record(ctx context.Context, user *model.User, id, action, field, oldValue, newValue, now string) error {
	return s.audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), EntityType: "document", EntityID: id, UserID: user.ID,
		Action: action, FieldName: field, OldValue: oldValue, NewValue: newValue, CreatedAt: now,
	})
}

// GetDocument returns a document with its body. Requires viewer role.
func (s *DocumentService) GetDocument(ctx context.Context, user *model.User, productID, id string) (*model.Document, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindDocument, id}); err != nil {
		return nil, err
	}
	return s.documents.GetByID(ctx, id)
}

// UpdateDocument changes a document. A change to its title, body or format
// creates a new revision (FR-051); state, placement and milestone changes do
// not, and are recorded in the audit trail only. Requires owner role.
func (s *DocumentService) UpdateDocument(ctx context.Context, user *model.User, productID, id string, input UpdateDocumentInput) (*model.Document, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindDocument, id}, ref{kindDocument, opt(input.SupersededByID)},
		ref{kindComponent, opt(input.ComponentID)}, ref{kindCapability, opt(input.CapabilityID)},
		ref{kindMilestone, opt(input.MilestoneID)}); err != nil {
		return nil, err
	}
	var d *model.Document
	err := inTx(ctx, s.tx, func(ctx context.Context) error {
		// The entity's lock serializes concurrent edits: each one reads the
		// row as the previous one left it, so none overwrites another's change.
		// IfUpdatedAt carries the version the client edited, if it sent one.
		if err := lock(ctx, s.tx, lockEntity(id)); err != nil {
			return err
		}
		var err error
		d, err = s.documents.GetByID(ctx, id)
		if err != nil {
			return err
		}
		if err := checkVersion(input.IfUpdatedAt, d.UpdatedAt); err != nil {
			return err
		}
		old := *d

		if input.Title != nil {
			if d.Title, err = cleanDocumentTitle(*input.Title); err != nil {
				return err
			}
		}
		if input.Body != nil {
			if err := checkDocumentBody(*input.Body); err != nil {
				return err
			}
			d.Body = *input.Body
		}
		if input.Format != nil {
			if err := checkDocumentFormat(*input.Format); err != nil {
				return err
			}
			d.Format = *input.Format
		}
		if input.State != nil {
			if err := checkDocumentState(*input.State); err != nil {
				return err
			}
			d.State = *input.State
		}
		if input.SupersededByID != nil {
			d.SupersededByID = *input.SupersededByID
		} else if d.State != model.DocumentStateSuperseded {
			// Leaving the superseded state drops the successor it named.
			d.SupersededByID = ""
		}
		if input.ComponentID != nil {
			d.ComponentID = *input.ComponentID
		}
		if input.CapabilityID != nil {
			d.CapabilityID = *input.CapabilityID
		}
		if input.MilestoneID != nil {
			d.MilestoneID = *input.MilestoneID
		}
		if err := checkPlacement(d); err != nil {
			return err
		}

		now := time.Now().UTC().Format(time.RFC3339)
		var rev *model.DocumentRevision
		if d.Title != old.Title || d.Body != old.Body || d.Format != old.Format {
			d.Revision++
			rev = s.snapshot(d, user, now)
		}
		d.UpdatedAt = now
		err = inTx(ctx, s.tx, func(ctx context.Context) error {
			if err := s.documents.Update(ctx, d, rev); err != nil {
				return fmt.Errorf("update document: %w", err)
			}
			if rev != nil {
				if err := s.record(ctx, user, d.ID, "update", "revision", strconv.Itoa(old.Revision), strconv.Itoa(d.Revision), now); err != nil {
					return err
				}
			}
			if d.State != old.State {
				if err := s.record(ctx, user, d.ID, "update", "state", old.State, d.State, now); err != nil {
					return err
				}
			}
			if rev == nil && d.State == old.State {
				return s.record(ctx, user, d.ID, "update", "", "", "", now)
			}
			return nil
		})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// DeleteDocument removes a document with its revisions, links, comments and
// attachments. Requires owner role.
func (s *DocumentService) DeleteDocument(ctx context.Context, user *model.User, productID, id string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindDocument, id}); err != nil {
		return err
	}
	return deleteOwned(ctx, s.tx, s.comments, s.audit, user, productID, model.OwnerTypeDocument, id, s.documents.Delete)
}

// ListDocuments returns a page of the product's documents. Requires viewer
// role.
func (s *DocumentService) ListDocuments(ctx context.Context, user *model.User, productID string, filter store.DocumentFilter, params store.ListParams) ([]*model.Document, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.documents.ListPageByProductID(ctx, productID, filter, params)
}

// CountDocuments counts the documents a list with the same filter and search
// would return. Requires viewer role.
func (s *DocumentService) CountDocuments(ctx context.Context, user *model.User, productID string, filter store.DocumentFilter, search string) (int, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return 0, err
	}
	return s.documents.CountByProductID(ctx, productID, filter, search)
}

// ListRevisions returns a document's revisions, newest first, without
// bodies (GetRevision returns one with its body). Requires viewer role.
func (s *DocumentService) ListRevisions(ctx context.Context, user *model.User, productID, id string) ([]*model.DocumentRevision, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindDocument, id}); err != nil {
		return nil, err
	}
	return s.documents.ListRevisions(ctx, id)
}

// GetRevision returns one revision of a document. Requires viewer role.
func (s *DocumentService) GetRevision(ctx context.Context, user *model.User, productID, id string, revision int) (*model.DocumentRevision, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindDocument, id}); err != nil {
		return nil, err
	}
	rev, err := s.documents.GetRevision(ctx, id, revision)
	if err != nil {
		return nil, fmt.Errorf("%w: revision %d", ErrNotFound, revision)
	}
	return rev, nil
}

// Diff compares two revisions of a document (FR-051). Requires viewer role.
func (s *DocumentService) Diff(ctx context.Context, user *model.User, productID, id string, from, to int) (*DocumentDiff, error) {
	a, err := s.GetRevision(ctx, user, productID, id, from)
	if err != nil {
		return nil, err
	}
	b, err := s.GetRevision(ctx, user, productID, id, to)
	if err != nil {
		return nil, err
	}
	return &DocumentDiff{
		From: a, To: b, Lines: textdiff.Lines(a.Body, b.Body),
		TitleChanged: a.Title != b.Title, FormatChanged: a.Format != b.Format,
	}, nil
}

// RestoreRevision makes the content of an earlier revision current again by
// writing it as a new revision; history is never rewritten (FR-051).
// Requires owner role.
func (s *DocumentService) RestoreRevision(ctx context.Context, user *model.User, productID, id string, revision int) (*model.Document, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return nil, err
	}
	rev, err := s.GetRevision(ctx, user, productID, id, revision)
	if err != nil {
		return nil, err
	}
	d, err := s.documents.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if d.Title == rev.Title && d.Body == rev.Body && d.Format == rev.Format {
		return d, nil
	}
	oldRevision := d.Revision
	now := time.Now().UTC().Format(time.RFC3339)
	d.Title, d.Body, d.Format = rev.Title, rev.Body, rev.Format
	d.Revision++
	d.UpdatedAt = now
	err = inTx(ctx, s.tx, func(ctx context.Context) error {
		if err := s.documents.Update(ctx, d, s.snapshot(d, user, now)); err != nil {
			return fmt.Errorf("restore document: %w", err)
		}
		return s.record(ctx, user, d.ID, "restore", "revision", strconv.Itoa(oldRevision), strconv.Itoa(revision), now)
	})
	if err != nil {
		return nil, err
	}
	return d, nil
}

// linkKind maps a link target type to its ownership kind. Requirements are
// checked as requirements, so a task never passes (FR-048).
func linkKind(targetType string) (string, error) {
	switch targetType {
	case model.DocumentLinkRequirement:
		return kindRequirement, nil
	case model.DocumentLinkConstraint:
		return kindConstraint, nil
	case model.DocumentLinkTestCase:
		return kindTestCase, nil
	}
	return "", fmt.Errorf("documents can be linked to requirements, constraints and test cases, not to %q", targetType)
}

// LinkDocument links a document to a requirement, constraint or test case.
// Requires owner role (FR-334).
func (s *DocumentService) LinkDocument(ctx context.Context, user *model.User, productID, id, targetType, targetID string) error {
	return s.changeLink(ctx, user, productID, id, targetType, targetID, true)
}

// UnlinkDocument removes such a link. Requires owner role.
func (s *DocumentService) UnlinkDocument(ctx context.Context, user *model.User, productID, id, targetType, targetID string) error {
	return s.changeLink(ctx, user, productID, id, targetType, targetID, false)
}

func (s *DocumentService) changeLink(ctx context.Context, user *model.User, productID, id, targetType, targetID string, link bool) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleOwner); err != nil {
		return err
	}
	kind, err := linkKind(targetType)
	if err != nil {
		return err
	}
	if targetID == "" {
		return fmt.Errorf("no %s to link", strings.ReplaceAll(targetType, "_", " "))
	}
	if err := s.owns.require(ctx, productID, ref{kindDocument, id}, ref{kind, targetID}); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return inTx(ctx, s.tx, func(ctx context.Context) error {
		if !link {
			if err := s.documents.Unlink(ctx, id, targetType, targetID); err != nil {
				return fmt.Errorf("unlink document: %w", err)
			}
			return s.record(ctx, user, id, "unlink", targetType, "", targetID, now)
		}
		// Linking twice is no error. Look first rather than reading the
		// unique index's complaint: a failed statement would abort the
		// transaction on Postgres.
		links, err := s.documents.ListLinks(ctx, id)
		if err != nil {
			return fmt.Errorf("link document: %w", err)
		}
		for _, l := range links {
			if l.TargetType == targetType && l.TargetID == targetID {
				return nil
			}
		}
		if err := s.documents.Link(ctx, &model.DocumentLink{
			ID: uuid.New().String(), DocumentID: id, TargetType: targetType, TargetID: targetID, CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("link document: %w", err)
		}
		return s.record(ctx, user, id, "link", targetType, "", targetID, now)
	})
}

// ListLinks returns a document's links. Requires viewer role.
func (s *DocumentService) ListLinks(ctx context.Context, user *model.User, productID, id string) ([]*model.DocumentLink, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindDocument, id}); err != nil {
		return nil, err
	}
	return s.documents.ListLinks(ctx, id)
}

// DocumentsFor returns the documents linked to a requirement, constraint or
// test case, without bodies (FR-909). Requires viewer role.
func (s *DocumentService) DocumentsFor(ctx context.Context, user *model.User, productID, targetType, targetID string) ([]*model.Document, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	kind, err := linkKind(targetType)
	if err != nil {
		return nil, err
	}
	if targetType == model.DocumentLinkRequirement {
		kind = kindBacklogItem // a task has no documents, but asking is no error
	}
	if err := s.owns.require(ctx, productID, ref{kind, targetID}); err != nil {
		return nil, err
	}
	return s.documents.ListByTarget(ctx, targetType, targetID)
}

// ResolveDocumentRef turns a document reference into its ID: the ID itself,
// returned unchanged, or an identifier such as "DOC-3" (FR-050) with the
// product's current document prefix. Requires viewer role.
func (s *DocumentService) ResolveDocumentRef(ctx context.Context, user *model.User, productID, ref string) (string, error) {
	prefix, seq, ok := parseRef(ref)
	if !ok {
		return ref, nil
	}
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return "", err
	}
	if prefix != s.documentPrefix(ctx, productID) {
		return "", fmt.Errorf("%w: no document %s", ErrNotFound, ref)
	}
	d, err := s.documents.GetBySequence(ctx, productID, seq)
	if err != nil {
		return "", fmt.Errorf("%w: no document %s", ErrNotFound, ref)
	}
	return d.ID, nil
}

// documentPrefix returns the product's document prefix, or the default if
// the product cannot be read.
func (s *DocumentService) documentPrefix(ctx context.Context, productID string) string {
	if s.products != nil {
		if p, err := s.products.GetByID(ctx, productID); err == nil && p.DocumentPrefix != "" {
			return p.DocumentPrefix
		}
	}
	return model.DefaultDocumentPrefix
}

// LinkedItem is a requirement, constraint or test case a document is linked
// to, named the way the item's own lists name it.
type LinkedItem struct {
	ID         string
	Identifier string // FR-12, SC-3; empty for test cases
	Title      string
	// TestIdentifier is the test in code an automatic test case runs.
	TestIdentifier string
}

// LinkedItems returns the items of one target type a document is linked to,
// ordered by identifier (requirements, constraints) or title (test cases).
// Requires viewer role.
func (s *DocumentService) LinkedItems(ctx context.Context, user *model.User, productID, id, targetType string) ([]LinkedItem, error) {
	links, err := s.ListLinks(ctx, user, productID, id)
	if err != nil {
		return nil, err
	}
	if _, err := linkKind(targetType); err != nil {
		return nil, err
	}
	var prefixes map[string]string
	switch targetType {
	case model.DocumentLinkRequirement:
		prefixes, err = categoryPrefixes(ctx, s.categories, productID, model.RequirementKindRequirement)
	case model.DocumentLinkConstraint:
		prefixes, err = categoryPrefixes(ctx, s.categories, productID, "constraint")
	}
	if err != nil {
		return nil, err
	}
	type sortable struct {
		item LinkedItem
		key  string
		seq  int
	}
	var out []sortable
	for _, l := range links {
		if l.TargetType != targetType {
			continue
		}
		switch targetType {
		case model.DocumentLinkRequirement:
			r, err := s.requirements.GetByID(ctx, l.TargetID)
			if err != nil {
				continue
			}
			out = append(out, sortable{LinkedItem{ID: r.ID, Identifier: model.EntityIdentifier(prefixes[r.CategoryID], r.SequenceNumber), Title: r.Title},
				prefixes[r.CategoryID], r.SequenceNumber})
		case model.DocumentLinkConstraint:
			c, err := s.constraints.GetByID(ctx, l.TargetID)
			if err != nil {
				continue
			}
			out = append(out, sortable{LinkedItem{ID: c.ID, Identifier: model.EntityIdentifier(prefixes[c.CategoryID], c.SequenceNumber), Title: c.Title},
				prefixes[c.CategoryID], c.SequenceNumber})
		case model.DocumentLinkTestCase:
			tc, err := s.testCases.GetByID(ctx, l.TargetID)
			if err != nil {
				continue
			}
			out = append(out, sortable{LinkedItem{ID: tc.ID, Title: tc.Title, TestIdentifier: tc.TestIdentifier}, strings.ToLower(tc.Title), 0})
		}
	}
	slices.SortStableFunc(out, func(a, b sortable) int {
		if c := strings.Compare(a.key, b.key); c != 0 {
			return c
		}
		return a.seq - b.seq
	})
	items := make([]LinkedItem, len(out))
	for i, o := range out {
		items[i] = o.item
	}
	return items, nil
}
