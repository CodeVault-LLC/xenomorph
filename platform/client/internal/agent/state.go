package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RuntimeState tracks agent state that persists across restarts.
type RuntimeState struct {
	OnboardingSent bool `json:"onboarding_sent"`
}

// DefaultStatePath returns the default path for the runtime state file.
func DefaultStatePath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}

	return filepath.Join(homeDir, ".xenomorph", "agent-state.json"), nil
}

// LoadRuntimeState reads the runtime state from disk.
func LoadRuntimeState(path string) (RuntimeState, error) {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if os.IsNotExist(err) {
			return RuntimeState{}, nil
		}
		return RuntimeState{}, fmt.Errorf("read runtime state: %w", err)
	}

	var state RuntimeState
	if err := json.Unmarshal(data, &state); err != nil {
		return RuntimeState{}, fmt.Errorf("decode runtime state: %w", err)
	}

	return state, nil
}

// SaveRuntimeState persists the runtime state to disk.
func SaveRuntimeState(path string, state RuntimeState) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, stateDirPermission); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode runtime state: %w", err)
	}

	if err := os.WriteFile(filepath.Clean(path), data, stateFilePermission); err != nil {
		return fmt.Errorf("write runtime state: %w", err)
	}

	return nil
}
