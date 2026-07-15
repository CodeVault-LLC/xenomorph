//go:build linux || darwin

package filesystem

import (
	"os"
	"path/filepath"
)

const (
	homeFilesystemRootID = "home"
	unixFilesystemRootID = "filesystem"
)

func filesystemRoots() ([]rootDefinition, error) {
	roots := []rootDefinition{{ID: unixFilesystemRootID, Path: "/", DisplayLabel: "Filesystem"}}
	homeDir, err := os.UserHomeDir()

	if err != nil {
		return roots, err
	}

	if !filepath.IsAbs(homeDir) {
		return roots, nil
	}

	return append([]rootDefinition{{
		ID: homeFilesystemRootID, Path: filepath.Clean(homeDir), DisplayLabel: "Home",
	}}, roots...), nil
}
