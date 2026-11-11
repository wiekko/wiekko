package mcp

import (
	"context"
	"io"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"nexmap.net/internal/cli/ops"
	"nexmap.net/internal/cli/render"
)

// WriteToolNames lists the tools that --read-only leaves out.
var WriteToolNames = []string{"add_comment", "create_backlog_item", "create_defect", "set_defect_status", "set_requirement_status"}

func registerWriteTools(s *sdk.Server, h *handlers) {
	writeTool(s, h, "set_requirement_status", "Set requirement status",
		"Move a requirement or task to another workflow status by name, e.g. when starting or finishing work on it. "+
			"Requires the developer role, and the item must be active: in kanban, in a column the product owner marked active; in scrum, planned into a sprint. Returns the updated item.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.StatusChange) (*ops.RequirementDetail, func(io.Writer) error, error) {
			v, err := sess.SetRequirementStatus(ctx, in)
			return v, func(w io.Writer) error { return render.Requirement(w, v) }, err
		})

	writeTool(s, h, "create_backlog_item", "Create a task or requirement",
		"Create a task (default) or requirement, e.g. to split a requirement into tasks via parent. "+
			"Requires the product owner role. Returns the new item with its identifier.",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.NewBacklogItem) (*ops.RequirementDetail, func(io.Writer) error, error) {
			v, err := sess.CreateBacklogItem(ctx, in)
			return v, func(w io.Writer) error { return render.Requirement(w, v) }, err
		})

	writeTool(s, h, "create_defect", "Report a defect",
		"Report a bug found while working, optionally linked to the test case that reveals it and the requirement it violates. "+
			"Check list_defects first to avoid duplicates. Requires the tester role. Returns the new defect.",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.NewDefect) (*ops.DefectDetail, func(io.Writer) error, error) {
			v, err := sess.CreateDefect(ctx, in)
			return v, func(w io.Writer) error { return render.Defect(w, v) }, err
		})

	writeTool(s, h, "set_defect_status", "Set defect status",
		"Move a defect to another workflow status by name, e.g. after fixing it. Requires the developer role, and the defect must be active. "+
			"Returns the updated defect.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.StatusChange) (*ops.DefectDetail, func(io.Writer) error, error) {
			v, err := sess.SetDefectStatus(ctx, in)
			return v, func(w io.Writer) error { return render.Defect(w, v) }, err
		})

	writeTool(s, h, "add_comment", "Add a comment",
		"Add a Markdown comment to a requirement, task, constraint, defect, test case, capability or component, "+
			"e.g. to record a decision or a question. Requires the developer role (tester for defects and test cases).",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.NewComment) (*ops.CommentResult, func(io.Writer) error, error) {
			v, err := sess.AddComment(ctx, in)
			return v, func(w io.Writer) error { return render.Comment(w, v) }, err
		})
}
