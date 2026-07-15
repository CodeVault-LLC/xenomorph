package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/agentquic"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/fileworkspace"
	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const maximumQUICTransferCapabilities = 256

type quicTransferCapability struct {
	transferID string
	token      string
	direction  uint64
	expiresAt  time.Time
}

type quicTransferRegistry struct {
	mu      sync.Mutex
	entries map[string]quicTransferCapability
}

func newQUICTransferRegistry() *quicTransferRegistry {
	return &quicTransferRegistry{entries: make(map[string]quicTransferCapability)}
}

func (registry *quicTransferRegistry) authorize(
	service *fileworkspace.Service,
	receipt agentquic.IngressReceipt,
	open wire.TransferOpen,
) (quicTransferCapability, error) {
	if registry == nil {
		return quicTransferCapability{}, fmt.Errorf("authorize QUIC transfer: registry is unavailable")
	}
	transferID, token, transfer, err := loadTransferContract(service, receipt, open)
	if err != nil {
		return quicTransferCapability{}, err
	}
	if err := validateTransferManifestContract(transfer.Manifest, open); err != nil {
		return quicTransferCapability{}, err
	}
	expiresAt, err := validateTransferExpiry(transfer.LeaseExpiresAt, open.ExpiresAtMilliseconds)
	if err != nil {
		return quicTransferCapability{}, err
	}
	if err := service.ValidateAgentTransferLease(receipt.AgentID, transferID, token); err != nil {
		return quicTransferCapability{}, err
	}
	capability := quicTransferCapability{transferID: transferID, token: token, direction: open.Direction, expiresAt: expiresAt}
	if err := registry.store(receipt.AgentID, capability); err != nil {
		return quicTransferCapability{}, err
	}
	return capability, nil
}

func loadTransferContract(
	service *fileworkspace.Service,
	receipt agentquic.IngressReceipt,
	open wire.TransferOpen,
) (string, string, fileworkspace.Transfer, error) {
	if service == nil || receipt.OperationID == [16]byte{} ||
		receipt.OperationID != open.TransferID || len(open.SignedCapability) != 64 {
		return "", "", fileworkspace.Transfer{}, fmt.Errorf("authorize QUIC transfer: invalid operation or capability")
	}
	transferID := uuid.UUID(open.TransferID).String()
	token := string(open.SignedCapability)
	if _, err := hex.DecodeString(token); err != nil {
		return "", "", fileworkspace.Transfer{}, fmt.Errorf("authorize QUIC transfer: malformed capability")
	}
	transfer, exists := service.Transfer(receipt.AgentID, transferID)
	if !exists {
		return "", "", fileworkspace.Transfer{}, fmt.Errorf("authorize QUIC transfer: transfer scope mismatch")
	}
	return transferID, token, transfer, nil
}

func validateTransferManifestContract(manifest fileprotocol.TransferManifest, open wire.TransferOpen) error {
	digest, err := fileprotocol.TransferManifestDigest(manifest)
	if err != nil {
		return err
	}
	expectedDirection := uint64(wire.TransferAgentToGateway)
	if manifest.Direction == fileprotocol.TransferUpload {
		expectedDirection = uint64(wire.TransferGatewayToAgent)
	}
	expectedTotalSize, err := uint64FromNonnegativeInt64(manifest.Size, "transfer total size")
	if err != nil {
		return err
	}
	chunkSize, err := uint64FromNonnegativeInt64(manifest.ChunkSize, "transfer chunk size")
	if err != nil {
		return err
	}
	if open.ManifestDigest != digest || open.Direction != expectedDirection ||
		open.ExpectedTotalSize != expectedTotalSize || open.ChunkSize != chunkSize {
		return fmt.Errorf("authorize QUIC transfer: signed manifest mismatch")
	}
	return nil
}

func validateTransferExpiry(leaseExpiry time.Time, expiryMilliseconds uint64) (time.Time, error) {
	expiresMilliseconds, err := int64FromBoundedUint64(expiryMilliseconds, "transfer expiry")
	if err != nil {
		return time.Time{}, err
	}
	expiresAt := time.UnixMilli(expiresMilliseconds).UTC()
	if time.Now().UTC().After(expiresAt) || expiresAt.Sub(leaseExpiry.UTC()).Abs() >= time.Millisecond {
		return time.Time{}, fmt.Errorf("authorize QUIC transfer: capability expiry mismatch")
	}
	return expiresAt, nil
}

func (registry *quicTransferRegistry) store(agentID string, capability quicTransferCapability) error {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.pruneLocked(time.Now().UTC())
	key := transferCapabilityKey(agentID, capability.transferID)
	if _, exists := registry.entries[key]; !exists && len(registry.entries) >= maximumQUICTransferCapabilities {
		return fmt.Errorf("authorize QUIC transfer: capability registry full")
	}
	registry.entries[key] = capability
	return nil
}

func (registry *quicTransferRegistry) capability(agentID string, operationID [16]byte) (quicTransferCapability, error) {
	if registry == nil || operationID == [16]byte{} {
		return quicTransferCapability{}, fmt.Errorf("load QUIC transfer capability: invalid operation")
	}
	transferID := uuid.UUID(operationID).String()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	registry.pruneLocked(time.Now().UTC())
	capability, exists := registry.entries[transferCapabilityKey(agentID, transferID)]
	if !exists {
		return quicTransferCapability{}, fmt.Errorf("load QUIC transfer capability: operation not authorized")
	}
	return capability, nil
}

func (registry *quicTransferRegistry) complete(agentID string, operationID [16]byte) {
	registry.mu.Lock()
	delete(registry.entries, transferCapabilityKey(agentID, uuid.UUID(operationID).String()))
	registry.mu.Unlock()
}

func (registry *quicTransferRegistry) pruneLocked(now time.Time) {
	for key, capability := range registry.entries {
		if now.After(capability.expiresAt) {
			delete(registry.entries, key)
		}
	}
}

func transferCapabilityKey(agentID, transferID string) string {
	return agentID + "\x00" + transferID
}

func manifestObjectDigest(manifest fileprotocol.TransferManifest) ([sha256.Size]byte, error) {
	decoded, err := hex.DecodeString(manifest.SHA256)
	if err != nil || len(decoded) != sha256.Size {
		return [sha256.Size]byte{}, fmt.Errorf("decode transfer object digest")
	}
	var result [sha256.Size]byte
	copy(result[:], decoded)
	return result, nil
}
