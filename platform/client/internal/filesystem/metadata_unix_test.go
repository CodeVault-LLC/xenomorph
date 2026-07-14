//go:build linux || darwin

package filesystem

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func TestSetMetadataAppliesExplicitFieldsWithoutFollowingLinks(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.txt")
	if err := os.WriteFile(target, []byte("bounded"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := openRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer closeRootAfterRead(root)
	mode := uint32(0o640)
	modified := time.Unix(1_700_000_000, 123_000_000).UTC()
	results := root.setMetadata([]string{"target.txt"}, fileprotocol.MetadataDelta{ModifiedAt: &modified, POSIXMode: &mode})
	if len(results) != 2 || results[0].State != fileprotocol.MetadataApplied || results[1].State != fileprotocol.MetadataApplied {
		t.Fatalf("setMetadata() = %+v, want two applied fields", results)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 || !info.ModTime().UTC().Equal(modified) {
		t.Fatalf("metadata = (%o, %s), want (640, %s)", info.Mode().Perm(), info.ModTime(), modified)
	}
	link := filepath.Join(directory, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	linkResults := root.setMetadata([]string{"link.txt"}, fileprotocol.MetadataDelta{POSIXMode: &mode})
	if len(linkResults) != 1 || linkResults[0].State == fileprotocol.MetadataApplied {
		t.Fatalf("setMetadata(link) = %+v, want explicit non-applied result", linkResults)
	}
}

func TestSetMetadataRejectsOutOfRangeMode(t *testing.T) {
	root, err := openRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer closeRootAfterRead(root)
	mode := uint32(0o10000)
	results := root.setMetadata([]string{"missing"}, fileprotocol.MetadataDelta{POSIXMode: &mode})
	if len(results) != 1 || results[0].State == fileprotocol.MetadataApplied {
		t.Fatalf("setMetadata() = %+v, want explicit failure", results)
	}
}
