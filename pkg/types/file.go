package types

type FileMetadata struct {
	ID        string `json:"id"`        // Matches command/session
	Name      string `json:"name"`      // Original filename
	Size      int64  `json:"size"`      // Total size
	From      string `json:"from"`      // Sender UUID
	To        string `json:"to"`        // Receiver UUID
	Direction string `json:"direction"` // "upload" or "download"
}

type FileChunk struct {
	ID    string `json:"id"`    // same as in FileMetadata
	Chunk []byte `json:"chunk"` // base64 encoded or raw []byte
}

type FileEnd struct {
	ID string `json:"id"`
}
