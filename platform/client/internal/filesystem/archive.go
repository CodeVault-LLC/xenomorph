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
	archiveMaxSources          = 100
	archiveMaxEntries          = 10_000
	archiveMaxDepth            = 64
	archiveMaxBytes            = 1 << 30
	archiveMaxCompressionRatio = 100
	archiveMaxRuntime          = 30 * time.Second
	archiveMaxListedEntries    = 250
	archiveMaxListedNameBytes  = 32 << 10
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
	if request.ProtocolVersion != fileprotocol.Version || request.OperationID == "" || request.Format != fileprotocol.ArchiveZIP {
		return fmt.Errorf("invalid archive protocol envelope")
	}

	if err := validateArchiveLimits(request.Limits); err != nil {
		return err
	}

	if _, err := validateArchivePath(request.ArchivePath); err != nil {
		return fmt.Errorf("archive path is invalid: %w", err)
	}

	return nil
}

func validateArchiveLimits(limits fileprotocol.ArchiveLimits) error {
	if !withinArchiveInt(limits.MaxEntries, archiveMaxEntries) || !withinArchiveInt(limits.MaxDepth, archiveMaxDepth) {
		return fmt.Errorf("archive limits are outside fixed maxima")
	}

	if !withinArchiveInt64(limits.MaxExpandedBytes, archiveMaxBytes) || !withinArchiveInt64(limits.MaxTemporaryBytes, archiveMaxBytes) {
		return fmt.Errorf("archive byte limits are outside fixed maxima")
	}

	if !withinArchiveInt64(limits.MaxCompressionRatio, archiveMaxCompressionRatio) || !withinArchiveDuration(limits.MaxRuntime, archiveMaxRuntime) {
		return fmt.Errorf("archive execution limits are outside fixed maxima")
	}

	if !withinArchiveInt(limits.MaxListedEntries, archiveMaxListedEntries) || !withinArchiveInt(limits.MaxListedNameBytes, archiveMaxListedNameBytes) {
		return fmt.Errorf("archive listing limits are outside fixed maxima")
	}

	return nil
}

func withinArchiveInt(value, maximum int) bool { return value > 0 && value <= maximum }

func withinArchiveInt64(value, maximum int64) bool { return value > 0 && value <= maximum }

func withinArchiveDuration(value, maximum time.Duration) bool { return value > 0 && value <= maximum }

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

	if err := writeZIPPlan(ctx, root, stage.file, plan, request.Limits.MaxTemporaryBytes); err != nil {
		return fileprotocol.ArchiveResult{}, err
	}

	state, err := publishArchiveStage(stage, request.Conflict)
	if err != nil {
		return fileprotocol.ArchiveResult{}, err
	}

	committed = true

	return archiveResult(request, state, len(plan), total), nil
}

func writeZIPPlan(ctx context.Context, root *rootHandle, file *os.File, plan []archivePlanEntry, temporaryLimit int64) error {
	bounded := &archiveBoundedWriter{writer: file, remaining: temporaryLimit}
	writer := zip.NewWriter(bounded)

	for _, entry := range plan {
		if err := ctx.Err(); err != nil {
			_ = writer.Close()
			return err
		}

		if err := writeArchiveEntry(ctx, writer, root, entry); err != nil {
			_ = writer.Close()
			return err
		}
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("finalize ZIP archive: %w", err)
	}

	if err := file.Sync(); err != nil {
		return err
	}

	return file.Close()
}

func publishArchiveStage(stage archiveStage, conflict fileprotocol.ConflictStrategy) (string, error) {
	_, skipped, err := stage.publish(conflict)
	if err != nil {
		return "", err
	}

	if skipped {
		return "skipped", nil
	}

	return "completed", nil
}

func (root *rootHandle) buildArchivePlan(ctx context.Context, sources []string, limits fileprotocol.ArchiveLimits) ([]archivePlanEntry, int64, error) {
	queue, err := archiveSourceQueue(sources)
	if err != nil {
		return nil, 0, err
	}

	planner := archivePlanner{root: root, ctx: ctx, limits: limits, queue: queue, seen: make(map[string]struct{}, len(queue))}

	return planner.run()
}

func archiveSourceQueue(sources []string) ([]archivePlanEntry, error) {
	if len(sources) == 0 || len(sources) > archiveMaxSources {
		return nil, fmt.Errorf("archive source count exceeds limit")
	}

	queue := make([]archivePlanEntry, 0, len(sources))

	for _, source := range sources {
		components, err := validateArchivePath(source)
		if err != nil || len(components) == 0 {
			return nil, fmt.Errorf("archive source path is invalid")
		}

		queue = append(queue, archivePlanEntry{components: components, name: components[len(components)-1]})
	}

	return queue, nil
}

type archivePlanner struct {
	root   *rootHandle
	ctx    context.Context
	limits fileprotocol.ArchiveLimits
	queue  []archivePlanEntry
	plan   []archivePlanEntry
	seen   map[string]struct{}
	total  int64
}

func (planner *archivePlanner) run() ([]archivePlanEntry, int64, error) {
	planner.plan = make([]archivePlanEntry, 0, len(planner.queue))
	for len(planner.queue) > 0 {
		if err := planner.ctx.Err(); err != nil {
			return nil, 0, err
		}

		entry := planner.dequeue()
		if _, duplicate := planner.seen[entry.name]; duplicate {
			return nil, 0, fmt.Errorf("archive sources overlap")
		}

		planner.seen[entry.name] = struct{}{}
		if len(planner.plan) >= planner.limits.MaxEntries || len(entry.components) > planner.limits.MaxDepth {
			return nil, 0, fmt.Errorf("archive traversal exceeds limit")
		}

		if err := planner.inspect(&entry); err != nil {
			return nil, 0, err
		}
	}

	return planner.plan, planner.total, nil
}

func (planner *archivePlanner) dequeue() archivePlanEntry {
	entry := planner.queue[0]
	planner.queue = planner.queue[1:]

	return entry
}

func (planner *archivePlanner) inspect(entry *archivePlanEntry) error {
	info, err := planner.root.statNoFollow(entry.components)
	if err != nil {
		return err
	}

	if !info.IsDir() && !info.Mode().IsRegular() {
		return fmt.Errorf("archive sources must be regular files or directories")
	}

	entry.info = info

	planner.plan = append(planner.plan, *entry)
	if info.Mode().IsRegular() {
		return planner.addFileSize(info.Size())
	}

	return planner.enqueueDirectory(*entry)
}

func (planner *archivePlanner) addFileSize(size int64) error {
	if size < 0 || planner.total > planner.limits.MaxExpandedBytes-size {
		return fmt.Errorf("archive expanded bytes exceed limit")
	}

	planner.total += size

	return nil
}

func (planner *archivePlanner) enqueueDirectory(entry archivePlanEntry) error {
	directory, _, err := planner.root.openDirectory(entry.components)
	if err != nil {
		return err
	}

	names, readErr := directory.Readdirnames(planner.limits.MaxEntries + 1)
	_ = directory.Close()

	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}

	for _, name := range names {
		child := append(append([]string(nil), entry.components...), name)

		childName := entry.name + "/" + name
		if _, err := validateArchivePath(childName); err != nil {
			return fmt.Errorf("archive source name is unsafe")
		}

		planner.queue = append(planner.queue, archivePlanEntry{components: child, name: childName})
	}

	return nil
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

		uncompressed, err := boundedArchiveSize(entry.UncompressedSize64)
		if err != nil {
			return fileprotocol.ArchiveResult{}, err
		}

		compressed, err := boundedArchiveSize(entry.CompressedSize64)
		if err != nil {
			return fileprotocol.ArchiveResult{}, err
		}

		listed = append(listed, fileprotocol.ArchiveEntry{Path: strings.TrimSuffix(entry.Name, "/"), Kind: kind, UncompressedSize: uncompressed, CompressedSize: compressed})
		nameBytes += len(entry.Name)
	}

	result := archiveResult(request, "completed", len(entries), total)
	result.Entries, result.Truncated = listed, len(listed) < len(entries)

	return result, nil
}

func (root *rootHandle) inspectZIP(ctx context.Context, request fileprotocol.ArchiveRequest) (*os.File, []*zip.File, int64, error) {
	file, entries, err := root.openZIP(request)
	if err != nil {
		return nil, nil, 0, err
	}

	total, kinds, err := inspectZIPEntries(ctx, entries, request)
	if err == nil {
		err = validateArchiveHierarchy(kinds)
	}

	if err != nil {
		_ = file.Close()
		return nil, nil, 0, err
	}

	return file, entries, total, nil
}

func (root *rootHandle) openZIP(request fileprotocol.ArchiveRequest) (*os.File, []*zip.File, error) {
	components, _ := validateArchivePath(request.ArchivePath)
	if hasPreconditions(request.Preconditions) {
		if err := root.checkPreconditions(components, request.Preconditions); err != nil {
			return nil, nil, err
		}
	}

	file, info, err := root.openRegularFile(components)
	if err != nil {
		return nil, nil, err
	}

	if info.Size() < 0 || info.Size() > request.Limits.MaxTemporaryBytes {
		_ = file.Close()
		return nil, nil, fmt.Errorf("archive file size exceeds limit")
	}

	if err := preflightZIP(file, info.Size(), request.Limits.MaxEntries); err != nil {
		_ = file.Close()
		return nil, nil, err
	}

	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		_ = file.Close()
		return nil, nil, fmt.Errorf("parse ZIP archive: %w", err)
	}

	if len(reader.File) > request.Limits.MaxEntries {
		_ = file.Close()
		return nil, nil, fmt.Errorf("archive entry count exceeds limit")
	}

	return file, reader.File, nil
}

func inspectZIPEntries(ctx context.Context, entries []*zip.File, request fileprotocol.ArchiveRequest) (int64, map[string]bool, error) {
	var total int64

	entryKinds := make(map[string]bool, len(entries))

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}

		key, directory, uncompressed, compressed, err := inspectZIPEntry(entry, request)
		if err != nil {
			return 0, nil, err
		}

		if _, duplicate := entryKinds[key]; duplicate {
			return 0, nil, fmt.Errorf("archive contains colliding entry names")
		}

		entryKinds[key] = directory

		byteLimit := archiveExpandedLimit(request)
		if total > byteLimit-uncompressed || exceedsCompressionRatio(uncompressed, compressed, request.Limits.MaxCompressionRatio) {
			return 0, nil, fmt.Errorf("archive expansion exceeds limit")
		}

		total += uncompressed
	}

	return total, entryKinds, nil
}

func inspectZIPEntry(entry *zip.File, request fileprotocol.ArchiveRequest) (string, bool, int64, int64, error) {
	components, err := validateArchiveEntry(entry)
	if err != nil || len(components) > request.Limits.MaxDepth {
		return "", false, 0, 0, fmt.Errorf("archive contains an unsafe entry")
	}

	uncompressed, err := boundedArchiveSize(entry.UncompressedSize64)
	if err != nil || uncompressed > archiveExpandedLimit(request) {
		return "", false, 0, 0, fmt.Errorf("archive entry size exceeds limit")
	}

	compressed, err := boundedArchiveSize(entry.CompressedSize64)
	if err != nil || compressed > request.Limits.MaxTemporaryBytes {
		return "", false, 0, 0, fmt.Errorf("archive entry size exceeds limit")
	}

	key := strings.ToLower(strings.Join(components, "/"))

	return key, entry.FileInfo().IsDir(), uncompressed, compressed, nil
}

func archiveExpandedLimit(request fileprotocol.ArchiveRequest) int64 {
	if request.Action == fileprotocol.ArchiveExtract && request.Limits.MaxTemporaryBytes < request.Limits.MaxExpandedBytes {
		return request.Limits.MaxTemporaryBytes
	}

	return request.Limits.MaxExpandedBytes
}

func validateArchiveHierarchy(entryKinds map[string]bool) error {
	for path := range entryKinds {
		components := strings.Split(path, "/")
		for index := 1; index < len(components); index++ {
			if directory, exists := entryKinds[strings.Join(components[:index], "/")]; exists && !directory {
				return fmt.Errorf("archive contains a file-directory collision")
			}
		}
	}

	return nil
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

func boundedArchiveSize(value uint64) (int64, error) {
	if value > archiveMaxBytes {
		return 0, fmt.Errorf("archive entry size exceeds limit")
	}
	// #nosec G115 -- value is bounded to 1 GiB immediately above.
	return int64(value), nil
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

		written, err := root.extractZIPEntry(ctx, request.Conflict, destination, entry)
		if err != nil {
			return fileprotocol.ArchiveResult{}, err
		}

		processed++
		bytesProcessed += written
	}

	result := archiveResult(request, "completed", processed, bytesProcessed)
	result.BytesProcessed = total

	return result, nil
}

func (root *rootHandle) extractZIPEntry(ctx context.Context, conflict fileprotocol.ConflictStrategy, destination []string, entry *zip.File) (int64, error) {
	entryComponents, _ := validateArchiveEntry(entry)

	target := append(append([]string(nil), destination...), entryComponents...)
	if entry.FileInfo().IsDir() {
		return 0, root.ensureArchiveDirectories(target)
	}

	if err := root.ensureArchiveDirectories(target[:len(target)-1]); err != nil {
		return 0, err
	}

	stage, err := root.openArchiveStage(target)
	if err != nil {
		return 0, err
	}

	written, err := copyZIPEntry(ctx, stage.file, entry)
	if err != nil {
		stage.abort()
		return 0, err
	}

	if _, _, err := stage.publish(conflict); err != nil {
		stage.abort()
		return 0, err
	}

	return written, nil
}

func copyZIPEntry(ctx context.Context, destination *os.File, entry *zip.File) (int64, error) {
	source, err := entry.Open()
	if err != nil {
		return 0, err
	}

	written, copyErr := io.CopyBuffer(destination, &archiveContextReader{ctx: ctx, reader: source}, make([]byte, archiveCopyBufferSize))
	closeErr := source.Close()
	expected, sizeErr := boundedArchiveSize(entry.UncompressedSize64)

	if copyErr != nil || closeErr != nil || sizeErr != nil || written != expected {
		return 0, fmt.Errorf("extract archive entry: %w", errors.Join(copyErr, closeErr, sizeErr))
	}

	if err := destination.Sync(); err != nil {
		return 0, err
	}

	return written, destination.Close()
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
