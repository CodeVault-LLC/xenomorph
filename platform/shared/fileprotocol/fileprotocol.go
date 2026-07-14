// Package fileprotocol defines the versioned filesystem command and result
// contract shared by the gateway and agent. Command identifiers are
// server-authored. Every filesystem observation in a result is client-authored
// evidence and must not be used for identity or authorization.
package fileprotocol

import (
	"encoding/json"
	"time"
)

// Version is the current file workspace protocol version.
const Version = 6

const (
	// CommandRootsList requests filesystem root and capability observations.
	CommandRootsList = "files.roots.list"
	// CommandDirectoryList requests one bounded directory page.
	CommandDirectoryList = "files.directory.list"
	// CommandDirectorySearch requests one bounded, cancellable subtree search.
	CommandDirectorySearch = "files.directory.search"
	// CommandMetadataGet requests metadata without following the target link.
	CommandMetadataGet = "files.metadata.get"
	// CommandMetadataSet requests explicit, capability-gated metadata deltas.
	CommandMetadataSet = "files.metadata.set"
	// CommandArchiveExecute requests one bounded archive operation.
	CommandArchiveExecute = "files.archive.execute"
	// CommandPreviewRead requests a bounded regular-file byte range.
	CommandPreviewRead = "files.preview.read"
	// CommandOperationExecute requests a bounded, preconditioned mutation.
	CommandOperationExecute = "files.operation.execute"
	// CommandTransferPrepare requests one staged file transfer.
	CommandTransferPrepare = "files.transfer.prepare"
	// CommandTransferResume resumes one staged file transfer from acknowledged chunks.
	CommandTransferResume = "files.transfer.resume"
	// CommandTransferAbort cancels one staged file transfer.
	CommandTransferAbort = "files.transfer.abort"
)

// Verb identifies a supported filesystem action family.
type Verb string

const (
	// VerbList permits no-follow directory enumeration.
	VerbList Verb = "list"
	// VerbMetadata permits no-follow metadata inspection.
	VerbMetadata Verb = "metadata"
	// VerbPreview permits bounded regular-file reads.
	VerbPreview Verb = "preview"
	// VerbTransfer permits staged regular-file upload and download.
	VerbTransfer Verb = "transfer"
	// VerbMutate permits explicitly requested, preconditioned mutations.
	VerbMutate Verb = "mutate"
)

// CapabilityState reports whether an optional filesystem behavior is usable.
type CapabilityState string

const (
	// CapabilityAvailable indicates that the agent can attempt the behavior.
	CapabilityAvailable CapabilityState = "available"
	// CapabilityUnavailable indicates that the platform cannot provide it.
	CapabilityUnavailable CapabilityState = "unavailable"
	// CapabilityDenied indicates that the operating environment prohibits the behavior.
	CapabilityDenied CapabilityState = "denied"
)

// EntryKind is a platform-neutral no-follow directory entry classification.
type EntryKind string

const (
	// EntryFile identifies a regular file.
	EntryFile EntryKind = "file"
	// EntryDirectory identifies a directory.
	EntryDirectory EntryKind = "directory"
	// EntrySymlink identifies a symbolic link or reparse-like entry.
	EntrySymlink EntryKind = "symlink"
	// EntrySpecial identifies a non-regular filesystem object.
	EntrySpecial EntryKind = "special"
)

// RootCapabilities is a client-authored, presentation-only capability report.
type RootCapabilities struct {
	ProtocolVersion      int             `json:"protocol_version"`
	OperatingSystem      string          `json:"operating_system"`
	FilesystemType       string          `json:"filesystem_type"`
	CaseSensitive        CapabilityState `json:"case_sensitive"`
	NoFollowResolution   CapabilityState `json:"no_follow_resolution"`
	AtomicRename         CapabilityState `json:"atomic_rename"`
	PermanentDelete      CapabilityState `json:"permanent_delete"`
	Symlinks             CapabilityState `json:"symlinks"`
	POSIXMode            CapabilityState `json:"posix_mode"`
	Owner                CapabilityState `json:"owner"`
	ACL                  CapabilityState `json:"acl"`
	ExtendedAttributes   CapabilityState `json:"extended_attributes"`
	SparseFiles          CapabilityState `json:"sparse_files"`
	SafeHandleRelativeIO CapabilityState `json:"safe_handle_relative_io"`
	MetadataWrite        CapabilityState `json:"metadata_write"`
	ArchiveCreate        CapabilityState `json:"archive_create"`
	ArchiveList          CapabilityState `json:"archive_list"`
	ArchiveExtract       CapabilityState `json:"archive_extract"`
	ArchiveFormats       []string        `json:"archive_formats"`
}

// RootObservation is the agent's client-authored observation of one local
// filesystem root.
type RootObservation struct {
	RootID       string           `json:"root_id"`
	DisplayLabel string           `json:"display_label"`
	AllowedVerbs []Verb           `json:"allowed_verbs"`
	Available    bool             `json:"available"`
	ReadOnly     bool             `json:"read_only"`
	Capabilities RootCapabilities `json:"capabilities"`
	ErrorClass   string           `json:"error_class,omitempty"`
}

// RootsListRequest asks the agent to enumerate its local filesystem roots.
type RootsListRequest struct {
	ProtocolVersion int `json:"protocol_version"`
}

// RootsListResult contains bounded client-authored root observations.
type RootsListResult struct {
	ProtocolVersion int               `json:"protocol_version"`
	Roots           []RootObservation `json:"roots"`
}

// DirectoryListRequest asks for a bounded page under an agent-reported root.
type DirectoryListRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	RootID          string `json:"root_id"`
	RelativePath    string `json:"relative_path"`
	Cursor          string `json:"cursor,omitempty"`
	PageSize        int    `json:"page_size"`
}

// FileEntry is a client-authored no-follow filesystem observation.
type FileEntry struct {
	EntryID       string    `json:"entry_id"`
	DisplayName   string    `json:"display_name"`
	OperationName string    `json:"operation_name,omitempty"`
	Kind          EntryKind `json:"kind"`
	Size          int64     `json:"size"`
	ModifiedAt    time.Time `json:"modified_at"`
	Mode          uint32    `json:"mode"`
	Hidden        bool      `json:"hidden"`
	Readable      bool      `json:"readable"`
}

// DirectoryPage is one client-authored native-order directory snapshot page.
type DirectoryPage struct {
	ProtocolVersion int         `json:"protocol_version"`
	RootID          string      `json:"root_id"`
	RelativePath    string      `json:"relative_path"`
	SnapshotID      string      `json:"snapshot_id"`
	Ordering        string      `json:"ordering"`
	Entries         []FileEntry `json:"entries"`
	NextCursor      string      `json:"next_cursor,omitempty"`
	HasMore         bool        `json:"has_more"`
}

// DirectorySearchRequest asks for a bounded name search below one directory.
type DirectorySearchRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	RootID          string `json:"root_id"`
	RelativePath    string `json:"relative_path"`
	Query           string `json:"query"`
	MaxResults      int    `json:"max_results"`
	MaxEntries      int    `json:"max_entries"`
	MaxDepth        int    `json:"max_depth"`
}

// SearchEntry binds a client-authored entry observation to its normalized
// root-relative location.
type SearchEntry struct {
	RelativePath string    `json:"relative_path"`
	Entry        FileEntry `json:"entry"`
}

// DirectorySearchResult contains bounded client-authored search observations.
type DirectorySearchResult struct {
	ProtocolVersion int           `json:"protocol_version"`
	RootID          string        `json:"root_id"`
	RelativePath    string        `json:"relative_path"`
	Query           string        `json:"query"`
	Entries         []SearchEntry `json:"entries"`
	ScannedEntries  int           `json:"scanned_entries"`
	Truncated       bool          `json:"truncated"`
}

// MetadataGetRequest asks for no-follow metadata under an agent-reported root.
type MetadataGetRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	RootID          string `json:"root_id"`
	RelativePath    string `json:"relative_path"`
}

// MetadataResult is a client-authored normalized metadata observation.
type MetadataResult struct {
	ProtocolVersion int                   `json:"protocol_version"`
	RootID          string                `json:"root_id"`
	RelativePath    string                `json:"relative_path"`
	Kind            EntryKind             `json:"kind"`
	Size            int64                 `json:"size"`
	ModifiedAt      time.Time             `json:"modified_at"`
	Mode            uint32                `json:"mode"`
	OptionalFields  map[string]FieldValue `json:"optional_fields"`
}

// MetadataDelta contains only operator-authored fields explicitly requested
// for update. Nil fields are not modified.
type MetadataDelta struct {
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
	POSIXMode  *uint32    `json:"posix_mode,omitempty"`
}

// MetadataSetRequest asks the agent to apply bounded metadata deltas without
// following the target link.
type MetadataSetRequest struct {
	ProtocolVersion int           `json:"protocol_version"`
	OperationID     string        `json:"operation_id"`
	RootID          string        `json:"root_id"`
	RelativePath    string        `json:"relative_path"`
	Preconditions   Preconditions `json:"preconditions"`
	Delta           MetadataDelta `json:"delta"`
}

// MetadataApplyState is the allowlisted outcome of one requested field update.
type MetadataApplyState string

const (
	// MetadataApplied indicates the native adapter applied the requested value.
	MetadataApplied MetadataApplyState = "applied"
	// MetadataUnavailable indicates the native adapter cannot update the field.
	MetadataUnavailable MetadataApplyState = "unavailable"
	// MetadataDenied indicates the operating system denied the update.
	MetadataDenied MetadataApplyState = "denied"
	// MetadataFailed indicates the update failed without being reported as applied.
	MetadataFailed MetadataApplyState = "failed"
)

// MetadataFieldResult reports exactly what happened to one requested field.
type MetadataFieldResult struct {
	Field      string             `json:"field"`
	State      MetadataApplyState `json:"state"`
	ErrorClass string             `json:"error_class,omitempty"`
}

// MetadataSetResult is client-authored evidence for an explicit metadata write.
type MetadataSetResult struct {
	ProtocolVersion int                   `json:"protocol_version"`
	OperationID     string                `json:"operation_id"`
	Fields          []MetadataFieldResult `json:"fields"`
}

// ArchiveAction identifies an allowlisted archive operation.
type ArchiveAction string

const (
	// ArchiveCreate creates an archive from explicit root-relative sources.
	ArchiveCreate ArchiveAction = "create"
	// ArchiveList inspects archive entries without extracting them.
	ArchiveList ArchiveAction = "list"
	// ArchiveExtract extracts safe regular files and directories.
	ArchiveExtract ArchiveAction = "extract"
)

// ArchiveFormat identifies an allowlisted archive representation.
type ArchiveFormat string

const (
	// ArchiveZIP is the ZIP container format handled by the native agent.
	ArchiveZIP ArchiveFormat = "zip"
)

// ArchiveLimits are gateway-authored, signed resource ceilings. The agent also
// applies its own fixed maxima and never treats larger values as authority.
type ArchiveLimits struct {
	MaxEntries          int           `json:"max_entries"`
	MaxDepth            int           `json:"max_depth"`
	MaxExpandedBytes    int64         `json:"max_expanded_bytes"`
	MaxTemporaryBytes   int64         `json:"max_temporary_bytes"`
	MaxCompressionRatio int64         `json:"max_compression_ratio"`
	MaxRuntime          time.Duration `json:"max_runtime"`
	MaxListedEntries    int           `json:"max_listed_entries"`
	MaxListedNameBytes  int           `json:"max_listed_name_bytes"`
}

// ArchiveRequest contains operator-authored root-relative operands and
// gateway-authored execution bounds.
type ArchiveRequest struct {
	ProtocolVersion int              `json:"protocol_version"`
	OperationID     string           `json:"operation_id"`
	RootID          string           `json:"root_id"`
	Action          ArchiveAction    `json:"action"`
	Format          ArchiveFormat    `json:"format"`
	ArchivePath     string           `json:"archive_path"`
	DestinationPath string           `json:"destination_path,omitempty"`
	SourcePaths     []string         `json:"source_paths,omitempty"`
	Conflict        ConflictStrategy `json:"conflict_strategy"`
	Preconditions   Preconditions    `json:"preconditions"`
	Limits          ArchiveLimits    `json:"limits"`
}

// ArchiveEntry is a bounded client-authored observation from an archive list.
type ArchiveEntry struct {
	Path             string    `json:"path"`
	Kind             EntryKind `json:"kind"`
	UncompressedSize int64     `json:"uncompressed_size"`
	CompressedSize   int64     `json:"compressed_size"`
}

// ArchiveResult contains bounded client-authored archive evidence.
type ArchiveResult struct {
	ProtocolVersion  int            `json:"protocol_version"`
	OperationID      string         `json:"operation_id"`
	State            string         `json:"state"`
	Entries          []ArchiveEntry `json:"entries,omitempty"`
	EntriesProcessed int            `json:"entries_processed"`
	BytesProcessed   int64          `json:"bytes_processed"`
	Truncated        bool           `json:"truncated,omitempty"`
}

// FieldValue explicitly represents an optional platform metadata field.
type FieldValue struct {
	State CapabilityState `json:"state"`
	Value string          `json:"value,omitempty"`
}

// PreviewReadRequest asks for a bounded byte range from a regular file.
type PreviewReadRequest struct {
	ProtocolVersion int    `json:"protocol_version"`
	RootID          string `json:"root_id"`
	RelativePath    string `json:"relative_path"`
	Offset          int64  `json:"offset"`
	Length          int64  `json:"length"`
}

// PreviewResult contains client-authored opaque bytes and conservative classification.
type PreviewResult struct {
	ProtocolVersion int    `json:"protocol_version"`
	RootID          string `json:"root_id"`
	RelativePath    string `json:"relative_path"`
	Offset          int64  `json:"offset"`
	Data            []byte `json:"data"`
	Classification  string `json:"classification"`
	Truncated       bool   `json:"truncated"`
}

// ConflictStrategy defines the recorded behavior when a destination exists.
type ConflictStrategy string

const (
	// ConflictFail rejects an existing destination.
	ConflictFail ConflictStrategy = "fail"
	// ConflictSkip reports an existing destination without changing it.
	ConflictSkip ConflictStrategy = "skip"
	// ConflictRenameNew publishes under a non-colliding generated name.
	ConflictRenameNew ConflictStrategy = "rename_new"
	// ConflictReplace atomically replaces an eligible destination.
	ConflictReplace ConflictStrategy = "replace"
)

// MutationVerb identifies an allowlisted filesystem mutation.
type MutationVerb string

const (
	// MutationCreateFile creates an empty regular file.
	MutationCreateFile MutationVerb = "create_file"
	// MutationCreateDirectory creates an empty directory.
	MutationCreateDirectory MutationVerb = "create_directory"
	// MutationRename renames an entry within one root.
	MutationRename MutationVerb = "rename"
	// MutationMove moves an entry within one root.
	MutationMove MutationVerb = "move"
	// MutationCopy copies one regular file.
	MutationCopy MutationVerb = "copy"
	// MutationDuplicate creates a copy at an explicit destination.
	MutationDuplicate MutationVerb = "duplicate"
	// MutationTouch updates a no-follow entry timestamp.
	MutationTouch MutationVerb = "touch"
	// MutationTruncate changes a regular file to an explicit size.
	MutationTruncate MutationVerb = "truncate"
	// MutationAppend appends bounded opaque bytes to a regular file.
	MutationAppend MutationVerb = "append"
	// MutationDelete permanently removes one entry or a bounded directory tree.
	MutationDelete MutationVerb = "delete"
)

// Preconditions bind a mutation or transfer to a client-observed version.
// The observations remain client-authored and are used only to detect change.
type Preconditions struct {
	MustExist       *bool     `json:"must_exist,omitempty"`
	ExpectedKind    EntryKind `json:"expected_kind,omitempty"`
	ExpectedSize    *int64    `json:"expected_size,omitempty"`
	ExpectedModTime time.Time `json:"expected_modified_at,omitempty"`
}

// MutationItem describes one bounded root-relative mutation operand.
type MutationItem struct {
	ItemID          string        `json:"item_id"`
	SourcePath      string        `json:"source_path,omitempty"`
	DestinationPath string        `json:"destination_path,omitempty"`
	AppendData      []byte        `json:"append_data,omitempty"`
	TruncateSize    int64         `json:"truncate_size,omitempty"`
	Preconditions   Preconditions `json:"preconditions"`
}

// MutationRequest asks the agent to plan or execute bounded mutation items.
type MutationRequest struct {
	ProtocolVersion int              `json:"protocol_version"`
	OperationID     string           `json:"operation_id"`
	RootID          string           `json:"root_id"`
	Verb            MutationVerb     `json:"verb"`
	DryRun          bool             `json:"dry_run"`
	Conflict        ConflictStrategy `json:"conflict_strategy"`
	Items           []MutationItem   `json:"items"`
}

// ItemResult is one client-authored mutation or transfer outcome.
type ItemResult struct {
	ItemID       string `json:"item_id"`
	State        string `json:"state"`
	ErrorClass   string `json:"error_class,omitempty"`
	ResultPath   string `json:"result_path,omitempty"`
	BytesApplied int64  `json:"bytes_applied,omitempty"`
}

// MutationResult contains bounded per-item results and never asserts identity.
type MutationResult struct {
	ProtocolVersion int          `json:"protocol_version"`
	OperationID     string       `json:"operation_id"`
	DryRun          bool         `json:"dry_run"`
	Items           []ItemResult `json:"items"`
}

// TransferDirection identifies which side authors staged bytes.
type TransferDirection string

const (
	// TransferUpload moves browser-staged bytes to an agent destination.
	TransferUpload TransferDirection = "upload"
	// TransferDownload moves agent-authored bytes to gateway staging.
	TransferDownload TransferDirection = "download"
)

// ChunkManifest binds one durable chunk to its expected byte range and digest.
type ChunkManifest struct {
	Index  int    `json:"index"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// TransferManifest is the immutable, checksummed single-file transfer plan.
type TransferManifest struct {
	ProtocolVersion int               `json:"protocol_version"`
	TransferID      string            `json:"transfer_id"`
	Direction       TransferDirection `json:"direction"`
	RootID          string            `json:"root_id"`
	RelativePath    string            `json:"relative_path"`
	Size            int64             `json:"size"`
	ChunkSize       int64             `json:"chunk_size"`
	SHA256          string            `json:"sha256"`
	Chunks          []ChunkManifest   `json:"chunks"`
	Conflict        ConflictStrategy  `json:"conflict_strategy"`
	Preconditions   Preconditions     `json:"preconditions"`
}

// DataPlaneLease is a gateway-authored, short-lived chunk capability.
type DataPlaneLease struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// TransferRequest asks the agent to prepare or resume one immutable manifest.
type TransferRequest struct {
	ProtocolVersion int              `json:"protocol_version"`
	Manifest        TransferManifest `json:"manifest"`
	Lease           DataPlaneLease   `json:"lease"`
}

// TransferResult reports verified progress for one transfer.
type TransferResult struct {
	ProtocolVersion int               `json:"protocol_version"`
	TransferID      string            `json:"transfer_id"`
	State           string            `json:"state"`
	Acknowledged    []int             `json:"acknowledged_chunks"`
	BytesVerified   int64             `json:"bytes_verified"`
	ErrorClass      string            `json:"error_class,omitempty"`
	Scanning        string            `json:"scanning"`
	Manifest        *TransferManifest `json:"manifest,omitempty"`
}

// Error is a stable, safe filesystem failure returned to the gateway.
type Error struct {
	Class     string `json:"class"`
	Retryable bool   `json:"retryable"`
	Message   string `json:"message"`
}

// CommandResult wraps a typed result or a structured filesystem failure.
type CommandResult struct {
	ProtocolVersion int             `json:"protocol_version"`
	Data            json.RawMessage `json:"data,omitempty"`
	Error           *Error          `json:"error,omitempty"`
}
