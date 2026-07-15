//go:build linux || darwin

package filesystem

import (
	"errors"
	"fmt"
	"os"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
	"golang.org/x/sys/unix"
)

func (root *rootHandle) openArchiveStage(components []string) (archiveStage, error) {
	stage, err := root.openUploadStage(components)
	if err != nil {
		return archiveStage{}, err
	}

	finished := false
	abort := func() {
		if finished {
			return
		}

		finished = true
		_ = stage.file.Close()
		_ = unix.Unlinkat(stage.parentFD, stage.temporaryName, 0)
		_ = stage.parent.Close()
	}
	publish := func(conflict fileprotocol.ConflictStrategy) (string, bool, error) {
		if finished {
			return "", false, fmt.Errorf("archive stage is closed")
		}

		name := stage.destinationName
		if conflict == fileprotocol.ConflictSkip && entryExists(stage.parentFD, name) {
			finished = true
			_ = unix.Unlinkat(stage.parentFD, stage.temporaryName, 0)
			_ = stage.parent.Close()

			return name, true, nil
		}

		if conflict == fileprotocol.ConflictRenameNew {
			available, availableErr := availableName(stage.parentFD, name)
			if availableErr != nil {
				return "", false, availableErr
			}

			name = available
		}

		if err := renameAt(stage.parentFD, stage.temporaryName, stage.parentFD, name, false); err != nil {
			return "", false, err
		}

		finished = true
		_ = stage.parent.Close()

		return name, false, nil
	}

	return archiveStage{file: stage.file, publish: publish, abort: abort}, nil
}

func (root *rootHandle) requireArchiveDirectory(components []string) error {
	info, err := root.statNoFollow(components)
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

		info, err := root.statNoFollow(current)
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("archive output parent is not a directory")
			}

			continue
		}

		if !errors.Is(err, os.ErrNotExist) {
			return err
		}

		parent, name, err := root.openParent(current)
		if err != nil {
			return err
		}

		fd, err := descriptorFromFile(parent)
		if err == nil {
			err = unix.Mkdirat(fd, name, archiveDirectoryPermission)
		}

		_ = parent.Close()

		if err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}

		if err := root.requireArchiveDirectory(current); err != nil {
			return err
		}
	}

	return nil
}
