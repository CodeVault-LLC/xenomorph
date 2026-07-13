package filesystem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

const (
	transferChunkSize      int64 = 4 << 20
	transferMaxSize        int64 = 1 << 30
	transferMaxChunks            = 256
	transferRetryAttempts        = 3
	transferRetryBaseDelay       = 100 * time.Millisecond
)

// TransferPlane moves scoped chunks through the authenticated gateway data
// plane. Implementations must not expose their HTTP client or credentials.
type TransferPlane interface {
	PutChunk(context.Context, string, string, int, []byte) error
	GetChunk(context.Context, string, string, int, int64) ([]byte, error)
	Finalize(context.Context, string, string) error
}

// TransferFailureResult converts a local or data-plane failure into a stable
// client-authored transfer state for gateway reconciliation.
func TransferFailureResult(request fileprotocol.TransferRequest, err error) fileprotocol.TransferResult {
	result := fileprotocol.TransferResult{ProtocolVersion: fileprotocol.Version, TransferID: request.Manifest.TransferID, State: "failed", ErrorClass: "permanent_failure", Scanning: "not_scanned"}
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		result.State, result.ErrorClass = "cancelled", "cancelled"
	case errors.Is(err, fs.ErrExist):
		result.ErrorClass = "conflict"
	case errors.Is(err, fs.ErrPermission):
		result.ErrorClass = "forbidden"
	case strings.Contains(err.Error(), "retry budget exhausted"):
		result.State, result.ErrorClass = "paused", "retryable_failure"
	}
	return result
}

// PrepareDownload freezes a checksum manifest from one no-follow regular-file
// handle before the gateway issues a data-plane lease.
func PrepareDownload(request fileprotocol.TransferRequest) (fileprotocol.TransferResult, error) {
	manifest := request.Manifest
	if err := validateDownloadPlan(request); err != nil {
		return fileprotocol.TransferResult{}, err
	}
	components, err := validateRelativePath(manifest.RelativePath)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}
	root, file, info, err := openTransferSource(manifest, components)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}
	defer closeRootAfterRead(root)
	defer closeFileAfterRead(file)
	if info.Size() < 0 || info.Size() > transferMaxSize {
		return fileprotocol.TransferResult{}, fmt.Errorf("download size exceeds limit")
	}
	chunkSize := manifest.ChunkSize
	if chunkSize <= 0 {
		chunkSize = transferChunkSize
	}
	manifest.Size = info.Size()
	manifest.ChunkSize = chunkSize
	if err := populateDownloadManifest(file, &manifest); err != nil {
		return fileprotocol.TransferResult{}, err
	}
	return fileprotocol.TransferResult{ProtocolVersion: fileprotocol.Version, TransferID: manifest.TransferID, State: "prepared", Manifest: &manifest, Scanning: "not_scanned"}, nil
}

func validateDownloadPlan(request fileprotocol.TransferRequest) error {
	manifest := request.Manifest
	if request.ProtocolVersion != fileprotocol.Version || manifest.ProtocolVersion != fileprotocol.Version {
		return fmt.Errorf("download preparation protocol is invalid")
	}
	if manifest.Direction != fileprotocol.TransferDownload || manifest.TransferID == "" {
		return fmt.Errorf("download preparation request is invalid")
	}
	return nil
}

func openTransferSource(manifest fileprotocol.TransferManifest, components []string) (*rootHandle, *os.File, os.FileInfo, error) {
	definition, err := resolveFilesystemRoot(manifest.RootID)
	if err != nil {
		return nil, nil, nil, err
	}
	root, err := openRoot(definition.Path)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := root.checkPreconditions(components, manifest.Preconditions); err != nil {
		_ = root.close()
		return nil, nil, nil, err
	}
	file, info, err := root.openRegularFile(components)
	if err != nil {
		_ = root.close()
		return nil, nil, nil, err
	}
	return root, file, info, nil
}

func populateDownloadManifest(file *os.File, manifest *fileprotocol.TransferManifest) error {
	info, err := file.Stat()
	if err != nil {
		return err
	}
	manifest.Chunks = make([]fileprotocol.ChunkManifest, 0, (info.Size()+manifest.ChunkSize-1)/manifest.ChunkSize)
	whole := sha256.New()
	for offset, index := int64(0), 0; offset < info.Size(); index++ {
		size := manifest.ChunkSize
		if remaining := info.Size() - offset; remaining < size {
			size = remaining
		}
		data := make([]byte, size)
		read, readErr := file.ReadAt(data, offset)
		if readErr != nil && readErr != io.EOF {
			return readErr
		}
		if int64(read) != size {
			return fsConflict("download source changed during preparation")
		}
		if _, err := whole.Write(data); err != nil {
			return err
		}
		manifest.Chunks = append(manifest.Chunks, fileprotocol.ChunkManifest{Index: index, Offset: offset, Size: size, SHA256: transferDigest(data)})
		offset += size
	}
	manifest.SHA256 = hex.EncodeToString(whole.Sum(nil))
	return nil
}

// ExecuteTransfer moves one immutable regular-file manifest through the
// gateway data plane and verifies every chunk and the complete object.
func ExecuteTransfer(ctx context.Context, request fileprotocol.TransferRequest, plane TransferPlane) (fileprotocol.TransferResult, error) {
	if plane == nil {
		return fileprotocol.TransferResult{}, fmt.Errorf("transfer data plane is unavailable")
	}
	if err := validateTransferRequest(request); err != nil {
		return fileprotocol.TransferResult{}, err
	}
	definition, err := resolveFilesystemRoot(request.Manifest.RootID)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}
	root, err := openRoot(definition.Path)
	if err != nil {
		return fileprotocol.TransferResult{}, fmt.Errorf("open filesystem root: %w", err)
	}
	defer closeRootAfterRead(root)
	if request.Manifest.Direction == fileprotocol.TransferUpload {
		return root.applyUpload(ctx, request, plane)
	}
	return root.stageDownload(ctx, request, plane)
}

func (root *rootHandle) stageDownload(ctx context.Context, request fileprotocol.TransferRequest, plane TransferPlane) (fileprotocol.TransferResult, error) {
	components, err := validateRelativePath(request.Manifest.RelativePath)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}
	if err := root.checkPreconditions(components, request.Manifest.Preconditions); err != nil {
		return fileprotocol.TransferResult{}, err
	}
	file, info, err := root.openRegularFile(components)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}
	defer closeFileAfterRead(file)
	if info.Size() != request.Manifest.Size {
		return fileprotocol.TransferResult{}, fsConflict("source size changed")
	}
	acknowledged, objectDigest, err := uploadDownloadChunks(ctx, request, plane, file)
	if err != nil {
		return fileprotocol.TransferResult{}, err
	}
	if objectDigest != request.Manifest.SHA256 {
		return fileprotocol.TransferResult{}, fsConflict("source digest changed")
	}
	if err := retryTransfer(ctx, func() error { return plane.Finalize(ctx, request.Manifest.TransferID, request.Lease.Token) }); err != nil {
		return fileprotocol.TransferResult{}, err
	}
	return fileprotocol.TransferResult{
		ProtocolVersion: fileprotocol.Version, TransferID: request.Manifest.TransferID,
		State: "completed", Acknowledged: acknowledged, BytesVerified: request.Manifest.Size,
		Scanning: "not_scanned",
	}, nil
}

func uploadDownloadChunks(ctx context.Context, request fileprotocol.TransferRequest, plane TransferPlane, file *os.File) ([]int, string, error) {
	whole := sha256.New()
	acknowledged := make([]int, 0, len(request.Manifest.Chunks))
	for _, chunk := range request.Manifest.Chunks {
		data := make([]byte, chunk.Size)
		read, err := file.ReadAt(data, chunk.Offset)
		if err != nil && err != io.EOF {
			return nil, "", fmt.Errorf("read transfer source: %w", err)
		}
		if int64(read) != chunk.Size || transferDigest(data) != chunk.SHA256 {
			return nil, "", fsConflict("source changed during transfer")
		}
		if _, err := whole.Write(data); err != nil {
			return nil, "", fmt.Errorf("hash transfer source: %w", err)
		}
		if err := retryTransfer(ctx, func() error {
			return plane.PutChunk(ctx, request.Manifest.TransferID, request.Lease.Token, chunk.Index, data)
		}); err != nil {
			return nil, "", err
		}
		acknowledged = append(acknowledged, chunk.Index)
	}
	return acknowledged, hex.EncodeToString(whole.Sum(nil)), nil
}

func validateTransferRequest(request fileprotocol.TransferRequest) error {
	manifest := request.Manifest
	if err := validateTransferEnvelope(request); err != nil {
		return err
	}
	if err := validateTransferManifestHeader(manifest); err != nil {
		return err
	}
	if err := validateTransferChunks(manifest); err != nil {
		return err
	}
	_, err := validateRelativePath(manifest.RelativePath)
	return err
}

func validateTransferEnvelope(request fileprotocol.TransferRequest) error {
	if request.ProtocolVersion != fileprotocol.Version || request.Manifest.ProtocolVersion != fileprotocol.Version {
		return fmt.Errorf("unsupported transfer protocol version")
	}
	if request.Manifest.TransferID == "" || request.Lease.Token == "" {
		return fmt.Errorf("transfer lease is invalid")
	}
	if time.Now().UTC().After(request.Lease.ExpiresAt) {
		return fmt.Errorf("transfer lease expired")
	}
	return nil
}

func validateTransferManifestHeader(manifest fileprotocol.TransferManifest) error {
	if manifest.Direction != fileprotocol.TransferUpload && manifest.Direction != fileprotocol.TransferDownload {
		return fmt.Errorf("transfer direction is invalid")
	}
	if manifest.Size < 0 || manifest.Size > transferMaxSize {
		return fmt.Errorf("transfer size is outside limit")
	}
	if manifest.ChunkSize <= 0 || manifest.ChunkSize > transferChunkSize {
		return fmt.Errorf("transfer chunk size is outside limit")
	}
	if len(manifest.SHA256) != sha256.Size*2 {
		return fmt.Errorf("transfer digest is invalid")
	}
	if !validTransferConflict(manifest.Conflict) {
		return fmt.Errorf("transfer conflict strategy is invalid")
	}
	return nil
}

func validTransferConflict(strategy fileprotocol.ConflictStrategy) bool {
	switch strategy {
	case fileprotocol.ConflictFail, fileprotocol.ConflictSkip, fileprotocol.ConflictRenameNew, fileprotocol.ConflictReplace:
		return true
	default:
		return false
	}
}

func validateTransferChunks(manifest fileprotocol.TransferManifest) error {
	if len(manifest.Chunks) == 0 {
		if manifest.Size != 0 || manifest.SHA256 != transferDigest(nil) {
			return fmt.Errorf("empty transfer manifest is invalid")
		}
	}
	if len(manifest.Chunks) > transferMaxChunks {
		return fmt.Errorf("transfer chunk count exceeds limit")
	}
	var offset int64
	for index, chunk := range manifest.Chunks {
		if err := validateTransferChunk(chunk, index, offset, manifest.ChunkSize); err != nil {
			return err
		}
		offset += chunk.Size
	}
	if offset != manifest.Size {
		return fmt.Errorf("transfer manifest size mismatch")
	}
	return nil
}

func validateTransferChunk(chunk fileprotocol.ChunkManifest, index int, offset, chunkSize int64) error {
	if chunk.Index != index || chunk.Offset != offset {
		return fmt.Errorf("transfer chunk ordering is invalid")
	}
	if chunk.Size <= 0 || chunk.Size > chunkSize {
		return fmt.Errorf("transfer chunk size is invalid")
	}
	if len(chunk.SHA256) != sha256.Size*2 {
		return fmt.Errorf("transfer chunk digest is invalid")
	}
	return nil
}

func retryTransfer(ctx context.Context, operation func() error) error {
	var lastErr error
	for attempt := 0; attempt < transferRetryAttempts; attempt++ {
		err := operation()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == transferRetryAttempts-1 {
			break
		}
		delay := time.Duration(1<<attempt) * transferRetryBaseDelay
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("transfer cancelled: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return fmt.Errorf("transfer retry budget exhausted: %w", lastErr)
}

func transferDigest(data []byte) string {
	value := sha256.Sum256(data)
	return hex.EncodeToString(value[:])
}
