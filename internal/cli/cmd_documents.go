// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"nexmap.net/internal/cli/ops"
	"nexmap.net/internal/cli/output"
	"nexmap.net/internal/cli/render"
)

// documentGroup is `nexmap doc`: documents such as protocols and schemas
// (FR-140). Nexmap is their source: a body set from a file is an ordinary
// update, and `doc get` writes a body back out.
func documentGroup() *command {
	return &command{name: "doc", summary: "documents: protocols, schemas and other texts that define the product",
		subs: []*command{
			documentListCommand(), documentShowCommand(), documentCreateCommand(), documentUpdateCommand(),
			documentDeleteCommand(), documentLinkCommand(false), documentLinkCommand(true),
			documentRevisionsCommand(), documentDiffCommand(), documentRestoreCommand(), documentGetCommand(),
		}}
}

func emitDocument(a *app, d *ops.DocumentDetail) error {
	return a.emit(d, func(w io.Writer, _ output.Format) error { return render.Document(w, d) })
}

func documentListCommand() *command {
	var q ops.DocumentQuery
	return &command{
		name:    "doc list",
		summary: "list documents",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&q.State, "state", "", "draft, current or superseded")
			fs.StringVar(&q.Format, "format", "", "markdown, text, json, yaml, xml, proto, sql or graphql")
			fs.StringVar(&q.Search, "search", "", "text in title or identifier")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("doc list", args); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			items, err := s.ListDocuments(ctx, q)
			if err != nil {
				return usageIfInput(err)
			}
			return a.emit(items, func(w io.Writer, f output.Format) error { return render.Documents(w, f, items) })
		},
	}
}

func documentShowCommand() *command {
	return &command{
		name:    "doc show",
		args:    "<DOC-3|ID>",
		summary: "show a document with its content, links and comments",
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg("doc show", "<identifier|ID>", args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.ShowDocument(ctx, ref)
			if err != nil {
				return err
			}
			return emitDocument(a, d)
		},
	}
}

func documentCreateCommand() *command {
	var in ops.NewDocument
	var body textFlags
	return &command{
		name:    "doc create",
		summary: "create a document (owner role)",
		flags: func(fs *flag.FlagSet) {
			fs.StringVar(&in.Title, "title", "", "title (required)")
			fs.StringVar(&in.Format, "format", "", "markdown (default), text, json, yaml, xml, proto, sql or graphql")
			fs.StringVar(&in.State, "state", "", "draft (default), current or superseded")
			fs.StringVar(&in.Component, "component", "", "component the document belongs to")
			fs.StringVar(&in.Capability, "capability", "", "capability the document belongs to")
			fs.StringVar(&in.Milestone, "milestone", "", "milestone")
			body.bind(fs, "body", "the content, stored byte for byte")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("doc create", args); err != nil {
				return err
			}
			if in.Title == "" {
				return usageErr(errors.New("--title is required"))
			}
			var err error
			if in.Body, err = body.value(a, "body"); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.CreateDocument(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return emitDocument(a, d)
		},
	}
}

func documentUpdateCommand() *command {
	var title, format, state, component, capability, milestone, supersededBy string
	var body textFlags
	var set flagsSet
	return &command{
		name:    "doc update",
		args:    "<DOC-3|ID>",
		summary: "change a document; new content becomes a new revision (owner role)",
		flags: func(fs *flag.FlagSet) {
			set.bind(fs)
			fs.StringVar(&title, "title", "", "new title")
			fs.StringVar(&format, "format", "", "new format")
			fs.StringVar(&state, "state", "", "draft, current or superseded")
			fs.StringVar(&supersededBy, "superseded-by", "", "the replacing document, e.g. DOC-4 (with --state superseded)")
			fs.StringVar(&component, "component", "", "component (empty clears it)")
			fs.StringVar(&capability, "capability", "", "capability (empty clears it)")
			fs.StringVar(&milestone, "milestone", "", "milestone (empty clears it)")
			body.bind(fs, "body", "new content, replacing the whole body")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) != 1 {
				return usageErr(errors.New("usage: nexmap doc update <DOC-3|ID> [--body-file FILE] [--state …] [--title …] …"))
			}
			in := ops.DocumentChange{Ref: args[0], Title: set.optional("title", title), Format: set.optional("format", format),
				State: set.optional("state", state), SupersededBy: set.optional("superseded-by", supersededBy),
				Component: set.optional("component", component), Capability: set.optional("capability", capability),
				Milestone: set.optional("milestone", milestone)}
			var err error
			if in.Body, err = set.optionalText(a, &body, "body"); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.UpdateDocument(ctx, in)
			if err != nil {
				return usageIfInput(err)
			}
			return emitDocument(a, d)
		},
	}
}

func documentDeleteCommand() *command {
	var yes bool
	return &command{
		name:    "doc delete",
		args:    "<DOC-3|ID>",
		summary: "delete a document with all its revisions (owner role)",
		flags:   func(fs *flag.FlagSet) { fs.BoolVar(&yes, "yes", false, "confirm the deletion") },
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg("doc delete", "<identifier|ID> --yes", args)
			if err != nil {
				return err
			}
			if err := confirmDelete("a document and all its revisions", yes); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.DeleteDocument(ctx, ref)
			if err != nil {
				return err
			}
			return emitDeleted(a, "document", &ops.Ref{ID: d.ID, Identifier: d.Identifier, Title: d.Title})
		},
	}
}

func documentLinkCommand(unlink bool) *command {
	verb, summary := "link", "link a document to requirements, constraints or test cases (owner role)"
	if unlink {
		verb, summary = "unlink", "remove a document's links to requirements, constraints or test cases (owner role)"
	}
	name := "doc " + verb
	return &command{
		name:    name,
		args:    "<DOC-3> <FR-12|SC-3|test>…",
		summary: summary,
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) < 2 {
				return usageErr(fmt.Errorf("usage: nexmap %s <DOC-3> <FR-12|SC-3|test identifier>…", name))
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.LinkDocument(ctx, ops.DocumentLinks{Document: args[0], Items: args[1:], Unlink: unlink})
			if err != nil {
				return usageIfInput(err)
			}
			return emitDocument(a, d)
		},
	}
}

func documentRevisionsCommand() *command {
	return &command{
		name:    "doc revisions",
		args:    "<DOC-3|ID>",
		summary: "list a document's revisions",
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg("doc revisions", "<identifier|ID>", args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			revs, err := s.DocumentRevisions(ctx, ref)
			if err != nil {
				return err
			}
			return a.emit(revs, func(w io.Writer, f output.Format) error { return render.Revisions(w, f, revs) })
		},
	}
}

func documentDiffCommand() *command {
	var q ops.DiffQuery
	return &command{
		name:    "doc diff",
		args:    "<DOC-3|ID>",
		summary: "compare two revisions of a document (default: the latest with the one before)",
		flags: func(fs *flag.FlagSet) {
			fs.IntVar(&q.From, "from", 0, "older revision")
			fs.IntVar(&q.To, "to", 0, "newer revision")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg("doc diff", "<identifier|ID> [--from N] [--to N]", args)
			if err != nil {
				return err
			}
			q.Ref = ref
			s, err := a.session()
			if err != nil {
				return err
			}
			v, err := s.DocumentDiff(ctx, q)
			if err != nil {
				return err
			}
			return a.emit(v, func(w io.Writer, _ output.Format) error { return render.Diff(w, v) })
		},
	}
}

func documentRestoreCommand() *command {
	return &command{
		name:    "doc restore",
		args:    "<DOC-3|ID> <revision>",
		summary: "make an earlier revision current again, as a new revision (owner role)",
		run: func(ctx context.Context, a *app, args []string) error {
			if len(args) != 2 {
				return usageErr(errors.New("usage: nexmap doc restore <DOC-3|ID> <revision>"))
			}
			rev, err := strconv.Atoi(args[1])
			if err != nil || rev < 1 {
				return usageErr(fmt.Errorf("%q is not a revision number", args[1]))
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			d, err := s.RestoreDocument(ctx, ops.RestoreRevision{Ref: args[0], Revision: rev})
			if err != nil {
				return usageIfInput(err)
			}
			return emitDocument(a, d)
		},
	}
}

// documentFileView is `doc get`'s -o json output.
type documentFileView struct {
	Name  string `json:"name"`
	File  string `json:"file,omitempty"`
	Bytes int    `json:"bytes"`
	Body  string `json:"body,omitempty"` // without --file
}

func documentGetCommand() *command {
	var revision int
	var file string
	return &command{
		name:    "doc get",
		args:    "<DOC-3|ID>",
		summary: "print a document's content byte for byte, or write it to a file",
		flags: func(fs *flag.FlagSet) {
			fs.IntVar(&revision, "revision", 0, "an earlier revision instead of the current content")
			fs.StringVar(&file, "file", "", "write to this file instead of stdout")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			ref, err := oneArg("doc get", "<identifier|ID> [--revision N] [--file PATH]", args)
			if err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			f, err := s.DocumentFileOf(ctx, ref, revision)
			if err != nil {
				return err
			}
			view := documentFileView{Name: f.Name, Bytes: len(f.Body)}
			if file == "" {
				if a.format == output.JSON {
					view.Body = f.Body
					return output.WriteJSON(a.io.Stdout, view)
				}
				_, err := io.WriteString(a.io.Stdout, f.Body)
				return err
			}
			path := file
			if !filepath.IsAbs(path) {
				path = filepath.Join(a.io.Cwd, path)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
			}
			if err := writeFileAtomic(path, []byte(f.Body)); err != nil {
				return err
			}
			view.File = path
			if a.format == output.JSON {
				return output.WriteJSON(a.io.Stdout, view)
			}
			_, err = fmt.Fprintf(a.io.Stdout, "%s written to %s\n", f.Name, path)
			return err
		},
	}
}
