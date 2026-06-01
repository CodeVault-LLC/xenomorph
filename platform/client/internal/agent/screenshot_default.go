//go:build !linux

package agent

import "fmt"

func captureScreenOS(outputPath string) ([]byte, error) {
	return nil, fmt.Errorf("screenshot not supported on this platform")
}
