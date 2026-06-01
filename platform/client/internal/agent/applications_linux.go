//go:build linux

package agent

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func collectInstalledApplications() []string {
	homeDir, _ := os.UserHomeDir()
	dirs := []string{
		"/usr/share/applications",
		"/usr/local/share/applications",
		filepath.Join(homeDir, ".local", "share", "applications"),
	}

	seen := make(map[string]struct{})
	apps := make([]string, 0, maxInstalledApps)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if len(apps) >= maxInstalledApps {
				break
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".desktop" {
				continue
			}

			desktopPath := filepath.Clean(filepath.Join(dir, entry.Name()))
			name := parseDesktopName(desktopPath)
			name = strings.TrimSpace(name)
			if name == "" {
				name = strings.TrimSuffix(entry.Name(), ".desktop")
			}

			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			apps = append(apps, name)
		}
	}

	sort.Strings(apps)
	if len(apps) > maxInstalledApps {
		apps = apps[:maxInstalledApps]
	}

	return apps
}

func parseDesktopName(path string) string {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return ""
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Name=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Name="))
		}
	}

	return ""
}
