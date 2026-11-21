// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"wiekko.net/internal/service"
	"wiekko.net/internal/storage"
	"wiekko.net/web/templates/patterns"
)

const testMaxUpload = 10 << 20 // 10 MiB

func TestDiscussionErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantIn     string
	}{
		{
			name:       "forbidden",
			err:        fmt.Errorf("%w: nope", service.ErrForbidden),
			wantStatus: http.StatusForbidden,
			wantIn:     "permission",
		},
		{
			name:       "not found",
			err:        fmt.Errorf("%w: comment", service.ErrNotFound),
			wantStatus: http.StatusNotFound,
			wantIn:     "no longer exists",
		},
		{
			name:       "missing blob is a not found",
			err:        storage.ErrNotFound,
			wantStatus: http.StatusNotFound,
			wantIn:     "no longer exists",
		},
		{
			name:       "too large names the limit",
			err:        storage.ErrTooLarge,
			wantStatus: http.StatusRequestEntityTooLarge,
			wantIn:     "10.0 MB",
		},
		{
			name:       "unsupported type",
			err:        storage.ErrTypeNotAllowed,
			wantStatus: http.StatusUnsupportedMediaType,
			wantIn:     "not allowed",
		},
		{
			name:       "anything else",
			err:        errors.New("boom"),
			wantStatus: http.StatusUnprocessableEntity,
			wantIn:     "did not work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, msg := discussionError(tt.err, testMaxUpload)
			if status != tt.wantStatus {
				t.Fatalf("status = %d, want %d", status, tt.wantStatus)
			}
			if !strings.Contains(msg, tt.wantIn) {
				t.Fatalf("message %q does not contain %q", msg, tt.wantIn)
			}
		})
	}
}

// Every message must be safe to show a user: no wrapped Go error text, no
// internal identifiers leaking into the UI.
func TestDiscussionErrorMessagesAreUserFacing(t *testing.T) {
	leaky := fmt.Errorf("%w: sql: no rows in result set for table comments", service.ErrNotFound)
	_, msg := discussionError(leaky, testMaxUpload)
	for _, forbidden := range []string{"sql:", "no rows", "comments"} {
		if strings.Contains(msg, forbidden) {
			t.Fatalf("message leaks internals (%q): %s", forbidden, msg)
		}
	}
}

// The policy error carries the allow-list, which is genuinely useful, so that
// one is passed through rather than replaced with a generic sentence.
func TestUploadRejectionMessageKeepsTheAllowList(t *testing.T) {
	err := fmt.Errorf("%w: allowed types: .log, .png", storage.ErrTypeNotAllowed)
	msg := uploadRejectionMessage(err)
	if !strings.Contains(msg, ".log") || !strings.Contains(msg, ".png") {
		t.Fatalf("message dropped the allow-list: %s", msg)
	}
	if strings.HasSuffix(msg, "..") {
		t.Fatalf("message has a doubled full stop: %s", msg)
	}
}

func TestUploadRejectionMessageFallsBackWhenBare(t *testing.T) {
	msg := uploadRejectionMessage(storage.ErrTypeNotAllowed)
	if msg != "That file type is not allowed." {
		t.Fatalf("msg = %q, want the generic sentence", msg)
	}
}

func TestHumanizeBytesFormatting(t *testing.T) {
	tests := map[int64]string{
		0:        "0 B",
		512:      "512 B",
		1024:     "1.0 KB",
		10 << 20: "10.0 MB",
		1536:     "1.5 KB",
		3 << 30:  "3.0 GB",
		1023:     "1023 B",
	}
	for in, want := range tests {
		if got := patterns.HumanizeBytes(in); got != want {
			t.Fatalf("patterns.HumanizeBytes(%d) = %q, want %q", in, got, want)
		}
	}
}
