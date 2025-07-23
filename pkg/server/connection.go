package server

import (
	"errors"
	"strings"
	"syscall"
)

func IsConnectionReset(err error) bool {
	if err == nil {
		return false
	}

	// Covers syscall error
	if errors.Is(err, syscall.ECONNRESET) {
		return true
	}

	// Windows-specific network disconnect error messages
	errMsg := strings.ToLower(err.Error())

	return strings.Contains(errMsg, "connection reset by peer") ||
	       strings.Contains(errMsg, "wsarecv: an existing connection was forcibly closed by the remote host") ||
	       strings.Contains(errMsg, "use of closed network connection")
}