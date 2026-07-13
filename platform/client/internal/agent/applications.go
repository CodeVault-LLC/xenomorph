package agent

import (
	"sort"
	"strings"
	"sync"
)

const maxApplicationTypes = 8

// ApplicationTypeUsage is a client-authored count of installed applications
// assigned to a bounded, allowlisted category.
type ApplicationTypeUsage struct {
	Category string `json:"category"`
	Count    uint32 `json:"count"`
}

// installedApplicationCache is process-lifetime static telemetry. The source
// scan is capped at maxInstalledApps, and callers receive a copy.
var installedApplicationCache = sync.OnceValue(scanInstalledApplications)

// applicationTypeCache keeps the bounded classification and sort work out of
// the heartbeat path. Callers receive a copy of the immutable cached slice.
var applicationTypeCache = sync.OnceValue(func() []ApplicationTypeUsage {
	return summarizeApplicationTypes(installedApplicationCache())
})

func collectInstalledApplications() []string {
	return append([]string(nil), installedApplicationCache()...)
}

func collectApplicationTypes() []ApplicationTypeUsage {
	return append([]ApplicationTypeUsage(nil), applicationTypeCache()...)
}

func summarizeApplicationTypes(applications []string) []ApplicationTypeUsage {
	counts := make(map[string]uint32, maxApplicationTypes)
	for _, application := range applications {
		counts[classifyApplication(application)]++
	}

	result := make([]ApplicationTypeUsage, 0, len(counts))
	for category, count := range counts {
		result = append(result, ApplicationTypeUsage{Category: category, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Category < result[j].Category
		}
		return result[i].Count > result[j].Count
	})
	if len(result) > maxApplicationTypes {
		result = result[:maxApplicationTypes]
	}
	return result
}

func classifyApplication(application string) string {
	name := strings.ToLower(application)
	categories := []struct {
		category string
		keywords []string
	}{
		{category: "Browsers", keywords: []string{"browser", "chrome", "chromium", "edge", "firefox", "opera", "vivaldi", "brave"}},
		{category: "Development", keywords: []string{"code", "developer", "git", "ide", "studio", "terminal", "docker", "podman"}},
		{category: "Communication", keywords: []string{"chat", "mail", "meet", "slack", "teams", "telegram", "zoom"}},
		{category: "Media", keywords: []string{"audio", "image", "media", "music", "photo", "player", "spotify", "video", "vlc"}},
		{category: "Games", keywords: []string{"game", "steam", "epic"}},
		{category: "Productivity", keywords: []string{"calendar", "document", "excel", "libreoffice", "office", "pdf", "powerpoint", "word"}},
		{category: "Security", keywords: []string{"antivirus", "firewall", "security", "vpn"}},
	}
	for _, candidate := range categories {
		for _, keyword := range candidate.keywords {
			if strings.Contains(name, keyword) {
				return candidate.category
			}
		}
	}
	return "Utilities and other"
}
