//go:build windows

package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type rootHandle struct {
	path string
}

func openRoot(path string) (*rootHandle, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("filesystem root must be absolute")
	}
	cleaned := filepath.Clean(path)
	info, err := os.Lstat(cleaned)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("filesystem root is not a no-follow directory")
	}
	return &rootHandle{path: cleaned}, nil
}

func (root *rootHandle) close() error { return nil }

func (root *rootHandle) openDirectory(components []string) (*os.File, string, error) {
	path, info, err := root.resolveNoFollow(components)
	if err != nil {
		return nil, "", err
	}
	if !info.IsDir() {
		return nil, "", fmt.Errorf("target is not a directory")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	return file, snapshotIDFromStat(0, 0, info.Size(), info.ModTime()), nil
}

func (root *rootHandle) statNoFollow(components []string) (os.FileInfo, error) {
	_, info, err := root.resolveNoFollow(components)
	return info, err
}

func (root *rootHandle) openRegularFile(components []string) (*os.File, os.FileInfo, error) {
	path, info, err := root.resolveNoFollow(components)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("preview target is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	postInfo, err := file.Stat()
	if err != nil {
		_ = file.Close() // The primary stat failure is returned.
		return nil, nil, err
	}
	if !os.SameFile(info, postInfo) {
		_ = file.Close() // The identity mismatch is the security-relevant error.
		return nil, nil, fmt.Errorf("preview target changed during resolution")
	}
	return file, postInfo, nil
}

func (root *rootHandle) resolveNoFollow(components []string) (string, os.FileInfo, error) {
	current := root.path
	info, err := os.Lstat(current)
	if err != nil {
		return "", nil, err
	}
	for _, component := range components {
		current = filepath.Join(current, component)
		info, err = os.Lstat(current)
		if err != nil {
			return "", nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", nil, fmt.Errorf("symbolic links and reparse points are not followed")
		}
	}
	relative, err := filepath.Rel(root.path, current)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", nil, fmt.Errorf("resolved path escaped filesystem root")
	}
	return current, info, nil
}
