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

	"nexmap.net/internal/cli/ops"
	"nexmap.net/internal/cli/output"
)

// exportView is export's -o json output.
type exportView struct {
	ProductID string `json:"product_id"`
	Tasks     bool   `json:"tasks"`
	SHA256    string `json:"sha256"`             // hash of the document, to compare with a committed copy
	File      string `json:"file,omitempty"`     // where it was written, with --file
	Markdown  string `json:"markdown,omitempty"` // the document, without --file
	// Documents lists the document files written, with --documents.
	Documents []string `json:"documents,omitempty"`
}

func exportCommand() *command {
	var tasks, drafts bool
	var file, documents string
	return &command{
		name:    "export",
		summary: "export the product structure, documents, constraints and requirements as a Markdown specification",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&tasks, "tasks", false, "include tasks")
			fs.BoolVar(&drafts, "drafts", false, "include draft documents")
			fs.StringVar(&file, "file", "", "write the specification to this file instead of stdout")
			fs.StringVar(&documents, "documents", "", "write each listed document's content to its own file in this directory (FR-139)")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("export", args); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			doc, spec, err := s.Specification(ctx, ops.SpecificationQuery{Tasks: tasks, Drafts: drafts})
			if err != nil {
				return err
			}
			view := exportView{ProductID: s.ProductID, Tasks: tasks, SHA256: spec.SHA256}
			if documents != "" {
				if view.Documents, err = writeDocumentFiles(ctx, a, s, documents, drafts); err != nil {
					return err
				}
			}
			if file == "" {
				if a.format == output.JSON {
					view.Markdown = doc
					return output.WriteJSON(a.io.Stdout, view)
				}
				_, err := io.WriteString(a.io.Stdout, doc)
				return err
			}
			path := file
			if !filepath.IsAbs(path) {
				path = filepath.Join(a.io.Cwd, path)
			}
			if err := writeFileAtomic(path, []byte(doc)); err != nil {
				return err
			}
			view.File = path
			if a.format == output.JSON {
				return output.WriteJSON(a.io.Stdout, view)
			}
			_, err = fmt.Fprintf(a.io.Stdout, "Specification written to %s\n", path)
			return err
		},
	}
}

// writeDocumentFiles writes the bodies of the documents the specification
// lists into dir, byte for byte, one file each (FR-139). File names come from
// ops.ExportedDocumentFiles, which checks them against the identifier
// pattern, and are joined to dir only after filepath.Base: nothing the server
// sends can place a file outside dir.
func writeDocumentFiles(ctx context.Context, a *app, s *ops.Session, dir string, drafts bool) ([]string, error) {
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(a.io.Cwd, dir)
	}
	files, err := s.ExportedDocumentFiles(ctx, drafts)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	written := []string{}
	for _, f := range files {
		name := filepath.Base(f.Name)
		if name != f.Name {
			return nil, fmt.Errorf("refusing to write a document as %q", f.Name)
		}
		path := filepath.Join(dir, name)
		if err := writeFileAtomic(path, []byte(f.Body)); err != nil {
			return nil, err
		}
		written = append(written, path)
	}
	if a.format != output.JSON && len(written) > 0 {
		fmt.Fprintf(a.io.Stderr, "%d document file(s) written to %s\n", len(written), dir)
	}
	return written, nil
}

// writeFileAtomic replaces path in one step, so an interrupted export never
// leaves a half-written specification in the repository.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
