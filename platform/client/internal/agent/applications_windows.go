//go:build windows

package agent

import (
	"sort"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const uninstallRegistryPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

func scanInstalledApplications() []string {
	seen := make(map[string]struct{})
	applications := make([]string, 0, maxInstalledApps)
	locations := []struct {
		root registry.Key
		view uint32
	}{
		{root: registry.LOCAL_MACHINE, view: registry.WOW64_64KEY},
		{root: registry.LOCAL_MACHINE, view: registry.WOW64_32KEY},
		{root: registry.CURRENT_USER, view: registry.WOW64_64KEY},
		{root: registry.CURRENT_USER, view: registry.WOW64_32KEY},
	}
	for _, location := range locations {
		scanWindowsUninstallKey(location.root, location.view, seen, &applications)
		if len(applications) >= maxInstalledApps {
			break
		}
	}
	sort.Strings(applications)
	return applications
}

func scanWindowsUninstallKey(root registry.Key, view uint32, seen map[string]struct{}, applications *[]string) {
	key, err := registry.OpenKey(root, uninstallRegistryPath, registry.READ|view)
	if err != nil {
		return
	}
	defer func() { _ = key.Close() }()

	names, err := key.ReadSubKeyNames(maxInstalledApps)
	if err != nil {
		return
	}
	for _, subkeyName := range names {
		if len(*applications) >= maxInstalledApps {
			return
		}
		subkey, err := registry.OpenKey(key, subkeyName, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		displayName, _, readErr := subkey.GetStringValue("DisplayName")
		closeErr := subkey.Close()
		if readErr != nil || closeErr != nil {
			continue
		}
		displayName = strings.TrimSpace(displayName)
		if displayName == "" {
			continue
		}
		if _, ok := seen[displayName]; ok {
			continue
		}
		seen[displayName] = struct{}{}
		*applications = append(*applications, displayName)
	}
}
