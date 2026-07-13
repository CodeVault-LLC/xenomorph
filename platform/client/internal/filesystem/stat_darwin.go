//go:build darwin

package filesystem

import (
	"io/fs"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const unixPermissionBits uint16 = 0o777

type statFileInfo struct {
	name string
	stat *unix.Stat_t
}

func fileInfoFromStat(name string, stat *unix.Stat_t) os.FileInfo {
	return statFileInfo{name: name, stat: stat}
}

func (info statFileInfo) Name() string      { return info.name }
func (info statFileInfo) Size() int64       { return info.stat.Size }
func (info statFileInfo) Mode() fs.FileMode { return unixFileMode(info.stat.Mode) }
func (info statFileInfo) ModTime() time.Time {
	return time.Unix(info.stat.Mtim.Sec, info.stat.Mtim.Nsec)
}
func (info statFileInfo) IsDir() bool { return info.Mode().IsDir() }
func (info statFileInfo) Sys() any    { return info.stat }

func unixIdentity(info os.FileInfo) (uint64, uint64) {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0
	}
	return uint64(stat.Dev), stat.Ino
}

func unixFileMode(mode uint16) fs.FileMode {
	result := fs.FileMode(mode & unixPermissionBits)
	switch mode & unix.S_IFMT {
	case unix.S_IFDIR:
		result |= fs.ModeDir
	case unix.S_IFLNK:
		result |= fs.ModeSymlink
	case unix.S_IFIFO:
		result |= fs.ModeNamedPipe
	case unix.S_IFSOCK:
		result |= fs.ModeSocket
	case unix.S_IFCHR:
		result |= fs.ModeCharDevice | fs.ModeDevice
	case unix.S_IFBLK:
		result |= fs.ModeDevice
	}
	return result
}
