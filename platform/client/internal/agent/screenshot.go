package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

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
