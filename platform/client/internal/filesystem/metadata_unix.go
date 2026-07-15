//go:build linux || darwin

package filesystem

import (
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
	"golang.org/x/sys/unix"
)

const metadataWritableFieldCount = 2

func metadataWriteCapability() fileprotocol.CapabilityState {
	return fileprotocol.CapabilityAvailable
}

func (root *rootHandle) setMetadata(components []string, delta fileprotocol.MetadataDelta) []fileprotocol.MetadataFieldResult {
	results := make([]fileprotocol.MetadataFieldResult, 0, metadataWritableFieldCount)
	if delta.ModifiedAt != nil {
		results = append(results, metadataFieldOutcome("modified_at", root.setModifiedAt(components, delta.ModifiedAt.UTC())))
	}

	if delta.POSIXMode != nil {
		results = append(results, metadataFieldOutcome("posix_mode", root.setPOSIXMode(components, *delta.POSIXMode)))
	}

	return results
}

func (root *rootHandle) setModifiedAt(components []string, modified time.Time) error {
	if len(components) == 0 {
		return fmt.Errorf("filesystem root metadata cannot be changed")
	}

	parent, name, err := root.openParent(components)
	if err != nil {
		return err
	}

	defer closeFileAfterRead(parent)

	fd, err := descriptorFromFile(parent)
	if err != nil {
		return err
	}

	var current unix.Stat_t
	if err := unix.Fstatat(fd, name, &current, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}

	times := []unix.Timespec{current.Atim, unix.NsecToTimespec(modified.UnixNano())}

	return unix.UtimesNanoAt(fd, name, times, unix.AT_SYMLINK_NOFOLLOW)
}

func (root *rootHandle) setPOSIXMode(components []string, mode uint32) error {
	if mode > 0o7777 || len(components) == 0 {
		return fmt.Errorf("POSIX mode is outside limit")
	}

	file, err := root.walk(components, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK)
	if err != nil {
		return err
	}

	defer closeFileAfterRead(file)

	fd, err := descriptorFromFile(file)
	if err != nil {
		return err
	}

	return unix.Fchmod(fd, mode)
}

func metadataFieldOutcome(field string, err error) fileprotocol.MetadataFieldResult {
	result := fileprotocol.MetadataFieldResult{Field: field, State: fileprotocol.MetadataApplied}
	if err == nil {
		return result
	}

	result.State, result.ErrorClass = fileprotocol.MetadataFailed, mutationErrorClass(err)
	if errors.Is(err, fs.ErrPermission) {
		result.State = fileprotocol.MetadataDenied
	}

	return result
}
