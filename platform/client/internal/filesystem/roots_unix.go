//go:build linux || darwin

package filesystem

const unixFilesystemRootID = "filesystem"

func filesystemRoots() ([]rootDefinition, error) {
	return []rootDefinition{{ID: unixFilesystemRootID, Path: "/", DisplayLabel: "Filesystem"}}, nil
}
