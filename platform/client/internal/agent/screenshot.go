package agent

import (
	"fmt"
	"os"
	"path/filepath"
)

// CaptureScreenshot captures the local screen and returns the image bytes.
// The implementation is platform-specific and writes a temporary file that is
// removed before the function returns.
func CaptureScreenshot() ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "xeno-screenshot-*")
	if err != nil {
		return nil, fmt.Errorf("create screenshot directory: %w", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	outputPath := filepath.Join(tmpDir, "screenshot.png")
	return captureScreenOS(outputPath)
}
