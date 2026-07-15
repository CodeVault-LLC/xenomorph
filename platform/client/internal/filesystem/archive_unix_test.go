//go:build linux || darwin

package filesystem

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func TestZIPArchiveCreationRejectsSymbolicLinkSource(t *testing.T) {
	directory := t.TempDir()

	target := filepath.Join(directory, "target.txt")
	if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(target, filepath.Join(directory, "link.txt")); err != nil {
		t.Fatal(err)
	}

	root, err := openRoot(directory)
	if err != nil {
		t.Fatal(err)
	}

	defer closeRootAfterRead(root)

	request := testArchiveRequest(fileprotocol.ArchiveCreate)
	request.SourcePaths, request.ArchivePath = []string{"link.txt"}, "bundle.zip"

	if _, err := root.createZIP(context.Background(), request); err == nil {
		t.Fatal("createZIP() error = nil, want no-follow link rejection")
	}
}
