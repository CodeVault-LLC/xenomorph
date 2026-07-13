//go:build darwin

package agent

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func scanInstalledApplications() []string {
	homeDir, _ := os.UserHomeDir()
	dirs := []string{"/Applications", "/System/Applications", filepath.Join(homeDir, "Applications")}
	seen := make(map[string]struct{})
	applications := make([]string, 0, maxInstalledApps)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if len(applications) >= maxInstalledApps {
				break
			}
			if !entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".app") {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			applications = append(applications, name)
		}
	}
	sort.Strings(applications)
	return applications
}
