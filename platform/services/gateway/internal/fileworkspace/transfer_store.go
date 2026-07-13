package fileworkspace

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
	"github.com/google/uuid"
)

const (
	defaultChunkSize  int64 = 4 << 20
	maxTransferBytes  int64 = 1 << 30
	maxTransferChunks       = 256
	maxTransfers            = 2_000
	transferRetention       = 24 * time.Hour
	leaseDuration           = 2 * time.Minute
	spoolKeyBytes           = 32
	leaseTokenBytes         = 32
)

// TransferState is a gateway-owned durable transfer lifecycle state.
type TransferState string

const (
	// TransferStaging indicates that browser-authored upload chunks are arriving.
	TransferStaging TransferState = "staging"
	// TransferQueued indicates that a signed agent command is pending.
	TransferQueued TransferState = "queued"
	// TransferRunning indicates that at least one chunk was verified.
	TransferRunning TransferState = "running"
	// TransferPaused indicates that a lease expired or connectivity was lost.
	TransferPaused TransferState = "paused"
	// TransferCompleted indicates that the complete object was verified.
	TransferCompleted TransferState = "completed"
	// TransferFailed indicates a terminal classified failure.
	TransferFailed TransferState = "failed"
	// TransferCancelled indicates an idempotent cancellation result.
	TransferCancelled TransferState = "cancelled"
)

// Transfer is a gateway-owned durable transfer record. Manifest paths and
// checksums are client- or website-authored observations, not trust evidence.
type Transfer struct {
	TransferID        string                        `json:"transfer_id"`
	AgentID           string                        `json:"agent_id"`
	OperatorID        string                        `json:"operator_id"`
	State             TransferState                 `json:"state"`
	Manifest          fileprotocol.TransferManifest `json:"manifest"`
	Acknowledged      []int                         `json:"acknowledged_chunks"`
	BytesVerified     int64                         `json:"bytes_verified"`
	ErrorClass        string                        `json:"error_class,omitempty"`
	LeaseTokenHash    string                        `json:"lease_token_hash,omitempty"`
	LeaseExpiresAt    time.Time                     `json:"lease_expires_at,omitempty"`
	CreatedAt         time.Time                     `json:"created_at"`
	UpdatedAt         time.Time                     `json:"updated_at"`
	ExpiresAt         time.Time                     `json:"expires_at"`
	RetentionDeadline time.Time                     `json:"retention_deadline"`
}

// TransferStore owns bounded encrypted staging bytes and durable chunk
// acknowledgements. It does not issue browser or agent identity.
type TransferStore struct {
	mu        sync.RWMutex
	path      string
	spoolPath string
	aead      cipher.AEAD
	transfers map[string]Transfer
}

func newTransferStore(statePath string) (*TransferStore, error) {
	directory := filepath.Dir(statePath)
	aead, err := loadOrCreateSpoolCipher(filepath.Join(directory, "file-spool.key"))
	if err != nil {
		return nil, err
	}
	store := &TransferStore{
		path:      filepath.Join(directory, "file-transfers.json"),
		spoolPath: filepath.Join(directory, "file-spool"),
		aead:      aead, transfers: make(map[string]Transfer),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	return store, nil
}

// CreateTransfer validates and persists an immutable single-file manifest.
func (store *TransferStore) CreateTransfer(agentID, operatorID string, manifest fileprotocol.TransferManifest) (Transfer, fileprotocol.DataPlaneLease, error) {
	if err := validateManifestOrDownloadPlan(manifest); err != nil {
		return Transfer{}, fileprotocol.DataPlaneLease{}, err
	}
	if agentID == "" || operatorID == "" {
		return Transfer{}, fileprotocol.DataPlaneLease{}, fmt.Errorf("transfer agent and operator are required")
	}
	now := time.Now().UTC()
	manifest.ProtocolVersion = fileprotocol.Version
	manifest.TransferID = uuid.New().String()
	token, tokenHash, err := newLeaseToken()
	if err != nil {
		return Transfer{}, fileprotocol.DataPlaneLease{}, err
	}
	state := TransferQueued
	if manifest.Direction == fileprotocol.TransferUpload {
		state = TransferStaging
	}
	transfer := Transfer{
		TransferID: manifest.TransferID, AgentID: agentID, OperatorID: operatorID,
		State: state, Manifest: manifest, Acknowledged: make([]int, 0), LeaseTokenHash: tokenHash,
		LeaseExpiresAt: now.Add(leaseDuration), CreatedAt: now, UpdatedAt: now,
		ExpiresAt: now.Add(transferRetention), RetentionDeadline: now.Add(transferRetention),
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	store.evictExpiredLocked(now)
	if len(store.transfers) >= maxTransfers {
		return Transfer{}, fileprotocol.DataPlaneLease{}, fmt.Errorf("transfer store is full")
	}
	store.transfers[transfer.TransferID] = transfer
	if err := store.persistLocked(); err != nil {
		delete(store.transfers, transfer.TransferID)
		return Transfer{}, fileprotocol.DataPlaneLease{}, err
	}
	return cloneTransfer(transfer), fileprotocol.DataPlaneLease{Token: token, ExpiresAt: transfer.LeaseExpiresAt}, nil
}

// Transfer returns an agent-scoped durable transfer without its token hash.
func (store *TransferStore) Transfer(agentID, transferID string) (Transfer, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	transfer, ok := store.transfers[transferID]
	if !ok || transfer.AgentID != agentID {
		return Transfer{}, false
	}
	transfer.LeaseTokenHash = ""
	return cloneTransfer(transfer), true
}

// Transfers returns a bounded newest-first agent-scoped transfer snapshot.
func (store *TransferStore) Transfers(agentID string) []Transfer {
	store.mu.RLock()
	defer store.mu.RUnlock()
	result := make([]Transfer, 0)
	for _, transfer := range store.transfers {
		if transfer.AgentID == agentID {
			transfer.LeaseTokenHash = ""
			result = append(result, cloneTransfer(transfer))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })
	return result
}

// Remove deletes one terminal agent-scoped transfer and its encrypted staging
// bytes. Active transfers must be cancelled before removal.
func (store *TransferStore) Remove(agentID, transferID string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	transfer, ok := store.transfers[transferID]
	if !ok || transfer.AgentID != agentID {
		return fmt.Errorf("transfer not found")
	}
	if !terminalTransferState(transfer.State) {
		return fmt.Errorf("active transfer cannot be removed")
	}
	if err := os.RemoveAll(filepath.Join(store.spoolPath, transferID)); err != nil {
		return fmt.Errorf("remove transfer spool: %w", err)
	}
	delete(store.transfers, transferID)
	if err := store.persistLocked(); err != nil {
		store.transfers[transferID] = transfer
		return err
	}
	return nil
}

// RemoveFinished deletes all terminal transfers for one agent and returns the
// number removed. Transfers for other agents and active transfers are retained.
func (store *TransferStore) RemoveFinished(agentID string) (int, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	removed := make(map[string]Transfer)
	for transferID, transfer := range store.transfers {
		if transfer.AgentID == agentID && terminalTransferState(transfer.State) {
			removed[transferID] = transfer
		}
	}
	if len(removed) == 0 {
		return 0, nil
	}
	for transferID := range removed {
		if err := os.RemoveAll(filepath.Join(store.spoolPath, transferID)); err != nil {
			return 0, fmt.Errorf("remove transfer spool: %w", err)
		}
	}
	for transferID := range removed {
		delete(store.transfers, transferID)
	}
	if err := store.persistLocked(); err != nil {
		for transferID, transfer := range removed {
			store.transfers[transferID] = transfer
		}
		return 0, err
	}
	return len(removed), nil
}

// PutChunk durably acknowledges a checksum-verified encrypted chunk.
func (store *TransferStore) PutChunk(agentID, transferID, token string, index int, data []byte) (Transfer, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	transfer, err := store.authorizeLocked(agentID, transferID, token, index)
	if err != nil {
		return Transfer{}, err
	}
	chunk := transfer.Manifest.Chunks[index]
	if int64(len(data)) != chunk.Size || digest(data) != chunk.SHA256 {
		return Transfer{}, fmt.Errorf("transfer chunk integrity mismatch")
	}
	if containsIndex(transfer.Acknowledged, index) {
		return cloneTransfer(transfer), nil
	}
	if err := store.writeChunkLocked(transferID, index, data); err != nil {
		return Transfer{}, err
	}
	transfer.Acknowledged = append(transfer.Acknowledged, index)
	sort.Ints(transfer.Acknowledged)
	transfer.BytesVerified += int64(len(data))
	transfer.UpdatedAt = time.Now().UTC()
	transfer.State = TransferRunning
	store.transfers[transferID] = transfer
	if err := store.persistLocked(); err != nil {
		return Transfer{}, err
	}
	return cloneTransfer(transfer), nil
}

// StageBrowserChunk accepts bytes through the gateway-controlled browser API;
// it does not expose a storage capability to the browser.
func (store *TransferStore) StageBrowserChunk(agentID, transferID string, index int, data []byte) (Transfer, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	transfer, err := store.validateBrowserChunkLocked(agentID, transferID, index, data)
	if err != nil {
		return Transfer{}, err
	}
	if containsIndex(transfer.Acknowledged, index) {
		return cloneTransfer(transfer), nil
	}
	if err := store.writeChunkLocked(transferID, index, data); err != nil {
		return Transfer{}, err
	}
	transfer.Acknowledged = append(transfer.Acknowledged, index)
	sort.Ints(transfer.Acknowledged)
	transfer.BytesVerified += int64(len(data))
	transfer.UpdatedAt = time.Now().UTC()
	store.transfers[transferID] = transfer
	if err := store.persistLocked(); err != nil {
		return Transfer{}, err
	}
	return cloneTransfer(transfer), nil
}

func (store *TransferStore) validateBrowserChunkLocked(agentID, transferID string, index int, data []byte) (Transfer, error) {
	transfer, ok := store.transfers[transferID]
	if !ok || transfer.AgentID != agentID {
		return Transfer{}, fmt.Errorf("upload transfer scope mismatch")
	}
	if transfer.Manifest.Direction != fileprotocol.TransferUpload || transfer.State != TransferStaging {
		return Transfer{}, fmt.Errorf("upload transfer is not staging")
	}
	if index < 0 || index >= len(transfer.Manifest.Chunks) {
		return Transfer{}, fmt.Errorf("transfer chunk index is invalid")
	}
	chunk := transfer.Manifest.Chunks[index]
	if int64(len(data)) != chunk.Size || digest(data) != chunk.SHA256 {
		return Transfer{}, fmt.Errorf("transfer chunk integrity mismatch")
	}
	return transfer, nil
}

// IssueLease rotates and persists a short-lived agent data-plane capability.
func (store *TransferStore) IssueLease(agentID, transferID string) (fileprotocol.DataPlaneLease, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	transfer, ok := store.transfers[transferID]
	if !ok || transfer.AgentID != agentID {
		return fileprotocol.DataPlaneLease{}, fmt.Errorf("transfer not found")
	}
	token, tokenHash, err := newLeaseToken()
	if err != nil {
		return fileprotocol.DataPlaneLease{}, err
	}
	transfer.LeaseTokenHash = tokenHash
	transfer.LeaseExpiresAt = time.Now().UTC().Add(leaseDuration)
	transfer.UpdatedAt = time.Now().UTC()
	if transfer.State == TransferPaused {
		transfer.State = TransferQueued
	}
	store.transfers[transferID] = transfer
	if err := store.persistLocked(); err != nil {
		return fileprotocol.DataPlaneLease{}, err
	}
	return fileprotocol.DataPlaneLease{Token: token, ExpiresAt: transfer.LeaseExpiresAt}, nil
}

// ValidateLease verifies the agent, transfer, token, and expiry binding.
func (store *TransferStore) ValidateLease(agentID, transferID, token string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	transfer, ok := store.transfers[transferID]
	if !ok || transfer.AgentID != agentID {
		return fmt.Errorf("transfer capability scope mismatch")
	}
	provided := sha256.Sum256([]byte(token))
	expected, err := hex.DecodeString(transfer.LeaseTokenHash)
	if err != nil || len(expected) != len(provided) || subtle.ConstantTimeCompare(provided[:], expected) != 1 {
		return fmt.Errorf("transfer capability is invalid")
	}
	if time.Now().UTC().After(transfer.LeaseExpiresAt) {
		return fmt.Errorf("transfer capability expired")
	}
	return nil
}

// ReadChunk returns one verified staged chunk to the scoped transfer holder.
func (store *TransferStore) ReadChunk(agentID, transferID, token string, index int) ([]byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	transfer, err := store.authorizeLocked(agentID, transferID, token, index)
	if err != nil {
		return nil, err
	}
	if !containsIndex(transfer.Acknowledged, index) {
		return nil, fmt.Errorf("transfer chunk is not staged")
	}
	return store.readChunkLocked(transferID, index)
}

// ReadCompletedChunk serves verified download bytes through the gateway API.
func (store *TransferStore) ReadCompletedChunk(agentID, transferID string, index int) ([]byte, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	transfer, ok := store.transfers[transferID]
	if !ok || transfer.AgentID != agentID || transfer.State != TransferCompleted || transfer.Manifest.Direction != fileprotocol.TransferDownload {
		return nil, fmt.Errorf("completed download transfer not found")
	}
	if index < 0 || index >= len(transfer.Manifest.Chunks) || !containsIndex(transfer.Acknowledged, index) {
		return nil, fmt.Errorf("completed transfer chunk not found")
	}
	return store.readChunkLocked(transferID, index)
}

// Finalize verifies the complete staged object before marking it publishable.
func (store *TransferStore) Finalize(agentID, transferID string) (Transfer, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	transfer, ok := store.transfers[transferID]
	if !ok || transfer.AgentID != agentID {
		return Transfer{}, fmt.Errorf("transfer not found")
	}
	if len(transfer.Acknowledged) != len(transfer.Manifest.Chunks) {
		return Transfer{}, fmt.Errorf("transfer has missing chunks")
	}
	valid, err := store.verifyObjectLocked(transfer)
	if err != nil {
		return Transfer{}, err
	}
	if !valid {
		transfer.State = TransferFailed
		transfer.ErrorClass = "integrity_failure"
		store.transfers[transferID] = transfer
		_ = store.persistLocked()
		return Transfer{}, fmt.Errorf("transfer object integrity mismatch")
	}
	transfer.State = TransferCompleted
	if transfer.Manifest.Direction == fileprotocol.TransferUpload {
		transfer.State = TransferQueued
	}
	transfer.UpdatedAt = time.Now().UTC()
	transfer.LeaseTokenHash = ""
	store.transfers[transferID] = transfer
	if err := store.persistLocked(); err != nil {
		return Transfer{}, err
	}
	return cloneTransfer(transfer), nil
}

func (store *TransferStore) verifyObjectLocked(transfer Transfer) (bool, error) {
	hash := sha256.New()
	var size int64
	for index := range transfer.Manifest.Chunks {
		data, err := store.readChunkLocked(transfer.TransferID, index)
		if err != nil {
			return false, err
		}
		size += int64(len(data))
		if _, err := hash.Write(data); err != nil {
			return false, fmt.Errorf("hash staged transfer: %w", err)
		}
	}
	return size == transfer.Manifest.Size && hex.EncodeToString(hash.Sum(nil)) == transfer.Manifest.SHA256, nil
}

// RecordAgentResult applies a bounded authenticated-agent transfer result and
// rejects invalid or regressive state transitions.
func (store *TransferStore) RecordAgentResult(agentID string, result fileprotocol.TransferResult) (Transfer, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	transfer, ok := store.transfers[result.TransferID]
	if !ok || transfer.AgentID != agentID {
		return Transfer{}, fmt.Errorf("transfer result scope mismatch")
	}
	if transfer.State == TransferCompleted || transfer.State == TransferCancelled {
		return cloneTransfer(transfer), nil
	}
	transfer, err := applyTransferResult(transfer, result)
	if err != nil {
		return Transfer{}, err
	}
	transfer.UpdatedAt = time.Now().UTC()
	transfer.LeaseTokenHash = ""
	store.transfers[transfer.TransferID] = transfer
	if err := store.persistLocked(); err != nil {
		return Transfer{}, err
	}
	return cloneTransfer(transfer), nil
}

func applyTransferResult(transfer Transfer, result fileprotocol.TransferResult) (Transfer, error) {
	switch result.State {
	case "prepared":
		return applyPreparedManifest(transfer, result.Manifest)
	case "completed", "skipped":
		if result.State == "completed" && result.BytesVerified != transfer.Manifest.Size {
			return Transfer{}, fmt.Errorf("transfer result byte count mismatch")
		}
		transfer.State = TransferCompleted
	case "paused":
		transfer.State = TransferPaused
	case "cancelled":
		transfer.State = TransferCancelled
	case "failed":
		transfer.State, transfer.ErrorClass = TransferFailed, result.ErrorClass
	default:
		return Transfer{}, fmt.Errorf("transfer result state is invalid")
	}
	return transfer, nil
}

func applyPreparedManifest(transfer Transfer, manifest *fileprotocol.TransferManifest) (Transfer, error) {
	if manifest == nil {
		return Transfer{}, fmt.Errorf("prepared transfer manifest is missing")
	}
	if manifest.TransferID != transfer.TransferID || manifest.Direction != fileprotocol.TransferDownload {
		return Transfer{}, fmt.Errorf("prepared transfer manifest scope mismatch")
	}
	if manifest.RootID != transfer.Manifest.RootID || manifest.RelativePath != transfer.Manifest.RelativePath {
		return Transfer{}, fmt.Errorf("prepared transfer path scope mismatch")
	}
	if err := validateManifest(*manifest); err != nil {
		return Transfer{}, err
	}
	transfer.Manifest, transfer.State = *manifest, TransferQueued
	return transfer, nil
}

func (store *TransferStore) authorizeLocked(agentID, transferID, token string, index int) (Transfer, error) {
	transfer, ok := store.transfers[transferID]
	if !ok || transfer.AgentID != agentID || index < 0 || index >= len(transfer.Manifest.Chunks) {
		return Transfer{}, fmt.Errorf("transfer capability scope mismatch")
	}
	provided := sha256.Sum256([]byte(token))
	expected, err := hex.DecodeString(transfer.LeaseTokenHash)
	if err != nil || len(expected) != len(provided) || subtle.ConstantTimeCompare(provided[:], expected) != 1 {
		return Transfer{}, fmt.Errorf("transfer capability is invalid")
	}
	if time.Now().UTC().After(transfer.LeaseExpiresAt) {
		transfer.State = TransferPaused
		store.transfers[transferID] = transfer
		return Transfer{}, fmt.Errorf("transfer capability expired")
	}
	return transfer, nil
}

func (store *TransferStore) writeChunkLocked(transferID string, index int, data []byte) error {
	directory := filepath.Join(store.spoolPath, transferID)
	if err := os.MkdirAll(directory, stateDirectoryPermission); err != nil {
		return fmt.Errorf("create transfer spool: %w", err)
	}
	nonce := make([]byte, store.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("create transfer nonce: %w", err)
	}
	sealed := store.aead.Seal(nonce, nonce, data, []byte(transferID))
	path := filepath.Join(directory, fmt.Sprintf("%06d.chunk", index))
	if err := os.WriteFile(path, sealed, stateFilePermission); err != nil {
		return fmt.Errorf("write encrypted transfer chunk: %w", err)
	}
	return nil
}

func (store *TransferStore) readChunkLocked(transferID string, index int) ([]byte, error) {
	path := filepath.Join(store.spoolPath, transferID, fmt.Sprintf("%06d.chunk", index))
	data, err := os.ReadFile(path) // #nosec G304 -- both path components are gateway-generated.
	if err != nil {
		return nil, fmt.Errorf("read encrypted transfer chunk: %w", err)
	}
	nonceSize := store.aead.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("encrypted transfer chunk is truncated")
	}
	plain, err := store.aead.Open(nil, data[:nonceSize], data[nonceSize:], []byte(transferID))
	if err != nil {
		return nil, fmt.Errorf("decrypt transfer chunk: %w", err)
	}
	return plain, nil
}

func (store *TransferStore) load() error {
	data, err := os.ReadFile(store.path) // #nosec G304 -- the path is gateway configuration.
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read transfer store: %w", err)
	}
	var transfers []Transfer
	if err := json.Unmarshal(data, &transfers); err != nil {
		return fmt.Errorf("decode transfer store: %w", err)
	}
	if len(transfers) > maxTransfers {
		return fmt.Errorf("transfer store exceeds limit")
	}
	for _, transfer := range transfers {
		if _, err := uuid.Parse(transfer.TransferID); err != nil || transfer.Manifest.TransferID != transfer.TransferID {
			return fmt.Errorf("transfer store contains an invalid server-authored identifier")
		}
		if transfer.Acknowledged == nil {
			transfer.Acknowledged = make([]int, 0)
		}
		store.transfers[transfer.TransferID] = transfer
	}
	return nil
}

func (store *TransferStore) persistLocked() error {
	transfers := make([]Transfer, 0, len(store.transfers))
	for _, transfer := range store.transfers {
		transfers = append(transfers, transfer)
	}
	sort.Slice(transfers, func(i, j int) bool { return transfers[i].CreatedAt.Before(transfers[j].CreatedAt) })
	data, err := json.Marshal(transfers)
	if err != nil {
		return fmt.Errorf("encode transfer store: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(store.path), stateDirectoryPermission); err != nil {
		return fmt.Errorf("create transfer state directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(store.path), ".file-transfers-*")
	if err != nil {
		return fmt.Errorf("create transfer snapshot: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(stateFilePermission); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("protect transfer snapshot: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write transfer snapshot: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync transfer snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close transfer snapshot: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		return fmt.Errorf("publish transfer snapshot: %w", err)
	}
	return nil
}

func (store *TransferStore) evictExpiredLocked(now time.Time) {
	for transferID, transfer := range store.transfers {
		if now.After(transfer.RetentionDeadline) {
			delete(store.transfers, transferID)
			_ = os.RemoveAll(filepath.Join(store.spoolPath, transferID))
		}
	}
}

func validateManifest(manifest fileprotocol.TransferManifest) error {
	if err := validateManifestHeader(manifest); err != nil {
		return err
	}
	if len(manifest.Chunks) == 0 && (manifest.Size != 0 || manifest.SHA256 != digest(nil)) {
		return fmt.Errorf("transfer chunk count exceeds limit")
	}
	if len(manifest.Chunks) > maxTransferChunks {
		return fmt.Errorf("transfer chunk count exceeds limit")
	}
	var offset int64
	for index, chunk := range manifest.Chunks {
		if err := validateManifestChunk(chunk, index, offset, manifest.ChunkSize); err != nil {
			return err
		}
		offset += chunk.Size
	}
	if offset != manifest.Size {
		return fmt.Errorf("transfer manifest size mismatch")
	}
	return nil
}

func validateManifestHeader(manifest fileprotocol.TransferManifest) error {
	if manifest.Direction != fileprotocol.TransferUpload && manifest.Direction != fileprotocol.TransferDownload {
		return fmt.Errorf("transfer direction is invalid")
	}
	if manifest.Size < 0 || manifest.Size > maxTransferBytes {
		return fmt.Errorf("transfer size exceeds limit")
	}
	if manifest.ChunkSize <= 0 || manifest.ChunkSize > defaultChunkSize {
		return fmt.Errorf("transfer chunk size exceeds limit")
	}
	if len(manifest.SHA256) != sha256.Size*2 {
		return fmt.Errorf("transfer digest is invalid")
	}
	if !validConflictStrategy(manifest.Conflict) {
		return fmt.Errorf("transfer conflict strategy is invalid")
	}
	return nil
}

func validateManifestChunk(chunk fileprotocol.ChunkManifest, index int, offset, chunkSize int64) error {
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

func validateManifestOrDownloadPlan(manifest fileprotocol.TransferManifest) error {
	if manifest.Direction == fileprotocol.TransferDownload && len(manifest.Chunks) == 0 {
		if manifest.RootID == "" || manifest.RelativePath == "" || !validConflictStrategy(manifest.Conflict) {
			return fmt.Errorf("download transfer plan is invalid")
		}
		return nil
	}
	return validateManifest(manifest)
}

func loadOrCreateSpoolCipher(path string) (cipher.AEAD, error) {
	key, err := os.ReadFile(path) // #nosec G304 -- the path is gateway state configuration.
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read transfer spool key: %w", err)
	}
	if os.IsNotExist(err) {
		key = make([]byte, spoolKeyBytes)
		if _, err := io.ReadFull(rand.Reader, key); err != nil {
			return nil, fmt.Errorf("generate transfer spool key: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(path), stateDirectoryPermission); err != nil {
			return nil, fmt.Errorf("create transfer key directory: %w", err)
		}
		if err := os.WriteFile(path, key, stateFilePermission); err != nil {
			return nil, fmt.Errorf("write transfer spool key: %w", err)
		}
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create transfer spool cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create transfer spool AEAD: %w", err)
	}
	return aead, nil
}

func newLeaseToken() (string, string, error) {
	value := make([]byte, leaseTokenBytes)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", "", fmt.Errorf("generate transfer capability: %w", err)
	}
	token := hex.EncodeToString(value)
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:]), nil
}

func digest(data []byte) string { value := sha256.Sum256(data); return hex.EncodeToString(value[:]) }
func containsIndex(values []int, target int) bool {
	index := sort.SearchInts(values, target)
	return index < len(values) && values[index] == target
}
func terminalTransferState(state TransferState) bool {
	return state == TransferCompleted || state == TransferFailed || state == TransferCancelled
}
func cloneTransfer(transfer Transfer) Transfer {
	acknowledged := make([]int, len(transfer.Acknowledged))
	copy(acknowledged, transfer.Acknowledged)
	transfer.Acknowledged = acknowledged
	transfer.Manifest.Chunks = append([]fileprotocol.ChunkManifest(nil), transfer.Manifest.Chunks...)
	return transfer
}
