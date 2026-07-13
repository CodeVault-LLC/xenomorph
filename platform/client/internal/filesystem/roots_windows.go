//go:build windows

package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

const homeFilesystemRootID = "home"

func filesystemRoots() ([]rootDefinition, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("enumerate logical drives: %w", err)
	}
	roots := make([]rootDefinition, 0, 27)
	if homeDir, homeErr := os.UserHomeDir(); homeErr == nil && filepath.IsAbs(homeDir) {
		roots = append(roots, rootDefinition{
			ID: homeFilesystemRootID, Path: filepath.Clean(homeDir), DisplayLabel: "Home",
		})
	}
	for index := 0; index < 26; index++ {
		if mask&(1<<index) == 0 {
			continue
		}
		letter := string(rune('A' + index))
		roots = append(roots, rootDefinition{
			ID:           "drive-" + strings.ToLower(letter),
			Path:         letter + `:\`,
			DisplayLabel: letter + ":",
		})
	}
	return roots, nil
}
