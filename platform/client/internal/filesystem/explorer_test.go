package filesystem

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func TestListDirectoryPaginatesWithoutFollowingLinks(t *testing.T) {
	root := directoryFixture(t)
	rootID, relativePath := testFilesystemPath(t, root)
	request := fileprotocol.DirectoryListRequest{
		ProtocolVersion: fileprotocol.Version, RootID: rootID, RelativePath: relativePath, PageSize: 2,
	}
	first, err := ListDirectory(request)
	if err != nil {
		t.Fatalf("ListDirectory() error = %v", err)
	}
	if len(first.Entries) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want two entries and a cursor", first)
	}
	request.Cursor = first.NextCursor
	second, err := ListDirectory(request)
	if err != nil {
		t.Fatalf("ListDirectory() second page error = %v", err)
	}
	if len(second.Entries) != 2 || second.HasMore {
		t.Fatalf("second page = %+v, want final two entries", second)
	}
	entries := append(append([]fileprotocol.FileEntry(nil), first.Entries...), second.Entries...)
	if !containsSymlink(entries, "link.txt") {
		t.Fatal("symlink was not returned as a no-follow symlink entry")
	}
}

func directoryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"alpha.txt", "beta.txt", "gamma.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(name), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q) error = %v", name, err)
		}
	}
	if err := os.Symlink(filepath.Join(root, "alpha.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}
	return root
}

func containsSymlink(entries []fileprotocol.FileEntry, name string) bool {
	for _, entry := range entries {
		if entry.DisplayName == name && entry.Kind == fileprotocol.EntrySymlink {
			return true
		}
	}
	return false
}

func TestListDirectoryRejectsTraversal(t *testing.T) {
	request := fileprotocol.DirectoryListRequest{
		ProtocolVersion: fileprotocol.Version, RootID: testRootID(t),
		RelativePath: "../escape", PageSize: 1,
	}
	if _, err := ListDirectory(request); err == nil {
		t.Fatal("ListDirectory() error = nil, want traversal rejection")
	}
}

func TestReadPreviewIsBoundedAndRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "file.txt"), []byte("0123456789"), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	rootID, relativeRoot := testFilesystemPath(t, root)
	result, err := ReadPreview(fileprotocol.PreviewReadRequest{
		ProtocolVersion: fileprotocol.Version, RootID: rootID,
		RelativePath: joinProtocolPath(relativeRoot, "file.txt"), Offset: 2, Length: 4,
	})
	if err != nil {
		t.Fatalf("ReadPreview() error = %v", err)
	}
	if string(result.Data) != "2345" || !result.Truncated || result.Classification != contentText {
		t.Fatalf("ReadPreview() = %+v, want bounded text range", result)
	}

	if err := os.Symlink(filepath.Join(root, "file.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatalf("os.Symlink() error = %v", err)
	}
	if _, err := ReadPreview(fileprotocol.PreviewReadRequest{
		ProtocolVersion: fileprotocol.Version, RootID: rootID,
		RelativePath: joinProtocolPath(relativeRoot, "link.txt"), Length: 4,
	}); err == nil {
		t.Fatal("ReadPreview() symlink error = nil, want no-follow rejection")
	}
}

func TestUnknownFilesystemRootIsRejected(t *testing.T) {
	request := fileprotocol.MetadataGetRequest{ProtocolVersion: fileprotocol.Version, RootID: "unknown-root"}
	if _, err := GetMetadata(request); err == nil {
		t.Fatal("GetMetadata() error = nil, want unknown root rejection")
	}
}

func TestListRootsDiscoversFilesystem(t *testing.T) {
	result, err := ListRoots(fileprotocol.RootsListRequest{ProtocolVersion: fileprotocol.Version})
	if err != nil {
		t.Fatalf("ListRoots() error = %v", err)
	}
	if len(result.Roots) == 0 || !result.Roots[0].Available || len(result.Roots[0].AllowedVerbs) != len(readOnlyVerbs) {
		t.Fatalf("ListRoots() = %+v, want an available read-only root", result)
	}
}

func testRootID(t *testing.T) string {
	t.Helper()
	roots, err := filesystemRoots()
	if err != nil || len(roots) == 0 {
		t.Fatalf("filesystemRoots() = (%+v, %v), want at least one root", roots, err)
	}
	return roots[0].ID
}

func testFilesystemPath(t *testing.T, path string) (string, string) {
	t.Helper()
	roots, err := filesystemRoots()
	if err != nil {
		t.Fatalf("filesystemRoots() error = %v", err)
	}
	for _, root := range roots {
		relative, relativeErr := filepath.Rel(root.Path, path)
		if relativeErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			if relative == "." {
				relative = ""
			}
			return root.ID, filepath.ToSlash(relative)
		}
	}
	t.Fatalf("path %q is not beneath a discovered filesystem root", path)
	return "", ""
}

func joinProtocolPath(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "/" + name
}
