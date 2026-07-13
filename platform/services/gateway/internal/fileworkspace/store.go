// Package fileworkspace owns durable read-operation state and command dispatch
// for the remote file workspace. It does not read local agent files or trust
// client-authored filesystem observations.
package fileworkspace

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	stateDirectoryPermission  os.FileMode = 0o700
	stateFilePermission       os.FileMode = 0o600
	maxOperations                         = 10_000
	maxAuditRecordBytes                   = 16 << 10
	auditScannerInitialBuffer             = 4096
)

// OperationState is a gateway-owned file operation lifecycle state.
type OperationState string

const (
	// StateQueued indicates that durable state exists and a command is pending.
	StateQueued OperationState = "queued"
	// StateCompleted indicates that an authenticated agent result was recorded.
	StateCompleted OperationState = "completed"
	// StateFailed indicates that command dispatch or agent execution failed.
	StateFailed OperationState = "failed"
)

// Operation is a gateway-owned durable command record. Result is
// client-authored data received on the authenticated agent channel.
type Operation struct {
	OperationID  string          `json:"operation_id"`
	CommandID    string          `json:"command_id"`
	AgentID      string          `json:"agent_id"`
	OperatorID   string          `json:"operator_id"`
	RootID       string          `json:"root_id"`
	Type         string          `json:"type"`
	State        OperationState  `json:"state"`
	Result       json.RawMessage `json:"result,omitempty"`
	ErrorClass   string          `json:"error_class,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
	ExpiresAt    time.Time       `json:"expires_at"`
	AuditTraceID string          `json:"audit_trace_id"`
}

// AuditRecord is a server-authored append-only file workspace audit event.
// It contains no file bytes or full local paths. Hash and PreviousHash form a
// verifiable chain within one gateway audit file.
type AuditRecord struct {
	Sequence       uint64    `json:"sequence"`
	EventType      string    `json:"event_type"`
	OperationID    string    `json:"operation_id"`
	CommandID      string    `json:"command_id"`
	AgentID        string    `json:"agent_id"`
	OperatorID     string    `json:"operator_id"`
	RootID         string    `json:"root_id"`
	OperationType  string    `json:"operation_type"`
	Classification string    `json:"classification"`
	TraceID        string    `json:"trace_id"`
	OccurredAt     time.Time `json:"occurred_at"`
	PreviousHash   string    `json:"previous_hash"`
	Hash           string    `json:"hash"`
}

// Store persists bounded operation state as an atomically replaced snapshot.
type Store struct {
	mu            sync.RWMutex
	path          string
	operations    map[string]Operation
	byCommand     map[string]string
	auditPath     string
	auditHash     string
	auditSequence uint64
}

// NewStore loads or creates a durable file operation store at path.
func NewStore(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("file operation store path is required")
	}
	cleaned := filepath.Clean(path)
	store := &Store{
		path: cleaned, auditPath: filepath.Join(filepath.Dir(cleaned), "file-audit.jsonl"),
		operations: make(map[string]Operation), byCommand: make(map[string]string),
	}
	if err := store.load(); err != nil {
		return nil, err
	}
	if err := store.loadAudit(); err != nil {
		return nil, err
	}
	return store, nil
}

// Create persists an operation before command dispatch.
func (store *Store) Create(operation Operation) (Operation, error) {
	if operation.AgentID == "" || operation.OperatorID == "" || operation.Type == "" {
		return Operation{}, fmt.Errorf("operation identity, operator, and type are required")
	}
	if operation.OperationID == "" {
		operation.OperationID = uuid.New().String()
	}
	now := time.Now().UTC()
	operation.CreatedAt = now
	operation.UpdatedAt = now
	operation.State = StateQueued

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.operations) >= maxOperations {
		store.evictExpiredLocked(now)
	}
	if len(store.operations) >= maxOperations {
		return Operation{}, fmt.Errorf("file operation store is full")
	}
	store.operations[operation.OperationID] = operation
	if operation.CommandID != "" {
		store.byCommand[operation.CommandID] = operation.OperationID
	}
	if err := store.persistLocked(); err != nil {
		delete(store.operations, operation.OperationID)
		delete(store.byCommand, operation.CommandID)
		return Operation{}, err
	}
	if err := store.appendAuditLocked(operation, "operation_accepted", "accepted"); err != nil {
		operation.State = StateFailed
		operation.ErrorClass = "audit_unavailable"
		store.operations[operation.OperationID] = operation
		_ = store.persistLocked()
		return Operation{}, err
	}
	return operation, nil
}

// BindCommand persists the signed command ID assigned after operation creation.
func (store *Store) BindCommand(operationID, commandID string) error {
	if operationID == "" || commandID == "" {
		return fmt.Errorf("operation and command IDs are required")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	operation, ok := store.operations[operationID]
	if !ok {
		return fmt.Errorf("file operation not found")
	}
	operation.CommandID = commandID
	operation.UpdatedAt = time.Now().UTC()
	store.operations[operationID] = operation
	store.byCommand[commandID] = operationID
	return store.persistLocked()
}

// Fail records a safe gateway failure classification.
func (store *Store) Fail(operationID, class string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	operation, ok := store.operations[operationID]
	if !ok {
		return fmt.Errorf("file operation not found")
	}
	operation.State = StateFailed
	operation.ErrorClass = class
	operation.UpdatedAt = time.Now().UTC()
	store.operations[operationID] = operation
	if err := store.persistLocked(); err != nil {
		return err
	}
	return store.appendAuditLocked(operation, "operation_failed", class)
}

// Complete records a result only when the authenticated agent and command match.
func (store *Store) Complete(agentID, commandID, status string, result json.RawMessage) (Operation, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	operationID, ok := store.byCommand[commandID]
	if !ok {
		return Operation{}, fmt.Errorf("file command result is unknown")
	}
	operation := store.operations[operationID]
	if operation.AgentID != agentID {
		return Operation{}, fmt.Errorf("file command result agent mismatch")
	}
	if operation.State != StateQueued {
		return operation, nil
	}
	operation.UpdatedAt = time.Now().UTC()
	operation.Result = append(json.RawMessage(nil), result...)
	if status == "executed" {
		operation.State = StateCompleted
	} else {
		operation.State = StateFailed
		operation.ErrorClass = "agent_rejected"
	}
	store.operations[operationID] = operation
	if err := store.persistLocked(); err != nil {
		return Operation{}, err
	}
	classification := "completed"
	if operation.State == StateFailed {
		classification = operation.ErrorClass
	}
	if err := store.appendAuditLocked(operation, "operation_result", classification); err != nil {
		return Operation{}, err
	}
	return operation, nil
}

// Get returns an operation only when it belongs to the requested agent.
func (store *Store) Get(agentID, operationID string) (Operation, bool) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	operation, ok := store.operations[operationID]
	if !ok || operation.AgentID != agentID {
		return Operation{}, false
	}
	operation.Result = append(json.RawMessage(nil), operation.Result...)
	return operation, true
}

func (store *Store) load() error {
	data, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read file operation store: %w", err)
	}
	var operations []Operation
	if err := json.Unmarshal(data, &operations); err != nil {
		return fmt.Errorf("decode file operation store: %w", err)
	}
	if len(operations) > maxOperations {
		return fmt.Errorf("file operation store exceeds limit")
	}
	for _, operation := range operations {
		store.operations[operation.OperationID] = operation
		if operation.CommandID != "" {
			store.byCommand[operation.CommandID] = operation.OperationID
		}
	}
	return nil
}

func (store *Store) persistLocked() error {
	directory := filepath.Dir(store.path)
	if err := os.MkdirAll(directory, stateDirectoryPermission); err != nil {
		return fmt.Errorf("create file operation store directory: %w", err)
	}
	operations := make([]Operation, 0, len(store.operations))
	for _, operation := range store.operations {
		operations = append(operations, operation)
	}
	sort.Slice(operations, func(i, j int) bool { return operations[i].CreatedAt.Before(operations[j].CreatedAt) })
	data, err := json.Marshal(operations)
	if err != nil {
		return fmt.Errorf("encode file operation store: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".file-operations-*")
	if err != nil {
		return fmt.Errorf("create file operation snapshot: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(stateFilePermission); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set file operation snapshot permissions: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write file operation snapshot: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync file operation snapshot: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close file operation snapshot: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		return fmt.Errorf("publish file operation snapshot: %w", err)
	}
	return nil
}

func (store *Store) evictExpiredLocked(now time.Time) {
	for operationID, operation := range store.operations {
		if operation.State != StateQueued && now.After(operation.ExpiresAt) {
			delete(store.operations, operationID)
			delete(store.byCommand, operation.CommandID)
		}
	}
}

func (store *Store) loadAudit() error {
	file, err := os.Open(store.auditPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open file workspace audit: %w", err)
	}
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, auditScannerInitialBuffer), maxAuditRecordBytes)
	previousHash := ""
	var sequence uint64
	for scanner.Scan() {
		var record AuditRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return fmt.Errorf("decode file workspace audit: %w", err)
		}
		if record.Sequence != sequence+1 || record.PreviousHash != previousHash {
			return fmt.Errorf("file workspace audit chain sequence is invalid")
		}
		expectedHash, err := auditHash(record)
		if err != nil {
			return err
		}
		if record.Hash != expectedHash {
			return fmt.Errorf("file workspace audit chain hash is invalid")
		}
		sequence = record.Sequence
		previousHash = record.Hash
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read file workspace audit: %w", err)
	}
	store.auditSequence = sequence
	store.auditHash = previousHash
	return nil
}

func (store *Store) appendAuditLocked(operation Operation, eventType, classification string) error {
	record := AuditRecord{
		Sequence: store.auditSequence + 1, EventType: eventType,
		OperationID: operation.OperationID, CommandID: operation.CommandID,
		AgentID: operation.AgentID, OperatorID: operation.OperatorID, RootID: operation.RootID,
		OperationType: operation.Type, Classification: classification,
		TraceID: operation.AuditTraceID, OccurredAt: time.Now().UTC(), PreviousHash: store.auditHash,
	}
	hash, err := auditHash(record)
	if err != nil {
		return err
	}
	record.Hash = hash
	encoded, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode file workspace audit: %w", err)
	}
	if len(encoded) > maxAuditRecordBytes {
		return fmt.Errorf("file workspace audit record exceeds limit")
	}
	file, err := os.OpenFile(store.auditPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, stateFilePermission)
	if err != nil {
		return fmt.Errorf("open file workspace audit: %w", err)
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		_ = file.Close()
		return fmt.Errorf("append file workspace audit: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync file workspace audit: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close file workspace audit: %w", err)
	}
	store.auditSequence = record.Sequence
	store.auditHash = record.Hash
	return nil
}

func auditHash(record AuditRecord) (string, error) {
	record.Hash = ""
	encoded, err := json.Marshal(record)
	if err != nil {
		return "", fmt.Errorf("encode file workspace audit hash input: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return fmt.Sprintf("sha256:%x", digest), nil
}
