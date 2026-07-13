package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CaptureScreenshot captures the local screen and returns the image bytes.
// The implementation is platform-specific and writes a temporary file that is
// removed before the function returns.
func CaptureScreenshot() ([]byte, error) {
	tmpDir := os.TempDir()
	if tmpDir == "" {
		tmpDir = "/tmp"
	}

	outputPath := filepath.Join(tmpDir, fmt.Sprintf("xeno-screenshot-%d.png", time.Now().UnixMilli()))
	defer func() {
		_ = os.Remove(outputPath)
	}()

	return captureScreenOS(outputPath)
}
