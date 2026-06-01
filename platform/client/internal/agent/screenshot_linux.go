//go:build linux

package agent

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func captureScreenOS(outputPath string) ([]byte, error) {
	tools := []struct {
		name string
		args []string
	}{
		{"import", []string{"-window", "root", outputPath}},
		{"gnome-screenshot", []string{"-f", outputPath}},
		{"scrot", []string{outputPath}},
		{"maim", []string{outputPath}},
	}

	for _, t := range tools {
		if _, err := exec.LookPath(t.name); err != nil {
			continue
		}

		args := make([]string, len(t.args))
		for i, a := range t.args {
			args[i] = filepath.Clean(a)
		}
		//nolint:gosec // outputPath is system-generated (temp dir + timestamp), not user input.
		cmd := exec.Command(t.name, args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("%s failed: %w\noutput: %s", t.name, err, string(out))
		}

		data, err := os.ReadFile(filepath.Clean(outputPath))
		if err != nil {
			return nil, fmt.Errorf("read screenshot: %w", err)
		}

		return data, nil
	}

	return nil, fmt.Errorf("no screenshot tool found")
}
