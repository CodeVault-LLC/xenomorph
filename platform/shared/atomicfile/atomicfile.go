// Package atomicfile provides owner-scoped, synchronized state-file writes.
package atomicfile

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Replace writes data to a synchronized temporary file, atomically renames it
// over path, and synchronizes the containing directory where the OS exposes
// that durability primitive.
func Replace(path string, data []byte, directoryMode, fileMode fs.FileMode) error {
	cleaned, directory, err := preparePath(path, directoryMode)
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(cleaned)+"-*")
	if err != nil {
		return fmt.Errorf("create atomic temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := writeSynchronizedFile(temporary, data, fileMode); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, cleaned); err != nil {
		return fmt.Errorf("replace atomic state file: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("synchronize atomic state directory: %w", err)
	}
	committed = true
	return nil
}

// Create writes and synchronizes a new file without replacing an existing
// identity or authentication key.
func Create(path string, data []byte, directoryMode, fileMode fs.FileMode) error {
	cleaned, directory, err := preparePath(path, directoryMode)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(cleaned, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode) // #nosec G304 -- callers supply validated immutable state paths.
	if err != nil {
		return fmt.Errorf("create atomic state file: %w", err)
	}
	if err := writeSynchronizedFile(file, data, fileMode); err != nil {
		_ = os.Remove(cleaned)
		return err
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("synchronize atomic state directory: %w", err)
	}
	return nil
}

func preparePath(path string, directoryMode fs.FileMode) (string, string, error) {
	if strings.TrimSpace(path) == "" {
		return "", "", fmt.Errorf("prepare atomic state file: path is required")
	}
	cleaned := filepath.Clean(path)
	directory := filepath.Dir(cleaned)
	if err := os.MkdirAll(directory, directoryMode); err != nil {
		return "", "", fmt.Errorf("create atomic state directory: %w", err)
	}
	return cleaned, directory, nil
}

func writeSynchronizedFile(file *os.File, data []byte, mode fs.FileMode) error {
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return fmt.Errorf("protect atomic state file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write atomic state file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("synchronize atomic state file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close atomic state file: %w", err)
	}
	return nil
}
