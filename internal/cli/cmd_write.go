// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"wiekko.net/internal/cli/ops"
	"wiekko.net/internal/cli/output"
	"wiekko.net/internal/cli/render"
)

// Write commands. Every one is bounded by the token user's product role on
// the server; the CLI never decides what a user may do.

// textFlags registers --NAME and --NAME-file for a long Markdown text.
type textFlags struct {
	text, file string
}

func (t *textFlags) bind(fs *flag.FlagSet, name, what string) {
	fs.StringVar(&t.text, name, "", what)
	fs.StringVar(&t.file, name+"-file", "", "read "+name+" from a file, or - for stdin")
}

// value returns the text from the flag or the file.
func (t *textFlags) value(a *app, name string) (string, error) {
	if t.text != "" && t.file != "" {
		return "", usageErr(fmt.Errorf("--%s and --%s-file are mutually exclusive", name, name))
	}
	if t.file == "" {
		return t.text, nil
	}
	data, err := a.readData("@" + t.file)
	if err != nil {
		return "", usageErr(fmt.Errorf("--%s-file: %w", name, err))
	}
	return string(data), nil
}

// statusArgs splits "<ref> <status words…>"; status names may contain spaces.
func statusArgs(name string, args []string) (string, string, error) {
	if len(args) < 2 {
		return "", "", usageErr(fmt.Errorf("usage: wiekko %s <item> <status>", name))
	}
	return args[0], strings.Join(args[1:], " "), nil
}

func requirementStatusCommand(group, fixedKind string) *command {
	name := group + " status"
	return &command{
		name:    name,
		args:    "<FR-12|ID> <status>",
		summary: "move an item to another status by name (developer role)",
		run: func(ctx context.Context, a *app, args []string) error {
			ref, status, err := statusArgs(name, args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.SetRequirementStatus(ctx, ops.StatusChange{Ref: ref, Status: status, Kind: fixedKind})
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Requirement(w, d) })
		},
	}
}

func requirementCreateCommand(group, fixedKind string) *command {
	var in ops.NewBacklogItem
	var desc, dod textFlags
	return &command{
		name:    group + " create",
		summary: "create a task or requirement, e.g. to split up a requirement (owner role)",
		flags: func(fs *flag.FlagSet) {
			if fixedKind == "" {
				fs.StringVar(&in.Kind, "kind", "task", "task or requirement")
			}
			fs.StringVar(&in.Title, "title", "", "title (required)")
			fs.StringVar(&in.Category, "category", "", "category prefix or name (default: the product's default)")
			fs.StringVar(&in.Parent, "parent", "", "identifier of the requirement this item refines, e.g. FR-12")
			fs.StringVar(&in.Status, "status", "", "initial status name")
			fs.IntVar(&in.Priority, "priority", 0, priorityUsage)
			if fixedKind != "task" {
				fs.Var((*repeatedFlag)(&in.Capabilities), "capability", "capability the requirement belongs to, by name; repeat for several (requirements only)")
			}
			desc.bind(fs, "body", "Markdown description")
			dod.bind(fs, "dod", "Markdown definition of done")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs(group+" create", args); err != nil {
				return err
			}
			if fixedKind != "" {
				in.Kind = fixedKind
			}
			if strings.TrimSpace(in.Title) == "" {
				return usageErr(errors.New("--title is required"))
			}
			var err error
			if in.Description, err = desc.value(a, "body"); err != nil {
				return err
			}
			if in.DefinitionOfDone, err = dod.value(a, "dod"); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.CreateBacklogItem(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Requirement(w, d) })
		},
	}
}

// priorityUsage describes --priority, which orders items lowest number
// first (FR-149).
const priorityUsage = "priority: 1 is the highest, higher numbers come later, 0 for none"

// optionalPriority returns &value if --priority was given, nil otherwise.
func (f *flagsSet) optionalPriority(value int) *int {
	if !f.has("priority") {
		return nil
	}
	return &value
}

func requirementUpdateCommand(group, fixedKind string) *command {
	var parent string
	var priority int
	var set flagsSet
	name := group + " update"
	return &command{
		name:    name,
		args:    "<FR-12|ID>",
		summary: "change an item's parent or priority (owner role)",
		flags: func(fs *flag.FlagSet) {
			set.bind(fs)
			fs.StringVar(&parent, "parent", "", `new parent, e.g. FR-3; "" moves the item to the top level`)
			fs.IntVar(&priority, "priority", 0, priorityUsage)
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) != 1 {
				return usageErr(fmt.Errorf("usage: wiekko %s <FR-12|ID> [--parent <FR-3|\"\">] [--priority N]", name))
			}
			in := ops.BacklogItemChange{Ref: args[0], Parent: set.optional("parent", parent),
				Priority: set.optionalPriority(priority), Kind: fixedKind}
			if in.Parent == nil && in.Priority == nil {
				return usageErr(errors.New("nothing to change: give --parent or --priority"))
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.UpdateBacklogItem(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Requirement(w, d) })
		},
	}
}

func defectStatusCommand() *command {
	return &command{
		name:    "defect status",
		args:    "<DF-17|ID> <status>",
		summary: "move a defect to another status by name (developer role)",
		run: func(ctx context.Context, a *app, args []string) error {
			ref, status, err := statusArgs("defect status", args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.SetDefectStatus(ctx, ops.StatusChange{Ref: ref, Status: status})
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Defect(w, d) })
		},
	}
}

func defectCreateCommand() *command {
	var in ops.NewDefect
	var desc textFlags
	return &command{
		name:    "defect create",
		summary: "report a defect, optionally linked to a test case, a requirement, components and capabilities (tester role)",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&in.Title, "title", "", "title (required)")
			fs.StringVar(&in.Criticality, "criticality", "", "critical, high, medium (default) or low")
			fs.IntVar(&in.Priority, "priority", 0, priorityUsage)
			fs.StringVar(&in.Status, "status", "", "initial status name")
			fs.StringVar(&in.TestCase, "test", "", "test case revealing the defect: test identifier, short ID or ID")
			fs.StringVar(&in.Requirement, "req", "", "requirement or task the defect violates, e.g. FR-12")
			fs.Var((*listFlag)(&in.Components), "component", "component the defect affects (repeatable or comma-separated)")
			fs.Var((*listFlag)(&in.Capabilities), "capability", "capability the defect affects (repeatable or comma-separated)")
			desc.bind(fs, "body", "Markdown description")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("defect create", args); err != nil {
				return err
			}
			if strings.TrimSpace(in.Title) == "" {
				return usageErr(errors.New("--title is required"))
			}
			var err error
			if in.Description, err = desc.value(a, "body"); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.CreateDefect(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Defect(w, d) })
		},
	}
}

// defectUpdateCommand changes a defect's priority (FR-149).
func defectUpdateCommand() *command {
	var priority int
	var set flagsSet
	return &command{
		name:    "defect update",
		args:    "<DF-17>",
		summary: "change a defect's priority (owner role)",
		flags: func(fs *flag.FlagSet) {
			set.bind(fs)
			fs.IntVar(&priority, "priority", 0, priorityUsage)
		},
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg("defect update", "<DF-17>", args)
			if err != nil {
				return err
			}
			in := ops.DefectChange{Ref: ref, Priority: set.optionalPriority(priority)}
			if in.Priority == nil {
				return usageErr(errors.New("nothing to change: give --priority"))
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.UpdateDefect(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Defect(w, d) })
		},
	}
}

// listFlag is a repeatable flag whose values may also be comma-separated.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*l = append(*l, part)
		}
	}
	return nil
}

// defectLinkCommand links a defect to, or unlinks it from, requirements,
// test cases, components and capabilities (FR-147).
func defectLinkCommand(unlink bool) *command {
	var in ops.DefectLinks
	verb, summary := "link", "link a defect to requirements, test cases, components or capabilities (tester role)"
	if unlink {
		verb, summary = "unlink", "remove a defect's links to requirements, test cases, components or capabilities (developer role)"
	}
	name := "defect " + verb
	return &command{
		name:    name,
		args:    "<DF-17>",
		summary: summary,
		flags: func(fs *flag.FlagSet) {
			fs.Var((*listFlag)(&in.Requirements), "req", "requirement or task, e.g. FR-12 (repeatable or comma-separated)")
			fs.Var((*listFlag)(&in.TestCases), "test", "test case: test identifier, short ID or ID (repeatable or comma-separated)")
			fs.Var((*listFlag)(&in.Components), "component", "component name or ID (repeatable or comma-separated)")
			fs.Var((*listFlag)(&in.Capabilities), "capability", "capability name or ID (repeatable or comma-separated)")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg(name, "<DF-17>", args)
			if err != nil {
				return err
			}
			if len(in.Requirements)+len(in.TestCases)+len(in.Components)+len(in.Capabilities) == 0 {
				return usageErr(errors.New("give --req, --test, --component or --capability"))
			}
			in.Defect, in.Unlink = ref, unlink
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.LinkDefect(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Defect(w, d) })
		},
	}
}

func commentGroup() *command {
	var in ops.NewComment
	var body textFlags
	add := &command{
		name:    "comment add",
		args:    "<FR-12|ref>",
		summary: "add a Markdown comment to a requirement, task, constraint, defect, test case, capability or component",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&in.Type, "type", "", "requirement, task, constraint, defect, test_case, capability or component (needed unless the ref is an identifier like FR-12)")
			body.bind(fs, "body", "comment text (Markdown)")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) == 0 {
				return usageErr(errors.New("usage: wiekko comment add <FR-12|ref> --body-file -"))
			}
			// Capability and component names may contain spaces.
			in.Ref = strings.Join(args, " ")
			var err error
			if in.Body, err = body.value(a, "body"); err != nil {
				return err
			}
			if strings.TrimSpace(in.Body) == "" {
				return usageErr(errors.New("the comment is empty: pass --body or --body-file"))
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			c, err := s.AddComment(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(c, func(w io.Writer, _ output.Format) error { return render.Comment(w, c) })
		},
	}
	return &command{name: "comment", summary: "comments on any item", subs: []*command{add}}
}

// repeatedFlag collects every occurrence of a flag, e.g. --capability A
// --capability B.
type repeatedFlag []string

func (f *repeatedFlag) String() string { return strings.Join(*f, ", ") }

func (f *repeatedFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}

// requirementDeleteCommand deletes a requirement or task (FR-153). Like every
// deletion it needs --yes; the MCP server offers no counterpart.
func requirementDeleteCommand(group, fixedKind string) *command {
	var yes bool
	var children string
	name := group + " delete"
	what := "a requirement or task"
	if fixedKind != "" {
		what = "a " + fixedKind
	}
	return &command{
		name:    name,
		args:    "<FR-12|ID>",
		summary: "delete " + what + " (owner role)",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&yes, "yes", false, "confirm the deletion")
			fs.StringVar(&children, "children", ops.KeepChildren,
				"keep: move the children to the item's parent; delete: delete them too")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg(name, "<identifier|ID> --yes [--children keep|delete]", args)
			if err != nil {
				return err
			}
			if children != ops.KeepChildren && children != ops.DeleteChildren {
				return usageErr(fmt.Errorf("--children must be %s or %s, not %q", ops.KeepChildren, ops.DeleteChildren, children))
			}
			if err := confirmDelete(what, yes); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			r, kind, err := s.DeleteBacklogItem(ctx, ref, fixedKind, children)
			if err != nil {
				return usageIfInput(err)
			}
			return emitDeleted(a, strings.TrimSpace(kind+" "+r.Identifier), r)
		},
	}
}

// defectDeleteCommand deletes a defect (FR-153); needs --yes.
func defectDeleteCommand() *command {
	var yes bool
	return &command{
		name:    "defect delete",
		args:    "<DF-17|ID>",
		summary: "delete a defect (owner role)",
		flags:   func(fs *flag.FlagSet) { fs.BoolVar(&yes, "yes", false, "confirm the deletion") },
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg("defect delete", "<identifier|ID> --yes", args)
			if err != nil {
				return err
			}
			if err := confirmDelete("a defect", yes); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			r, err := s.DeleteDefect(ctx, ref)
			if err != nil {
				return usageIfInput(err)
			}
			return emitDeleted(a, strings.TrimSpace("defect "+r.Identifier), r)
		},
	}
}

// archiveVerb names the archive or unarchive command.
func archiveVerb(archived bool) string {
	if archived {
		return "archive"
	}
	return "unarchive"
}

// oneRef reads the single item reference of a command.
func oneRef(name string, args []string) (string, error) {
	if len(args) != 1 {
		return "", usageErr(fmt.Errorf("usage: wiekko %s <item>", name))
	}
	return args[0], nil
}

func requirementArchiveCommand(group, fixedKind string, archived bool) *command {
	name := group + " " + archiveVerb(archived)
	summary := "archive a closed item with all items below it, hiding it from default lists (owner role)"
	if !archived {
		summary = "bring an archived item and the items below it back (owner role)"
	}
	return &command{
		name:    name,
		args:    "<FR-12|ID>",
		summary: summary,
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneRef(name, args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.SetRequirementArchived(ctx, ops.ArchiveChange{Ref: ref, Archived: archived, Kind: fixedKind})
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Requirement(w, d) })
		},
	}
}

func defectArchiveCommand(archived bool) *command {
	name := "defect " + archiveVerb(archived)
	summary := "archive a closed defect, hiding it from default lists (owner role)"
	if !archived {
		summary = "bring an archived defect back (owner role)"
	}
	return &command{
		name:    name,
		args:    "<DF-17|ID>",
		summary: summary,
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneRef(name, args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.SetDefectArchived(ctx, ops.ArchiveChange{Ref: ref, Archived: archived})
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Defect(w, d) })
		},
	}
}
