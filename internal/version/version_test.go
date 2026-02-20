package version

import (
	"testing"
)

func TestVersionDefaults(t *testing.T) {
	if Version != "dev" {
		t.Errorf("expected Version default 'dev', got %q", Version)
	}
	if GitCommit != "unknown" {
		t.Errorf("expected GitCommit default 'unknown', got %q", GitCommit)
	}
	if BuildDate != "unknown" {
		t.Errorf("expected BuildDate default 'unknown', got %q", BuildDate)
	}
}

func TestVersionString(t *testing.T) {
	want := "dev (commit: unknown, built: unknown)"
	got := String()
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}
