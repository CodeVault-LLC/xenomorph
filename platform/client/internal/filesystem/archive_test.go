package filesystem

import (
	"archive/zip"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

func TestZIPArchiveCreateListExtractRoundTrip(t *testing.T) {
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "source", "nested"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "source", "nested", "report.txt"), []byte("bounded archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, "output"), 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := openRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer closeRootAfterRead(root)

	request := testArchiveRequest(fileprotocol.ArchiveCreate)
	request.SourcePaths = []string{"source"}
	request.ArchivePath = "bundle.zip"
	created, err := root.createZIP(context.Background(), request)
	if err != nil || created.State != "completed" || created.EntriesProcessed != 3 {
		t.Fatalf("createZIP() = (%+v, %v)", created, err)
	}
	request.Action = fileprotocol.ArchiveList
	listed, err := root.listZIP(context.Background(), request)
	if err != nil || len(listed.Entries) != 3 || listed.Entries[0].Path != "source" {
		t.Fatalf("listZIP() = (%+v, %v)", listed, err)
	}
	request.Action = fileprotocol.ArchiveExtract
	request.DestinationPath = "output"
	extracted, err := root.extractZIP(context.Background(), request)
	if err != nil || extracted.EntriesProcessed != 3 {
		t.Fatalf("extractZIP() = (%+v, %v)", extracted, err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "output", "source", "nested", "report.txt"))
	if err != nil || string(data) != "bounded archive" {
		t.Fatalf("extracted data = %q, %v", data, err)
	}
}

func TestZIPArchiveRejectsTraversalBeforeExtraction(t *testing.T) {
	directory := t.TempDir()
	writeZIPFixture(t, filepath.Join(directory, "unsafe.zip"), "../escape.txt", []byte("escape"), zip.Store)
	if err := os.Mkdir(filepath.Join(directory, "output"), 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := openRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer closeRootAfterRead(root)
	request := testArchiveRequest(fileprotocol.ArchiveExtract)
	request.ArchivePath, request.DestinationPath = "unsafe.zip", "output"
	if _, err := root.extractZIP(context.Background(), request); err == nil {
		t.Fatal("extractZIP() error = nil, want traversal rejection")
	}
	if _, err := os.Stat(filepath.Join(directory, "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("escape path stat error = %v, want not exist", err)
	}
}

func TestZIPArchiveRejectsExcessiveCompressionRatio(t *testing.T) {
	directory := t.TempDir()
	writeZIPFixture(t, filepath.Join(directory, "bomb.zip"), "large.txt", []byte(strings.Repeat("A", 2<<20)), zip.Deflate)
	root, err := openRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer closeRootAfterRead(root)
	request := testArchiveRequest(fileprotocol.ArchiveList)
	request.ArchivePath = "bomb.zip"
	request.Limits.MaxCompressionRatio = 10
	if _, err := root.listZIP(context.Background(), request); err == nil {
		t.Fatal("listZIP() error = nil, want compression-ratio rejection")
	}
}

func TestZIPPreflightRejectsEntryCountBeforeParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "many.zip")
	file, err := os.Create(path) // #nosec G304 -- path is an isolated test fixture.
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for index := 0; index < 11; index++ {
		if _, err := writer.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("%d.txt", index), Method: zip.Store}); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	file, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer closeFileAfterRead(file)
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := preflightZIP(file, info.Size(), 10); err == nil {
		t.Fatal("preflightZIP() error = nil, want entry-count rejection")
	}
}

func testArchiveRequest(action fileprotocol.ArchiveAction) fileprotocol.ArchiveRequest {
	return fileprotocol.ArchiveRequest{
		ProtocolVersion: fileprotocol.Version, OperationID: "operation-1",
		Action: action, Format: fileprotocol.ArchiveZIP, Conflict: fileprotocol.ConflictFail,
		Limits: fileprotocol.ArchiveLimits{
			MaxEntries: 100, MaxDepth: 16, MaxExpandedBytes: 8 << 20,
			MaxTemporaryBytes: 8 << 20, MaxCompressionRatio: 100,
			MaxRuntime: time.Second, MaxListedEntries: 100, MaxListedNameBytes: 4096,
		},
	}
}

func writeZIPFixture(t *testing.T, path, name string, data []byte, method uint16) {
	t.Helper()
	file, err := os.Create(path) // #nosec G304 -- path is an isolated test fixture.
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: method})
	if err == nil {
		_, err = entry.Write(data)
	}
	if closeErr := writer.Close(); err == nil {
		err = closeErr
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
}
