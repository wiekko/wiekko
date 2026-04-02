package logging

import (
	"log/slog"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input   string
		want    slog.Level
		wantErr bool
	}{
		{"debug", slog.LevelDebug, false},
		{"info", slog.LevelInfo, false},
		{"warn", slog.LevelWarn, false},
		{"warning", slog.LevelWarn, false},
		{"error", slog.LevelError, false},
		{"DEBUG", slog.LevelDebug, false},
		{"Info", slog.LevelInfo, false},
		{"ERROR", slog.LevelError, false},
		{"unknown", slog.LevelInfo, true},
		{"", slog.LevelInfo, true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseLevel(tc.input)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ParseLevel(%q) err = %v, wantErr = %v", tc.input, err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}
