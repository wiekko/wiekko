package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"nexmap.net/internal/cli/ops"
	"nexmap.net/internal/cli/output"
	"nexmap.net/internal/cli/render"
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
		return "", "", usageErr(fmt.Errorf("usage: nexmap %s <item> <status>", name))
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

func requirementUpdateCommand(group, fixedKind string) *command {
	var parent string
	var set flagsSet
	name := group + " update"
	return &command{
		name:    name,
		args:    "<FR-12|ID>",
		summary: "change an item, e.g. move it below another one (owner role)",
		flags: func(fs *flag.FlagSet) {
			set.bind(fs)
			fs.StringVar(&parent, "parent", "", `new parent, e.g. FR-3; "" moves the item to the top level`)
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) != 1 {
				return usageErr(fmt.Errorf("usage: nexmap %s <FR-12|ID> --parent <FR-3|\"\">", name))
			}
			in := ops.BacklogItemChange{Ref: args[0], Parent: set.optional("parent", parent), Kind: fixedKind}
			if in.Parent == nil {
				return usageErr(errors.New("nothing to change: give --parent"))
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
		summary: "report a defect, optionally linked to a test case and a requirement (tester role)",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&in.Title, "title", "", "title (required)")
			fs.StringVar(&in.Criticality, "criticality", "", "critical, high, medium (default) or low")
			fs.StringVar(&in.Status, "status", "", "initial status name")
			fs.StringVar(&in.TestCase, "test", "", "test case revealing the defect: test identifier, short ID or ID")
			fs.StringVar(&in.Requirement, "req", "", "requirement or task the defect violates, e.g. FR-12")
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
				return usageErr(errors.New("usage: nexmap comment add <FR-12|ref> --body-file -"))
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
