package utils

import "runtime"

func IsWindows() bool {
	return runtime.GOOS == "windows"
}

func IsLinux() bool {
	return runtime.GOOS == "linux"
}

func IsMacOS() bool {
	return runtime.GOOS == "darwin"
}