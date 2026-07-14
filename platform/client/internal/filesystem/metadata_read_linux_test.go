//go:build linux

package filesystem

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func TestGetMetadataReadsBoundedLinuxOptionalFields(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	path := filepath.Join(directory, "metadata.txt")
	requireNoError(t, os.WriteFile(path, []byte("metadata"), 0o600))
	const attributeName = "user.xenomorph_test"
	const secretValue = "value-must-not-be-returned"
	if err := unix.Setxattr(path, attributeName, []byte(secretValue), 0); err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EPERM) {
			t.Skipf("test filesystem does not permit user xattrs: %v", err)
		}
		t.Fatalf("unix.Setxattr() error = %v", err)
	}
	rootID, relativePath := testFilesystemPath(t, path)
	result, err := GetMetadata(context.Background(), fileprotocol.MetadataGetRequest{
		ProtocolVersion: fileprotocol.Version, RootID: rootID, RelativePath: relativePath,
	})
	requireNoError(t, err)
	xattrs := result.OptionalFields["extended_attributes"]
	if xattrs.State != fileprotocol.CapabilityAvailable || !strings.Contains(xattrs.Value, attributeName) {
		t.Fatalf("extended attributes = %+v, want bounded available name", xattrs)
	}
	if strings.Contains(xattrs.Value, secretValue) {
		t.Fatal("extended attributes exposed a value")
	}
	acl := result.OptionalFields["acl"]
	if acl.State != fileprotocol.CapabilityAvailable || acl.Value == "" {
		t.Fatalf("ACL = %+v, want explicit available observation", acl)
	}
}

func TestGetMetadataReportsLinuxACLPresence(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "acl.txt")
	requireNoError(t, os.WriteFile(path, nil, 0o600))
	if err := unix.Setxattr(path, posixACLAccess, testAccessACL(), 0); err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EPERM) || errors.Is(err, unix.EINVAL) {
			t.Skipf("test filesystem does not permit POSIX ACLs: %v", err)
		}
		t.Fatalf("unix.Setxattr(ACL) error = %v", err)
	}
	rootID, relativePath := testFilesystemPath(t, path)
	result, err := GetMetadata(context.Background(), fileprotocol.MetadataGetRequest{
		ProtocolVersion: fileprotocol.Version, RootID: rootID, RelativePath: relativePath,
	})
	requireNoError(t, err)
	acl := result.OptionalFields["acl"]
	if acl.State != fileprotocol.CapabilityAvailable || !strings.Contains(acl.Value, "user:1:r--") {
		t.Fatalf("ACL = %+v, want bounded canonical ACL entries", acl)
	}
}

func TestGetMetadataDoesNotFollowSymlinkForLinuxOptionalFields(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	target := filepath.Join(directory, "target.txt")
	link := filepath.Join(directory, "link.txt")
	requireNoError(t, os.WriteFile(target, nil, 0o600))
	if err := unix.Setxattr(target, "user.target_secret", []byte("hidden"), 0); err != nil {
		if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.EPERM) {
			t.Skipf("test filesystem does not permit user xattrs: %v", err)
		}
		t.Fatalf("unix.Setxattr() error = %v", err)
	}
	requireNoError(t, os.Symlink(target, link))
	rootID, relativePath := testFilesystemPath(t, link)
	result, err := GetMetadata(context.Background(), fileprotocol.MetadataGetRequest{
		ProtocolVersion: fileprotocol.Version, RootID: rootID, RelativePath: relativePath,
	})
	requireNoError(t, err)
	if result.Kind != fileprotocol.EntrySymlink {
		t.Fatalf("metadata kind = %q, want symlink", result.Kind)
	}
	xattrs := result.OptionalFields["extended_attributes"]
	if xattrs.State != fileprotocol.CapabilityUnavailable || strings.Contains(xattrs.Value, "target_secret") {
		t.Fatalf("symlink extended attributes = %+v, want no-follow unavailable result", xattrs)
	}
}

func TestBirthTimeFieldDistinguishesAvailabilityAndDenial(t *testing.T) {
	t.Parallel()
	available := birthTimeField(unix.Statx_t{
		Mask: unix.STATX_BTIME, Btime: unix.StatxTimestamp{Sec: 1_700_000_000, Nsec: 123},
	}, nil)
	want := time.Unix(1_700_000_000, 123).UTC().Format(time.RFC3339Nano)
	if available.State != fileprotocol.CapabilityAvailable || available.Value != want {
		t.Fatalf("available birth time = %+v, want %q", available, want)
	}
	if unavailable := birthTimeField(unix.Statx_t{}, nil); unavailable.State != fileprotocol.CapabilityUnavailable {
		t.Fatalf("missing birth time = %+v, want unavailable", unavailable)
	}
	if denied := birthTimeField(unix.Statx_t{}, unix.EACCES); denied.State != fileprotocol.CapabilityDenied {
		t.Fatalf("denied birth time = %+v, want denied", denied)
	}
}

func TestFormatXattrNamesSanitizesAndBoundsClientMetadata(t *testing.T) {
	t.Parallel()
	buffer := make([]byte, 0, maxXattrListBytes)
	for index := 0; index < maxXattrDisplayNames+1; index++ {
		buffer = append(buffer, []byte("user.name\n")...)
		buffer = append(buffer, 0)
	}
	value := formatXattrNames(buffer)
	if strings.Contains(value, "\n") || len(value) > maxXattrDisplayBytes || !strings.HasSuffix(value, "…") {
		t.Fatalf("formatXattrNames() returned unsafe or unbounded value of %d bytes: %q", len(value), value)
	}
}

func TestLinuxMetadataCapabilitiesAdvertiseNativeReads(t *testing.T) {
	t.Parallel()
	capabilities := platformCapabilities()
	if capabilities.ACL != fileprotocol.CapabilityAvailable || capabilities.ExtendedAttributes != fileprotocol.CapabilityAvailable {
		t.Fatalf("platformCapabilities() = %+v, want Linux ACL and xattr reads", capabilities)
	}
}

func testAccessACL() []byte {
	const (
		aclVersion   = posixACLVersion
		namedACLUser = uint32(1)
		undefinedID  = ^uint32(0)
	)
	entries := []struct {
		tag        uint16
		permission uint16
		id         uint32
	}{
		{tag: posixACLUserObject, permission: 0o6, id: undefinedID},
		{tag: posixACLUser, permission: 0o4, id: namedACLUser},
		{tag: posixACLGroupObject, permission: 0o0, id: undefinedID},
		{tag: posixACLMask, permission: 0o4, id: undefinedID},
		{tag: posixACLOther, permission: 0o0, id: undefinedID},
	}
	data := make([]byte, 4+len(entries)*8)
	binary.LittleEndian.PutUint32(data, aclVersion)
	for index, entry := range entries {
		offset := 4 + index*8
		binary.LittleEndian.PutUint16(data[offset:], entry.tag)
		binary.LittleEndian.PutUint16(data[offset+2:], entry.permission)
		binary.LittleEndian.PutUint32(data[offset+4:], entry.id)
	}
	return data
}
