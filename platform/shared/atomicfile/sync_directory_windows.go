//go:build windows

package atomicfile

// Windows does not expose POSIX directory fsync through os.File. The file is
// synchronized before the same-directory atomic rename.
func syncDirectory(string) error { return nil }
