//go:build !windows

package atomicfile

import "os"

func syncDirectory(path string) error {
	directory, err := os.Open(path) // #nosec G304 -- path is the validated parent of the state file.
	if err != nil {
		return err
	}
	syncError := directory.Sync()
	closeError := directory.Close()
	if syncError != nil {
		return syncError
	}
	return closeError
}
