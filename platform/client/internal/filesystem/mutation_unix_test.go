//go:build linux || darwin

package filesystem

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

const plannedMutationState = "planned"

func TestMutationUsesNoFollowPreconditionsAndAtomicConflict(t *testing.T) {
	t.Parallel()
	rootPath := t.TempDir()
	root, err := openRoot(rootPath)
	if err != nil {
		t.Fatalf("openRoot() error = %v", err)
	}
	defer func() { _ = root.close() }()
	if err := os.WriteFile(filepath.Join(rootPath, "source.txt"), []byte("source"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, "existing.txt"), []byte("existing"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	expectedSize := int64(6)
	item := fileprotocol.MutationItem{SourcePath: "source.txt", DestinationPath: "existing.txt", Preconditions: fileprotocol.Preconditions{ExpectedSize: &expectedSize}}
	if _, err := root.mutate(fileprotocol.MutationRename, fileprotocol.ConflictFail, false, item); err == nil {
		t.Fatal("mutate() conflict error = nil")
	}
	// #nosec G304 -- rootPath is an isolated test directory.
	data, err := os.ReadFile(filepath.Join(rootPath, "existing.txt"))
	if err != nil || string(data) != "existing" {
		t.Fatalf("destination = %q, %v; want unchanged", data, err)
	}
	if _, err := os.Lstat(filepath.Join(rootPath, "source.txt")); err != nil {
		t.Fatalf("source disappeared after conflict: %v", err)
	}
}

func TestMutationRejectsSymlinkParentAndStaleVersion(t *testing.T) {
	t.Parallel()
	rootPath := t.TempDir()
	outside := t.TempDir()
	root, err := openRoot(rootPath)
	if err != nil {
		t.Fatalf("openRoot() error = %v", err)
	}
	defer func() { _ = root.close() }()
	if err := os.Symlink(outside, filepath.Join(rootPath, "escape")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	create := fileprotocol.MutationItem{DestinationPath: "escape/payload"}
	if _, err := root.mutate(fileprotocol.MutationCreateFile, fileprotocol.ConflictFail, false, create); err == nil {
		t.Fatal("mutate() symlink-parent error = nil")
	}
	if _, err := os.Lstat(filepath.Join(outside, "payload")); !os.IsNotExist(err) {
		t.Fatalf("outside payload exists: %v", err)
	}
	path := filepath.Join(rootPath, "versioned.txt")
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	stale := time.Now().UTC().Add(-time.Hour)
	item := fileprotocol.MutationItem{SourcePath: "versioned.txt", AppendData: []byte("new"), Preconditions: fileprotocol.Preconditions{ExpectedModTime: stale}}
	if _, err := root.mutate(fileprotocol.MutationAppend, fileprotocol.ConflictFail, false, item); err == nil {
		t.Fatal("mutate() stale-version error = nil")
	}
}

func TestMutationDryRunDoesNotChangeFilesystem(t *testing.T) {
	t.Parallel()
	rootPath := t.TempDir()
	root, err := openRoot(rootPath)
	if err != nil {
		t.Fatalf("openRoot() error = %v", err)
	}
	defer func() { _ = root.close() }()
	item := fileprotocol.MutationItem{DestinationPath: "planned.txt"}
	result, err := root.mutate(fileprotocol.MutationCreateFile, fileprotocol.ConflictFail, true, item)
	if err != nil {
		t.Fatalf("mutate() error = %v", err)
	}
	if result.State != plannedMutationState {
		t.Fatalf("state = %q, want planned", result.State)
	}
	if _, err := os.Lstat(filepath.Join(rootPath, "planned.txt")); !os.IsNotExist(err) {
		t.Fatalf("dry-run target exists: %v", err)
	}
}

func TestMutationDeleteRemovesFilesLinksAndEmptyDirectories(t *testing.T) {
	t.Parallel()
	rootPath := t.TempDir()
	mustMakeDirectory(t, filepath.Join(rootPath, "empty"))
	root, err := openRoot(rootPath)
	if err != nil {
		t.Fatalf("openRoot() error = %v", err)
	}
	defer func() { _ = root.close() }()
	mustWriteTestFile(t, filepath.Join(rootPath, "report.txt"), "report")
	mustWriteTestFile(t, filepath.Join(rootPath, "target.txt"), "target")
	if err := os.Symlink("target.txt", filepath.Join(rootPath, "link.txt")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	result, err := root.mutate(fileprotocol.MutationDelete, fileprotocol.ConflictFail, false, fileprotocol.MutationItem{SourcePath: "report.txt"})
	if err != nil {
		t.Fatalf("delete file mutate() error = %v", err)
	}
	if result.State != completedTestState {
		t.Fatalf("delete file state = %q, want completed", result.State)
	}
	requireNotExist(t, filepath.Join(rootPath, "report.txt"))
	if _, err := root.mutate(fileprotocol.MutationDelete, fileprotocol.ConflictFail, false, fileprotocol.MutationItem{SourcePath: "link.txt"}); err != nil {
		t.Fatalf("delete link mutate() error = %v", err)
	}
	requireNotExist(t, filepath.Join(rootPath, "link.txt"))
	requireExists(t, filepath.Join(rootPath, "target.txt"))
	if _, err := root.mutate(fileprotocol.MutationDelete, fileprotocol.ConflictFail, false, fileprotocol.MutationItem{SourcePath: "empty"}); err != nil {
		t.Fatalf("delete directory mutate() error = %v", err)
	}
	requireNotExist(t, filepath.Join(rootPath, "empty"))
}

func TestMutationDeleteRecursivelyRemovesDirectoryWithoutFollowingLinks(t *testing.T) {
	t.Parallel()
	rootPath := t.TempDir()
	outsidePath := t.TempDir()
	directoryPath := filepath.Join(rootPath, "tree")
	if err := os.MkdirAll(filepath.Join(directoryPath, "nested"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	mustWriteTestFile(t, filepath.Join(directoryPath, "nested", "remove.txt"), "remove")
	outsideFile := filepath.Join(outsidePath, "keep.txt")
	mustWriteTestFile(t, outsideFile, "keep")
	if err := os.Symlink(outsidePath, filepath.Join(directoryPath, "outside-link")); err != nil {
		t.Fatalf("Symlink() error = %v", err)
	}
	root, err := openRoot(rootPath)
	if err != nil {
		t.Fatalf("openRoot() error = %v", err)
	}
	defer func() { _ = root.close() }()
	item := fileprotocol.MutationItem{SourcePath: "tree"}
	result, err := root.mutate(fileprotocol.MutationDelete, fileprotocol.ConflictFail, true, item)
	if err != nil || result.State != plannedMutationState {
		t.Fatalf("dry-run recursive delete = %+v, %v; want planned", result, err)
	}
	requireExists(t, filepath.Join(directoryPath, "nested", "remove.txt"))
	if _, err := root.mutate(fileprotocol.MutationDelete, fileprotocol.ConflictFail, false, item); err != nil {
		t.Fatalf("recursive delete mutate() error = %v", err)
	}
	requireNotExist(t, directoryPath)
	requireExists(t, outsideFile)
}

func mustMakeDirectory(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir(%q) error = %v", path, err)
	}
}

func mustWriteTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
}

func requireExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("Lstat(%q) error = %v", path, err)
	}
}

func requireNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("Lstat(%q) error = %v, want not found", path, err)
	}
}

func TestMutationDeletePlanIsBounded(t *testing.T) {
	t.Parallel()
	rootPath := t.TempDir()
	directoryPath := filepath.Join(rootPath, "tree")
	if err := os.Mkdir(directoryPath, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(directoryPath, "one.txt"), []byte("one"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	root, err := openRoot(rootPath)
	if err != nil {
		t.Fatalf("openRoot() error = %v", err)
	}
	defer func() { _ = root.close() }()
	if plan, err := root.buildDeletePlan([]string{"tree"}, 1); err == nil {
		t.Fatalf("buildDeletePlan() = %+v, nil; want entry-limit error", plan)
	}
	if _, err := os.Lstat(filepath.Join(directoryPath, "one.txt")); err != nil {
		t.Fatalf("bounded plan changed filesystem: %v", err)
	}
}
