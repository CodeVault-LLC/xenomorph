package fileprotocol

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// TransferManifestDigest returns the stable SHA-256 binding for the complete
// versioned manifest carried inside a signed gateway command.
func TransferManifestDigest(manifest TransferManifest) ([sha256.Size]byte, error) {
	canonical, err := json.Marshal(manifest)
	if err != nil {
		return [sha256.Size]byte{}, fmt.Errorf("encode transfer manifest digest input: %w", err)
	}
	return sha256.Sum256(canonical), nil
}
