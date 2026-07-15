//go:build linux

package filesystem

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/sys/unix"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

const (
	maxXattrListBytes      = 64 << 10
	maxXattrDisplayBytes   = 2 << 10
	maxXattrDisplayNames   = 64
	maxACLValueBytes       = 16 << 10
	maxACLDisplayBytes     = 2 << 10
	posixACLVersion        = 2
	posixACLPermissionMask = 0o7
	posixACLAccess         = "system.posix_acl_access"
	posixACLDefault        = "system.posix_acl_default"
)

const (
	posixACLUserObject  uint16 = 0x01
	posixACLUser        uint16 = 0x02
	posixACLGroupObject uint16 = 0x04
	posixACLGroup       uint16 = 0x08
	posixACLMask        uint16 = 0x10
	posixACLOther       uint16 = 0x20
)

func (root *rootHandle) platformMetadataFields(ctx context.Context, components []string, info os.FileInfo) map[string]fileprotocol.FieldValue {
	fields := unavailableMetadataFields()
	if stat, ok := info.Sys().(*unix.Stat_t); ok {
		fields["owner"] = availableMetadataField(fmt.Sprint(stat.Uid))
		fields["group"] = availableMetadataField(fmt.Sprint(stat.Gid))
	}

	fields["birth_time"] = root.readBirthTime(components)
	if err := ctx.Err(); err != nil {
		return fields
	}

	if !info.Mode().IsRegular() && !info.IsDir() {
		return fields
	}

	file, err := root.openMetadataHandle(components, info.IsDir())
	if err != nil {
		state := metadataReadErrorState(err)
		fields["acl"] = fileprotocol.FieldValue{State: state}
		fields["extended_attributes"] = fileprotocol.FieldValue{State: state}

		return fields
	}

	defer closeFileAfterRead(file)

	fd, err := descriptorFromFile(file)
	if err != nil {
		return fields
	}

	fields["acl"] = readPOSIXACL(fd, info.IsDir())
	if ctx.Err() == nil {
		fields["extended_attributes"] = readExtendedAttributeNames(fd)
	}

	return fields
}

func unavailableMetadataFields() map[string]fileprotocol.FieldValue {
	unavailable := fileprotocol.FieldValue{State: fileprotocol.CapabilityUnavailable}

	return map[string]fileprotocol.FieldValue{
		"owner": unavailable, "group": unavailable, "acl": unavailable,
		"birth_time": unavailable, "extended_attributes": unavailable,
	}
}

func availableMetadataField(value string) fileprotocol.FieldValue {
	return fileprotocol.FieldValue{State: fileprotocol.CapabilityAvailable, Value: value}
}

func (root *rootHandle) readBirthTime(components []string) fileprotocol.FieldValue {
	rootFD, err := descriptorFromFile(root.file)
	if err != nil {
		return fileprotocol.FieldValue{State: fileprotocol.CapabilityUnavailable}
	}

	directoryFD, name, flags := rootFD, "", unix.AT_EMPTY_PATH|unix.AT_SYMLINK_NOFOLLOW

	var parent *os.File

	if len(components) > 0 {
		parent, name, err = root.openParent(components)
		if err != nil {
			return fileprotocol.FieldValue{State: metadataReadErrorState(err)}
		}

		defer closeFileAfterRead(parent)

		directoryFD, err = descriptorFromFile(parent)
		if err != nil {
			return fileprotocol.FieldValue{State: fileprotocol.CapabilityUnavailable}
		}

		flags = unix.AT_SYMLINK_NOFOLLOW
	}

	var stat unix.Statx_t
	err = unix.Statx(directoryFD, name, flags, unix.STATX_BTIME, &stat)

	return birthTimeField(stat, err)
}

func birthTimeField(stat unix.Statx_t, err error) fileprotocol.FieldValue {
	if err != nil {
		return fileprotocol.FieldValue{State: metadataReadErrorState(err)}
	}

	if stat.Mask&unix.STATX_BTIME == 0 {
		return fileprotocol.FieldValue{State: fileprotocol.CapabilityUnavailable}
	}

	timestamp := time.Unix(stat.Btime.Sec, int64(stat.Btime.Nsec)).UTC()

	return availableMetadataField(timestamp.Format("2006-01-02T15:04:05.999999999Z07:00"))
}

func (root *rootHandle) openMetadataHandle(components []string, directory bool) (*os.File, error) {
	flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW | unix.O_NONBLOCK
	if directory {
		flags |= unix.O_DIRECTORY
	}

	return root.walk(components, flags)
}

func readPOSIXACL(fd int, directory bool) fileprotocol.FieldValue {
	types := []struct {
		name   string
		prefix string
	}{{name: posixACLAccess}}
	if directory {
		types = append(types, struct {
			name   string
			prefix string
		}{name: posixACLDefault, prefix: "default:"})
	}

	entries := make([]string, 0, len(types))

	for _, aclType := range types {
		value, present, err := readACLValue(fd, aclType.name)
		if err != nil {
			return fileprotocol.FieldValue{State: metadataReadErrorState(err)}
		}

		if present {
			parsed, parseErr := parsePOSIXACL(value, aclType.prefix)
			if parseErr != nil {
				return fileprotocol.FieldValue{State: fileprotocol.CapabilityUnavailable}
			}

			entries = append(entries, parsed...)
		}
	}

	if len(entries) == 0 {
		return availableMetadataField("none")
	}

	value := strings.Join(entries, ", ")
	if len(value) > maxACLDisplayBytes {
		return fileprotocol.FieldValue{State: fileprotocol.CapabilityUnavailable}
	}

	return availableMetadataField(value)
}

func readACLValue(fd int, name string) ([]byte, bool, error) {
	size, err := unix.Fgetxattr(fd, name, nil)
	if errors.Is(err, unix.ENODATA) {
		return nil, false, nil
	}

	if err != nil {
		return nil, false, err
	}

	if size <= 0 || size > maxACLValueBytes {
		return nil, false, unix.E2BIG
	}

	value := make([]byte, size)

	read, err := unix.Fgetxattr(fd, name, value)
	if err != nil {
		return nil, false, err
	}

	if read != size {
		return nil, false, unix.EAGAIN
	}

	return value, true, nil
}

func parsePOSIXACL(value []byte, prefix string) ([]string, error) {
	const headerBytes = 4

	const entryBytes = 8
	if len(value) < headerBytes || (len(value)-headerBytes)%entryBytes != 0 || binary.LittleEndian.Uint32(value) != posixACLVersion {
		return nil, fmt.Errorf("posix ACL encoding is invalid")
	}

	entries := make([]string, 0, (len(value)-headerBytes)/entryBytes)

	for offset := headerBytes; offset < len(value); offset += entryBytes {
		tag := binary.LittleEndian.Uint16(value[offset:])
		permission := binary.LittleEndian.Uint16(value[offset+2:])
		id := binary.LittleEndian.Uint32(value[offset+4:])

		entry, err := formatACLEntry(prefix, tag, permission, id)
		if err != nil {
			return nil, err
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

func formatACLEntry(prefix string, tag, permission uint16, id uint32) (string, error) {
	if permission > posixACLPermissionMask {
		return "", fmt.Errorf("posix ACL permissions are invalid")
	}

	principal, err := formatACLPrincipal(tag, id)
	if err != nil {
		return "", err
	}

	return prefix + principal + formatACLPermissions(permission), nil
}

func formatACLPermissions(permission uint16) string {
	permissions := []byte("---")
	if permission&0o4 != 0 {
		permissions[0] = 'r'
	}

	if permission&0o2 != 0 {
		permissions[1] = 'w'
	}

	if permission&0o1 != 0 {
		permissions[2] = 'x'
	}

	return string(permissions)
}

func formatACLPrincipal(tag uint16, id uint32) (string, error) {
	switch tag {
	case posixACLUserObject:
		return "user::", nil
	case posixACLUser:
		return fmt.Sprintf("user:%d:", id), nil
	case posixACLGroupObject:
		return "group::", nil
	case posixACLGroup:
		return fmt.Sprintf("group:%d:", id), nil
	case posixACLMask:
		return "mask::", nil
	case posixACLOther:
		return "other::", nil
	default:
		return "", fmt.Errorf("posix ACL tag is invalid")
	}
}

func readExtendedAttributeNames(fd int) fileprotocol.FieldValue {
	size, err := unix.Flistxattr(fd, nil)
	if err != nil {
		return fileprotocol.FieldValue{State: metadataReadErrorState(err)}
	}

	if size == 0 {
		return availableMetadataField("none")
	}

	if size < 0 || size > maxXattrListBytes {
		return fileprotocol.FieldValue{State: fileprotocol.CapabilityUnavailable}
	}

	buffer := make([]byte, size)
	read, err := unix.Flistxattr(fd, buffer)

	if err != nil || read < 0 || read > len(buffer) {
		return fileprotocol.FieldValue{State: metadataReadErrorState(err)}
	}

	return availableMetadataField(formatXattrNames(buffer[:read]))
}

func formatXattrNames(buffer []byte) string {
	names := make([]string, 0, maxXattrDisplayNames)
	for len(buffer) > 0 && len(names) < maxXattrDisplayNames {
		end := bytes.IndexByte(buffer, 0)
		if end < 0 {
			break
		}

		name := sanitizeMetadataText(string(buffer[:end]))
		buffer = buffer[end+1:]

		if name != "" {
			names = append(names, name)
		}
	}

	sort.Strings(names)

	value := strings.Join(names, ", ")
	if len(value) > maxXattrDisplayBytes {
		value = value[:maxXattrDisplayBytes-len("…")]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}

		value += "…"
	} else if len(buffer) > 0 {
		value += ", …"
	}

	if value == "" {
		return "none"
	}

	return value
}

func sanitizeMetadataText(value string) string {
	value = strings.ToValidUTF8(value, "�")

	return strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return '�'
		}

		return character
	}, value)
}

func metadataReadErrorState(err error) fileprotocol.CapabilityState {
	if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EPERM) {
		return fileprotocol.CapabilityDenied
	}

	return fileprotocol.CapabilityUnavailable
}
