//go:build linux

package filesystem

import "golang.org/x/sys/unix"

func renameAt(sourceFD int, source string, destinationFD int, destination string, replace bool) error {
	flags := uint(unix.RENAME_NOREPLACE)
	if replace {
		flags = 0
	}

	return unix.Renameat2(sourceFD, source, destinationFD, destination, flags)
}
