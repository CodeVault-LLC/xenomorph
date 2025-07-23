package types

import "encoding/json"

type FileMetadata struct {
	ID        uint32 `json:"id"`        // Matches command/session
	Name      string `json:"name"`      // Original filename
	Size      int64  `json:"size"`      // Total size
	Direction string `json:"direction"` // "upload" or "download"
}

type FileChunk struct {
	ID    uint32 `json:"id"`    // same as in FileMetadata
	Chunk []byte `json:"chunk"` // base64 encoded or raw []byte
}

type FileEnd struct {
	ID uint32 `json:"id"`
}

type File struct {
	ID        uint32 `json:"id"`        // Unique identifier for the file
	Name      string `json:"name"`      // Name of the file
	Size      int64  `json:"size"`      // Size of the file in bytes
	FileType  string `json:"file_type"` // Type of the file (e.g., "text", "binary")
	Direction string `json:"direction"` // Direction of the file transfer ("upload" or "download")
	Chunks    []FileChunk `json:"chunks"` // List of file chunks
}

func (f *File) ToJSON() string {
	data, err := json.Marshal(f)
	if err != nil {
		return ""
	}
	return string(data)
}

func (f *FileMetadata) ToJSON() string {
	data, err := json.Marshal(f)
	if err != nil {
		return ""
	}
	return string(data)
}

func (f *FileChunk) ToJSON() string {
	data, err := json.Marshal(f)
	if err != nil {
		return ""
	}
	return string(data)
}

func (f *FileEnd) ToJSON() string {
	data, err := json.Marshal(f)
	if err != nil {
		return ""
	}
	return string(data)
}