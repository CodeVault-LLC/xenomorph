//go:build windows

package agent

import (
	"os"
	"path/filepath"
	"strings"
)

func systemDiskPath() string {
	volume := filepath.VolumeName(os.Getenv("SystemRoot"))
	if len(volume) == 2 && volume[1] == ':' {
		return strings.ToUpper(volume) + `\`
	}
	return `C:\`
}
