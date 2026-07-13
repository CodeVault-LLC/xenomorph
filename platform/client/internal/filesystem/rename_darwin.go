//go:build darwin

package filesystem

import "golang.org/x/sys/unix"

func renameAt(sourceFD int, source string, destinationFD int, destination string, replace bool) error {
	if replace {
		return unix.Renameat(sourceFD, source, destinationFD, destination)
	}
	return unix.RenameatxNp(sourceFD, source, destinationFD, destination, unix.RENAME_EXCL)
}
