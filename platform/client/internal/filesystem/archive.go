package filesystem

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

const (
	archiveCopyBufferSize      = 64 << 10
	archiveFilePermission      = 0o600
	archiveDirectoryPermission = 0o700
)

type archivePlanEntry struct {
	components []string
	name       string
	info       os.FileInfo
}

type archiveStage struct {
	file    *os.File
	publish func(fileprotocol.ConflictStrategy) (string, bool, error)
	abort   func()
}

// ExecuteArchive performs one bounded ZIP operation below a currently
// discovered root. Archive paths and results remain untrusted client evidence.
func ExecuteArchive(ctx context.Context, request fileprotocol.ArchiveRequest) (fileprotocol.ArchiveResult, error) {
	if err := validateArchiveRequest(request); err != nil {
		return fileprotocol.ArchiveResult{}, err
	}
	definition, err := resolveFilesystemRoot(request.RootID)
	if err != nil {
		return fileprotocol.ArchiveResult{}, err
	}
	root, err := openRoot(definition.Path)
	if err != nil {
		return fileprotocol.ArchiveResult{}, fmt.Errorf("open filesystem root: %w", err)
	}
	defer closeRootAfterRead(root)
	ctx, cancel := context.WithTimeout(ctx, request.Limits.MaxRuntime)
	defer cancel()
	switch request.Action {
	case fileprotocol.ArchiveCreate:
		return root.createZIP(ctx, request)
	case fileprotocol.ArchiveList:
		return root.listZIP(ctx, request)
	case fileprotocol.ArchiveExtract:
		return root.extractZIP(ctx, request)
	default:
		return fileprotocol.ArchiveResult{}, fmt.Errorf("archive action is not supported")
	}
}

func validateArchiveRequest(request fileprotocol.ArchiveRequest) error {
	limits := request.Limits
	if request.ProtocolVersion != fileprotocol.Version || request.OperationID == "" || request.Format != fileprotocol.ArchiveZIP {
		return fmt.Errorf("invalid archive protocol envelope")
	}
	if limits.MaxEntries <= 0 || limits.MaxEntries > 10_000 || limits.MaxDepth <= 0 || limits.MaxDepth > 64 ||
		limits.MaxExpandedBytes <= 0 || limits.MaxExpandedBytes > 1<<30 || limits.MaxTemporaryBytes <= 0 || limits.MaxTemporaryBytes > 1<<30 ||
		limits.MaxCompressionRatio <= 0 || limits.MaxCompressionRatio > 100 || limits.MaxRuntime <= 0 || limits.MaxRuntime > 30*time.Second ||
		limits.MaxListedEntries <= 0 || limits.MaxListedEntries > 250 || limits.MaxListedNameBytes <= 0 || limits.MaxListedNameBytes > 32<<10 {
		return fmt.Errorf("archive limits are outside fixed maxima")
	}
	if _, err := validateArchivePath(request.ArchivePath); err != nil {
		return fmt.Errorf("archive path is invalid: %w", err)
	}
	return nil
}

func (root *rootHandle) createZIP(ctx context.Context, request fileprotocol.ArchiveRequest) (fileprotocol.ArchiveResult, error) {
	plan, total, err := root.buildArchivePlan(ctx, request.SourcePaths, request.Limits)
	if err != nil {
		return fileprotocol.ArchiveResult{}, err
	}
	destination, _ := validateArchivePath(request.ArchivePath)
	stage, err := root.openArchiveStage(destination)
	if err != nil {
		return fileprotocol.ArchiveResult{}, err
	}
	committed := false
	defer func() {
		if !committed {
			stage.abort()
		}
	}()
	bounded := &archiveBoundedWriter{writer: stage.file, remaining: request.Limits.MaxTemporaryBytes}
	writer := zip.NewWriter(bounded)
	for _, entry := range plan {
		if err := ctx.Err(); err != nil {
			_ = writer.Close()
			return fileprotocol.ArchiveResult{}, err
		}
		if err := writeArchiveEntry(ctx, writer, root, entry); err != nil {
			_ = writer.Close()
			return fileprotocol.ArchiveResult{}, err
		}
	}
	if err := writer.Close(); err != nil {
		return fileprotocol.ArchiveResult{}, fmt.Errorf("finalize ZIP archive: %w", err)
	}
	if err := stage.file.Sync(); err != nil {
		return fileprotocol.ArchiveResult{}, err
	}
	if err := stage.file.Close(); err != nil {
		return fileprotocol.ArchiveResult{}, err
	}
	_, skipped, err := stage.publish(request.Conflict)
	if err != nil {
		return fileprotocol.ArchiveResult{}, err
	}
	committed = true
	state := "completed"
	if skipped {
		state = "skipped"
	}
	return archiveResult(request, state, len(plan), total), nil
}

func (root *rootHandle) buildArchivePlan(ctx context.Context, sources []string, limits fileprotocol.ArchiveLimits) ([]archivePlanEntry, int64, error) {
	if len(sources) == 0 || len(sources) > 100 {
		return nil, 0, fmt.Errorf("archive source count exceeds limit")
	}
	queue := make([]archivePlanEntry, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		components, err := validateArchivePath(source)
		if err != nil || len(components) == 0 {
			return nil, 0, fmt.Errorf("archive source path is invalid")
		}
		queue = append(queue, archivePlanEntry{components: components, name: components[len(components)-1]})
	}
	plan := make([]archivePlanEntry, 0, len(queue))
	var total int64
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, 0, err
		}
		entry := queue[0]
		queue = queue[1:]
		if _, duplicate := seen[entry.name]; duplicate {
			return nil, 0, fmt.Errorf("archive sources overlap")
		}
		seen[entry.name] = struct{}{}
		if len(plan) >= limits.MaxEntries || len(entry.components) > limits.MaxDepth {
			return nil, 0, fmt.Errorf("archive traversal exceeds limit")
		}
		info, err := root.statNoFollow(entry.components)
		if err != nil {
			return nil, 0, err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return nil, 0, fmt.Errorf("archive sources must be regular files or directories")
		}
		entry.info = info
		plan = append(plan, entry)
		if info.Mode().IsRegular() {
			if info.Size() < 0 || total > limits.MaxExpandedBytes-info.Size() {
				return nil, 0, fmt.Errorf("archive expanded bytes exceed limit")
			}
			total += info.Size()
			continue
		}
		directory, _, err := root.openDirectory(entry.components)
		if err != nil {
			return nil, 0, err
		}
		names, readErr := directory.Readdirnames(limits.MaxEntries + 1)
		_ = directory.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return nil, 0, readErr
		}
		for _, name := range names {
			child := append(append([]string(nil), entry.components...), name)
			childName := entry.name + "/" + name
			if _, err := validateArchivePath(childName); err != nil {
				return nil, 0, fmt.Errorf("archive source name is unsafe")
			}
			queue = append(queue, archivePlanEntry{components: child, name: childName})
		}
	}
	return plan, total, nil
}

func writeArchiveEntry(ctx context.Context, writer *zip.Writer, root *rootHandle, entry archivePlanEntry) error {
	header, err := zip.FileInfoHeader(entry.info)
	if err != nil {
		return err
	}
	header.Name = entry.name
	header.Method = zip.Store
	if entry.info.IsDir() {
		header.Name += "/"
		_, err = writer.CreateHeader(header)
		return err
	}
	file, current, err := root.openRegularFile(entry.components)
	if err != nil {
		return err
	}
	defer closeFileAfterRead(file)
	if current.Size() != entry.info.Size() || !current.ModTime().UTC().Equal(entry.info.ModTime().UTC()) {
		return fsConflict("archive source changed during creation")
	}
	destination, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	written, err := io.CopyBuffer(destination, &archiveContextReader{ctx: ctx, reader: file}, make([]byte, archiveCopyBufferSize))
	if err != nil {
		return err
	}
	if written != current.Size() {
		return fsConflict("archive source size changed during creation")
	}
	return nil
}

func (root *rootHandle) listZIP(ctx context.Context, request fileprotocol.ArchiveRequest) (fileprotocol.ArchiveResult, error) {
	archive, entries, total, err := root.inspectZIP(ctx, request)
	if archive != nil {
		defer closeFileAfterRead(archive)
	}
	if err != nil {
		return fileprotocol.ArchiveResult{}, err
	}
	listed := make([]fileprotocol.ArchiveEntry, 0, min(len(entries), request.Limits.MaxListedEntries))
	nameBytes := 0
	for _, entry := range entries {
		if len(listed) >= request.Limits.MaxListedEntries || nameBytes+len(entry.Name) > request.Limits.MaxListedNameBytes {
			break
		}
		kind := fileprotocol.EntryFile
		if entry.FileInfo().IsDir() {
			kind = fileprotocol.EntryDirectory
		}
		listed = append(listed, fileprotocol.ArchiveEntry{Path: strings.TrimSuffix(entry.Name, "/"), Kind: kind, UncompressedSize: int64(entry.UncompressedSize64), CompressedSize: int64(entry.CompressedSize64)})
		nameBytes += len(entry.Name)
	}
	result := archiveResult(request, "completed", len(entries), total)
	result.Entries, result.Truncated = listed, len(listed) < len(entries)
	return result, nil
}

func (root *rootHandle) inspectZIP(ctx context.Context, request fileprotocol.ArchiveRequest) (*os.File, []*zip.File, int64, error) {
	components, _ := validateArchivePath(request.ArchivePath)
	if hasPreconditions(request.Preconditions) {
		if err := root.checkPreconditions(components, request.Preconditions); err != nil {
			return nil, nil, 0, err
		}
	}
	file, info, err := root.openRegularFile(components)
	if err != nil {
		return nil, nil, 0, err
	}
	if info.Size() < 0 || info.Size() > request.Limits.MaxTemporaryBytes {
		_ = file.Close()
		return nil, nil, 0, fmt.Errorf("archive file size exceeds limit")
	}
	if err := preflightZIP(file, info.Size(), request.Limits.MaxEntries); err != nil {
		_ = file.Close()
		return nil, nil, 0, err
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		_ = file.Close()
		return nil, nil, 0, fmt.Errorf("parse ZIP archive: %w", err)
	}
	if len(reader.File) > request.Limits.MaxEntries {
		_ = file.Close()
		return nil, nil, 0, fmt.Errorf("archive entry count exceeds limit")
	}
	var total int64
	for _, entry := range reader.File {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, nil, 0, err
		}
		components, err := validateArchiveEntry(entry)
		if err != nil || len(components) > request.Limits.MaxDepth {
			_ = file.Close()
			return nil, nil, 0, fmt.Errorf("archive contains an unsafe entry")
		}
		byteLimit := request.Limits.MaxExpandedBytes
		if request.Action == fileprotocol.ArchiveExtract && request.Limits.MaxTemporaryBytes < byteLimit {
			byteLimit = request.Limits.MaxTemporaryBytes
		}
		if entry.UncompressedSize64 > uint64(byteLimit) || entry.CompressedSize64 > uint64(request.Limits.MaxTemporaryBytes) {
			_ = file.Close()
			return nil, nil, 0, fmt.Errorf("archive entry size exceeds limit")
		}
		uncompressed := int64(entry.UncompressedSize64)
		compressed := int64(entry.CompressedSize64)
		if total > byteLimit-uncompressed || exceedsCompressionRatio(uncompressed, compressed, request.Limits.MaxCompressionRatio) {
			_ = file.Close()
			return nil, nil, 0, fmt.Errorf("archive expansion exceeds limit")
		}
		total += uncompressed
	}
	return file, reader.File, total, nil
}

func preflightZIP(file *os.File, size int64, maxEntries int) error {
	const (
		endRecordMinimum = int64(22)
		endRecordMaximum = int64(22 + 65_535)
	)
	if size < endRecordMinimum {
		return fmt.Errorf("ZIP archive is truncated")
	}
	length := min(size, endRecordMaximum)
	buffer := make([]byte, length)
	if _, err := file.ReadAt(buffer, size-length); err != nil {
		return fmt.Errorf("read ZIP directory footer: %w", err)
	}
	signature := []byte{'P', 'K', 5, 6}
	index := -1
	for candidate := len(buffer) - int(endRecordMinimum); candidate >= 0; candidate-- {
		if bytes.Equal(buffer[candidate:candidate+4], signature) {
			index = candidate
			break
		}
	}
	if index < 0 {
		return fmt.Errorf("ZIP directory footer is missing")
	}
	entries := binary.LittleEndian.Uint16(buffer[index+10 : index+12])
	if entries == ^uint16(0) {
		return fmt.Errorf("ZIP64 archives are not supported")
	}
	if int(entries) > maxEntries {
		return fmt.Errorf("archive entry count exceeds limit")
	}
	return nil
}

func validateArchiveEntry(entry *zip.File) ([]string, error) {
	name := strings.TrimSuffix(entry.Name, "/")
	components, err := validateArchivePath(name)
	if err != nil || len(components) == 0 {
		return nil, fmt.Errorf("archive entry path is invalid")
	}
	mode := entry.Mode()
	if mode&os.ModeSymlink != 0 || !mode.IsRegular() && !mode.IsDir() {
		return nil, fmt.Errorf("archive entry type is unsafe")
	}
	return components, nil
}

func exceedsCompressionRatio(uncompressed, compressed, limit int64) bool {
	if uncompressed == 0 {
		return false
	}
	if compressed == 0 {
		return true
	}
	return uncompressed > compressed*limit
}

func (root *rootHandle) extractZIP(ctx context.Context, request fileprotocol.ArchiveRequest) (fileprotocol.ArchiveResult, error) {
	archive, entries, total, err := root.inspectZIP(ctx, request)
	if err != nil {
		return fileprotocol.ArchiveResult{}, err
	}
	defer closeFileAfterRead(archive)
	destination, err := validateArchivePath(request.DestinationPath)
	if err != nil || len(destination) == 0 {
		return fileprotocol.ArchiveResult{}, fmt.Errorf("archive destination is invalid")
	}
	if err := root.requireArchiveDirectory(destination); err != nil {
		return fileprotocol.ArchiveResult{}, err
	}
	processed := 0
	var bytesProcessed int64
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return fileprotocol.ArchiveResult{}, err
		}
		entryComponents, _ := validateArchiveEntry(entry)
		target := append(append([]string(nil), destination...), entryComponents...)
		if entry.FileInfo().IsDir() {
			if err := root.ensureArchiveDirectories(target); err != nil {
				return fileprotocol.ArchiveResult{}, err
			}
			processed++
			continue
		}
		if err := root.ensureArchiveDirectories(target[:len(target)-1]); err != nil {
			return fileprotocol.ArchiveResult{}, err
		}
		stage, err := root.openArchiveStage(target)
		if err != nil {
			return fileprotocol.ArchiveResult{}, err
		}
		source, err := entry.Open()
		if err != nil {
			stage.abort()
			return fileprotocol.ArchiveResult{}, err
		}
		written, copyErr := io.CopyBuffer(stage.file, &archiveContextReader{ctx: ctx, reader: source}, make([]byte, archiveCopyBufferSize))
		closeErr := source.Close()
		if copyErr != nil || closeErr != nil || written != int64(entry.UncompressedSize64) {
			stage.abort()
			return fileprotocol.ArchiveResult{}, fmt.Errorf("extract archive entry: %w", errors.Join(copyErr, closeErr))
		}
		if err := stage.file.Sync(); err != nil {
			stage.abort()
			return fileprotocol.ArchiveResult{}, err
		}
		if err := stage.file.Close(); err != nil {
			stage.abort()
			return fileprotocol.ArchiveResult{}, err
		}
		_, _, err = stage.publish(request.Conflict)
		if err != nil {
			stage.abort()
			return fileprotocol.ArchiveResult{}, err
		}
		processed++
		bytesProcessed += written
	}
	result := archiveResult(request, "completed", processed, bytesProcessed)
	result.BytesProcessed = total
	return result, nil
}

func validateArchivePath(value string) ([]string, error) {
	components, err := validateRelativePath(value)
	if err != nil {
		return nil, err
	}
	for _, component := range components {
		if strings.ContainsAny(component, `:*?"<>|`) || strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") {
			return nil, fmt.Errorf("archive path contains a device-like component")
		}
		base := strings.ToUpper(strings.TrimSuffix(component, filepath.Ext(component)))
		if reservedArchiveBase(base) {
			return nil, fmt.Errorf("archive path contains a reserved component")
		}
	}
	return components, nil
}

func reservedArchiveBase(base string) bool {
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	return len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9'
}

func archiveResult(request fileprotocol.ArchiveRequest, state string, entries int, bytes int64) fileprotocol.ArchiveResult {
	return fileprotocol.ArchiveResult{ProtocolVersion: fileprotocol.Version, OperationID: request.OperationID, State: state, EntriesProcessed: entries, BytesProcessed: bytes}
}

type archiveBoundedWriter struct {
	writer    io.Writer
	remaining int64
}

func (writer *archiveBoundedWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > writer.remaining {
		return 0, fmt.Errorf("archive temporary bytes exceed limit")
	}
	written, err := writer.writer.Write(data)
	writer.remaining -= int64(written)
	return written, err
}

type archiveContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (reader *archiveContextReader) Read(data []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(data)
}
