package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"nexmap.net/internal/cli/output"
)

// exportView is export's -o json output.
type exportView struct {
	ProductID string `json:"product_id"`
	Tasks     bool   `json:"tasks"`
	File      string `json:"file,omitempty"`     // where it was written, with --file
	Markdown  string `json:"markdown,omitempty"` // the document, without --file
}

func exportCommand() *command {
	var tasks bool
	var file string
	return &command{
		name:    "export",
		summary: "export the product structure, constraints and requirements as a Markdown specification",
		flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&tasks, "tasks", false, "include tasks")
			fs.StringVar(&file, "file", "", "write the specification to this file instead of stdout")
		},
		run: func(ctx context.Context, a *app, args []string) error {
			if err := noArgs("export", args); err != nil {
				return err
			}
			s, err := a.session()
			if err != nil {
				return err
			}
			doc, err := s.C.Specification(ctx, s.ProductID, tasks)
			if err != nil {
				return err
			}
			view := exportView{ProductID: s.ProductID, Tasks: tasks}
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
