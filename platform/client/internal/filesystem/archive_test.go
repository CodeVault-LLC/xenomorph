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

const completedTestState = "completed"

func TestZIPArchiveCreateListExtractRoundTrip(t *testing.T) {
	directory := t.TempDir()
	requireNoError(t, os.MkdirAll(filepath.Join(directory, "source", "nested"), 0o700))
	requireNoError(t, os.WriteFile(filepath.Join(directory, "source", "nested", "report.txt"), []byte("bounded archive"), 0o600))
	requireNoError(t, os.Mkdir(filepath.Join(directory, "output"), 0o700))
	root, err := openRoot(directory)
	requireNoError(t, err)
	defer closeRootAfterRead(root)

	request := testArchiveRequest(fileprotocol.ArchiveCreate)
	request.SourcePaths = []string{"source"}
	request.ArchivePath = "bundle.zip"
	created, err := root.createZIP(context.Background(), request)
	requireArchiveResult(t, created, err, 3)
	request.Action = fileprotocol.ArchiveList
	listed, err := root.listZIP(context.Background(), request)
	if err != nil || len(listed.Entries) != 3 || listed.Entries[0].Path != "source" {
		t.Fatalf("listZIP() = (%+v, %v)", listed, err)
	}
	request.Action = fileprotocol.ArchiveExtract
	request.DestinationPath = "output"
	extracted, err := root.extractZIP(context.Background(), request)
	requireArchiveResult(t, extracted, err, 3)
	// #nosec G304 -- the path is below the isolated test root.
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

func TestZIPArchiveRejectsCollidingEntryNames(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "colliding.zip")
	file, err := os.Create(path) // #nosec G304 -- path is an isolated test fixture.
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, name := range []string{"Report.txt", "report.txt"} {
		entry, createErr := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if createErr == nil {
			_, createErr = entry.Write([]byte(name))
		}
		if createErr != nil {
			t.Fatal(createErr)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	root, err := openRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer closeRootAfterRead(root)
	request := testArchiveRequest(fileprotocol.ArchiveList)
	request.ArchivePath = "colliding.zip"
	if _, err := root.listZIP(context.Background(), request); err == nil {
		t.Fatal("listZIP() error = nil, want collision rejection")
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
	// #nosec G304 -- path is an isolated test fixture.
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

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func requireArchiveResult(t *testing.T, result fileprotocol.ArchiveResult, err error, entries int) {
	t.Helper()
	if err != nil || result.State != completedTestState || result.EntriesProcessed != entries {
		t.Fatalf("archive result = (%+v, %v), want completed with %d entries", result, err, entries)
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
