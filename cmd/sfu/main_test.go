package main

import (
	"log/slog"
	"testing"
)

func TestParsedLogLevel(t *testing.T) {
	tests := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"info":  slog.LevelInfo,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		"other": slog.LevelInfo,
	}
	for input, want := range tests {
		if got := parsedLogLevel(input); got != want {
			t.Errorf("parsedLogLevel(%q) = %v, want %v", input, got, want)
		}
	}
}
