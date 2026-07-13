//go:build !windows

package agent

func systemDiskPath() string {
	return "/"
}
