package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// RuntimeState tracks agent state that persists across restarts.
type RuntimeState struct {
	OnboardingSent    bool     `json:"onboarding_sent"`
	SeenCommandNonces []string `json:"seen_command_nonces,omitempty"`
}

// RecordCommandNonce appends a verified nonce and evicts the oldest replay
// record when the bounded persistent history reaches its limit.
func (s *RuntimeState) RecordCommandNonce(nonce string) {
	if s == nil || nonce == "" {
		return
	}
	s.SeenCommandNonces = append(s.SeenCommandNonces, nonce)
	if overflow := len(s.SeenCommandNonces) - maxSeenCommandNonces; overflow > 0 {
		s.SeenCommandNonces = append([]string(nil), s.SeenCommandNonces[overflow:]...)
	}
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

	temporary, err := os.CreateTemp(dir, ".agent-state-*")
	if err != nil {
		return fmt.Errorf("create temporary state file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(stateFilePermission); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set temporary state permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary state: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary state: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary state: %w", err)
	}
	if err := os.Rename(temporaryPath, filepath.Clean(path)); err != nil {
		return fmt.Errorf("write runtime state: %w", err)
	}

	return nil
}
