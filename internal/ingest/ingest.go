// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Package ingest turns test runner reports into test results for upload to a
// Wiekko campaign (FR-118). Format-specific parsers live in subpackages; this
// package holds the shared result type, format detection and the dispatcher.
//
// It is part of the CLI and must not import server packages.
package ingest

import (
	"bytes"
	"fmt"
	"io"
	"unicode/utf8"

	"wiekko.net/internal/ingest/gotest"
	"wiekko.net/internal/ingest/junit"
	"wiekko.net/internal/ingest/report"
)

// Result and Report are shared by all parsers.
type (
	Result = report.Result
	Report = report.Report
)

// Result statuses.
const (
	Passed  = report.Passed
	Failed  = report.Failed
	Skipped = report.Skipped
)

// Formats.
const (
	FormatAuto   = "auto"
	FormatGoJSON = "go-json"
	FormatGoText = "go-text"
	FormatJUnit  = "junit"
)

// Formats lists the accepted --format values.
var Formats = []string{FormatAuto, FormatGoJSON, FormatGoText, FormatJUnit}

// Identifier styles.
const (
	// StylePackage qualifies identifiers: "pkg/path.TestName" for Go,
	// "classname.name" for JUnit.
	StylePackage = "package"
	// StyleName uses the bare test name: "TestName", "name".
	StyleName = "name"
)

// MaxOutput is how much captured output of one failing test is uploaded:
// the server keeps no more than this (its tail) in the defect.
const MaxOutput = 64 << 10

// Options control parsing.
type Options struct {
	Format          string // one of Formats; "" means auto
	IdentifierStyle string // StylePackage (default) or StyleName
}

// Source names the format for the server ("source" of the run).
func Source(format string) string {
	switch format {
	case FormatGoJSON, FormatGoText:
		return "go-test"
	default:
		return format
	}
}

// Detect guesses the format from the first non-blank bytes: JSON events are
// `go test -json`, XML is JUnit, anything else is `go test -v` text.
func Detect(data []byte) string {
	trimmed := bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // UTF-8 byte order mark
	trimmed = bytes.TrimLeft(trimmed, " \t\r\n")
	switch {
	case bytes.HasPrefix(trimmed, []byte("{")):
		return FormatGoJSON
	case bytes.HasPrefix(trimmed, []byte("<")):
		return FormatJUnit
	default:
		return FormatGoText
	}
}

// Parse parses one report. The returned format is the one actually used.
func Parse(r io.Reader, opts Options) (*Report, string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", err
	}
	format := opts.Format
	if format == "" || format == FormatAuto {
		format = Detect(data)
	}
	qualified := opts.IdentifierStyle != StyleName
	var rep *Report
	switch format {
	case FormatGoJSON:
		rep, err = gotest.ParseJSON(bytes.NewReader(data), qualified)
	case FormatGoText:
		rep, err = gotest.ParseText(bytes.NewReader(data), qualified)
	case FormatJUnit:
		rep, err = junit.Parse(bytes.NewReader(data), qualified)
	default:
		return nil, "", fmt.Errorf("unknown format %q (use %v)", format, Formats)
	}
	if err != nil {
		return nil, format, fmt.Errorf("parse %s: %w", format, err)
	}
	for i := range rep.Results {
		rep.Results[i].Output = Truncate(rep.Results[i].Output)
	}
	return rep, format, nil
}

// Truncate keeps the last MaxOutput bytes of s (at a rune boundary).
func Truncate(s string) string {
	if len(s) <= MaxOutput {
		return s
	}
	s = s[len(s)-MaxOutput:]
	for len(s) > 0 && !utf8.RuneStart(s[0]) {
		s = s[1:]
	}
	return s
}
