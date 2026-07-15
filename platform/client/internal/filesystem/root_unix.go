//go:build linux || darwin

package filesystem

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

type rootHandle struct {
	file *os.File
}

func openRoot(path string) (*rootHandle, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("filesystem root must be absolute")
	}

	fd, err := unix.Open(filepath.Clean(path), unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}

	file, err := fileFromDescriptor(fd, "authorized-root")
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}

	return &rootHandle{file: file}, nil
}

func (root *rootHandle) close() error {
	return root.file.Close()
}

func (root *rootHandle) openDirectory(components []string) (*os.File, string, error) {
	file, err := root.walk(components, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW)
	if err != nil {
		return nil, "", err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, "", err
	}

	device, inode := unixIdentity(info)

	return file, snapshotIDFromStat(device, inode, info.Size(), info.ModTime()), nil
}

func (root *rootHandle) statNoFollow(components []string) (os.FileInfo, error) {
	if len(components) == 0 {
		return root.file.Stat()
	}

	parent, err := root.walk(components[:len(components)-1], unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW)
	if err != nil {
		return nil, err
	}

	defer closeFileAfterRead(parent)

	var stat unix.Stat_t

	parentFD, err := descriptorFromFile(parent)
	if err != nil {
		return nil, err
	}

	if err := unix.Fstatat(parentFD, components[len(components)-1], &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, err
	}

	return fileInfoFromStat(components[len(components)-1], &stat), nil
}

func (root *rootHandle) openRegularFile(components []string) (*os.File, os.FileInfo, error) {
	if len(components) == 0 {
		return nil, nil, fmt.Errorf("root directory is not a regular file")
	}

	file, err := root.walk(components, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK)
	if err != nil {
		return nil, nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, fmt.Errorf("preview target is not a regular file")
	}

	return file, info, nil
}

func (root *rootHandle) walk(components []string, finalFlags int) (*os.File, error) {
	rootFD, err := descriptorFromFile(root.file)
	if err != nil {
		return nil, err
	}

	current, err := unix.Openat(rootFD, ".", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}

	if len(components) == 0 {
		return fileFromDescriptor(current, "authorized-entry")
	}

	for index, component := range components {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_DIRECTORY | unix.O_NOFOLLOW
		if index == len(components)-1 {
			flags = finalFlags
		}

		next, openErr := unix.Openat(current, component, flags, 0)
		_ = unix.Close(current)

		if openErr != nil {
			return nil, openErr
		}

		current = next
	}

	return fileFromDescriptor(current, "authorized-entry")
}

func descriptorFromFile(file *os.File) (int, error) {
	raw := file.Fd()
	if raw > uintptr(^uint(0)>>1) {
		return 0, fmt.Errorf("file descriptor is outside platform range")
	}
	// #nosec G115 -- the descriptor is checked against the maximum int value.
	return int(raw), nil
}

func fileFromDescriptor(descriptor int, name string) (*os.File, error) {
	if descriptor < 0 {
		return nil, fmt.Errorf("file descriptor is invalid")
	}
	// #nosec G115 -- a nonnegative int is representable as uintptr.
	file := os.NewFile(uintptr(descriptor), name)
	if file == nil {
		return nil, fmt.Errorf("create file handle from descriptor")
	}

	return file, nil
}
