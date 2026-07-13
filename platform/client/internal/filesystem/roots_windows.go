//go:build windows

package filesystem

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

func filesystemRoots() ([]rootDefinition, error) {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		return nil, fmt.Errorf("enumerate logical drives: %w", err)
	}
	roots := make([]rootDefinition, 0, 26)
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
