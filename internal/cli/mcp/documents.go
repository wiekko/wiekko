package mcp

import (
	"context"
	"io"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"nexmap.net/internal/cli/ops"
	"nexmap.net/internal/cli/output"
	"nexmap.net/internal/cli/render"
)

// registerDocumentReadTools offers the documents that define the product
// (FR-50, FR-140): protocols, schemas and other reference texts an agent
// implements against or validates code with.
func registerDocumentReadTools(s *sdk.Server, h *handlers) {
	readTool(s, h, "list_documents", "List documents",
		"List the product's documents: protocols, schemas, interface contracts and other texts that define the product. "+
			"Only current documents are binding; drafts are work in progress, superseded ones are history.",
		func(ctx context.Context, sess *ops.Session, q ops.DocumentQuery) (Items[ops.DocumentSummary], func(io.Writer) error, error) {
			items, err := sess.ListDocuments(ctx, q)
			return Items[ops.DocumentSummary]{items}, func(w io.Writer) error { return render.Documents(w, output.Markdown, items) }, err
		})
	readTool(s, h, "get_document", "Show a document",
		"One document by identifier (e.g. DOC-3) or ID, with its full content (Markdown, or the schema or protocol text "+
			"itself for other formats), the requirements, constraints and test cases it is linked to, and comments. Read the "+
			"documents a requirement names before implementing it, and treat a current document as the definition to follow.",
		func(ctx context.Context, sess *ops.Session, in RefInput) (*ops.DocumentDetail, func(io.Writer) error, error) {
			if err := required(in.Ref); err != nil {
				return nil, nil, err
			}
			v, err := sess.ShowDocument(ctx, in.Ref)
			return v, func(w io.Writer) error { return render.Document(w, v) }, err
		})
	readTool(s, h, "document_revisions", "List document revisions",
		"The revisions of a document, newest first. Every change to its title, content or format is a revision.",
		func(ctx context.Context, sess *ops.Session, in RefInput) (Items[ops.RevisionView], func(io.Writer) error, error) {
			if err := required(in.Ref); err != nil {
				return Items[ops.RevisionView]{}, nil, err
			}
			items, err := sess.DocumentRevisions(ctx, in.Ref)
			return Items[ops.RevisionView]{items}, func(w io.Writer) error { return render.Revisions(w, output.Markdown, items) }, err
		})
	readTool(s, h, "document_diff", "Compare document revisions",
		"Compare two revisions of a document as a unified diff; by default the latest with the one before it. "+
			"Use it to see what changed in a definition since the code was written.",
		func(ctx context.Context, sess *ops.Session, q ops.DiffQuery) (*ops.DocumentDiffView, func(io.Writer) error, error) {
			if err := required(q.Ref); err != nil {
				return nil, nil, err
			}
			v, err := sess.DocumentDiff(ctx, q)
			return v, func(w io.Writer) error { return render.Diff(w, v) }, err
		})
}

// registerDocumentWriteTools offers document writes (FR-136): no deletion.
func registerDocumentWriteTools(s *sdk.Server, h *handlers) {
	document := func(v *ops.DocumentDetail) func(io.Writer) error {
		return func(w io.Writer) error { return render.Document(w, v) }
	}
	writeTool(s, h, "create_document", "Create a document",
		"Create a document that defines part of the product, such as a protocol (markdown) or a schema (json, yaml, "+
			"proto, ...). The content is stored byte for byte. New documents are drafts unless state is current. "+
			"Requires the product owner role. Returns the document.",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.NewDocument) (*ops.DocumentDetail, func(io.Writer) error, error) {
			v, err := sess.CreateDocument(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return v, document(v), nil
		})
	writeTool(s, h, "update_document", "Update a document",
		"Change a document; omitted fields stay. New content replaces the whole body and becomes a new revision, so "+
			"nothing is lost. Set state current to make it binding, or superseded with superseded_by naming its "+
			"replacement. Requires the product owner role.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.DocumentChange) (*ops.DocumentDetail, func(io.Writer) error, error) {
			v, err := sess.UpdateDocument(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return v, document(v), nil
		})
	writeTool(s, h, "link_document", "Link a document",
		"Link a document to the requirements (FR-12) that introduce or change it, the constraints (SC-3) that require "+
			"conformance to it, and the test cases that verify it. Tasks cannot be linked. Requires the product owner role.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.DocumentLinks) (*ops.DocumentDetail, func(io.Writer) error, error) {
			v, err := sess.LinkDocument(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return v, document(v), nil
		})
	writeTool(s, h, "unlink_document", "Unlink a document",
		"Remove links between a document and requirements, constraints or test cases. Requires the product owner role.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.DocumentLinks) (*ops.DocumentDetail, func(io.Writer) error, error) {
			in.Unlink = true
			v, err := sess.LinkDocument(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return v, document(v), nil
		})
	writeTool(s, h, "restore_document", "Restore a document revision",
		"Make the content of an earlier revision current again. It becomes a new revision; history is never rewritten. "+
			"Requires the product owner role.",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.RestoreRevision) (*ops.DocumentDetail, func(io.Writer) error, error) {
			v, err := sess.RestoreDocument(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return v, document(v), nil
		})
}
