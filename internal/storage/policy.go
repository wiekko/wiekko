package storage

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
)

// ErrTypeNotAllowed is returned when an upload's type is not permitted.
var ErrTypeNotAllowed = fmt.Errorf("file type is not allowed")

// SniffLen is how many leading bytes are inspected to identify a file.
const SniffLen = 512

// Presets that can be named in the allow-list instead of listing extensions.
var presets = map[string][]string{
	"images":    {".png", ".jpg", ".jpeg", ".gif", ".webp"},
	"logs":      {".txt", ".log", ".json", ".xml", ".csv", ".md", ".yaml", ".yml"},
	"documents": {".pdf"},
}

// family classifies what a given extension is expected to contain.
type family int

const (
	familyOther family = iota
	familyImage
	familyText
)

var extFamily = map[string]family{
	".png": familyImage, ".jpg": familyImage, ".jpeg": familyImage,
	".gif": familyImage, ".webp": familyImage,
	".txt": familyText, ".log": familyText, ".json": familyText,
	".xml": familyText, ".csv": familyText, ".md": familyText,
	".yaml": familyText, ".yml": familyText,
}

// expectedImageType maps an image extension to the type http.DetectContentType
// reports for it.
var expectedImageType = map[string]string{
	".png": "image/png", ".gif": "image/gif", ".webp": "image/webp",
	".jpg": "image/jpeg", ".jpeg": "image/jpeg",
}

// Policy restricts which files may be uploaded.
//
// It combines an extension allow-list with a magic-byte check, which is a
// meaningful but not airtight control:
//
//   - Image extensions are verified strictly. Image formats have reliable
//     signatures, so a non-image renamed to .png is always rejected.
//   - Text extensions only reject content that sniffs as a recognized binary
//     or rich format (zip, gzip, pdf, audio, video, …). Go's sniffer has no
//     signature for ELF or PE executables — both report
//     application/octet-stream, which is indistinguishable from a log file in
//     an unusual encoding. A renamed executable can therefore still be stored
//     as example.log.
//
// The residual risk is bounded: uploads are always served with
// Content-Disposition: attachment and X-Content-Type-Options: nosniff, so a
// stored file is never executed or rendered by the application. This control
// stops casual mislabeling and unwanted file classes; it is not anti-malware.
type Policy struct {
	// extensions is the set of allowed lowercase extensions, including the
	// leading dot. A nil or empty set allows everything.
	extensions map[string]bool
}

// NewPolicy builds a policy from a comma-separated spec of preset names
// ("images", "logs", "documents") and/or explicit extensions ("png", ".log").
// An empty spec allows every file type.
func NewPolicy(spec string) Policy {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return Policy{}
	}

	allowed := map[string]bool{}
	for _, raw := range strings.Split(spec, ",") {
		token := strings.ToLower(strings.TrimSpace(raw))
		if token == "" {
			continue
		}
		if exts, ok := presets[token]; ok {
			for _, e := range exts {
				allowed[e] = true
			}
			continue
		}
		if !strings.HasPrefix(token, ".") {
			token = "." + token
		}
		allowed[token] = true
	}
	if len(allowed) == 0 {
		return Policy{}
	}
	return Policy{extensions: allowed}
}

// Restricted reports whether the policy limits uploads at all.
func (p Policy) Restricted() bool { return len(p.extensions) > 0 }

// AllowedExtensions returns the permitted extensions, sorted, for error
// messages and help text.
func (p Policy) AllowedExtensions() []string {
	out := make([]string, 0, len(p.extensions))
	for e := range p.extensions {
		out = append(out, e)
	}
	// Small, stable ordering for predictable messages.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Check validates a filename and the leading bytes of its content.
// head may be shorter than SniffLen for small files.
func (p Policy) Check(filename string, head []byte) error {
	if !p.Restricted() {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" || !p.extensions[ext] {
		return fmt.Errorf("%w: %s", ErrTypeNotAllowed, p.describe())
	}

	sniffed := http.DetectContentType(head)
	switch extFamily[ext] {
	case familyImage:
		// Reliable signatures: demand an exact match.
		if want := expectedImageType[ext]; !strings.HasPrefix(sniffed, want) {
			return fmt.Errorf("%w: %s is not a valid %s file", ErrTypeNotAllowed, filename, strings.TrimPrefix(ext, "."))
		}
	case familyText:
		// Cannot positively identify text, so reject only content that is
		// recognizably something else.
		if !strings.HasPrefix(sniffed, "text/") && sniffed != "application/octet-stream" {
			return fmt.Errorf("%w: %s contains %s data", ErrTypeNotAllowed, filename, sniffed)
		}
	}
	return nil
}

func (p Policy) describe() string {
	return "allowed types: " + strings.Join(p.AllowedExtensions(), ", ")
}
