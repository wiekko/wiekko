// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Package output renders command results as a human table, markdown, or JSON.
// JSON output is the stable, machine-readable contract; table and markdown are
// for people and for agent context windows respectively.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

// Format is an output format.
type Format string

// Supported formats.
const (
	Table    Format = "table"
	Markdown Format = "md"
	JSON     Format = "json"
)

// ParseFormat validates a format name. An empty name selects Table.
func ParseFormat(s string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "table", "text":
		return Table, nil
	case "md", "markdown":
		return Markdown, nil
	case "json":
		return JSON, nil
	}
	return "", fmt.Errorf("unknown output format %q (want table, md or json)", s)
}

// Field is one labelled value of a record.
type Field struct {
	Label string
	Value string
}

// Record renders a single object: fields for table and markdown, data for
// JSON. Empty field values are shown as "-" in table and markdown.
func Record(w io.Writer, f Format, title string, fields []Field, data any) error {
	switch f {
	case JSON:
		return WriteJSON(w, data)
	case Markdown:
		var b strings.Builder
		if title != "" {
			fmt.Fprintf(&b, "## %s\n\n", title)
		}
		for _, fl := range fields {
			fmt.Fprintf(&b, "- **%s:** %s\n", fl.Label, orDash(fl.Value))
		}
		_, err := io.WriteString(w, b.String())
		return err
	default:
		tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
		for _, fl := range fields {
			fmt.Fprintf(tw, "%s:\t%s\n", fl.Label, orDash(fl.Value))
		}
		return tw.Flush()
	}
}

// WriteJSON writes v as indented JSON followed by a newline.
func WriteJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
