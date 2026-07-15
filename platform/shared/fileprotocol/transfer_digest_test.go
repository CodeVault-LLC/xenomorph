package fileprotocol

import "testing"

func TestTransferManifestDigestBindsManifest(t *testing.T) {
	t.Parallel()
	manifest := TransferManifest{ProtocolVersion: Version, TransferID: "5f9ee36a-80c2-4f32-9257-975b00236f98", Direction: TransferDownload, Size: 10, ChunkSize: 10}
	first, err := TransferManifestDigest(manifest)
	if err != nil {
		t.Fatalf("TransferManifestDigest() error = %v", err)
	}
	manifest.Size++
	second, err := TransferManifestDigest(manifest)
	if err != nil {
		t.Fatalf("TransferManifestDigest() changed error = %v", err)
	}
	if first == second {
		t.Fatal("TransferManifestDigest() did not bind manifest fields")
	}
}
