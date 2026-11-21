// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package client

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// Document is a long-form reference text of a product (FR-50).
type Document struct {
	ID             string `json:"id"`
	Identifier     string `json:"identifier"` // e.g. "DOC-3"
	ProductID      string `json:"product_id"`
	SequenceNumber int    `json:"sequence_number"`
	Title          string `json:"title"`
	// Body is empty in lists unless asked for (DocumentFilter.Body).
	Body           string `json:"body"`
	Format         string `json:"format"`
	State          string `json:"state"`
	SupersededByID string `json:"superseded_by_id"`
	ComponentID    string `json:"component_id"`
	CapabilityID   string `json:"capability_id"`
	MilestoneID    string `json:"milestone_id"`
	Revision       int    `json:"revision"`
	FileName       string `json:"file_name"` // e.g. "DOC-3.md" (FR-139)
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// DocumentRef names a document linked to a requirement, constraint or test
// case (FR-141). Without the documents scope only Identifier is set.
type DocumentRef struct {
	ID         string `json:"id,omitempty"`
	Identifier string `json:"identifier"`
	Title      string `json:"title,omitempty"`
	State      string `json:"state,omitempty"`
}

// DocumentRevision is one revision of a document (FR-57).
type DocumentRevision struct {
	Revision  int    `json:"revision"`
	Title     string `json:"title"`
	Format    string `json:"format"`
	Body      string `json:"body"` // empty in the revision list
	UserID    string `json:"user_id"`
	CreatedAt string `json:"created_at"`
}

// DocumentDiffLine is one line of a revision diff.
type DocumentDiffLine struct {
	Op   string `json:"op"` // equal, delete or insert
	Text string `json:"text"`
	Old  int    `json:"old"`
	New  int    `json:"new"`
}

// DocumentDiff compares two revisions of a document.
type DocumentDiff struct {
	From          DocumentRevision   `json:"from"`
	To            DocumentRevision   `json:"to"`
	TitleChanged  bool               `json:"title_changed"`
	FormatChanged bool               `json:"format_changed"`
	Unified       string             `json:"unified"`
	Lines         []DocumentDiffLine `json:"lines"`
}

// LinkedItem is a requirement, constraint or test case a document is linked
// to.
type LinkedItem struct {
	ID             string `json:"id"`
	Identifier     string `json:"identifier"`
	Title          string `json:"title"`
	TestIdentifier string `json:"test_identifier,omitempty"`
}

// DocumentFilter narrows a document list.
type DocumentFilter struct {
	State, Format, Search string
	Body                  bool // include the bodies
}

// Documents lists the product's documents.
func (c *Client) Documents(ctx context.Context, productID string, f DocumentFilter) ([]Document, error) {
	q := url.Values{"state": {f.State}, "format": {f.Format}, "q": {f.Search}}
	if f.Body {
		q.Set("body", "true")
	}
	return getList[Document](ctx, c, withQuery(productPath(productID, "documents"), q))
}

// Document returns a document with its body, by ID or identifier ("DOC-3").
func (c *Client) Document(ctx context.Context, productID, ref string) (*Document, error) {
	return getOne[Document](ctx, c, productPath(productID, "documents", ref))
}

// DocumentRevisions lists a document's revisions, newest first.
func (c *Client) DocumentRevisions(ctx context.Context, productID, ref string) ([]DocumentRevision, error) {
	return getList[DocumentRevision](ctx, c, productPath(productID, "documents", ref, "revisions"))
}

// DocumentRevision returns one revision with its body.
func (c *Client) DocumentRevision(ctx context.Context, productID, ref string, revision int) (*DocumentRevision, error) {
	return getOne[DocumentRevision](ctx, c, productPath(productID, "documents", ref, "revisions", strconv.Itoa(revision)))
}

// DocumentDiff compares two revisions; zero values select the latest
// revision and the one before it.
func (c *Client) DocumentDiff(ctx context.Context, productID, ref string, from, to int) (*DocumentDiff, error) {
	q := url.Values{}
	if from > 0 {
		q.Set("from", strconv.Itoa(from))
	}
	if to > 0 {
		q.Set("to", strconv.Itoa(to))
	}
	return getOne[DocumentDiff](ctx, c, withQuery(productPath(productID, "documents", ref, "diff"), q))
}

// documentLinkSegment is the path segment of a link target type.
func documentLinkSegment(targetType string) string {
	switch targetType {
	case "constraint":
		return "constraints"
	case "test_case":
		return "test-cases"
	}
	return "requirements"
}

// DocumentLinks lists the items of one type ("requirement", "constraint",
// "test_case") a document is linked to.
func (c *Client) DocumentLinks(ctx context.Context, productID, ref, targetType string) ([]LinkedItem, error) {
	return getList[LinkedItem](ctx, c, productPath(productID, "documents", ref, documentLinkSegment(targetType)))
}

// DocumentCreate is the payload of CreateDocument.
type DocumentCreate struct {
	Title          string `json:"title"`
	Body           string `json:"body,omitempty"`
	Format         string `json:"format,omitempty"`
	State          string `json:"state,omitempty"`
	SupersededByID string `json:"superseded_by_id,omitempty"`
	ComponentID    string `json:"component_id,omitempty"`
	CapabilityID   string `json:"capability_id,omitempty"`
	MilestoneID    string `json:"milestone_id,omitempty"`
}

// CreateDocument creates a document (owner role, FR-108).
func (c *Client) CreateDocument(ctx context.Context, productID string, in DocumentCreate) (*Document, error) {
	return send[Document](ctx, c, http.MethodPost, productPath(productID, "documents"), in)
}

// DocumentPatch is a partial update of a document; an empty string clears
// an optional reference.
type DocumentPatch struct {
	Title          *string `json:"title,omitempty"`
	Body           *string `json:"body,omitempty"`
	Format         *string `json:"format,omitempty"`
	State          *string `json:"state,omitempty"`
	SupersededByID *string `json:"superseded_by_id,omitempty"`
	ComponentID    *string `json:"component_id,omitempty"`
	CapabilityID   *string `json:"capability_id,omitempty"`
	MilestoneID    *string `json:"milestone_id,omitempty"`
}

// UpdateDocument patches a document by identifier or ID.
func (c *Client) UpdateDocument(ctx context.Context, productID, ref string, p DocumentPatch) (*Document, error) {
	return send[Document](ctx, c, http.MethodPatch, productPath(productID, "documents", ref), p)
}

// DeleteDocument deletes a document with its revisions.
func (c *Client) DeleteDocument(ctx context.Context, productID, ref string) error {
	return c.Do(ctx, http.MethodDelete, productPath(productID, "documents", ref), nil, nil)
}

// RestoreDocument makes an earlier revision's content current again.
func (c *Client) RestoreDocument(ctx context.Context, productID, ref string, revision int) (*Document, error) {
	return send[Document](ctx, c, http.MethodPost, productPath(productID, "documents", ref, "restore"),
		map[string]int{"revision": revision})
}

// LinkDocument links a document to a requirement, constraint or test case;
// target is an ID or, for requirements and constraints, an identifier.
func (c *Client) LinkDocument(ctx context.Context, productID, ref, targetType, target string) error {
	return c.Do(ctx, http.MethodPost, productPath(productID, "documents", ref, documentLinkSegment(targetType)),
		map[string]string{"target_id": target}, nil)
}

// UnlinkDocument removes such a link.
func (c *Client) UnlinkDocument(ctx context.Context, productID, ref, targetType, target string) error {
	return c.Do(ctx, http.MethodDelete, productPath(productID, "documents", ref, documentLinkSegment(targetType), target), nil, nil)
}
