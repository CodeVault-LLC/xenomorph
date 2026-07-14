package agent

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
)

// BrowserInstallation describes a detected browser on the system.
type BrowserInstallation struct {
	Name       string `json:"name"`
	BinaryPath string `json:"binary_path"`
	ProfileDir string `json:"profile_dir"`
}

// EndpointAttestation is the one-time endpoint attestation payload sent to the
// gateway for an unprovisioned device (NIST 800-53 IA-3(4) device attestation
// and CM-8 system component inventory).
type EndpointAttestation struct {
	Hostname              string                `json:"hostname"`
	OSVersion             string                `json:"os_version"`
	RequiresAttestation   bool                  `json:"requires_attestation"`
	Browsers              []BrowserInstallation `json:"browsers,omitempty"`
	InstalledApplications []string              `json:"installed_applications,omitempty"`
}

// BuildEndpointAttestation constructs the endpoint attestation payload from runtime data.
func BuildEndpointAttestation(requiresAttestation bool, hostnameProvider func() (string, error), homeProvider func() (string, error)) EndpointAttestation {
	if hostnameProvider == nil {
		hostnameProvider = os.Hostname
	}
	if homeProvider == nil {
		homeProvider = os.UserHomeDir
	}

	payload := EndpointAttestation{
		Hostname:            resolveHostname(hostnameProvider),
		OSVersion:           runtime.GOOS + "/" + runtime.GOARCH,
		RequiresAttestation: requiresAttestation,
	}

	if !requiresAttestation {
		return payload
	}

	payload.Browsers = collectBrowsers(homeProvider)
	payload.InstalledApplications = collectInstalledApplications()

	return payload
}

func collectBrowsers(homeProvider func() (string, error)) []BrowserInstallation {
	homeDir, _ := homeProvider()

	type candidate struct {
		name       string
		binary     string
		profileDir string
	}

	candidates := []candidate{
		{name: "Firefox", binary: "firefox", profileDir: filepath.Join(homeDir, ".mozilla", "firefox")},
		{name: "Chromium", binary: "chromium", profileDir: filepath.Join(homeDir, ".config", "chromium")},
		{name: "Google Chrome", binary: "google-chrome", profileDir: filepath.Join(homeDir, ".config", "google-chrome")},
		{name: "Brave", binary: "brave-browser", profileDir: filepath.Join(homeDir, ".config", "BraveSoftware", "Brave-Browser")},
		{name: "Microsoft Edge", binary: "microsoft-edge", profileDir: filepath.Join(homeDir, ".config", "microsoft-edge")},
		{name: "Vivaldi", binary: "vivaldi", profileDir: filepath.Join(homeDir, ".config", "vivaldi")},
		{name: "Opera", binary: "opera", profileDir: filepath.Join(homeDir, ".config", "opera")},
	}

	browsers := make([]BrowserInstallation, 0, len(candidates))
	for _, c := range candidates {
		binaryPath, err := exec.LookPath(c.binary)
		if err != nil {
			continue
		}

		profileDir := ""
		if dirExists(c.profileDir) {
			profileDir = c.profileDir
		}

		browsers = append(browsers, BrowserInstallation{
			Name:       c.name,
			BinaryPath: binaryPath,
			ProfileDir: profileDir,
		})
	}

	sort.Slice(browsers, func(i, j int) bool {
		return browsers[i].Name < browsers[j].Name
	})

	return browsers
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
