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

// Commands that change the product structure, constraints and traceability
// links (FR-902, FR-903). Like every write, the server decides what the
// token's user may do.

// flagsSet reports which flags were given on the command line, so update
// commands can tell "not given" from "set to empty".
type flagsSet struct{ fs *flag.FlagSet }

func (f *flagsSet) bind(fs *flag.FlagSet) { f.fs = fs }

func (f *flagsSet) has(names ...string) bool {
	found := false
	f.fs.Visit(func(fl *flag.Flag) {
		for _, n := range names {
			if fl.Name == n {
				found = true
			}
		}
	})
	return found
}

// optional returns &value if the flag was given, nil otherwise.
func (f *flagsSet) optional(name, value string) *string {
	if !f.has(name) {
		return nil
	}
	return &value
}

// optionalText is optional for a --NAME/--NAME-file pair.
func (f *flagsSet) optionalText(a *app, t *textFlags, name string) (*string, error) {
	if !f.has(name, name+"-file") {
		return nil, nil
	}
	v, err := t.value(a, name)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// joinedRef accepts a name given as several unquoted words.
func joinedRef(usage string, args []string) (string, error) {
	ref := strings.TrimSpace(strings.Join(args, " "))
	if ref == "" {
		return "", usageErr(errors.New("usage: nexmap " + usage))
	}
	return ref, nil
}

// confirmDelete refuses a deletion without --yes: deletions cannot be undone.
func confirmDelete(what string, yes bool) error {
	if !yes {
		return usageErr(fmt.Errorf("deleting %s cannot be undone; pass --yes to confirm", what))
	}
	return nil
}

func emitComponent(a *app, v *ops.ComponentView) error {
	st := &ops.Structure{Components: []ops.ComponentView{*v}, ProductLevel: []ops.CapabilityView{}}
	return a.emit(v, func(w io.Writer, f output.Format) error { return render.Structure(w, f, st) })
}

func emitDeleted(a *app, what string, r *ops.Ref) error {
	return a.emit(r, func(w io.Writer, _ output.Format) error {
		_, err := fmt.Fprintf(w, "Deleted %s %q.\n", what, r.Title)
		return err
	})
}

// --- Components ---

func componentCreateCommand() *command {
	var in ops.NewComponent
	var desc textFlags
	return &command{
		name:    "component create",
		summary: "create a component (owner role)",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&in.Name, "name", "", "component name (required)")
			desc.bind(fs, "body", "Markdown description")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("component create", args); err != nil {
				return err
			}
			if strings.TrimSpace(in.Name) == "" {
				return usageErr(errors.New("--name is required"))
			}
			var err error
			if in.Description, err = desc.value(a, "body"); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			v, err := s.CreateComponent(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return emitComponent(a, v)
		},
	}
}

func componentUpdateCommand() *command {
	var name string
	var desc textFlags
	var set flagsSet
	return &command{
		name:    "component update",
		args:    "<name|ID>",
		summary: "rename a component or change its description (owner role)",
		flags: func(fs *flag.FlagSet) {
			set.bind(fs)
			fs.StringVar(&name, "name", "", "new name")
			desc.bind(fs, "body", "new Markdown description (empty clears it)")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := joinedRef("component update <name|ID> [--name …] [--body …]", args)
			if err != nil {
				return err
			}
			in := ops.ComponentChange{Ref: ref, Name: set.optional("name", name)}
			if in.Description, err = set.optionalText(a, &desc, "body"); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			v, err := s.UpdateComponent(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return emitComponent(a, v)
		},
	}
}

func componentDeleteCommand() *command {
	var yes bool
	return &command{
		name:    "component delete",
		args:    "<name|ID>",
		summary: "delete a component; its capabilities stay, without a component (owner role)",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&yes, "yes", false, "confirm the deletion")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := joinedRef("component delete <name|ID> --yes", args)
			if err != nil {
				return err
			}
			if err := confirmDelete("a component", yes); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			r, err := s.DeleteComponent(ctx, ref)
			if err != nil {
				return usageIfInput(err)
			}
			return emitDeleted(a, "component", r)
		},
	}
}

// --- Capabilities ---

func emitCapability(a *app, d *ops.CapabilityDetail) error {
	return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Capability(w, d) })
}

func capabilityCreateCommand() *command {
	var in ops.NewCapability
	var desc textFlags
	return &command{
		name:    "capability create",
		summary: "create a capability, optionally under a component (owner role)",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&in.Name, "name", "", "capability name (required)")
			fs.StringVar(&in.Component, "component", "", "component name or ID (default: product level)")
			desc.bind(fs, "body", "Markdown description")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("capability create", args); err != nil {
				return err
			}
			if strings.TrimSpace(in.Name) == "" {
				return usageErr(errors.New("--name is required"))
			}
			var err error
			if in.Description, err = desc.value(a, "body"); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.CreateCapability(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return emitCapability(a, d)
		},
	}
}

func capabilityUpdateCommand() *command {
	var name, component string
	var desc textFlags
	var set flagsSet
	return &command{
		name:    "capability update",
		args:    "<name|ID>",
		summary: "rename a capability, move it to another component or change its description (owner role)",
		flags: func(fs *flag.FlagSet) {
			set.bind(fs)
			fs.StringVar(&name, "name", "", "new name")
			fs.StringVar(&component, "component", "", `component name or ID; "" makes it product level`)
			desc.bind(fs, "body", "new Markdown description (empty clears it)")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := joinedRef("capability update <name|ID> [--name …] [--component …] [--body …]", args)
			if err != nil {
				return err
			}
			in := ops.CapabilityChange{Ref: ref, Name: set.optional("name", name), Component: set.optional("component", component)}
			if in.Description, err = set.optionalText(a, &desc, "body"); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.UpdateCapability(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return emitCapability(a, d)
		},
	}
}

func capabilityDeleteCommand() *command {
	var yes bool
	return &command{
		name:    "capability delete",
		args:    "<name|ID>",
		summary: "delete a capability (owner role)",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&yes, "yes", false, "confirm the deletion")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := joinedRef("capability delete <name|ID> --yes", args)
			if err != nil {
				return err
			}
			if err := confirmDelete("a capability", yes); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			r, err := s.DeleteCapability(ctx, ref)
			if err != nil {
				return usageIfInput(err)
			}
			return emitDeleted(a, "capability", r)
		},
	}
}

// splitCapabilityItems separates "<capability name words…> <FR-12|AC-3>…":
// the identifiers at the end are the items, everything before names the
// capability, so names with spaces need no quotes.
func splitCapabilityItems(args []string) (string, []string) {
	i := len(args)
	for i > 0 && ops.IsIdentifier(args[i-1]) {
		i--
	}
	return strings.TrimSpace(strings.Join(args[:i], " ")), args[i:]
}

func capabilityLinkCommand(unlink bool) *command {
	verb, summary := "link", "link requirements and constraints to a capability (owner role)"
	if unlink {
		verb, summary = "unlink", "remove links between a capability and requirements or constraints (owner role)"
	}
	name := "capability " + verb
	return &command{
		name:    name,
		args:    "<capability> <FR-12|AC-3>…",
		summary: summary,
		run: func(ctx context.Context, a *app, args []string) error {
			capability, items := splitCapabilityItems(args)
			if capability == "" || len(items) == 0 {
				return usageErr(fmt.Errorf("usage: nexmap %s <capability> <FR-12|AC-3>…", name))
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.LinkCapability(ctx, ops.CapabilityLinks{Capability: capability, Items: items, Unlink: unlink})
			if err != nil {
				return usageIfInput(err)
			}
			return emitCapability(a, d)
		},
	}
}

// --- Constraints ---

func emitConstraint(a *app, d *ops.ConstraintDetail) error {
	return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Constraint(w, d) })
}

func constraintCreateCommand() *command {
	var in ops.NewConstraint
	var desc textFlags
	return &command{
		name:    "constraint create",
		summary: "create a constraint (owner role)",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&in.Title, "title", "", "the rule, as a short statement (required)")
			fs.StringVar(&in.Category, "category", "", "category prefix or name (default: the product's default)")
			fs.StringVar(&in.Compliance, "compliance", "", "not_assessed (default), met or violated")
			desc.bind(fs, "body", "Markdown description")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("constraint create", args); err != nil {
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
			d, err := s.CreateConstraint(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return emitConstraint(a, d)
		},
	}
}

func constraintUpdateCommand() *command {
	var title, category, compliance string
	var desc textFlags
	var set flagsSet
	return &command{
		name:    "constraint update",
		args:    "<AC-3|ID>",
		summary: "change a constraint, e.g. record that it is met or violated (owner role)",
		flags: func(fs *flag.FlagSet) {
			set.bind(fs)
			fs.StringVar(&title, "title", "", "new title")
			fs.StringVar(&category, "category", "", "new category prefix or name")
			fs.StringVar(&compliance, "compliance", "", "not_assessed, met or violated")
			desc.bind(fs, "body", "new Markdown description (empty clears it)")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) != 1 {
				return usageErr(errors.New("usage: nexmap constraint update <AC-3|ID> [--compliance …] [--title …] [--category …] [--body …]"))
			}
			in := ops.ConstraintChange{Ref: args[0], Title: set.optional("title", title),
				Category: set.optional("category", category), Compliance: set.optional("compliance", compliance)}
			var err error
			if in.Description, err = set.optionalText(a, &desc, "body"); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.UpdateConstraint(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return emitConstraint(a, d)
		},
	}
}

// --- Test cases ---

func testLinkCommand(unlink bool) *command {
	var in ops.TestCaseLinks
	verb, summary := "link", "link a test case to a requirement, capability or component (developer role)"
	if unlink {
		verb, summary = "unlink", "remove a test case's link to a requirement, capability or component (developer role)"
	}
	name := "test " + verb
	return &command{
		name:    name,
		args:    "<test identifier|short ID>",
		summary: summary,
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&in.Requirement, "req", "", "requirement, e.g. FR-12")
			fs.StringVar(&in.Capability, "capability", "", "capability name or ID")
			fs.StringVar(&in.Component, "component", "", "component name or ID")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) != 1 {
				return usageErr(fmt.Errorf("usage: nexmap %s <test> [--req FR-12] [--capability …] [--component …]", name))
			}
			if in.Requirement == "" && in.Capability == "" && in.Component == "" {
				return usageErr(errors.New("give --req, --capability or --component"))
			}
			in.TestCase, in.Unlink = args[0], unlink
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.LinkTestCase(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.TestCase(w, d) })
		},
	}
}
