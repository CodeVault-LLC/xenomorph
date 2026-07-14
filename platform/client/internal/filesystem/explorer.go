// Package filesystem owns safe local read-only filesystem operations for the
// agent. It does not own agent identity. It accepts cryptographically verified
// gateway commands and returns client-authored observations that the gateway
// must treat as untrusted evidence.
package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

const (
	maxRelativePath   = 4096
	maxPathComponents = 256
	maxCursorOffset   = 10_000_000
	maxPageSize       = 500
	maxPreviewBytes   = 1 << 20
	maxSearchQuery    = 256
	maxSearchResults  = 250
	maxSearchEntries  = 10_000
	maxSearchDepth    = 16
	windowsOS         = "windows"
	contentText       = "text"
)

type cursor struct {
	Offset     int    `json:"offset"`
	SnapshotID string `json:"snapshot_id"`
}

type searchDirectory struct {
	components []string
	depth      int
}

type rootDefinition struct {
	ID           string
	Path         string
	DisplayLabel string
}

var supportedVerbs = []fileprotocol.Verb{
	fileprotocol.VerbList,
	fileprotocol.VerbMetadata,
	fileprotocol.VerbPreview,
	fileprotocol.VerbTransfer,
	fileprotocol.VerbMutate,
}

// ListRoots enumerates and probes the agent's local filesystem roots.
func ListRoots(request fileprotocol.RootsListRequest) (fileprotocol.RootsListResult, error) {
	if request.ProtocolVersion != fileprotocol.Version {
		return fileprotocol.RootsListResult{}, fmt.Errorf("unsupported file protocol version")
	}
	roots, err := filesystemRoots()
	if err != nil {
		return fileprotocol.RootsListResult{}, err
	}
	result := fileprotocol.RootsListResult{ProtocolVersion: fileprotocol.Version, Roots: make([]fileprotocol.RootObservation, 0, len(roots))}
	for _, definition := range roots {
		observation := fileprotocol.RootObservation{
			RootID: definition.ID, DisplayLabel: definition.DisplayLabel,
			AllowedVerbs: append([]fileprotocol.Verb(nil), supportedVerbs...),
			Capabilities: platformCapabilities(), ReadOnly: false,
		}
		root, err := openRoot(definition.Path)
		if err != nil {
			observation.ErrorClass = classifyError(err)
			result.Roots = append(result.Roots, observation)
			continue
		}
		observation.Available = true
		if err := root.close(); err != nil {
			return fileprotocol.RootsListResult{}, fmt.Errorf("close root probe: %w", err)
		}
		result.Roots = append(result.Roots, observation)
	}
	return result, nil
}

// ListDirectory returns one bounded, no-follow directory page.
func ListDirectory(request fileprotocol.DirectoryListRequest) (fileprotocol.DirectoryPage, error) {
	components, err := validateDirectoryRequest(request)
	if err != nil {
		return fileprotocol.DirectoryPage{}, err
	}
	definition, err := resolveFilesystemRoot(request.RootID)
	if err != nil {
		return fileprotocol.DirectoryPage{}, err
	}
	root, err := openRoot(definition.Path)
	if err != nil {
		return fileprotocol.DirectoryPage{}, fmt.Errorf("open filesystem root: %w", err)
	}
	defer closeRootAfterRead(root)
	directory, snapshotID, err := root.openDirectory(components)
	if err != nil {
		return fileprotocol.DirectoryPage{}, fmt.Errorf("open directory: %w", err)
	}
	defer closeFileAfterRead(directory)
	offset, err := decodeCursor(request.Cursor, snapshotID)
	if err != nil {
		return fileprotocol.DirectoryPage{}, err
	}
	entries, nextCursor, hasMore, err := readDirectoryPage(root, directory, components, snapshotID, offset, request.PageSize)
	if err != nil {
		return fileprotocol.DirectoryPage{}, err
	}
	return fileprotocol.DirectoryPage{
		ProtocolVersion: fileprotocol.Version,
		RootID:          request.RootID, RelativePath: request.RelativePath,
		SnapshotID: snapshotID, Ordering: "native-stable-within-snapshot",
		Entries: entries, NextCursor: nextCursor, HasMore: hasMore,
	}, nil
}

// SearchDirectory performs a bounded, cancellable, no-follow name search.
func SearchDirectory(ctx context.Context, request fileprotocol.DirectorySearchRequest) (fileprotocol.DirectorySearchResult, error) {
	components, err := validateSearchRequest(request)
	if err != nil {
		return fileprotocol.DirectorySearchResult{}, err
	}
	definition, err := resolveFilesystemRoot(request.RootID)
	if err != nil {
		return fileprotocol.DirectorySearchResult{}, err
	}
	root, err := openRoot(definition.Path)
	if err != nil {
		return fileprotocol.DirectorySearchResult{}, fmt.Errorf("open filesystem root: %w", err)
	}
	defer closeRootAfterRead(root)
	result := fileprotocol.DirectorySearchResult{
		ProtocolVersion: fileprotocol.Version, RootID: request.RootID,
		RelativePath: request.RelativePath, Query: request.Query,
		Entries: make([]fileprotocol.SearchEntry, 0, request.MaxResults),
	}
	queue := []searchDirectory{{components: components}}
	query := strings.ToLower(request.Query)
	for len(queue) > 0 && result.ScannedEntries < request.MaxEntries && len(result.Entries) < request.MaxResults {
		if err := ctx.Err(); err != nil {
			return fileprotocol.DirectorySearchResult{}, fmt.Errorf("search cancelled: %w", err)
		}
		current := queue[0]
		queue = queue[1:]
		directory, snapshotID, openErr := root.openDirectory(current.components)
		if openErr != nil {
			continue
		}
		for result.ScannedEntries < request.MaxEntries && len(result.Entries) < request.MaxResults {
			names, readErr := directory.Readdirnames(maxPageSize)
			for _, name := range names {
				result.ScannedEntries++
				entryComponents := append(append([]string(nil), current.components...), name)
				info, statErr := root.statNoFollow(entryComponents)
				if statErr != nil {
					continue
				}
				entry := entryFromInfo(snapshotID, info)
				if strings.Contains(strings.ToLower(entry.DisplayName), query) {
					result.Entries = append(result.Entries, fileprotocol.SearchEntry{RelativePath: strings.Join(entryComponents, "/"), Entry: entry})
				}
				if entry.Kind == fileprotocol.EntryDirectory && current.depth < request.MaxDepth {
					queue = append(queue, searchDirectory{components: entryComponents, depth: current.depth + 1})
				}
				if result.ScannedEntries >= request.MaxEntries || len(result.Entries) >= request.MaxResults {
					break
				}
			}
			if readErr == io.EOF {
				break
			}
			if readErr != nil {
				break
			}
		}
		closeFileAfterRead(directory)
	}
	result.Truncated = len(queue) > 0 || result.ScannedEntries >= request.MaxEntries || len(result.Entries) >= request.MaxResults
	return result, nil
}

func validateSearchRequest(request fileprotocol.DirectorySearchRequest) ([]string, error) {
	if request.ProtocolVersion != fileprotocol.Version || strings.TrimSpace(request.RootID) == "" {
		return nil, fmt.Errorf("invalid directory search request")
	}
	if strings.TrimSpace(request.Query) != request.Query || len(request.Query) < 2 || len(request.Query) > maxSearchQuery || !utf8.ValidString(request.Query) {
		return nil, fmt.Errorf("directory search query is outside limit")
	}
	if request.MaxResults <= 0 || request.MaxResults > maxSearchResults || request.MaxEntries <= 0 || request.MaxEntries > maxSearchEntries || request.MaxDepth < 0 || request.MaxDepth > maxSearchDepth {
		return nil, fmt.Errorf("directory search bounds are outside limit")
	}
	return validateRelativePath(request.RelativePath)
}

func validateDirectoryRequest(request fileprotocol.DirectoryListRequest) ([]string, error) {
	if request.ProtocolVersion != fileprotocol.Version {
		return nil, fmt.Errorf("unsupported file protocol version")
	}
	if strings.TrimSpace(request.RootID) == "" {
		return nil, fmt.Errorf("filesystem root is required")
	}
	if request.PageSize <= 0 || request.PageSize > maxPageSize {
		return nil, fmt.Errorf("directory page size is outside limit")
	}
	return validateRelativePath(request.RelativePath)
}

func readDirectoryPage(root *rootHandle, directory *os.File, components []string, snapshotID string, offset, pageSize int) ([]fileprotocol.FileEntry, string, bool, error) {
	if err := discardEntries(directory, offset); err != nil {
		return nil, "", false, err
	}
	names, err := directory.Readdirnames(pageSize + 1)
	if err != nil && err != io.EOF {
		return nil, "", false, fmt.Errorf("read directory page: %w", err)
	}
	hasMore := len(names) > pageSize
	if hasMore {
		names = names[:pageSize]
	}
	entries := entriesFromNames(root, components, snapshotID, names)
	if !hasMore {
		return entries, "", false, nil
	}
	nextCursor, err := encodeCursor(cursor{Offset: offset + len(names), SnapshotID: snapshotID})
	if err != nil {
		return nil, "", false, err
	}
	return entries, nextCursor, true, nil
}

func entriesFromNames(root *rootHandle, components []string, snapshotID string, names []string) []fileprotocol.FileEntry {
	entries := make([]fileprotocol.FileEntry, 0, len(names))
	for _, name := range names {
		entryComponents := append(append([]string(nil), components...), name)
		info, err := root.statNoFollow(entryComponents)
		if err == nil {
			entries = append(entries, entryFromInfo(snapshotID, info))
		}
	}
	return entries
}

// GetMetadata returns normalized no-follow metadata for one entry.
func GetMetadata(request fileprotocol.MetadataGetRequest) (fileprotocol.MetadataResult, error) {
	if request.ProtocolVersion != fileprotocol.Version {
		return fileprotocol.MetadataResult{}, fmt.Errorf("unsupported file protocol version")
	}
	components, err := validateRelativePath(request.RelativePath)
	if err != nil {
		return fileprotocol.MetadataResult{}, err
	}
	definition, err := resolveFilesystemRoot(request.RootID)
	if err != nil {
		return fileprotocol.MetadataResult{}, err
	}
	root, err := openRoot(definition.Path)
	if err != nil {
		return fileprotocol.MetadataResult{}, fmt.Errorf("open filesystem root: %w", err)
	}
	defer closeRootAfterRead(root)
	info, err := root.statNoFollow(components)
	if err != nil {
		return fileprotocol.MetadataResult{}, fmt.Errorf("read metadata: %w", err)
	}
	unavailable := fileprotocol.FieldValue{State: fileprotocol.CapabilityUnavailable}
	return fileprotocol.MetadataResult{
		ProtocolVersion: fileprotocol.Version, RootID: request.RootID,
		RelativePath: request.RelativePath, Kind: kindFromMode(info.Mode()),
		Size: info.Size(), ModifiedAt: info.ModTime().UTC(), Mode: uint32(info.Mode()),
		OptionalFields: map[string]fileprotocol.FieldValue{
			"owner": unavailable, "acl": unavailable, "birth_time": unavailable,
			"extended_attributes": unavailable,
		},
	}, nil
}

// ReadPreview returns a bounded range from a no-follow regular file handle.
func ReadPreview(request fileprotocol.PreviewReadRequest) (fileprotocol.PreviewResult, error) {
	components, err := validatePreviewRequest(request)
	if err != nil {
		return fileprotocol.PreviewResult{}, err
	}
	definition, err := resolveFilesystemRoot(request.RootID)
	if err != nil {
		return fileprotocol.PreviewResult{}, err
	}
	root, err := openRoot(definition.Path)
	if err != nil {
		return fileprotocol.PreviewResult{}, fmt.Errorf("open filesystem root: %w", err)
	}
	defer closeRootAfterRead(root)
	file, info, err := root.openRegularFile(components)
	if err != nil {
		return fileprotocol.PreviewResult{}, fmt.Errorf("open preview file: %w", err)
	}
	defer closeFileAfterRead(file)
	data := make([]byte, request.Length)
	read, readErr := file.ReadAt(data, request.Offset)
	if readErr != nil && readErr != io.EOF {
		return fileprotocol.PreviewResult{}, fmt.Errorf("read preview: %w", readErr)
	}
	data = data[:read]
	return fileprotocol.PreviewResult{
		ProtocolVersion: fileprotocol.Version, RootID: request.RootID,
		RelativePath: request.RelativePath, Offset: request.Offset, Data: data,
		Classification: classifyContent(data), Truncated: request.Offset+int64(read) < info.Size(),
	}, nil
}

func validatePreviewRequest(request fileprotocol.PreviewReadRequest) ([]string, error) {
	if request.ProtocolVersion != fileprotocol.Version {
		return nil, fmt.Errorf("unsupported file protocol version")
	}
	if strings.TrimSpace(request.RootID) == "" {
		return nil, fmt.Errorf("filesystem root is required")
	}
	if request.Offset < 0 || request.Length <= 0 {
		return nil, fmt.Errorf("preview range is outside limit")
	}
	if request.Length > maxPreviewBytes {
		return nil, fmt.Errorf("preview range is outside limit")
	}
	return validateRelativePath(request.RelativePath)
}

func resolveFilesystemRoot(rootID string) (rootDefinition, error) {
	roots, err := filesystemRoots()
	if err != nil {
		return rootDefinition{}, err
	}
	for _, root := range roots {
		if root.ID == rootID {
			return root, nil
		}
	}
	return rootDefinition{}, fmt.Errorf("filesystem root is unavailable")
}

func validateRelativePath(path string) ([]string, error) {
	if err := validatePathEncoding(path); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}
	components := strings.Split(path, "/")
	if len(components) > maxPathComponents {
		return nil, fmt.Errorf("relative path depth exceeds limit")
	}
	for _, component := range components {
		if err := validatePathComponent(component); err != nil {
			return nil, err
		}
	}
	return components, nil
}

func validatePathEncoding(path string) error {
	if len(path) > maxRelativePath || !utf8.ValidString(path) || strings.ContainsAny(path, "\x00\\") {
		return fmt.Errorf("relative path encoding is invalid")
	}
	if strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return fmt.Errorf("relative path is not normalized")
	}
	return nil
}

func validatePathComponent(component string) error {
	if component == "" || component == "." || component == ".." || strings.ContainsAny(component, "\r\n") {
		return fmt.Errorf("relative path contains a forbidden component")
	}
	return nil
}

func discardEntries(directory *os.File, count int) error {
	for count > 0 {
		batch := count
		if batch > maxPageSize {
			batch = maxPageSize
		}
		names, err := directory.Readdirnames(batch)
		count -= len(names)
		if err == io.EOF && count > 0 {
			return fmt.Errorf("directory cursor exceeds snapshot")
		}
		if err != nil && err != io.EOF {
			return fmt.Errorf("advance directory cursor: %w", err)
		}
	}
	return nil
}

func entryFromInfo(snapshotID string, info os.FileInfo) fileprotocol.FileEntry {
	digest := sha256.Sum256([]byte(snapshotID + "\x00" + info.Name()))
	return fileprotocol.FileEntry{
		EntryID: hex.EncodeToString(digest[:16]), DisplayName: safeDisplayName(info.Name()), OperationName: operationName(info.Name()),
		Kind: kindFromMode(info.Mode()), Size: info.Size(), ModifiedAt: info.ModTime().UTC(),
		Mode: uint32(info.Mode()), Hidden: strings.HasPrefix(info.Name(), "."), Readable: true,
	}
}

func operationName(name string) string {
	if !utf8.ValidString(name) || name == "" || strings.ContainsAny(name, "\x00\\/\r\n") {
		return ""
	}
	if runtime.GOOS != windowsOS {
		return name
	}
	if strings.ContainsAny(name, `:*?"<>|`) || strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return ""
	}
	base := strings.ToUpper(strings.TrimSuffix(name, filepath.Ext(name)))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return ""
	default:
		return name
	}
}

func kindFromMode(mode os.FileMode) fileprotocol.EntryKind {
	switch {
	case mode&os.ModeSymlink != 0:
		return fileprotocol.EntrySymlink
	case mode.IsDir():
		return fileprotocol.EntryDirectory
	case mode.IsRegular():
		return fileprotocol.EntryFile
	default:
		return fileprotocol.EntrySpecial
	}
}

func safeDisplayName(name string) string {
	return strings.Map(func(value rune) rune {
		if value < 0x20 || value == 0x7f {
			return '\uFFFD'
		}
		return value
	}, strings.ToValidUTF8(name, "�"))
}

func classifyContent(data []byte) string {
	if len(data) == 0 {
		return contentText
	}
	for _, value := range data {
		if value == 0 {
			return "binary"
		}
	}
	if !utf8.Valid(data) {
		return "binary"
	}
	return contentText
}

func encodeCursor(value cursor) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode directory cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func decodeCursor(encoded, snapshotID string) (int, error) {
	if encoded == "" {
		return 0, nil
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(data) > 512 {
		return 0, fmt.Errorf("directory cursor is invalid")
	}
	var value cursor
	if err := json.Unmarshal(data, &value); err != nil {
		return 0, fmt.Errorf("decode directory cursor: %w", err)
	}
	if value.Offset < 0 || value.Offset > maxCursorOffset || value.SnapshotID != snapshotID {
		return 0, fmt.Errorf("directory cursor is stale")
	}
	return value.Offset, nil
}

func snapshotIDFromStat(device, inode uint64, size int64, modified time.Time) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%d:%d", device, inode, size, modified.UnixNano())))
	return hex.EncodeToString(digest[:16])
}

func platformCapabilities() fileprotocol.RootCapabilities {
	available := fileprotocol.CapabilityAvailable
	unavailable := fileprotocol.CapabilityUnavailable
	capabilities := fileprotocol.RootCapabilities{
		ProtocolVersion: fileprotocol.Version, OperatingSystem: runtime.GOOS,
		CaseSensitive: unavailable, NoFollowResolution: available,
		AtomicRename: available, PermanentDelete: available, Symlinks: available,
		POSIXMode: unavailable, Owner: unavailable, ACL: unavailable,
		ExtendedAttributes: unavailable, SparseFiles: unavailable,
		SafeHandleRelativeIO: available,
		MetadataWrite:        unavailable, ArchiveCreate: unavailable,
		ArchiveList: unavailable, ArchiveExtract: unavailable,
		ArchiveFormats: []string{},
	}
	if runtime.GOOS != windowsOS {
		capabilities.CaseSensitive = available
		capabilities.POSIXMode = available
	}
	if runtime.GOOS == windowsOS {
		capabilities.SafeHandleRelativeIO = unavailable
		capabilities.AtomicRename = unavailable
	}
	return capabilities
}

func closeRootAfterRead(root *rootHandle) {
	// A close failure cannot invalidate already completed read-only work.
	_ = root.close()
}

func closeFileAfterRead(file *os.File) {
	// A close failure cannot invalidate already completed read-only work.
	_ = file.Close()
}

func classifyError(err error) string {
	if os.IsNotExist(err) {
		return "not_found"
	}
	if os.IsPermission(err) {
		return "denied"
	}
	return "unavailable"
}
