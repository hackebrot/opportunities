package log_test

import (
	"testing"

	oppslog "github.com/hackebrot/opportunities/internal/log"
)

func TestNewAcceptsKnownLevels(t *testing.T) {
	t.Parallel()

	for _, level := range []string{"debug", "info", "warn", "error"} {
		t.Run(level, func(t *testing.T) {
			t.Parallel()

			logger, err := oppslog.New(level)
			if err != nil {
				t.Fatalf("New(%q): %v", level, err)
			}
			if logger == nil {
				t.Fatalf("New(%q): nil logger", level)
			}
		})
	}
}

func TestNewEmptyLevelDefaultsToInfo(t *testing.T) {
	t.Parallel()

	logger, err := oppslog.New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if logger == nil {
		t.Fatal("New: nil logger")
	}
}

func TestNewRejectsUnknownLevel(t *testing.T) {
	t.Parallel()

	logger, err := oppslog.New("verbose")
	if err == nil {
		t.Fatal("New(\"verbose\"): expected error, got nil")
	}
	if logger != nil {
		t.Errorf("New(\"verbose\"): expected nil logger on error, got %v", logger)
	}
}
