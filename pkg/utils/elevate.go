package utils

import (
	"os"
	"os/exec"
	"runtime"
	"syscall"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/sys/windows"
)

// Checks if the process is running with elevated privileges.
// On Windows, it checks if the current process token is elevated.
// On Unix-like systems, it checks if the effective user ID is 0 (root).
func IsRunningElevated() bool {
	if runtime.GOOS == "windows" {
		token := windows.GetCurrentProcessToken()

		elevated := token.IsElevated()
		return elevated
	} else {
		return syscall.Geteuid() == 0
	}
}

// Attempts to re-run the current process with elevated privileges.
func RerunAsAdmin() {
	if IsRunningElevated() {
		return
	}

	// Avoid infinite loop
	if len(os.Args) > 1 && os.Args[1] == "-elevated" {
		logger.L().Info("Already running with elevated privileges.")
		return
	}

	exePath, err := os.Executable()
	if err != nil {
		logger.L().Error("Failed to get executable path", zap.Error(err))
	}

	if runtime.GOOS == "windows" {
		// Fix regarding file not existing if not running from a executable
		if os.Getenv("XENOMORPH_DEV") == "1" {
			cmd := exec.Command("cmd", "/C", "start", "cmd.exe", "/K", "go run ./cmd -elevated")
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			err := cmd.Run()
			if err != nil {
				logger.L().Error("Failed to re-run with elevated privileges", zap.Error(err))
			}
			os.Exit(0)
			return
		}

		verbPtr, _ := syscall.UTF16PtrFromString("runas")
		exePtr, _ := syscall.UTF16PtrFromString(exePath)
		paramsPtr, _ := syscall.UTF16PtrFromString("-elevated")

		err := windows.ShellExecute(0, verbPtr, exePtr, paramsPtr, nil, windows.SW_NORMAL)
		if errno, ok := err.(syscall.Errno); ok && errno == 1223 {
			logger.L().Info("User canceled elevation prompt.")
			os.Exit(0)
		} else if err != nil {
			logger.L().Error("Failed to re-run with elevated privileges", zap.Error(err))
		}

		os.Exit(0)
	} else {
		// Attempt sudo on Unix-like systems
		exePath, err := os.Executable()
		if err != nil {
			panic("Failed to get executable path: " + err.Error())
		}

		cmd := exec.Command("sudo", exePath)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		err = cmd.Run()
		if err != nil {
			panic("Failed to re-run with sudo: " + err.Error())
		}
		os.Exit(0)
	}
}
