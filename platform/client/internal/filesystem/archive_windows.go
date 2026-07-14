//go:build windows

package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func (root *rootHandle) openArchiveStage(components []string) (archiveStage, error) {
	if len(components) == 0 {
		return archiveStage{}, fmt.Errorf("archive output path is required")
	}
	parentPath, parentInfo, err := root.resolveNoFollow(components[:len(components)-1])
	if err != nil || !parentInfo.IsDir() {
		return archiveStage{}, fmt.Errorf("archive output parent is invalid")
	}
	destination := filepath.Join(parentPath, components[len(components)-1])
	id, err := randomMutationID()
	if err != nil {
		return archiveStage{}, err
	}
	temporary := destination + ".xenomorph-archive-" + id
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, archiveFilePermission)
	if err != nil {
		return archiveStage{}, err
	}
	finished := false
	abort := func() {
		if finished {
			return
		}
		finished = true
		_ = file.Close()
		_ = os.Remove(temporary)
	}
	publish := func(conflict fileprotocol.ConflictStrategy) (string, bool, error) {
		if finished {
			return "", false, fmt.Errorf("archive stage is closed")
		}
		path := destination
		if _, statErr := os.Lstat(path); statErr == nil {
			switch conflict {
			case fileprotocol.ConflictSkip:
				finished = true
				_ = os.Remove(temporary)
				return filepath.Base(path), true, nil
			case fileprotocol.ConflictRenameNew:
				path = availableWindowsName(path)
			default:
				return "", false, os.ErrExist
			}
		}
		if err := os.Rename(temporary, path); err != nil {
			return "", false, err
		}
		finished = true
		return filepath.Base(path), false, nil
	}
	return archiveStage{file: file, publish: publish, abort: abort}, nil
}

func (root *rootHandle) requireArchiveDirectory(components []string) error {
	_, info, err := root.resolveNoFollow(components)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("archive destination is not a directory")
	}
	return nil
}

func (root *rootHandle) ensureArchiveDirectories(components []string) error {
	for index := range components {
		current := components[:index+1]
		path, info, err := root.resolveNoFollow(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("archive output parent is not a directory")
			}
			continue
		}
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		parent, parentInfo, err := root.resolveNoFollow(current[:len(current)-1])
		if err != nil || !parentInfo.IsDir() {
			return fmt.Errorf("archive output parent is invalid")
		}
		path = filepath.Join(parent, current[len(current)-1])
		if err := os.Mkdir(path, archiveDirectoryPermission); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		if err := root.requireArchiveDirectory(current); err != nil {
			return err
		}
	}
	return nil
}
