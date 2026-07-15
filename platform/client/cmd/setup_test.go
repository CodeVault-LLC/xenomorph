package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveLegacyRuntimeStateAt(t *testing.T) {
	t.Parallel()
	homeDir := t.TempDir()

	stateDir := filepath.Join(homeDir, ".xenomorph")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatalf("create legacy state directory: %v", err)
	}

	statePath := filepath.Join(stateDir, "agent-state.json")
	if err := os.WriteFile(statePath, []byte(`{"onboarding_sent":true}`), 0o600); err != nil {
		t.Fatalf("write legacy state: %v", err)
	}

	if err := removeLegacyRuntimeStateAt(homeDir); err != nil {
		t.Fatalf("removeLegacyRuntimeStateAt() error = %v", err)
	}

	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy state remains or cannot be checked: %v", err)
	}
}

func TestRemoveLegacyRuntimeStateAtMissingFile(t *testing.T) {
	t.Parallel()

	if err := removeLegacyRuntimeStateAt(t.TempDir()); err != nil {
		t.Fatalf("removeLegacyRuntimeStateAt() error = %v", err)
	}
}
