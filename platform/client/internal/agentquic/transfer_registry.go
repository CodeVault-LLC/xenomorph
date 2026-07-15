package agentquic

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
)

const (
	maximumActiveTransferContracts = 64
	maximumTransferChunks          = 256
	transferCapabilityHexLength    = 64
	maximumTransferSize            = 1 << 30
	maximumTransferChunkSize       = 4 << 20
)

type transferRegistry struct {
	mu        sync.Mutex
	contracts map[string]fileprotocol.TransferRequest
	sessions  map[string]*transferSession
}

func newTransferRegistry() transferRegistry {
	return transferRegistry{
		contracts: make(map[string]fileprotocol.TransferRequest),
		sessions:  make(map[string]*transferSession),
	}
}

func (registry *transferRegistry) applySignedCommand(commandType agent.CommandType, payload []byte) error {
	if commandType == agent.CommandTypeFilesTransferAbort {
		var request fileprotocol.TransferRequest
		if err := json.Unmarshal(payload, &request); err == nil {
			registry.remove(request.Manifest.TransferID)
		}
		return nil
	}
	if commandType != agent.CommandTypeFilesTransferPrepare && commandType != agent.CommandTypeFilesTransferResume {
		return nil
	}
	var request fileprotocol.TransferRequest
	decoderError := json.Unmarshal(payload, &request)
	if decoderError != nil {
		return fmt.Errorf("register signed transfer contract: %w", decoderError)
	}
	if err := validateTransferContract(request); err != nil {
		return err
	}
	registry.mu.Lock()
	expired := registry.pruneLocked(time.Now().UTC())
	if _, exists := registry.contracts[request.Manifest.TransferID]; !exists && len(registry.contracts) >= maximumActiveTransferContracts {
		registry.mu.Unlock()
		closeTransferSessions(expired)
		return fmt.Errorf("register signed transfer contract: capacity reached")
	}
	registry.contracts[request.Manifest.TransferID] = request
	previous := registry.sessions[request.Manifest.TransferID]
	delete(registry.sessions, request.Manifest.TransferID)
	registry.mu.Unlock()
	closeTransferSessions(expired)
	if previous != nil {
		_ = previous.close()
	}
	return nil
}

func validateTransferContract(request fileprotocol.TransferRequest) error {
	if err := validateTransferIdentity(request); err != nil {
		return err
	}
	if err := validateTransferBounds(request.Manifest); err != nil {
		return err
	}
	return validateTransferCapability(request.Lease)
}

func validateTransferIdentity(request fileprotocol.TransferRequest) error {
	manifest := request.Manifest
	if request.ProtocolVersion != fileprotocol.Version || manifest.ProtocolVersion != fileprotocol.Version {
		return fmt.Errorf("register signed transfer contract: protocol version mismatch")
	}
	if _, err := uuid.Parse(manifest.TransferID); err != nil {
		return fmt.Errorf("register signed transfer contract: invalid transfer ID")
	}
	if manifest.Direction != fileprotocol.TransferUpload && manifest.Direction != fileprotocol.TransferDownload {
		return fmt.Errorf("register signed transfer contract: invalid direction")
	}
	return nil
}

func validateTransferBounds(manifest fileprotocol.TransferManifest) error {
	if manifest.Size < 0 || manifest.Size > maximumTransferSize || manifest.ChunkSize <= 0 ||
		manifest.ChunkSize > maximumTransferChunkSize || len(manifest.Chunks) > maximumTransferChunks {
		return fmt.Errorf("register signed transfer contract: manifest bound exceeded")
	}
	_, err := fileprotocol.TransferManifestDigest(manifest)
	return err
}

func validateTransferCapability(lease fileprotocol.DataPlaneLease) error {
	if len(lease.Token) != transferCapabilityHexLength {
		return fmt.Errorf("register signed transfer contract: invalid capability length")
	}
	if _, err := hex.DecodeString(lease.Token); err != nil || !lease.ExpiresAt.After(time.Now().UTC()) {
		return fmt.Errorf("register signed transfer contract: invalid or expired capability")
	}
	return nil
}

func (registry *transferRegistry) contract(transferID, token string) (fileprotocol.TransferRequest, error) {
	registry.mu.Lock()
	expired := registry.pruneLocked(time.Now().UTC())
	request, exists := registry.contracts[transferID]
	registry.mu.Unlock()
	closeTransferSessions(expired)
	if !exists || request.Lease.Token != token {
		return fileprotocol.TransferRequest{}, fmt.Errorf("use signed transfer contract: capability scope mismatch")
	}
	return request, nil
}

func (registry *transferRegistry) session(transferID string) *transferSession {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return registry.sessions[transferID]
}

func (registry *transferRegistry) setSession(transferID string, session *transferSession) {
	registry.mu.Lock()
	registry.sessions[transferID] = session
	registry.mu.Unlock()
}

func (registry *transferRegistry) clearSession(transferID string, candidate *transferSession) {
	registry.mu.Lock()
	if registry.sessions[transferID] == candidate {
		delete(registry.sessions, transferID)
	}
	registry.mu.Unlock()
}

func (registry *transferRegistry) remove(transferID string) {
	registry.mu.Lock()
	session := registry.sessions[transferID]
	delete(registry.sessions, transferID)
	delete(registry.contracts, transferID)
	registry.mu.Unlock()
	if session != nil {
		_ = session.close()
	}
}

func (registry *transferRegistry) complete(transferID string, candidate *transferSession) {
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.sessions[transferID] == candidate {
		delete(registry.sessions, transferID)
		delete(registry.contracts, transferID)
	}
}

func (registry *transferRegistry) pruneLocked(now time.Time) []*transferSession {
	expired := make([]*transferSession, 0)
	for transferID, request := range registry.contracts {
		if now.After(request.Lease.ExpiresAt) {
			if session := registry.sessions[transferID]; session != nil {
				expired = append(expired, session)
			}
			delete(registry.sessions, transferID)
			delete(registry.contracts, transferID)
		}
	}
	return expired
}

func closeTransferSessions(sessions []*transferSession) {
	for _, session := range sessions {
		_ = session.close()
	}
}
