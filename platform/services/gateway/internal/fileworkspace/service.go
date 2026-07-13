package fileworkspace

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/shared/fileprotocol"
	"github.com/google/uuid"
)

const (
	operationExpiry     = 2 * time.Minute
	maxRootIDBytes      = 128
	maxDirectoryPage    = 500
	maxPreviewReadBytes = 1 << 20
)

// Service validates and dispatches durable read-only file operations.
type Service struct {
	queue *command.Queue
	store *Store
}

// NewService creates an unrestricted read-only file workspace. Filesystem
// roots are discovered and resolved by each agent.
func NewService(queue *command.Queue, store *Store) (*Service, error) {
	if queue == nil || store == nil {
		return nil, fmt.Errorf("file workspace queue and store are required")
	}
	return &Service{queue: queue, store: store}, nil
}

// ProbeRoots persists and dispatches automatic filesystem-root discovery.
func (service *Service) ProbeRoots(agentID, operatorID, traceID string) (Operation, error) {
	now := time.Now().UTC()
	payload, err := json.Marshal(fileprotocol.RootsListRequest{ProtocolVersion: fileprotocol.Version})
	if err != nil {
		return Operation{}, fmt.Errorf("encode file root probe: %w", err)
	}
	commandID := uuid.New().String()
	operation, err := service.store.Create(Operation{
		CommandID: commandID, AgentID: agentID, OperatorID: operatorID,
		RootID: "*", Type: fileprotocol.CommandRootsList,
		ExpiresAt: now.Add(operationExpiry), AuditTraceID: traceID,
	})
	if err != nil {
		return Operation{}, err
	}
	envelope := &command.Envelope{
		CommandID: commandID, Type: fileprotocol.CommandRootsList, Payload: payload,
		RequestedBy: operatorID, Reason: "Automatic filesystem root discovery",
	}
	if err := service.queue.Enqueue(agentID, envelope); err != nil {
		_ = service.store.Fail(operation.OperationID, "dispatch_failed")
		return Operation{}, fmt.Errorf("enqueue file root probe: %w", err)
	}
	return operation, nil
}

// Dispatch validates bounded protocol input, persists state, then enqueues a
// signed command for the selected agent. The agent resolves the root ID against
// its current automatically discovered roots.
func (service *Service) Dispatch(agentID, operatorID, rootID, commandType, traceID string, request any) (Operation, error) {
	if err := validateRootID(rootID); err != nil {
		return Operation{}, err
	}
	now := time.Now().UTC()
	if err := prepareRequest(commandType, request, rootID); err != nil {
		return Operation{}, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return Operation{}, fmt.Errorf("encode file command request: %w", err)
	}
	commandID := uuid.New().String()
	operation, err := service.store.Create(Operation{
		CommandID: commandID,
		AgentID:   agentID, OperatorID: operatorID, RootID: rootID, Type: commandType,
		ExpiresAt: now.Add(operationExpiry), AuditTraceID: traceID,
	})
	if err != nil {
		return Operation{}, err
	}
	envelope := &command.Envelope{
		CommandID: commandID,
		Type:      commandType, Payload: payload, RequestedBy: operatorID,
		Reason: "Read-only file workspace operation",
	}
	if err := service.queue.Enqueue(agentID, envelope); err != nil {
		_ = service.store.Fail(operation.OperationID, "dispatch_failed")
		return Operation{}, fmt.Errorf("enqueue file command: %w", err)
	}
	return operation, nil
}

// Complete records an authenticated client result for a file command.
func (service *Service) Complete(agentID, commandID, status string, result json.RawMessage) error {
	if service == nil {
		return nil
	}
	_, err := service.store.Complete(agentID, commandID, status, result)
	return err
}

// Operation returns one agent-scoped durable operation.
func (service *Service) Operation(agentID, operationID string) (Operation, bool) {
	return service.store.Get(agentID, operationID)
}

func prepareRequest(commandType string, request any, rootID string) error {
	switch typed := request.(type) {
	case *fileprotocol.DirectoryListRequest:
		if commandType != fileprotocol.CommandDirectoryList || typed.PageSize <= 0 || typed.PageSize > maxDirectoryPage {
			return fmt.Errorf("invalid directory request")
		}
		typed.ProtocolVersion = fileprotocol.Version
		typed.RootID = rootID
	case *fileprotocol.MetadataGetRequest:
		if commandType != fileprotocol.CommandMetadataGet {
			return fmt.Errorf("invalid metadata request")
		}
		typed.ProtocolVersion = fileprotocol.Version
		typed.RootID = rootID
	case *fileprotocol.PreviewReadRequest:
		if commandType != fileprotocol.CommandPreviewRead || typed.Offset < 0 || typed.Length <= 0 || typed.Length > maxPreviewReadBytes {
			return fmt.Errorf("invalid preview request")
		}
		typed.ProtocolVersion = fileprotocol.Version
		typed.RootID = rootID
	default:
		return fmt.Errorf("unsupported file command request")
	}
	return nil
}

func validateRootID(rootID string) error {
	if rootID == "" || len(rootID) > maxRootIDBytes || !utf8.ValidString(rootID) || strings.TrimSpace(rootID) != rootID {
		return fmt.Errorf("filesystem root ID is invalid")
	}
	for _, character := range rootID {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '-' {
			return fmt.Errorf("filesystem root ID is invalid")
		}
	}
	return nil
}
