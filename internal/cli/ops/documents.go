// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package ops

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"wiekko.net/internal/client"
)

// Documents (FR-50..FR-58, FR-140): long-form texts that define part of
// the product, such as protocols and schemas. Wiekko is their source; files
// in a repository are written from it (FR-51, FR-139).

// DocumentSummary is one row of a document list.
type DocumentSummary struct {
	ID         string `json:"id"`
	Identifier string `json:"identifier"` // e.g. DOC-3
	Title      string `json:"title"`
	Format     string `json:"format"` // markdown, json, yaml, proto, ...
	State      string `json:"state"`  // draft, current, superseded
	Revision   int    `json:"revision"`
	File       string `json:"file"` // the file name it exports as, e.g. DOC-3.md
	UpdatedAt  string `json:"updated_at"`
}

// DocumentDetail is a document with its body, placement and links.
type DocumentDetail struct {
	DocumentSummary
	Body         string        `json:"body"`
	SupersededBy string        `json:"superseded_by,omitempty"` // identifier of the replacing document
	Component    string        `json:"component,omitempty"`
	Capability   string        `json:"capability,omitempty"`
	Milestone    string        `json:"milestone,omitempty"`
	Requirements []Ref         `json:"requirements"`
	Constraints  []Ref         `json:"constraints"`
	TestCases    []Ref         `json:"test_cases"`
	Comments     []CommentView `json:"comments"`
}

// RevisionView is one revision of a document.
type RevisionView struct {
	Revision  int    `json:"revision"`
	Title     string `json:"title"`
	Format    string `json:"format"`
	Author    string `json:"author"`
	CreatedAt string `json:"created_at"`
}

// DocumentDiffView compares two revisions of a document.
type DocumentDiffView struct {
	Document      string `json:"document"` // identifier
	From          int    `json:"from"`
	To            int    `json:"to"`
	TitleChanged  bool   `json:"title_changed"`
	FormatChanged bool   `json:"format_changed"`
	// Unified is the body diff in unified format; empty if the bodies match.
	Unified string `json:"unified"`
}

func documentSummary(d client.Document) DocumentSummary {
	return DocumentSummary{ID: d.ID, Identifier: d.Identifier, Title: d.Title, Format: d.Format,
		State: d.State, Revision: d.Revision, File: d.FileName, UpdatedAt: d.UpdatedAt}
}

// documentRefs turns the documents named on an item into refs (FR-141).
func documentRefs(list []client.DocumentRef) []Ref {
	out := make([]Ref, 0, len(list))
	for _, d := range list {
		out = append(out, Ref{ID: d.ID, Identifier: d.Identifier, Title: d.Title})
	}
	return out
}

// DocumentQuery filters a document list.
type DocumentQuery struct {
	State  string `json:"state,omitempty" jsonschema:"only documents in this state: draft, current or superseded"`
	Format string `json:"format,omitempty" jsonschema:"only documents of this format, e.g. markdown, json, yaml, proto"`
	Search string `json:"search,omitempty" jsonschema:"substring of the title or identifier"`
}

// ListDocuments lists the product's documents, ordered by identifier.
func (s *Session) ListDocuments(ctx context.Context, q DocumentQuery) ([]DocumentSummary, error) {
	list, err := s.C.Documents(ctx, s.ProductID, client.DocumentFilter{State: q.State, Format: q.Format, Search: q.Search})
	if err != nil {
		return nil, err
	}
	out := make([]DocumentSummary, 0, len(list))
	for _, d := range list {
		out = append(out, documentSummary(d))
	}
	return out, nil
}

// document reads one document, reporting a missing one as NotFoundError.
func (s *Session) document(ctx context.Context, ref string) (*client.Document, error) {
	d, err := s.C.Document(ctx, s.ProductID, strings.TrimSpace(ref))
	if err != nil {
		if client.IsNotFound(err) {
			return nil, &NotFoundError{Kind: "document", Ref: ref}
		}
		return nil, err
	}
	return d, nil
}

// ShowDocument returns a document by identifier ("DOC-3") or ID, with its
// body and links. Links the token may not read are left out.
func (s *Session) ShowDocument(ctx context.Context, ref string) (*DocumentDetail, error) {
	d, err := s.document(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := &DocumentDetail{DocumentSummary: documentSummary(*d), Body: d.Body,
		Requirements: []Ref{}, Constraints: []Ref{}, TestCases: []Ref{}, Comments: []CommentView{}}
	if d.SupersededByID != "" {
		if next, err := s.C.Document(ctx, s.ProductID, d.SupersededByID); err == nil {
			out.SupersededBy = next.Identifier
		}
	}
	if d.ComponentID != "" {
		if m, err := s.componentNames(ctx); err == nil {
			out.Component = m[d.ComponentID]
		}
	}
	if d.CapabilityID != "" {
		if m, err := s.capabilityNames(ctx); err == nil {
			out.Capability = m[d.CapabilityID]
		}
	}
	if d.MilestoneID != "" {
		if m, err := s.milestoneNames(ctx); err == nil {
			out.Milestone = m[d.MilestoneID]
		}
	}
	for _, l := range []struct {
		typ  string
		into *[]Ref
	}{{"requirement", &out.Requirements}, {"constraint", &out.Constraints}, {"test_case", &out.TestCases}} {
		items, err := s.C.DocumentLinks(ctx, s.ProductID, d.ID, l.typ)
		if err != nil {
			if client.IsForbidden(err) {
				continue // the token lacks the linked items' scope
			}
			return nil, err
		}
		for _, it := range items {
			identifier := it.Identifier
			if identifier == "" {
				identifier = it.TestIdentifier
			}
			*l.into = append(*l.into, Ref{ID: it.ID, Identifier: identifier, Title: it.Title})
		}
	}
	if comments, err := s.comments(ctx, "document", d.ID); err == nil {
		out.Comments = comments
	} else if !client.IsForbidden(err) {
		return nil, err
	}
	return out, nil
}

// placement resolves component, capability and milestone names to IDs; nil
// stays nil, and an empty string clears.
func (s *Session) placement(ctx context.Context, component, capability, milestone *string) (comp, capa, ms *string, err error) {
	resolve := func(ref *string, find func(string) (string, error)) (*string, error) {
		if ref == nil {
			return nil, nil
		}
		v := strings.TrimSpace(*ref)
		if v == "" {
			return &v, nil
		}
		id, err := find(v)
		if err != nil {
			return nil, err
		}
		return &id, nil
	}
	if comp, err = resolve(component, func(r string) (string, error) {
		c, err := s.findComponent(ctx, r)
		return c.ID, err
	}); err != nil {
		return
	}
	if capa, err = resolve(capability, func(r string) (string, error) {
		c, err := s.findCapability(ctx, r)
		return c.ID, err
	}); err != nil {
		return
	}
	ms, err = resolve(milestone, func(r string) (string, error) {
		list, err := s.milestones(ctx)
		if err != nil {
			return "", err
		}
		m, err := match("milestone", r, list,
			func(m client.Milestone) string { return m.ID },
			func(m client.Milestone) string { return m.Name },
			func(m client.Milestone) bool { return strings.EqualFold(m.Name, r) })
		return m.ID, err
	})
	return
}

// NewDocument creates a document.
type NewDocument struct {
	Title        string `json:"title" jsonschema:"document title"`
	Body         string `json:"body,omitempty" jsonschema:"the content, stored byte for byte: Markdown for the markdown format, the schema or protocol text itself otherwise"`
	Format       string `json:"format,omitempty" jsonschema:"markdown (default), text, json, yaml, xml, proto, sql or graphql"`
	State        string `json:"state,omitempty" jsonschema:"draft (default), current or superseded; only current documents are binding"`
	Component    string `json:"component,omitempty" jsonschema:"component the document belongs to (name or ID)"`
	Capability   string `json:"capability,omitempty" jsonschema:"capability the document belongs to (name or ID); not together with component"`
	Milestone    string `json:"milestone,omitempty" jsonschema:"milestone (name or ID)"`
	SupersededBy string `json:"superseded_by,omitempty" jsonschema:"for a superseded document: the document replacing it, e.g. DOC-4"`
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// CreateDocument creates a document (owner role, FR-108).
func (s *Session) CreateDocument(ctx context.Context, in NewDocument) (*DocumentDetail, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return nil, errors.New("a title is required")
	}
	comp, capa, ms, err := s.placement(ctx, optional(in.Component), optional(in.Capability), optional(in.Milestone))
	if err != nil {
		return nil, err
	}
	payload := client.DocumentCreate{Title: title, Body: in.Body, Format: in.Format, State: in.State,
		SupersededByID: strings.TrimSpace(in.SupersededBy)}
	if comp != nil {
		payload.ComponentID = *comp
	}
	if capa != nil {
		payload.CapabilityID = *capa
	}
	if ms != nil {
		payload.MilestoneID = *ms
	}
	d, err := s.C.CreateDocument(ctx, s.ProductID, payload)
	if err != nil {
		return nil, err
	}
	return s.ShowDocument(ctx, d.ID)
}

// DocumentChange updates a document; omitted fields stay as they are. A
// change to title, body or format creates a new revision (FR-57).
type DocumentChange struct {
	Ref          string  `json:"ref" jsonschema:"the document, e.g. DOC-3"`
	Title        *string `json:"title,omitempty" jsonschema:"new title"`
	Body         *string `json:"body,omitempty" jsonschema:"new content, replacing the whole body"`
	Format       *string `json:"format,omitempty" jsonschema:"new format"`
	State        *string `json:"state,omitempty" jsonschema:"new state: draft, current or superseded"`
	Component    *string `json:"component,omitempty" jsonschema:"component (name or ID); empty clears it"`
	Capability   *string `json:"capability,omitempty" jsonschema:"capability (name or ID); empty clears it"`
	Milestone    *string `json:"milestone,omitempty" jsonschema:"milestone (name or ID); empty clears it"`
	SupersededBy *string `json:"superseded_by,omitempty" jsonschema:"the replacing document, e.g. DOC-4; empty clears it"`
}

// UpdateDocument changes a document (owner role, FR-108).
func (s *Session) UpdateDocument(ctx context.Context, in DocumentChange) (*DocumentDetail, error) {
	d, err := s.document(ctx, in.Ref)
	if err != nil {
		return nil, err
	}
	comp, capa, ms, err := s.placement(ctx, in.Component, in.Capability, in.Milestone)
	if err != nil {
		return nil, err
	}
	p := client.DocumentPatch{Title: in.Title, Body: in.Body, Format: in.Format, State: in.State,
		ComponentID: comp, CapabilityID: capa, MilestoneID: ms}
	if in.SupersededBy != nil {
		v := strings.TrimSpace(*in.SupersededBy)
		p.SupersededByID = &v
	}
	if _, err := s.C.UpdateDocument(ctx, s.ProductID, d.ID, p); err != nil {
		return nil, err
	}
	return s.ShowDocument(ctx, d.ID)
}

// DeleteDocument deletes a document with its revisions and links.
func (s *Session) DeleteDocument(ctx context.Context, ref string) (*DocumentSummary, error) {
	d, err := s.document(ctx, ref)
	if err != nil {
		return nil, err
	}
	if err := s.C.DeleteDocument(ctx, s.ProductID, d.ID); err != nil {
		return nil, err
	}
	sum := documentSummary(*d)
	return &sum, nil
}

// DocumentLinks links a document to requirements, constraints and test
// cases, or removes those links.
type DocumentLinks struct {
	Document string   `json:"document" jsonschema:"the document, e.g. DOC-3"`
	Items    []string `json:"items" jsonschema:"requirements and constraints by identifier (FR-12, SC-3), test cases by test identifier, short ID or ID"`
	Unlink   bool     `json:"-"`
}

// LinkDocument links (or unlinks) a document (owner role, FR-108).
func (s *Session) LinkDocument(ctx context.Context, in DocumentLinks) (*DocumentDetail, error) {
	if len(in.Items) == 0 {
		return nil, errors.New("name at least one requirement, constraint or test case, e.g. FR-12")
	}
	d, err := s.document(ctx, in.Document)
	if err != nil {
		return nil, err
	}
	type target struct{ typ, id string }
	targets := make([]target, 0, len(in.Items))
	for _, ref := range in.Items {
		ref = strings.TrimSpace(ref)
		if identifierPattern.MatchString(ref) {
			r, err := s.C.Requirement(ctx, s.ProductID, ref)
			if err == nil {
				if r.Kind == "task" {
					return nil, fmt.Errorf("%s is a task; documents link to requirements, constraints and test cases", r.Identifier)
				}
				targets = append(targets, target{"requirement", r.ID})
				continue
			}
			if !client.IsNotFound(err) {
				return nil, err
			}
			c, err := s.C.Constraint(ctx, s.ProductID, ref)
			if err == nil {
				targets = append(targets, target{"constraint", c.ID})
				continue
			}
			if !client.IsNotFound(err) {
				return nil, err
			}
		}
		tc, err := s.findTestCase(ctx, ref)
		if err != nil {
			var nf *NotFoundError
			if errors.As(err, &nf) {
				return nil, &NotFoundError{Kind: "requirement, constraint or test case", Ref: ref}
			}
			return nil, err
		}
		targets = append(targets, target{"test_case", tc.ID})
	}
	for _, t := range targets {
		if in.Unlink {
			err = s.C.UnlinkDocument(ctx, s.ProductID, d.ID, t.typ, t.id)
		} else {
			err = s.C.LinkDocument(ctx, s.ProductID, d.ID, t.typ, t.id)
		}
		if err != nil {
			return nil, err
		}
	}
	return s.ShowDocument(ctx, d.ID)
}

// DocumentRevisions lists a document's revisions, newest first.
func (s *Session) DocumentRevisions(ctx context.Context, ref string) ([]RevisionView, error) {
	d, err := s.document(ctx, ref)
	if err != nil {
		return nil, err
	}
	revs, err := s.C.DocumentRevisions(ctx, s.ProductID, d.ID)
	if err != nil {
		return nil, err
	}
	users, err := s.usernames(ctx)
	if err != nil {
		users = map[string]string{}
	}
	out := make([]RevisionView, 0, len(revs))
	for _, r := range revs {
		out = append(out, RevisionView{Revision: r.Revision, Title: r.Title, Format: r.Format,
			Author: user(users, r.UserID), CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// DiffQuery selects two revisions to compare.
type DiffQuery struct {
	Ref  string `json:"ref" jsonschema:"the document, e.g. DOC-3"`
	From int    `json:"from,omitempty" jsonschema:"older revision; default the one before 'to'"`
	To   int    `json:"to,omitempty" jsonschema:"newer revision; default the latest"`
}

// DocumentDiff compares two revisions of a document.
func (s *Session) DocumentDiff(ctx context.Context, q DiffQuery) (*DocumentDiffView, error) {
	d, err := s.document(ctx, q.Ref)
	if err != nil {
		return nil, err
	}
	diff, err := s.C.DocumentDiff(ctx, s.ProductID, d.ID, q.From, q.To)
	if err != nil {
		if client.IsNotFound(err) {
			return nil, &NotFoundError{Kind: "revision", Ref: fmt.Sprintf("%d or %d of %s", q.From, q.To, d.Identifier)}
		}
		return nil, err
	}
	return &DocumentDiffView{Document: d.Identifier, From: diff.From.Revision, To: diff.To.Revision,
		TitleChanged: diff.TitleChanged, FormatChanged: diff.FormatChanged, Unified: diff.Unified}, nil
}

// RestoreRevision makes an earlier revision current again.
type RestoreRevision struct {
	Ref      string `json:"ref" jsonschema:"the document, e.g. DOC-3"`
	Revision int    `json:"revision" jsonschema:"the revision whose content becomes current again, as a new revision"`
}

// RestoreDocument restores a revision (owner role, FR-57).
func (s *Session) RestoreDocument(ctx context.Context, in RestoreRevision) (*DocumentDetail, error) {
	if in.Revision < 1 {
		return nil, errors.New("name the revision to restore")
	}
	d, err := s.document(ctx, in.Ref)
	if err != nil {
		return nil, err
	}
	if _, err := s.C.RestoreDocument(ctx, s.ProductID, d.ID, in.Revision); err != nil {
		if client.IsNotFound(err) {
			return nil, &NotFoundError{Kind: "revision", Ref: fmt.Sprintf("%d of %s", in.Revision, d.Identifier)}
		}
		return nil, err
	}
	return s.ShowDocument(ctx, d.ID)
}

// documentFilePattern is what an exported document's file name must look
// like: an identifier and a known extension. Names are built from server
// data, so they are checked before anything is written.
var documentFilePattern = regexp.MustCompile(`^[A-Z][A-Z0-9]{0,5}-[1-9][0-9]{0,9}\.(md|txt|json|yaml|xml|proto|sql|graphql)$`)

// DocumentFile is a document's body as a file.
type DocumentFile struct {
	Name string // e.g. DOC-3.md
	Body string
}

// DocumentFileOf returns a document's body, or the body of one of its
// revisions, with the file name it exports as.
func (s *Session) DocumentFileOf(ctx context.Context, ref string, revision int) (*DocumentFile, error) {
	d, err := s.document(ctx, ref)
	if err != nil {
		return nil, err
	}
	body, format := d.Body, d.Format
	if revision > 0 {
		r, err := s.C.DocumentRevision(ctx, s.ProductID, d.ID, revision)
		if err != nil {
			if client.IsNotFound(err) {
				return nil, &NotFoundError{Kind: "revision", Ref: fmt.Sprintf("%d of %s", revision, d.Identifier)}
			}
			return nil, err
		}
		body, format = r.Body, r.Format
	}
	name := d.Identifier + extensionOf(format)
	if !documentFilePattern.MatchString(name) {
		return nil, fmt.Errorf("refusing to write a document as %q", name)
	}
	return &DocumentFile{Name: name, Body: body}, nil
}

// ExportedDocumentFiles returns the files of the documents a specification
// export lists: the current documents, and the drafts if asked for. A token
// without the documents scope gets none, like the export itself.
func (s *Session) ExportedDocumentFiles(ctx context.Context, drafts bool) ([]DocumentFile, error) {
	list, err := s.C.Documents(ctx, s.ProductID, client.DocumentFilter{Body: true})
	if err != nil {
		if client.IsForbidden(err) {
			return nil, nil
		}
		return nil, err
	}
	var files []DocumentFile
	for _, d := range list {
		if d.State != "current" && !(drafts && d.State == "draft") {
			continue
		}
		name := d.Identifier + extensionOf(d.Format)
		if !documentFilePattern.MatchString(name) {
			return nil, fmt.Errorf("refusing to write a document as %q", name)
		}
		files = append(files, DocumentFile{Name: name, Body: d.Body})
	}
	return files, nil
}

// extensionOf mirrors the server's format → extension mapping (FR-139).
func extensionOf(format string) string {
	switch format {
	case "markdown":
		return ".md"
	case "json", "yaml", "xml", "proto", "sql", "graphql":
		return "." + format
	}
	return ".txt"
}
