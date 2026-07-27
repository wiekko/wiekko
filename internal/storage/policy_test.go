package storage

import (
	"errors"
	"strings"
	"testing"
)

var (
	pngBytes  = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 13}
	jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10, 'J', 'F', 'I', 'F'}
	zipBytes  = []byte{'P', 'K', 0x03, 0x04, 0, 0, 0, 0}
	pdfBytes  = []byte("%PDF-1.4\n%hello")
	gzipBytes = []byte{0x1F, 0x8B, 0x08, 0, 0, 0, 0, 0}
	logBytes  = []byte("2024-01-01T00:00:00Z INFO service started\n")
	jsonBytes = []byte(`{"level":"info","msg":"started"}`)
	xmlBytes  = []byte(`<?xml version="1.0"?><log><entry/></log>`)
	elfBytes  = []byte{0x7F, 'E', 'L', 'F', 2, 1, 1, 0}
)

func TestPolicyUnrestrictedAllowsAnything(t *testing.T) {
	p := NewPolicy("")
	if p.Restricted() {
		t.Fatal("an empty spec must not restrict uploads")
	}
	for _, name := range []string{"evil.exe", "notes.log", "shot.png", "noextension"} {
		if err := p.Check(name, elfBytes); err != nil {
			t.Fatalf("Check(%q) = %v, want nil", name, err)
		}
	}
}

func TestPolicyPresetsExpand(t *testing.T) {
	p := NewPolicy("images, logs")
	got := strings.Join(p.AllowedExtensions(), ",")
	for _, want := range []string{".png", ".jpeg", ".log", ".json", ".yml"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expanded set %q missing %q", got, want)
		}
	}
	if strings.Contains(got, ".pdf") {
		t.Fatalf("documents preset leaked into %q", got)
	}
}

func TestPolicyAcceptsBareAndDottedExtensions(t *testing.T) {
	p := NewPolicy("png, .log")
	if err := p.Check("shot.png", pngBytes); err != nil {
		t.Fatalf("bare extension not honoured: %v", err)
	}
	if err := p.Check("app.log", logBytes); err != nil {
		t.Fatalf("dotted extension not honoured: %v", err)
	}
}

func TestPolicyRejectsDisallowedExtension(t *testing.T) {
	p := NewPolicy("images")
	err := p.Check("notes.log", logBytes)
	if !errors.Is(err, ErrTypeNotAllowed) {
		t.Fatalf("err = %v, want ErrTypeNotAllowed", err)
	}
	// The message should tell the user what is permitted.
	if !strings.Contains(err.Error(), ".png") {
		t.Fatalf("error does not name the allowed types: %v", err)
	}
}

func TestPolicyRejectsMissingExtension(t *testing.T) {
	p := NewPolicy("logs")
	if err := p.Check("README", logBytes); !errors.Is(err, ErrTypeNotAllowed) {
		t.Fatalf("err = %v, want ErrTypeNotAllowed", err)
	}
}

func TestPolicyExtensionCheckIsCaseInsensitive(t *testing.T) {
	p := NewPolicy("images")
	if err := p.Check("Screenshot.PNG", pngBytes); err != nil {
		t.Fatalf("uppercase extension rejected: %v", err)
	}
}

// Image formats have reliable signatures, so mislabeling is always caught.
func TestPolicyRejectsMislabeledImages(t *testing.T) {
	p := NewPolicy("images")

	for _, tc := range []struct {
		name    string
		content []byte
	}{
		{"archive renamed to png", zipBytes},
		{"text renamed to png", logBytes},
		{"jpeg renamed to png", jpegBytes},
		{"executable renamed to png", elfBytes},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := p.Check("payload.png", tc.content); !errors.Is(err, ErrTypeNotAllowed) {
				t.Fatalf("err = %v, want ErrTypeNotAllowed", err)
			}
		})
	}

	if err := p.Check("real.png", pngBytes); err != nil {
		t.Fatalf("a genuine PNG was rejected: %v", err)
	}
	if err := p.Check("real.jpg", jpegBytes); err != nil {
		t.Fatalf("a genuine JPEG was rejected: %v", err)
	}
}

// Text extensions accept anything that is not a recognizably different format.
func TestPolicyTextExtensions(t *testing.T) {
	p := NewPolicy("logs")

	for _, tc := range []struct {
		name    string
		file    string
		content []byte
	}{
		{"plain log", "app.log", logBytes},
		{"json", "trace.json", jsonBytes},
		{"xml", "report.xml", xmlBytes},
		{"empty file", "empty.log", nil},
	} {
		t.Run(tc.name+" is accepted", func(t *testing.T) {
			if err := p.Check(tc.file, tc.content); err != nil {
				t.Fatalf("Check = %v, want nil", err)
			}
		})
	}

	for _, tc := range []struct {
		name    string
		content []byte
	}{
		{"zip renamed to log", zipBytes},
		{"pdf renamed to log", pdfBytes},
		{"gzip renamed to log", gzipBytes},
	} {
		t.Run(tc.name+" is rejected", func(t *testing.T) {
			if err := p.Check("payload.log", tc.content); !errors.Is(err, ErrTypeNotAllowed) {
				t.Fatalf("err = %v, want ErrTypeNotAllowed", err)
			}
		})
	}
}

// Documents the known gap: Go's sniffer has no ELF or PE signature, so a
// renamed executable is indistinguishable from a log in an odd encoding.
// If this ever starts failing, the sniffer gained a signature and the policy
// doc comment should be updated.
func TestPolicyDoesNotCatchRenamedExecutables(t *testing.T) {
	p := NewPolicy("logs")
	if err := p.Check("payload.log", elfBytes); err != nil {
		t.Fatalf("behaviour changed — an ELF binary named .log is now rejected (%v); update the Policy doc comment", err)
	}
}

// Extensions outside the known families are allow-listed by name only.
func TestPolicyUnknownFamilyChecksExtensionOnly(t *testing.T) {
	p := NewPolicy("bin")
	if err := p.Check("firmware.bin", elfBytes); err != nil {
		t.Fatalf("Check = %v, want nil", err)
	}
	if err := p.Check("firmware.exe", elfBytes); !errors.Is(err, ErrTypeNotAllowed) {
		t.Fatalf("err = %v, want ErrTypeNotAllowed", err)
	}
}
