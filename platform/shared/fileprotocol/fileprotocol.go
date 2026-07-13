// Package fileprotocol defines the versioned read-only filesystem command and
// result contract shared by the gateway and agent. Command identifiers are
// server-authored. Every filesystem observation in a result is client-authored
// evidence and must not be used for identity or authorization.
package fileprotocol

import (
	"encoding/json"
	"time"
)

// Version is the current file workspace protocol version.
const Version = 2

const (
	// CommandRootsList requests filesystem root and capability observations.
	CommandRootsList = "files.roots.list"
	// CommandDirectoryList requests one bounded directory page.
	CommandDirectoryList = "files.directory.list"
	// CommandMetadataGet requests metadata without following the target link.
	CommandMetadataGet = "files.metadata.get"
	// CommandPreviewRead requests a bounded regular-file byte range.
	CommandPreviewRead = "files.preview.read"
)

// Verb identifies a supported read-only filesystem action.
type Verb string

const (
	// VerbList permits no-follow directory enumeration.
	VerbList Verb = "list"
	// VerbMetadata permits no-follow metadata inspection.
	VerbMetadata Verb = "metadata"
	// VerbPreview permits bounded regular-file reads.
	VerbPreview Verb = "preview"
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
	ManagedTrash         CapabilityState `json:"managed_trash"`
	Symlinks             CapabilityState `json:"symlinks"`
	POSIXMode            CapabilityState `json:"posix_mode"`
	Owner                CapabilityState `json:"owner"`
	ACL                  CapabilityState `json:"acl"`
	ExtendedAttributes   CapabilityState `json:"extended_attributes"`
	SparseFiles          CapabilityState `json:"sparse_files"`
	SafeHandleRelativeIO CapabilityState `json:"safe_handle_relative_io"`
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
