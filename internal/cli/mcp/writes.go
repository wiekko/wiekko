// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package mcp

import (
	"context"
	"fmt"
	"io"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"wiekko.net/internal/cli/ops"
	"wiekko.net/internal/cli/output"
	"wiekko.net/internal/cli/render"
)

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
		"Create a task (default) or requirement, e.g. to split a requirement into tasks via parent, optionally with a priority (1 is the highest). "+
			"Requires the product owner role. Returns the new item with its identifier.",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.NewBacklogItem) (*ops.RequirementDetail, func(io.Writer) error, error) {
			v, err := sess.CreateBacklogItem(ctx, in)
			return v, func(w io.Writer) error { return render.Requirement(w, v) }, err
		})

	writeTool(s, h, "update_backlog_item", "Update a task or requirement",
		"Move a requirement or task below another item (parent), or to the top level with an empty parent, or change its priority; omitted fields stay. "+
			"A requirement's parent must be a requirement, and a move may not create a cycle. "+
			"Requires the product owner role. Returns the updated item.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.BacklogItemChange) (*ops.RequirementDetail, func(io.Writer) error, error) {
			v, err := sess.UpdateBacklogItem(ctx, in)
			return v, func(w io.Writer) error { return render.Requirement(w, v) }, err
		})

	writeTool(s, h, "create_defect", "Report a defect",
		"Report a bug found while working, optionally linked to the test case that reveals it, the requirement it violates "+
			"and the components and capabilities it affects, optionally with a priority (1 is the highest). "+
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

	writeTool(s, h, "set_archived", "Archive or unarchive an item",
		"Archive a requirement, task or defect in a closed status, hiding it from default lists, the board and sprint planning, "+
			"or bring it back with archived false. A requirement or task is archived together with all items below it, which must be closed too. "+
			"Archived items stay part of the product state; list tools show them with archived include or only. "+
			"Requires the product owner role. Returns the updated item.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.ArchiveChange) (*ops.ArchiveResult, func(io.Writer) error, error) {
			v, err := sess.SetArchived(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return v, func(w io.Writer) error {
				if v.Defect != nil {
					return render.Defect(w, v.Defect)
				}
				return render.Requirement(w, v.Requirement)
			}, nil
		})

	writeTool(s, h, "add_comment", "Add a comment",
		"Add a Markdown comment to a requirement, task, constraint, defect, test case, capability or component, "+
			"e.g. to record a decision or a question. Requires the developer role (tester for defects and test cases).",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.NewComment) (*ops.CommentResult, func(io.Writer) error, error) {
			v, err := sess.AddComment(ctx, in)
			return v, func(w io.Writer) error { return render.Comment(w, v) }, err
		})

	registerStructureWriteTools(s, h)
	registerDocumentWriteTools(s, h)
}

// registerStructureWriteTools offers the structure, constraint and link
// writes of the CLI (FR-136). Deletions are deliberately left out: an agent
// can restructure, but not remove.
func registerStructureWriteTools(s *sdk.Server, h *handlers) {
	component := func(v *ops.ComponentView) func(io.Writer) error {
		return func(w io.Writer) error {
			return render.Structure(w, output.Markdown, &ops.Structure{Components: []ops.ComponentView{*v}, ProductLevel: []ops.CapabilityView{}})
		}
	}
	writeTool(s, h, "create_component", "Create a component",
		"Create a component, a structural part of the product. Requires the product owner role. Returns the component.",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.NewComponent) (*ops.ComponentView, func(io.Writer) error, error) {
			v, err := sess.CreateComponent(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return v, component(v), nil
		})
	writeTool(s, h, "update_component", "Update a component",
		"Rename a component or change its description; omitted fields stay. Requires the product owner role.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.ComponentChange) (*ops.ComponentView, func(io.Writer) error, error) {
			v, err := sess.UpdateComponent(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return v, component(v), nil
		})
	writeTool(s, h, "create_capability", "Create a capability",
		"Create a capability (a testable feature area), under a component or at product level. Requires the product owner role.",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.NewCapability) (*ops.CapabilityDetail, func(io.Writer) error, error) {
			v, err := sess.CreateCapability(ctx, in)
			return v, func(w io.Writer) error { return render.Capability(w, v) }, err
		})
	writeTool(s, h, "update_capability", "Update a capability",
		"Rename a capability, move it to another component (empty component: product level) or change its description; "+
			"omitted fields stay. Requires the product owner role.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.CapabilityChange) (*ops.CapabilityDetail, func(io.Writer) error, error) {
			v, err := sess.UpdateCapability(ctx, in)
			return v, func(w io.Writer) error { return render.Capability(w, v) }, err
		})
	writeTool(s, h, "link_capability", "Link items to a capability",
		"Link requirements (FR-12) and constraints (AC-3) to a capability, for traceability. Tasks cannot be linked. "+
			"Requires the product owner role. Returns the capability with everything traced to it.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.CapabilityLinks) (*ops.CapabilityDetail, func(io.Writer) error, error) {
			v, err := sess.LinkCapability(ctx, in)
			return v, func(w io.Writer) error { return render.Capability(w, v) }, err
		})
	writeTool(s, h, "unlink_capability", "Unlink items from a capability",
		"Remove the links between a capability and requirements or constraints. Requires the product owner role.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.CapabilityLinks) (*ops.CapabilityDetail, func(io.Writer) error, error) {
			in.Unlink = true
			v, err := sess.LinkCapability(ctx, in)
			return v, func(w io.Writer) error { return render.Capability(w, v) }, err
		})
	writeTool(s, h, "link_test_case", "Link a test case",
		"Link a test case to the requirement and/or the constraints it verifies, and/or set the capability or component "+
			"it belongs to (replacing the current one). Requires the developer role. Returns the test case.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.TestCaseLinks) (*ops.TestCaseDetail, func(io.Writer) error, error) {
			v, err := sess.LinkTestCase(ctx, in)
			return v, func(w io.Writer) error { return render.TestCase(w, v) }, err
		})
	writeTool(s, h, "unlink_test_case", "Unlink a test case",
		"Remove a test case's link to a requirement or constraints, or clear its capability or component if it is the one named. "+
			"Requires the developer role.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.TestCaseLinks) (*ops.TestCaseDetail, func(io.Writer) error, error) {
			in.Unlink = true
			v, err := sess.LinkTestCase(ctx, in)
			return v, func(w io.Writer) error { return render.TestCase(w, v) }, err
		})
	writeTool(s, h, "sync_test_cases", "Create test cases from a test run",
		"Create an automatic test case for every test in a run that has none yet, from a test report (go test -json or -v "+
			"output, JUnit XML) or a list of test identifiers. Subtests roll up into their parent. Optionally files the new "+
			"test cases under a capability or component, gives them a pyramid level, and links them to a requirement and constraints. "+
			"Use dry_run first to review. Requires the developer role.",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.TestSync) (*ops.SyncResult, func(io.Writer) error, error) {
			v, err := sess.SyncTestCases(ctx, in)
			if err != nil {
				return nil, nil, err
			}
			return v, func(w io.Writer) error {
				verb := "created"
				if v.DryRun {
					verb = "would be created"
				}
				if _, err := fmt.Fprintf(w, "%d test cases %s, %d already existed.\n", len(v.Created), verb, v.Existing); err != nil {
					return err
				}
				for _, tc := range v.Created {
					if _, err := fmt.Fprintf(w, "- `%s` %s\n", tc.TestIdentifier, tc.Title); err != nil {
						return err
					}
				}
				return nil
			}, nil
		})
	writeTool(s, h, "create_constraint", "Create a constraint",
		"Create a constraint, a rule the product must keep following (architecture, security, UX). "+
			"Requires the product owner role. Returns the constraint with its identifier.",
		false,
		func(ctx context.Context, sess *ops.Session, in ops.NewConstraint) (*ops.ConstraintDetail, func(io.Writer) error, error) {
			v, err := sess.CreateConstraint(ctx, in)
			return v, func(w io.Writer) error { return render.Constraint(w, v) }, err
		})
	writeTool(s, h, "update_constraint", "Update a constraint",
		"Change a constraint, e.g. record after a check whether it is met or violated (compliance); omitted fields stay. "+
			"Requires the product owner role.",
		true,
		func(ctx context.Context, sess *ops.Session, in ops.ConstraintChange) (*ops.ConstraintDetail, func(io.Writer) error, error) {
			v, err := sess.UpdateConstraint(ctx, in)
			return v, func(w io.Writer) error { return render.Constraint(w, v) }, err
		})
}
