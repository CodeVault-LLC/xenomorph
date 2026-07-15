package command

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

	"github.com/google/uuid"

	"github.com/codevault-llc/xenomorph/platform/shared/atomicfile"
)

const (
	journalVersion        = 1
	maximumJournalRecords = 4096
	journalDirectoryMode  = 0o700
	journalFileMode       = 0o600
	terminalRetention     = 24 * time.Hour
)

// JournalState is the gateway-authored durable command lifecycle state.
type JournalState string

const (
	// JournalQueued means a signed command is durable and eligible for dispatch.
	JournalQueued JournalState = "queued"
	// JournalDispatched means delivery began and automatic execution retry is unsafe.
	JournalDispatched JournalState = "dispatched"
	// JournalAccepted means the client durably reserved replay state before execution.
	JournalAccepted JournalState = "accepted"
	// JournalTerminal means one authenticated terminal result was committed.
	JournalTerminal JournalState = "terminal"
	// JournalOutcomeUnknown requires explicit reconciliation after interrupted delivery.
	JournalOutcomeUnknown JournalState = "outcome_unknown"
)

// ResultDisposition classifies a terminal result against durable command state.
type ResultDisposition uint8

const (
	// ResultCommitted means the result created the terminal journal transition.
	ResultCommitted ResultDisposition = iota + 1
	// ResultDuplicate means an identical terminal result was already committed.
	ResultDuplicate
)

// Journal is a bounded filesystem-backed command and operation journal.
type Journal struct {
	mu      sync.Mutex
	path    string
	records map[string]journalRecord
	now     func() time.Time
}

type journalRecord struct {
	AgentID        string       `json:"agent_id"`
	Envelope       Envelope     `json:"envelope"`
	State          JournalState `json:"state"`
	ResultDigest   string       `json:"result_digest,omitempty"`
	UpdatedAt      time.Time    `json:"updated_at"`
	RetentionUntil time.Time    `json:"retention_until"`
}

type journalDocument struct {
	Version int             `json:"version"`
	Records []journalRecord `json:"records"`
}

// NewJournal opens, validates, and recovers a durable command journal.
func NewJournal(path string) (*Journal, error) {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if strings.TrimSpace(path) == "" || cleaned == "." {
		return nil, fmt.Errorf("open command journal: path is required")
	}

	journal := &Journal{
		path:    cleaned,
		records: make(map[string]journalRecord),
		now:     func() time.Time { return time.Now().UTC() },
	}
	if err := journal.load(); err != nil {
		return nil, err
	}

	return journal, nil
}

// Queued returns detached signed commands eligible for initial dispatch.
func (journal *Journal) Queued() map[string][]*Envelope {
	journal.mu.Lock()
	defer journal.mu.Unlock()

	queued := make(map[string][]*Envelope)

	for _, record := range journal.records {
		if record.State != JournalQueued {
			continue
		}

		envelope := cloneEnvelope(record.Envelope)
		queued[record.AgentID] = append(queued[record.AgentID], &envelope)
	}

	for agentID := range queued {
		sort.Slice(queued[agentID], func(first, second int) bool {
			return queued[agentID][first].IssuedAt.Before(queued[agentID][second].IssuedAt)
		})
	}

	return queued
}

// RecordQueued persists a signed command before it becomes dispatch-visible.
func (journal *Journal) RecordQueued(agentID string, envelope Envelope) error {
	if journal == nil || strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("record queued command: journal and agent ID are required")
	}

	if err := validateJournalEnvelope(agentID, envelope); err != nil {
		return err
	}

	journal.mu.Lock()
	defer journal.mu.Unlock()
	journal.pruneLocked()

	if len(journal.records) >= maximumJournalRecords {
		return fmt.Errorf("record queued command: journal capacity reached")
	}

	if _, exists := journal.records[envelope.CommandID]; exists {
		return fmt.Errorf("record queued command: command ID already exists")
	}

	now := journal.now()
	record := journalRecord{
		AgentID: agentID, Envelope: cloneEnvelope(envelope), State: JournalQueued,
		UpdatedAt: now, RetentionUntil: envelope.ExpiresAt.Add(terminalRetention),
	}

	journal.records[envelope.CommandID] = record
	if err := journal.persistLocked(); err != nil {
		delete(journal.records, envelope.CommandID)
		return err
	}

	return nil
}

// MarkDispatched persists the ambiguity boundary before command bytes are written.
func (journal *Journal) MarkDispatched(agentID, commandID string) error {
	return journal.transition(agentID, commandID, JournalQueued, JournalDispatched)
}

// MarkAccepted persists the client acknowledgement after its replay ledger commit.
func (journal *Journal) MarkAccepted(agentID, commandID string) error {
	if journal == nil {
		return fmt.Errorf("mark command accepted: journal is nil")
	}

	journal.mu.Lock()
	defer journal.mu.Unlock()

	record, exists := journal.records[commandID]
	if !exists || record.AgentID != agentID {
		return fmt.Errorf("mark command accepted: command audience mismatch")
	}

	if record.State == JournalAccepted {
		return nil
	}

	if record.State != JournalDispatched {
		return fmt.Errorf("mark command accepted: invalid command state")
	}

	previous := record
	record.State = JournalAccepted
	record.UpdatedAt = journal.now()

	journal.records[commandID] = record
	if err := journal.persistLocked(); err != nil {
		journal.records[commandID] = previous
		return err
	}

	return nil
}

// MarkOutcomeUnknown prevents automatic replay after an ambiguous delivery failure.
func (journal *Journal) MarkOutcomeUnknown(agentID, commandID string) error {
	if journal == nil {
		return fmt.Errorf("mark command outcome unknown: journal is nil")
	}

	journal.mu.Lock()
	defer journal.mu.Unlock()

	record, exists := journal.records[commandID]
	if !exists || record.AgentID != agentID ||
		(record.State != JournalDispatched && record.State != JournalAccepted) {
		return fmt.Errorf("mark command outcome unknown: invalid command state")
	}

	previous := record
	record.State = JournalOutcomeUnknown
	record.UpdatedAt = journal.now()

	journal.records[commandID] = record
	if err := journal.persistLocked(); err != nil {
		journal.records[commandID] = previous
		return err
	}

	return nil
}

// CommitResult validates audience and terminal state, then persists a digest.
func (journal *Journal) CommitResult(agentID, commandID string, canonicalResult []byte) (ResultDisposition, error) {
	if journal == nil || strings.TrimSpace(agentID) == "" || strings.TrimSpace(commandID) == "" {
		return 0, fmt.Errorf("commit command result: journal, agent ID, and command ID are required")
	}

	digest := sha256.Sum256(canonicalResult)
	digestText := hex.EncodeToString(digest[:])

	journal.mu.Lock()
	defer journal.mu.Unlock()

	record, exists := journal.records[commandID]
	if !exists || record.AgentID != agentID || record.Envelope.AudienceAgentID != agentID {
		return 0, fmt.Errorf("commit command result: command audience mismatch")
	}

	return journal.commitResultLocked(commandID, record, digestText)
}

func (journal *Journal) commitResultLocked(commandID string, record journalRecord, digestText string) (ResultDisposition, error) {
	switch record.State {
	case JournalTerminal:
		if record.ResultDigest == digestText {
			return ResultDuplicate, nil
		}

		return 0, fmt.Errorf("commit command result: conflicting terminal result")
	case JournalDispatched, JournalAccepted:
		previous := record
		record.State = JournalTerminal
		record.ResultDigest = digestText
		record.UpdatedAt = journal.now()

		journal.records[commandID] = record
		if err := journal.persistLocked(); err != nil {
			journal.records[commandID] = previous
			return 0, err
		}

		return ResultCommitted, nil
	case JournalOutcomeUnknown:
		return 0, fmt.Errorf("commit command result: operation outcome requires reconciliation")
	default:
		return 0, fmt.Errorf("commit command result: command was not dispatched")
	}
}

// State returns a gateway-authored journal state for tests and reconciliation.
func (journal *Journal) State(agentID, commandID string) (JournalState, bool) {
	if journal == nil {
		return "", false
	}

	journal.mu.Lock()
	defer journal.mu.Unlock()

	record, exists := journal.records[commandID]
	if !exists || record.AgentID != agentID {
		return "", false
	}

	return record.State, true
}

// Envelope returns a detached gateway-authored command contract for validation.
func (journal *Journal) Envelope(agentID, commandID string) (Envelope, bool) {
	if journal == nil {
		return Envelope{}, false
	}

	journal.mu.Lock()
	defer journal.mu.Unlock()

	record, exists := journal.records[commandID]
	if !exists || record.AgentID != agentID {
		return Envelope{}, false
	}

	return cloneEnvelope(record.Envelope), true
}

func (journal *Journal) transition(agentID, commandID string, from, to JournalState) error {
	if journal == nil {
		return fmt.Errorf("transition command journal: journal is nil")
	}

	journal.mu.Lock()
	defer journal.mu.Unlock()

	record, exists := journal.records[commandID]
	if !exists || record.AgentID != agentID || record.State != from {
		return fmt.Errorf("transition command journal: invalid command state")
	}

	previous := record
	record.State = to
	record.UpdatedAt = journal.now()

	journal.records[commandID] = record
	if err := journal.persistLocked(); err != nil {
		journal.records[commandID] = previous
		return err
	}

	return nil
}

func (journal *Journal) load() error {
	document, exists, err := readJournalDocument(journal.path)
	if err != nil || !exists {
		return err
	}

	recovered := false

	for _, record := range document.Records {
		recordRecovered, err := journal.loadRecord(record)
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

func readJournalDocument(path string) (journalDocument, bool, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is validated gateway configuration.
	if errors.Is(err, os.ErrNotExist) {
		return journalDocument{}, false, nil
	}

	if err != nil {
		return journalDocument{}, false, fmt.Errorf("read command journal: %w", err)
	}

	var document journalDocument

	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&document); err != nil {
		return journalDocument{}, false, fmt.Errorf("decode command journal: %w", err)
	}

	if document.Version != journalVersion || len(document.Records) > maximumJournalRecords {
		return journalDocument{}, false, fmt.Errorf("decode command journal: unsupported version or record count")
	}

	return document, true, nil
}

func (journal *Journal) loadRecord(record journalRecord) (bool, error) {
	if err := validateJournalRecord(record); err != nil {
		return false, err
	}

	if _, duplicate := journal.records[record.Envelope.CommandID]; duplicate {
		return false, fmt.Errorf("decode command journal: duplicate command ID")
	}

	recovered := record.State == JournalDispatched || record.State == JournalAccepted
	if recovered {
		record.State = JournalOutcomeUnknown
		record.UpdatedAt = journal.now()
	}

	journal.records[record.Envelope.CommandID] = record

	return recovered, nil
}

func (journal *Journal) pruneLocked() {
	now := journal.now()
	for commandID, record := range journal.records {
		if (record.State == JournalTerminal || record.State == JournalOutcomeUnknown) && now.After(record.RetentionUntil) {
			delete(journal.records, commandID)
		}
	}
}

func (journal *Journal) persistLocked() error {
	document := journalDocument{Version: journalVersion, Records: make([]journalRecord, 0, len(journal.records))}
	for _, record := range journal.records {
		document.Records = append(document.Records, record)
	}

	sort.Slice(document.Records, func(first, second int) bool {
		return document.Records[first].Envelope.CommandID < document.Records[second].Envelope.CommandID
	})

	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode command journal: %w", err)
	}

	return writeJournalAtomically(journal.path, append(data, '\n'))
}

func writeJournalAtomically(path string, data []byte) error {
	if err := atomicfile.Replace(path, data, journalDirectoryMode, journalFileMode); err != nil {
		return fmt.Errorf("replace command journal: %w", err)
	}

	return nil
}

func validateJournalRecord(record journalRecord) error {
	if err := validateJournalEnvelope(record.AgentID, record.Envelope); err != nil {
		return fmt.Errorf("decode command journal: %w", err)
	}

	if record.UpdatedAt.IsZero() || record.RetentionUntil.IsZero() || !validJournalState(record.State) {
		return fmt.Errorf("decode command journal: invalid state metadata")
	}

	if record.State == JournalTerminal && len(record.ResultDigest) != sha256.Size*2 {
		return fmt.Errorf("decode command journal: terminal result digest missing")
	}

	return nil
}

func validateJournalEnvelope(agentID string, envelope Envelope) error {
	if _, err := uuid.Parse(envelope.CommandID); err != nil {
		return fmt.Errorf("validate journal command: invalid command ID")
	}

	if envelope.AudienceAgentID != agentID || envelope.Signature == "" || envelope.Nonce == "" || envelope.KeyID == "" {
		return fmt.Errorf("validate journal command: incomplete audience or authenticity fields")
	}

	if envelope.IssuedAt.IsZero() || !envelope.ExpiresAt.After(envelope.IssuedAt) {
		return fmt.Errorf("validate journal command: invalid validity window")
	}

	return nil
}

func validJournalState(state JournalState) bool {
	return state == JournalQueued || state == JournalDispatched || state == JournalAccepted ||
		state == JournalTerminal || state == JournalOutcomeUnknown
}

func cloneEnvelope(envelope Envelope) Envelope {
	clone := envelope
	clone.Payload = append(json.RawMessage(nil), envelope.Payload...)

	return clone
}
