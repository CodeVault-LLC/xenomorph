// Package operation owns durable idempotency state for non-command XBP operations.
package operation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/atomicfile"
)

const (
	documentVersion      = 1
	maximumRecords       = 4096
	retention            = 24 * time.Hour
	journalDirectoryMode = 0o700
	journalFileMode      = 0o600
)

type recordState string

const (
	statePending        recordState = "pending"
	stateTerminal       recordState = "terminal"
	stateOutcomeUnknown recordState = "outcome_unknown"
)

// Disposition describes whether an operation may execute or is a duplicate.
type Disposition uint8

const (
	// Execute means a new durable pending record was created.
	Execute Disposition = iota + 1
	// Duplicate means an identical terminal operation already committed.
	Duplicate
)

// Journal is a bounded durable idempotency state machine.
type Journal struct {
	mu      sync.Mutex
	path    string
	records map[string]record
	now     func() time.Time
}

type record struct {
	AgentID       string      `json:"agent_id"`
	MessageType   uint16      `json:"message_type"`
	OperationID   string      `json:"operation_id"`
	PayloadDigest string      `json:"payload_digest"`
	State         recordState `json:"state"`
	UpdatedAt     time.Time   `json:"updated_at"`
	RetainUntil   time.Time   `json:"retain_until"`
}

type document struct {
	Version int      `json:"version"`
	Records []record `json:"records"`
}

// Open verifies and recovers a durable operation journal.
func Open(path string) (*Journal, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("open operation journal: path is required")
	}
	journal := &Journal{
		path: filepath.Clean(path), records: make(map[string]record),
		now: func() time.Time { return time.Now().UTC() },
	}
	if err := journal.load(); err != nil {
		return nil, err
	}
	return journal, nil
}

// Begin durably reserves an authenticated operation before its side effect.
func (journal *Journal) Begin(agentID string, messageType uint16, operationID [16]byte, payload []byte) (Disposition, error) {
	if journal == nil || strings.TrimSpace(agentID) == "" || messageType == 0 || operationID == [16]byte{} {
		return 0, fmt.Errorf("begin operation: invalid scope")
	}
	digest := sha256.Sum256(payload)
	digestText := hex.EncodeToString(digest[:])
	key := operationKey(agentID, messageType, operationID)
	journal.mu.Lock()
	defer journal.mu.Unlock()
	journal.pruneLocked()
	if existing, found := journal.records[key]; found {
		if existing.PayloadDigest != digestText {
			return 0, fmt.Errorf("begin operation: operation payload conflict")
		}
		if existing.State == stateTerminal {
			return Duplicate, nil
		}
		return 0, fmt.Errorf("begin operation: operation outcome requires reconciliation")
	}
	if len(journal.records) >= maximumRecords {
		return 0, fmt.Errorf("begin operation: journal capacity reached")
	}
	now := journal.now()
	journal.records[key] = record{
		AgentID: agentID, MessageType: messageType,
		OperationID: hex.EncodeToString(operationID[:]), PayloadDigest: digestText,
		State: statePending, UpdatedAt: now, RetainUntil: now.Add(retention),
	}
	if err := journal.persistLocked(); err != nil {
		delete(journal.records, key)
		return 0, err
	}
	return Execute, nil
}

// Commit records the terminal application commit point.
func (journal *Journal) Commit(agentID string, messageType uint16, operationID [16]byte) error {
	if journal == nil {
		return fmt.Errorf("commit operation: journal is nil")
	}
	key := operationKey(agentID, messageType, operationID)
	journal.mu.Lock()
	defer journal.mu.Unlock()
	current, found := journal.records[key]
	if !found || (current.State != statePending && current.State != stateTerminal) {
		return fmt.Errorf("commit operation: invalid operation state")
	}
	if current.State == stateTerminal {
		return nil
	}
	previous := current
	current.State = stateTerminal
	current.UpdatedAt = journal.now()
	journal.records[key] = current
	if err := journal.persistLocked(); err != nil {
		journal.records[key] = previous
		return err
	}
	return nil
}

// Release removes work proven not to have reached its side-effect boundary.
func (journal *Journal) Release(agentID string, messageType uint16, operationID [16]byte) error {
	if journal == nil {
		return fmt.Errorf("release operation: journal is nil")
	}
	key := operationKey(agentID, messageType, operationID)
	journal.mu.Lock()
	defer journal.mu.Unlock()
	current, found := journal.records[key]
	if !found {
		return nil
	}
	if current.State != statePending {
		return fmt.Errorf("release operation: invalid operation state")
	}
	delete(journal.records, key)
	if err := journal.persistLocked(); err != nil {
		journal.records[key] = current
		return err
	}
	return nil
}

func (journal *Journal) load() error {
	stored, exists, err := readDocument(journal.path)
	if err != nil || !exists {
		return err
	}
	recovered := false
	for _, current := range stored.Records {
		recordRecovered, err := journal.loadRecord(current)
		if err != nil {
			return err
		}
		recovered = recovered || recordRecovered
	}
	journal.pruneLocked()
	if recovered {
		return journal.persistLocked()
	}
	return nil
}

func readDocument(path string) (document, bool, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is validated gateway configuration.
	if errors.Is(err, os.ErrNotExist) {
		return document{}, false, nil
	}
	if err != nil {
		return document{}, false, fmt.Errorf("read operation journal: %w", err)
	}
	var stored document
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&stored); err != nil {
		return document{}, false, fmt.Errorf("decode operation journal: %w", err)
	}
	if stored.Version != documentVersion || len(stored.Records) > maximumRecords {
		return document{}, false, fmt.Errorf("decode operation journal: unsupported version or record count")
	}
	return stored, true, nil
}

func (journal *Journal) loadRecord(current record) (bool, error) {
	operationID, err := decodeOperationID(current.OperationID)
	if err != nil || !validRecord(current) {
		return false, fmt.Errorf("decode operation journal: invalid record")
	}
	key := operationKey(current.AgentID, current.MessageType, operationID)
	if _, duplicate := journal.records[key]; duplicate {
		return false, fmt.Errorf("decode operation journal: duplicate record")
	}
	recovered := current.State == statePending
	if recovered {
		current.State = stateOutcomeUnknown
		current.UpdatedAt = journal.now()
	}
	journal.records[key] = current
	return recovered, nil
}

func (journal *Journal) persistLocked() error {
	stored := document{Version: documentVersion, Records: make([]record, 0, len(journal.records))}
	for _, current := range journal.records {
		stored.Records = append(stored.Records, current)
	}
	sort.Slice(stored.Records, func(first, second int) bool {
		left, right := stored.Records[first], stored.Records[second]
		if left.AgentID != right.AgentID {
			return left.AgentID < right.AgentID
		}
		if left.MessageType != right.MessageType {
			return left.MessageType < right.MessageType
		}
		return left.OperationID < right.OperationID
	})
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return fmt.Errorf("encode operation journal: %w", err)
	}
	return writeAtomically(journal.path, append(data, '\n'))
}

func (journal *Journal) pruneLocked() {
	now := journal.now()
	for key, current := range journal.records {
		if current.State != statePending && now.After(current.RetainUntil) {
			delete(journal.records, key)
		}
	}
}

func writeAtomically(path string, data []byte) error {
	if err := atomicfile.Replace(path, data, journalDirectoryMode, journalFileMode); err != nil {
		return fmt.Errorf("replace operation journal: %w", err)
	}
	return nil
}

func operationKey(agentID string, messageType uint16, operationID [16]byte) string {
	return fmt.Sprintf("%s\x00%d\x00%x", agentID, messageType, operationID)
}

func decodeOperationID(value string) ([16]byte, error) {
	var operationID [16]byte
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != len(operationID) {
		return operationID, fmt.Errorf("invalid operation ID")
	}
	copy(operationID[:], decoded)
	return operationID, nil
}

func validRecord(current record) bool {
	if strings.TrimSpace(current.AgentID) == "" || current.MessageType == 0 || len(current.PayloadDigest) != sha256.Size*2 ||
		current.UpdatedAt.IsZero() || current.RetainUntil.IsZero() {
		return false
	}
	return current.State == statePending || current.State == stateTerminal || current.State == stateOutcomeUnknown
}
