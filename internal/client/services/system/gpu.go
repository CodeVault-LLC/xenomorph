package system

import (
	"os/exec"
	"runtime"
	"strings"
)

func GetGPUModel() string {
	switch runtime.GOOS {
	case "linux":
		return getLinuxGPU()
	case "darwin":
		return getMacGPU()
	case "windows":
		return getWindowsGPU()
	default:
		return "Unsupported OS"
	}
}


func getLinuxGPU() string {
	out, err := exec.Command("lspci").Output()
	if err != nil {
		return "Unknown GPU"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "VGA compatible controller") || strings.Contains(line, "3D controller") {
			return line
		}
	}
	return "Unknown GPU"
}

func getMacGPU() string {
	out, err := exec.Command("system_profiler", "SPDisplaysDataType").Output()
	if err != nil {
		return "Unknown GPU"
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Chipset Model:") {
			return strings.TrimPrefix(line, "Chipset Model: ")
		}
	}
	return "Unknown GPU"
}

func getWindowsGPU() string {
	out, err := exec.Command("wmic", "path", "win32_VideoController", "get", "name").Output()
	if err != nil {
		return "Unknown GPU"
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) >= 2 {
		return strings.TrimSpace(lines[1])
	}
	return "Unknown GPU"
}