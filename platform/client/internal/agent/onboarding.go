package agent

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	maxInstalledApplications = 200
)

type BrowserInstallation struct {
	Name       string `json:"name"`
	BinaryPath string `json:"binary_path"`
	ProfileDir string `json:"profile_dir"`
}

type EntryPayload struct {
	Hostname              string                `json:"hostname"`
	OSVersion             string                `json:"os_version"`
	IsNewAgent            bool                  `json:"is_new_agent"`
	Browsers              []BrowserInstallation `json:"browsers,omitempty"`
	InstalledApplications []string              `json:"installed_applications,omitempty"`
}

type homeDirProvider func() (string, error)

func BuildEntryPayload(isNewAgent bool, hostnameProvider HostnameProvider, homeProvider homeDirProvider) EntryPayload {
	if hostnameProvider == nil {
		hostnameProvider = os.Hostname
	}
	if homeProvider == nil {
		homeProvider = os.UserHomeDir
	}

	payload := EntryPayload{
		Hostname:   resolveHostname(hostnameProvider),
		OSVersion:  runtime.GOOS + "/" + runtime.GOARCH,
		IsNewAgent: isNewAgent,
	}

	if !isNewAgent {
		return payload
	}

	payload.Browsers = collectBrowsers(homeProvider)
	payload.InstalledApplications = collectInstalledApplications(homeProvider)

	return payload
}

func collectBrowsers(homeProvider homeDirProvider) []BrowserInstallation {
	homeDir, _ := homeProvider()

	type browserCandidate struct {
		name       string
		binary     string
		profileDir string
	}

	candidates := []browserCandidate{
		{name: "Firefox", binary: "firefox", profileDir: filepath.Join(homeDir, ".mozilla", "firefox")},
		{name: "Chromium", binary: "chromium", profileDir: filepath.Join(homeDir, ".config", "chromium")},
		{name: "Google Chrome", binary: "google-chrome", profileDir: filepath.Join(homeDir, ".config", "google-chrome")},
		{name: "Brave", binary: "brave-browser", profileDir: filepath.Join(homeDir, ".config", "BraveSoftware", "Brave-Browser")},
		{name: "Microsoft Edge", binary: "microsoft-edge", profileDir: filepath.Join(homeDir, ".config", "microsoft-edge")},
		{name: "Vivaldi", binary: "vivaldi", profileDir: filepath.Join(homeDir, ".config", "vivaldi")},
		{name: "Opera", binary: "opera", profileDir: filepath.Join(homeDir, ".config", "opera")},
	}

	browsers := make([]BrowserInstallation, 0, len(candidates))
	for _, candidate := range candidates {
		binaryPath, err := exec.LookPath(candidate.binary)
		if err != nil {
			continue
		}

		profileDir := ""
		if dirExists(candidate.profileDir) {
			profileDir = candidate.profileDir
		}

		browsers = append(browsers, BrowserInstallation{
			Name:       candidate.name,
			BinaryPath: binaryPath,
			ProfileDir: profileDir,
		})
	}

	sort.Slice(browsers, func(i, j int) bool {
		return browsers[i].Name < browsers[j].Name
	})

	return browsers
}

func collectInstalledApplications(homeProvider homeDirProvider) []string {
	homeDir, _ := homeProvider()
	dirs := []string{
		"/usr/share/applications",
		"/usr/local/share/applications",
		filepath.Join(homeDir, ".local", "share", "applications"),
	}

	seen := make(map[string]struct{})
	apps := make([]string, 0, maxInstalledApplications)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if len(apps) >= maxInstalledApplications {
				break
			}
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".desktop" {
				continue
			}

			name := parseDesktopName(filepath.Join(dir, entry.Name()))
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
	if len(apps) > maxInstalledApplications {
		apps = apps[:maxInstalledApplications]
	}

	return apps
}

func parseDesktopName(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "Name=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Name="))
		}
	}

	return ""
}

func dirExists(path string) bool {
	if path == "" {
		return false
	}

	stat, err := os.Stat(path)
	if err != nil {
		return false
	}
	return stat.IsDir()
}
