//go:build linux

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseLinkSpeedMbps(t *testing.T) {
	tests := []struct {
		value string
		want  uint64
	}{
		{value: "866.7 MBit/s", want: 866},
		{value: "1.2 GBit/s", want: 1200},
		{value: "unknown", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := parseLinkSpeedMbps(tt.value); got != tt.want {
				t.Fatalf("parseLinkSpeedMbps(%q) = %d, want %d", tt.value, got, tt.want)
			}
		})
	}
}

func TestPCIDeviceName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pci.ids")
	data := "10de  NVIDIA Corporation\n\t2208  GA102 [GeForce RTX 3080 Ti]\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write pci ids fixture: %v", err)
	}

	if got := pciDeviceName(path, "10de", "2208"); got != "NVIDIA Corporation GA102 [GeForce RTX 3080 Ti]" {
		t.Fatalf("pciDeviceName() = %q", got)
	}
}

func TestLinuxPCIDeviceNameResolvesInstalledDatabase(t *testing.T) {
	if _, err := os.Stat("/usr/share/hwdata/pci.ids"); err != nil {
		t.Skip("system PCI database is not installed")
	}

	if got := linuxPCIDeviceName("0x10de", "0x2208"); got != "NVIDIA Corporation GA102 [GeForce RTX 3080 Ti]" {
		t.Fatalf("linuxPCIDeviceName() = %q", got)
	}
}
