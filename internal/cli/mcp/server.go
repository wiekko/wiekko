// Package mcp serves the agent operations of internal/cli/ops as Model Context
// Protocol tools, so MCP-capable agents get typed tools instead of shelling
// out to the CLI.
//
// It is a thin adapter: every tool calls one ops operation and returns its
// view twice — rendered as markdown (the `-o md` output) for the model to read,
// and as structured content with the same fields as the CLI's `-o json`. The
// MCP SDK is confined to this package.
package mcp

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync/atomic"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"nexmap.net/internal/buildinfo"
	"nexmap.net/internal/cli/ops"
	"nexmap.net/internal/cli/output"
	"nexmap.net/internal/cli/render"
	"nexmap.net/internal/client"
)

// Options configure a server.
type Options struct {
	// NewSession returns a fresh ops session for one tool call. A session
	// caches lookups (statuses, categories, …); creating one per call keeps a
	// long-running server from serving stale names.
	NewSession func() (*ops.Session, error)
	// ReadOnly leaves out every tool that changes data. The server still
	// enforces the token user's role on each write; this is the extra guard
	// for agents that should only read.
	ReadOnly bool
}

// instructions are sent to the client on initialization: how to use the tools.
//
//go:embed instructions.md
var instructions string

// New builds the MCP server with all tools registered.
func New(opts Options) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{
		Name:    "nexmap",
		Title:   "Nexmap",
		Version: buildinfo.String(),
	}, &sdk.ServerOptions{
		Instructions: instructions,
		// No logging capability: the server has nothing to log to the client.
		Capabilities: &sdk.ServerCapabilities{},
	})
	h := &handlers{newSession: opts.NewSession}
	registerReadTools(s, h)
	if !opts.ReadOnly {
		registerWriteTools(s, h)
	}
	return s
}

// Serve runs the server over newline-delimited JSON on r/w (stdin/stdout for
// `nexmap mcp`) until the client disconnects or ctx is cancelled.
func Serve(ctx context.Context, opts Options, r io.Reader, w io.Writer) error {
	in := &eofReader{r: r}
	err := New(opts).Run(ctx, &sdk.IOTransport{Reader: io.NopCloser(in), Writer: nopWriteCloser{w}})
	// The client closing stdin is the normal way to end a stdio server; the
	// SDK reports it as an error that does not always wrap io.EOF.
	if in.eof.Load() || errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// eofReader records that the input reached EOF.
type eofReader struct {
	r   io.Reader
	eof atomic.Bool
}

func (e *eofReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
		e.eof.Store(true)
	}
	return n, err
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

type handlers struct {
	newSession func() (*ops.Session, error)
}

// Items wraps list results: structured tool output must be a JSON object.
type Items[T any] struct {
	Items []T `json:"items"`
}

// RefInput selects one entity.
type RefInput struct {
	Ref string `json:"ref" jsonschema:"identifier (FR-12, AC-3, DF-17), short ID, full ID, or name where the tool says so"`
}

// CampaignInput optionally selects a campaign.
type CampaignInput struct {
	Ref string `json:"ref,omitempty" jsonschema:"campaign name or ID; omit for the active campaign"`
}

// readTool registers a read-only tool. run produces the view and its markdown
// rendering; errors are returned as tool errors the model can read.
func readTool[In, Out any](s *sdk.Server, h *handlers, name, title, desc string,
	run func(ctx context.Context, sess *ops.Session, in In) (Out, func(io.Writer) error, error)) {
	addTool(s, h, name, title, desc, &sdk.ToolAnnotations{Title: title, ReadOnlyHint: true, IdempotentHint: true}, run)
}

// writeTool registers a tool that changes data. None of them delete
// anything; idempotent marks tools where repeating a call changes nothing
// further (setting a status), as opposed to creating another item.
func writeTool[In, Out any](s *sdk.Server, h *handlers, name, title, desc string, idempotent bool,
	run func(ctx context.Context, sess *ops.Session, in In) (Out, func(io.Writer) error, error)) {
	f := false
	addTool(s, h, name, title, desc, &sdk.ToolAnnotations{Title: title, DestructiveHint: &f, IdempotentHint: idempotent}, run)
}

func addTool[In, Out any](s *sdk.Server, h *handlers, name, title, desc string, ann *sdk.ToolAnnotations,
	run func(ctx context.Context, sess *ops.Session, in In) (Out, func(io.Writer) error, error)) {
	t := true
	ann.OpenWorldHint = &t // talks to a remote server
	sdk.AddTool(s, &sdk.Tool{Name: name, Title: title, Description: desc, Annotations: ann},
		func(ctx context.Context, _ *sdk.CallToolRequest, in In) (*sdk.CallToolResult, Out, error) {
			var zero Out
			sess, err := h.newSession()
			if err != nil {
				return nil, zero, toolError(err)
			}
			out, md, err := run(ctx, sess, in)
			if err != nil {
				return nil, zero, toolError(err)
			}
			var b bytes.Buffer
			if err := md(&b); err != nil {
				return nil, zero, err
			}
			return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: b.String()}}}, out, nil
		})
}

func required(ref string) error {
	if strings.TrimSpace(ref) == "" {
		return errors.New(`"ref" is required`)
	}
	return nil
}

func registerReadTools(s *sdk.Server, h *handlers) {
	readTool(s, h, "get_context", "Product briefing",
		"One-shot briefing on the product: purpose, components and capabilities, constraints with compliance, "+
			"current sprint and active work, open defects by criticality, and the active testing campaign. "+
			"Call this once at the start of a task.",
		func(ctx context.Context, sess *ops.Session, _ struct{}) (*ops.ContextView, func(io.Writer) error, error) {
			v, err := sess.Context(ctx)
			return v, func(w io.Writer) error { return render.Context(w, v) }, err
		})

	readTool(s, h, "list_requirements", "List requirements and tasks",
		"List requirements and tasks with identifier, status and flags. Filters combine; all are optional.",
		func(ctx context.Context, sess *ops.Session, q ops.RequirementQuery) (Items[ops.RequirementSummary], func(io.Writer) error, error) {
			items, err := sess.ListRequirements(ctx, q)
			return Items[ops.RequirementSummary]{items}, func(w io.Writer) error { return render.Requirements(w, output.Markdown, items) }, err
		})

	readTool(s, h, "get_requirement", "Show a requirement or task",
		"One requirement or task by identifier (e.g. FR-12) or ID: description, definition of done, parent and children, "+
			"linked capabilities, covering test cases, linked defects and comments. Read it before implementing the item.",
		func(ctx context.Context, sess *ops.Session, in RefInput) (*ops.RequirementDetail, func(io.Writer) error, error) {
			if err := required(in.Ref); err != nil {
				return nil, nil, err
			}
			v, err := sess.ShowRequirement(ctx, in.Ref)
			return v, func(w io.Writer) error { return render.Requirement(w, v) }, err
		})

	readTool(s, h, "list_constraints", "List constraints",
		"List constraints (architecture, security, UX rules the product must obey) with compliance state.",
		func(ctx context.Context, sess *ops.Session, q ops.ConstraintQuery) (Items[ops.ConstraintSummary], func(io.Writer) error, error) {
			items, err := sess.ListConstraints(ctx, q)
			return Items[ops.ConstraintSummary]{items}, func(w io.Writer) error { return render.Constraints(w, output.Markdown, items) }, err
		})

	readTool(s, h, "get_constraint", "Show a constraint",
		"One constraint by identifier (e.g. AC-42) or ID: description, compliance, linked capabilities and comments.",
		func(ctx context.Context, sess *ops.Session, in RefInput) (*ops.ConstraintDetail, func(io.Writer) error, error) {
			if err := required(in.Ref); err != nil {
				return nil, nil, err
			}
			v, err := sess.ShowConstraint(ctx, in.Ref)
			return v, func(w io.Writer) error { return render.Constraint(w, v) }, err
		})

	readTool(s, h, "get_structure", "Product structure",
		"The component → capability tree of the product.",
		func(ctx context.Context, sess *ops.Session, _ struct{}) (*ops.Structure, func(io.Writer) error, error) {
			v, err := sess.Structure(ctx)
			return v, func(w io.Writer) error { return render.Structure(w, output.Markdown, v) }, err
		})

	readTool(s, h, "export_specification", "Export the specification",
		"The whole product specification as one Markdown document: components and capabilities, constraints and "+
			"requirements grouped by category with identifiers, descriptions, definitions of done and capability links; "+
			"tasks too if asked for. The order is stable and the document has no timestamps, so it can be written to a "+
			"spec file in the repository as is, and an unchanged product exports identically. The document is the text "+
			"result; the structured result carries its size and SHA-256, to check whether a committed copy is current.",
		func(ctx context.Context, sess *ops.Session, q ops.SpecificationQuery) (*ops.SpecificationView, func(io.Writer) error, error) {
			doc, v, err := sess.Specification(ctx, q)
			return v, func(w io.Writer) error { _, err := io.WriteString(w, doc); return err }, err
		})

	readTool(s, h, "get_capability", "Show a capability",
		"One capability by name or ID: its component, linked requirements, test cases and defects.",
		func(ctx context.Context, sess *ops.Session, in RefInput) (*ops.CapabilityDetail, func(io.Writer) error, error) {
			if err := required(in.Ref); err != nil {
				return nil, nil, err
			}
			v, err := sess.ShowCapability(ctx, in.Ref)
			return v, func(w io.Writer) error { return render.Capability(w, v) }, err
		})

	readTool(s, h, "list_defects", "List defects",
		"List defects, most critical first. Defects are referenced by their identifier, e.g. DF-17.",
		func(ctx context.Context, sess *ops.Session, q ops.DefectQuery) (Items[ops.DefectSummary], func(io.Writer) error, error) {
			items, err := sess.ListDefects(ctx, q)
			return Items[ops.DefectSummary]{items}, func(w io.Writer) error { return render.Defects(w, output.Markdown, items) }, err
		})

	readTool(s, h, "get_defect", "Show a defect",
		"One defect by identifier (DF-17) or ID: description, criticality, links to requirements and test cases, comments.",
		func(ctx context.Context, sess *ops.Session, in RefInput) (*ops.DefectDetail, func(io.Writer) error, error) {
			if err := required(in.Ref); err != nil {
				return nil, nil, err
			}
			v, err := sess.ShowDefect(ctx, in.Ref)
			return v, func(w io.Writer) error { return render.Defect(w, v) }, err
		})

	readTool(s, h, "list_test_cases", "List test cases",
		"List test cases with execution mode and test identifier.",
		func(ctx context.Context, sess *ops.Session, q ops.TestCaseQuery) (Items[ops.TestCaseSummary], func(io.Writer) error, error) {
			items, err := sess.ListTestCases(ctx, q)
			return Items[ops.TestCaseSummary]{items}, func(w io.Writer) error { return render.TestCases(w, output.Markdown, items) }, err
		})

	readTool(s, h, "get_test_case", "Show a test case",
		"One test case by test identifier (e.g. the Go test name), short ID or ID: steps, covered requirements, "+
			"latest results and linked defects.",
		func(ctx context.Context, sess *ops.Session, in RefInput) (*ops.TestCaseDetail, func(io.Writer) error, error) {
			if err := required(in.Ref); err != nil {
				return nil, nil, err
			}
			v, err := sess.ShowTestCase(ctx, in.Ref)
			return v, func(w io.Writer) error { return render.TestCase(w, v) }, err
		})

	readTool(s, h, "campaign_status", "Testing campaign status",
		"The active (or named) testing campaign: verdict, result counts and results that need attention.",
		func(ctx context.Context, sess *ops.Session, in CampaignInput) (*ops.CampaignStatusView, func(io.Writer) error, error) {
			v, err := sess.CampaignStatus(ctx, in.Ref)
			return v, func(w io.Writer) error { return render.Campaign(w, v) }, err
		})
}

// toolError turns an operation error into a message the model can act on.
func toolError(err error) error {
	switch {
	case client.IsUnauthorized(err):
		return fmt.Errorf("%w (the Nexmap token is missing, invalid, expired or revoked: run `nexmap login` or set NEXMAP_TOKEN)", err)
	case client.IsForbidden(err):
		return fmt.Errorf("%w (not permitted for your Nexmap role in this product; ask a product owner)", err)
	}
	return err
}
