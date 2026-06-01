//go:build !linux

package agent

import "fmt"

func init() {
	defaultApprover = DefaultApprover{}
}

// DefaultApprover rejects all commands on unsupported platforms.
type DefaultApprover struct{}

func (d DefaultApprover) Approve(cmd CommandEnvelope) (bool, error) {
	return false, fmt.Errorf("user approval not implemented on this platform")
}
