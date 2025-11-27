//go:build windows
// +build windows

package utils

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
	"golang.org/x/sys/windows"
)

// Checks if the process is running with elevated privileges on Windows.
func IsRunningElevated() bool {
	// Open current process token
	var token windows.Token
	err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token)
	if err != nil {
		logger.L().Warn("OpenProcessToken failed; assuming not elevated", zap.Error(err))
		return false
	}
	defer token.Close()

	// Query TokenElevation
	var elevation uint32
	var retLen uint32
	err = windows.GetTokenInformation(token, windows.TokenElevation, (*byte)(unsafe.Pointer(&elevation)), uint32(unsafe.Sizeof(elevation)), &retLen)
	if err != nil {
		logger.L().Warn("GetTokenInformation failed; assuming not elevated", zap.Error(err))
		return false
	}

	return elevation != 0
}

// Attempts to re-run the current process with elevated privileges on Windows.
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
		// continue — we may still attempt to elevate (ShellExecute expects a path)
	}

	// Developer convenience: when running in dev mode, run via `go run` in a new cmd window
	if os.Getenv("XENOMORPH_DEV") == "1" {
		// Start a new cmd window and run "go run ./cmd -elevated" there
		cmd := exec.Command("cmd", "/C", "start", "cmd.exe", "/K", "go run ./cmd -elevated")
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			logger.L().Error("Failed to re-run in dev mode", zap.Error(err))
		}
		os.Exit(0)
		return
	}

	verbPtr, _ := syscall.UTF16PtrFromString("runas")
	exePtr, _ := syscall.UTF16PtrFromString(exePath)

	// pass through existing args (but append -elevated to indicate we already elevated)
	// skip program name (os.Args[0])
	args := append(os.Args[1:], "-elevated")
	params := strings.Join(args, " ")
	paramsPtr, _ := syscall.UTF16PtrFromString(params)

	// ShellExecuteW will show UAC elevation dialog
	if err := windows.ShellExecute(0, verbPtr, exePtr, paramsPtr, nil, windows.SW_NORMAL); err != nil {
		// If user canceled the UAC prompt, Windows returns ERROR_CANCELLED (1223).
		if errno, ok := err.(syscall.Errno); ok && errno == 1223 {
			logger.L().Info("User canceled elevation prompt.")
			os.Exit(0)
		} else {
			logger.L().Error("Failed to re-run with elevated privileges", zap.Error(err))
		}
	}

	os.Exit(0)
}
