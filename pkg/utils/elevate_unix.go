//go:build !windows
// +build !windows

package utils

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

// Checks if the process is running with elevated privileges on Unix-like systems.
func IsRunningElevated() bool {
	// Effective UID 0 == root
	return syscall.Geteuid() == 0
}

// Attempts to re-run the current process with elevated privileges on Unix-like systems.
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
		// continue — still try to sudo, although it may fail
	}

	// Developer convenience: in dev mode, just run `go run` directly (no sudo)
	if os.Getenv("XENOMORPH_DEV") == "1" {
		cmd := exec.Command("go", "run", "./cmd", "-elevated")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			logger.L().Error("Failed to re-run in dev mode", zap.Error(err))
		}
		os.Exit(0)
		return
	}

	// Build args for sudo: [exePath, -elevated, <original args...>]
	sudoArgs := []string{}
	if exePath != "" {
		sudoArgs = append(sudoArgs, exePath)
	} else {
		// fallback: try to invoke using os.Args[0]
		sudoArgs = append(sudoArgs, os.Args[0])
	}
	sudoArgs = append(sudoArgs, "-elevated")
	if len(os.Args) > 1 {
		sudoArgs = append(sudoArgs, os.Args[1:]...)
	}

	cmd := exec.Command("sudo", sudoArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// Do not panic; log and exit
		logger.L().Error("Failed to re-run with sudo", zap.Error(err))
		os.Exit(1)
	}
	os.Exit(0)
}
