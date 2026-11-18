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
	"nexmap.net/internal/client"
)

// session builds the ops session for the configured server and product.
func (a *app) session() (*ops.Session, error) {
	c, err := a.newClient(true)
	if err != nil {
		return nil, err
	}
	productID, err := a.requireProduct()
	if err != nil {
		return nil, err
	}
	return ops.NewSession(c, productID), nil
}

// emit writes a view as JSON, or through its text renderer.
func (a *app) emit(view any, text func(w io.Writer, f output.Format) error) error {
	if a.format == output.JSON {
		return output.WriteJSON(a.io.Stdout, view)
	}
	return text(a.io.Stdout, a.format)
}

func noArgs(name string, args []string) error {
	if len(args) > 0 {
		return usageErr(fmt.Errorf("%s takes no arguments", name))
	}
	return nil
}

func oneArg(name, what string, args []string) (string, error) {
	if len(args) != 1 {
		return "", usageErr(fmt.Errorf("usage: nexmap %s %s", name, what))
	}
	return args[0], nil
}

func contextCommand() *command {
	return &command{
		name:    "context",
		summary: "product briefing for agents: structure, constraints, current work, open defects, testing",
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("context", args); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			v, err := s.Context(ctx)
			if err != nil {
				return err
			}
			return a.emit(v, func(w io.Writer, _ output.Format) error { return render.Context(w, v) })
		},
	}
}

func requirementGroup(name, summary, fixedKind string) *command {
	var q ops.RequirementQuery
	list := &command{
		name:    name + " list",
		summary: "list " + summary,
		flags: func(fs *flag.FlagSet) {
			if fixedKind == "" {
				fs.StringVar(&q.Kind, "kind", "", "requirement or task (default: both)")
			}
			fs.StringVar(&q.Status, "status", "", "status name")
			fs.StringVar(&q.Category, "category", "", "category prefix or name")
			fs.StringVar(&q.Sprint, "sprint", "", `sprint name or ID, or "current"`)
			fs.StringVar(&q.Search, "search", "", "text in title or identifier")
			fs.BoolVar(&q.Open, "open", false, "only items whose status is not closed")
			fs.BoolVar(&q.Active, "active", false, "only items activated for work")
			fs.BoolVar(&q.Coarse, "coarse", false, "only items that need refinement")
			fs.IntVar(&q.Limit, "limit", 0, "show at most N items")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs(name+" list", args); err != nil {
				return err
			}
			if fixedKind != "" {
				q.Kind = fixedKind
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			items, err := s.ListRequirements(ctx, q)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(items, func(w io.Writer, f output.Format) error { return render.Requirements(w, f, items) })
		},
	}
	show := &command{
		name:    name + " show",
		args:    "<FR-12|ID>",
		summary: "show one item with description, definition of done, tests, defects and comments",
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg(name+" show", "<identifier|ID>", args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.ShowRequirement(ctx, ref)
			if err != nil {
				return err
			}
			if fixedKind != "" && d.Kind != fixedKind {
				return &ops.NotFoundError{Kind: fixedKind, Ref: ref}
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Requirement(w, d) })
		},
	}
	return &command{name: name, summary: summary, subs: []*command{list, show,
		requirementStatusCommand(name, fixedKind), requirementCreateCommand(name, fixedKind),
		requirementUpdateCommand(name, fixedKind)}}
}

func constraintGroup() *command {
	var q ops.ConstraintQuery
	list := &command{
		name:    "constraint list",
		summary: "list constraints (architecture, security, UX rules)",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&q.Category, "category", "", "category prefix or name")
			fs.StringVar(&q.Compliance, "compliance", "", "not_assessed, met or violated")
			fs.StringVar(&q.Search, "search", "", "text in title or identifier")
			fs.IntVar(&q.Limit, "limit", 0, "show at most N items")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("constraint list", args); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			items, err := s.ListConstraints(ctx, q)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(items, func(w io.Writer, f output.Format) error { return render.Constraints(w, f, items) })
		},
	}
	show := &command{
		name:    "constraint show",
		args:    "<AC-42|ID>",
		summary: "show one constraint with description, capabilities and comments",
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg("constraint show", "<identifier|ID>", args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.ShowConstraint(ctx, ref)
			if err != nil {
				return err
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Constraint(w, d) })
		},
	}
	return &command{name: "constraint", summary: "constraints: rules that are met or violated",
		subs: []*command{list, show, constraintCreateCommand(), constraintUpdateCommand()}}
}

func structureCommand(name string) *command {
	return &command{
		name:    name,
		summary: "show the component → capability tree",
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs(name, args); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			v, err := s.Structure(ctx)
			if err != nil {
				return err
			}
			return a.emit(v, func(w io.Writer, f output.Format) error { return render.Structure(w, f, v) })
		},
	}
}

func componentGroup() *command {
	return &command{name: "component", summary: "components (structural parts of the product)",
		subs: []*command{structureCommand("component list"), componentCreateCommand(), componentUpdateCommand(), componentDeleteCommand()}}
}

func capabilityGroup() *command {
	show := &command{
		name:    "capability show",
		args:    "<name|ID>",
		summary: "show a capability with its requirements, test cases and defects",
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) == 0 {
				return usageErr(errors.New("usage: nexmap capability show <name|ID>"))
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			// Names may contain spaces; accept them unquoted.
			d, err := s.ShowCapability(ctx, strings.Join(args, " "))
			if err != nil {
				return err
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Capability(w, d) })
		},
	}
	return &command{name: "capability", summary: "capabilities (testable feature areas)",
		subs: []*command{structureCommand("capability list"), show, capabilityCreateCommand(), capabilityUpdateCommand(),
			capabilityDeleteCommand(), capabilityLinkCommand(false), capabilityLinkCommand(true)}}
}

func defectGroup() *command {
	var q ops.DefectQuery
	list := &command{
		name:    "defect list",
		summary: "list defects, most critical first",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&q.Open, "open", false, "only defects whose status is not closed")
			fs.StringVar(&q.Criticality, "criticality", "", "critical, high, medium or low")
			fs.StringVar(&q.Status, "status", "", "status name")
			fs.StringVar(&q.Search, "search", "", "text in title")
			fs.IntVar(&q.Limit, "limit", 0, "show at most N items")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("defect list", args); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			items, err := s.ListDefects(ctx, q)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(items, func(w io.Writer, f output.Format) error { return render.Defects(w, f, items) })
		},
	}
	show := &command{
		name:    "defect show",
		args:    "<short ID|ID>",
		summary: "show one defect with description, links and comments",
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg("defect show", "<short ID|ID>", args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.ShowDefect(ctx, ref)
			if err != nil {
				return err
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Defect(w, d) })
		},
	}
	return &command{name: "defect", summary: "defects (bugs and issues)", subs: []*command{list, show,
		defectStatusCommand(), defectCreateCommand()}}
}

func testGroup() *command {
	var q ops.TestCaseQuery
	var automatic, manual bool
	list := &command{
		name:    "test list",
		summary: "list test cases",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&automatic, "automatic", false, "only automatic test cases")
			fs.BoolVar(&manual, "manual", false, "only manual test cases")
			fs.StringVar(&q.Component, "component", "", "component name or ID")
			fs.StringVar(&q.Capability, "capability", "", "capability name or ID")
			fs.StringVar(&q.Search, "search", "", "text in title or test identifier")
			fs.IntVar(&q.Limit, "limit", 0, "show at most N items")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("test list", args); err != nil {
				return err
			}
			switch {
			case automatic && manual:
				return usageErr(errors.New("--automatic and --manual are mutually exclusive"))
			case automatic:
				q.Mode = "automatic"
			case manual:
				q.Mode = "manual"
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			items, err := s.ListTestCases(ctx, q)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(items, func(w io.Writer, f output.Format) error { return render.TestCases(w, f, items) })
		},
	}
	show := &command{
		name:    "test show",
		args:    "<test identifier|short ID|ID>",
		summary: "show one test case with covered requirements, results and defects",
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg("test show", "<test identifier|short ID|ID>", args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.ShowTestCase(ctx, ref)
			if err != nil {
				return err
			}
			return a.emit(d, func(w io.Writer, _ output.Format) error { return render.TestCase(w, d) })
		},
	}
	return &command{name: "test", summary: "test cases", subs: []*command{list, show, testSyncCommand(), testLinkCommand(false), testLinkCommand(true)}}
}

func campaignGroup() *command {
	status := &command{
		name:    "campaign status",
		args:    "[name|ID]",
		summary: "show the active (or named) testing campaign: verdict, counts, results needing attention",
		run: func(ctx context.Context, a *app, args []string) error {
			s, err := a.session()
			if err != nil {
				return err
			}
			v, err := s.CampaignStatus(ctx, strings.Join(args, " "))
			if err != nil {
				return err
			}
			return a.emit(v, func(w io.Writer, _ output.Format) error { return render.Campaign(w, v) })
		},
	}
	return &command{name: "campaign", summary: "testing campaigns", subs: []*command{status}}
}

// usageIfInput marks errors caused by bad filter values (unknown status,
// category, …) as usage errors; API errors keep their own exit code.
func usageIfInput(err error) error {
	var nf *ops.NotFoundError
	if errors.As(err, &nf) {
		return err
	}
	if isAPIError(err) {
		return err
	}
	return usageErr(err)
}

func isAPIError(err error) bool {
	var ae *client.APIError
	var te *client.TransportError
	return errors.As(err, &ae) || errors.As(err, &te)
}
