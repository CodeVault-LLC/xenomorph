package replay

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
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

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
	"github.com/codevault-llc/xenomorph/platform/shared/atomicfile"
)

const (
	ledgerVersion       = 1
	maximumEntries      = 4096
	authenticationBytes = 32
	incidentRetention   = 24 * time.Hour
	directoryMode       = 0o700
	stateFileMode       = 0o600
)

type state string

const (
	stateAccepted       state = "accepted"
	stateTerminal       state = "terminal"
	stateOutcomeUnknown state = "outcome_unknown"
)

// Ledger is an authenticated, filesystem-backed replay security ledger.
type Ledger struct {
	mu      sync.Mutex
	path    string
	key     [authenticationBytes]byte
	entries map[string]entry
	now     func() time.Time
}

type entry struct {
	CommandID      string    `json:"command_id"`
	NonceDigest    string    `json:"nonce_digest"`
	KeyID          string    `json:"key_id"`
	Audience       string    `json:"audience"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	State          state     `json:"state"`
	TerminalDigest string    `json:"terminal_digest,omitempty"`
	RetentionUntil time.Time `json:"retention_until"`
}

type payload struct {
	Version int     `json:"version"`
	Entries []entry `json:"entries"`
}

type authenticatedDocument struct {
	Payload []byte `json:"payload"`
	MAC     string `json:"mac"`
}

// Open loads or creates the local authentication key and verifies the ledger.
func Open(path, keyPath string) (*Ledger, error) {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(keyPath) == "" {
		return nil, fmt.Errorf("open replay ledger: state and key paths are required")
	}
	key, err := loadOrCreateAuthenticationKey(filepath.Clean(keyPath))
	if err != nil {
		return nil, err
	}
	ledger := &Ledger{
		path: filepath.Clean(path), key: key, entries: make(map[string]entry),
		now: func() time.Time { return time.Now().UTC() },
	}
	if err := ledger.load(); err != nil {
		return nil, err
	}
	return ledger, nil
}

// Reserve authenticates and persists minimal security state before execution.
func (ledger *Ledger) Reserve(candidate agent.CommandReplayEntry) error {
	if ledger == nil {
		return fmt.Errorf("reserve replay state: ledger is nil")
	}
	if err := validateCandidate(candidate); err != nil {
		return err
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	ledger.pruneLocked()
	nonceDigest := hex.EncodeToString(candidate.NonceDigest[:])
	if existing, found := ledger.entries[candidate.CommandID]; found {
		return replayStateError(existing.State)
	}
	for _, existing := range ledger.entries {
		if hmac.Equal([]byte(existing.NonceDigest), []byte(nonceDigest)) {
			return agent.ErrCommandReplay
		}
	}
	if len(ledger.entries) >= maximumEntries {
		return fmt.Errorf("reserve replay state: ledger capacity reached")
	}
	current := entry{
		CommandID: candidate.CommandID, NonceDigest: nonceDigest, KeyID: candidate.KeyID,
		Audience: candidate.Audience, IssuedAt: candidate.IssuedAt, ExpiresAt: candidate.ExpiresAt,
		State: stateAccepted, RetentionUntil: candidate.ExpiresAt.Add(incidentRetention),
	}
	ledger.entries[candidate.CommandID] = current
	if err := ledger.persistLocked(); err != nil {
		delete(ledger.entries, candidate.CommandID)
		return err
	}
	return nil
}

// Complete persists the terminal result digest after local execution.
func (ledger *Ledger) Complete(commandID string, terminalDigest [sha256.Size]byte) error {
	if ledger == nil || strings.TrimSpace(commandID) == "" {
		return fmt.Errorf("complete replay state: ledger and command ID are required")
	}
	digestText := hex.EncodeToString(terminalDigest[:])
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	current, found := ledger.entries[commandID]
	if !found {
		return fmt.Errorf("complete replay state: command was not reserved")
	}
	switch current.State {
	case stateTerminal:
		if hmac.Equal([]byte(current.TerminalDigest), []byte(digestText)) {
			return nil
		}
		return agent.ErrCommandReplay
	case stateOutcomeUnknown:
		return agent.ErrCommandOutcomeUnknown
	case stateAccepted:
		previous := current
		current.State = stateTerminal
		current.TerminalDigest = digestText
		ledger.entries[commandID] = current
		if err := ledger.persistLocked(); err != nil {
			ledger.entries[commandID] = previous
			return err
		}
		return nil
	default:
		return fmt.Errorf("complete replay state: invalid state")
	}
}

func (ledger *Ledger) load() error {
	data, err := os.ReadFile(ledger.path) // #nosec G304 -- path is validated agent configuration.
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read replay ledger: %w", err)
	}
	if err := verifyStateFileMode(ledger.path); err != nil {
		return err
	}
	stored, err := decodeAuthenticatedState(data, ledger.key)
	if err != nil {
		return err
	}
	return ledger.restore(stored)
}

func decodeAuthenticatedState(data []byte, key [authenticationBytes]byte) (payload, error) {
	var document authenticatedDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return payload{}, fmt.Errorf("decode replay ledger envelope: %w", err)
	}
	expectedMAC := hmac.New(sha256.New, key[:])
	if _, err := expectedMAC.Write(document.Payload); err != nil {
		return payload{}, fmt.Errorf("authenticate replay ledger: %w", err)
	}
	providedMAC, err := hex.DecodeString(document.MAC)
	if err != nil || !hmac.Equal(providedMAC, expectedMAC.Sum(nil)) {
		return payload{}, fmt.Errorf("authenticate replay ledger: MAC verification failed")
	}
	var stored payload
	payloadDecoder := json.NewDecoder(bytes.NewReader(document.Payload))
	payloadDecoder.DisallowUnknownFields()
	if err := payloadDecoder.Decode(&stored); err != nil {
		return payload{}, fmt.Errorf("decode replay ledger payload: %w", err)
	}
	if stored.Version != ledgerVersion || len(stored.Entries) > maximumEntries {
		return payload{}, fmt.Errorf("decode replay ledger: unsupported version or entry count")
	}
	return stored, nil
}

func (ledger *Ledger) restore(stored payload) error {
	recovered := false
	nonces := make(map[string]struct{}, len(stored.Entries))
	for _, current := range stored.Entries {
		if err := ledger.validateRecoveredEntry(current, nonces); err != nil {
			return err
		}
		if current.State == stateAccepted {
			current.State = stateOutcomeUnknown
			recovered = true
		}
		ledger.entries[current.CommandID] = current
		nonces[current.NonceDigest] = struct{}{}
	}
	ledger.pruneLocked()
	if recovered {
		return ledger.persistLocked()
	}
	return nil
}

func (ledger *Ledger) validateRecoveredEntry(current entry, nonces map[string]struct{}) error {
	if err := validateStoredEntry(current); err != nil {
		return err
	}
	if _, duplicate := ledger.entries[current.CommandID]; duplicate {
		return fmt.Errorf("decode replay ledger: duplicate command ID")
	}
	if _, duplicate := nonces[current.NonceDigest]; duplicate {
		return fmt.Errorf("decode replay ledger: duplicate nonce digest")
	}
	return nil
}

func (ledger *Ledger) persistLocked() error {
	entries := make([]entry, 0, len(ledger.entries))
	for _, current := range ledger.entries {
		entries = append(entries, current)
	}
	sort.Slice(entries, func(first, second int) bool { return entries[first].CommandID < entries[second].CommandID })
	payloadData, err := json.Marshal(payload{Version: ledgerVersion, Entries: entries})
	if err != nil {
		return fmt.Errorf("encode replay ledger payload: %w", err)
	}
	mac := hmac.New(sha256.New, ledger.key[:])
	if _, err := mac.Write(payloadData); err != nil {
		return fmt.Errorf("authenticate replay ledger payload: %w", err)
	}
	document := authenticatedDocument{Payload: payloadData, MAC: hex.EncodeToString(mac.Sum(nil))}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return fmt.Errorf("encode replay ledger envelope: %w", err)
	}
	return writeStateAtomically(ledger.path, append(data, '\n'))
}

func (ledger *Ledger) pruneLocked() {
	now := ledger.now()
	for commandID, current := range ledger.entries {
		if now.After(current.RetentionUntil) {
			delete(ledger.entries, commandID)
		}
	}
}

func loadOrCreateAuthenticationKey(path string) ([authenticationBytes]byte, error) {
	var key [authenticationBytes]byte
	data, err := os.ReadFile(path) // #nosec G304 -- path is validated agent configuration.
	if err == nil {
		if err := verifyStateFileMode(path); err != nil {
			return key, err
		}
		decoded, err := hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil || len(decoded) != authenticationBytes {
			return key, fmt.Errorf("load replay authentication key: invalid hexadecimal key")
		}
		copy(key[:], decoded)
		if key == [authenticationBytes]byte{} {
			return key, fmt.Errorf("load replay authentication key: zero key")
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return key, fmt.Errorf("read replay authentication key: %w", err)
	}
	if _, err := rand.Read(key[:]); err != nil {
		return key, fmt.Errorf("generate replay authentication key: %w", err)
	}
	if err := writeNewKey(path, []byte(hex.EncodeToString(key[:])+"\n")); err != nil {
		return [authenticationBytes]byte{}, err
	}
	return key, nil
}

func writeNewKey(path string, data []byte) error {
	if err := atomicfile.Create(path, data, directoryMode, stateFileMode); err != nil {
		return fmt.Errorf("create replay authentication key: %w", err)
	}
	return nil
}

func writeStateAtomically(path string, data []byte) error {
	if err := atomicfile.Replace(path, data, directoryMode, stateFileMode); err != nil {
		return fmt.Errorf("replace replay ledger: %w", err)
	}
	return nil
}

func verifyStateFileMode(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect replay state permissions: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("inspect replay state permissions: group and other access is prohibited")
	}
	return nil
}

func validateCandidate(candidate agent.CommandReplayEntry) error {
	if _, err := uuid.Parse(candidate.CommandID); err != nil || candidate.NonceDigest == [sha256.Size]byte{} {
		return fmt.Errorf("reserve replay state: invalid command ID or nonce digest")
	}
	if strings.TrimSpace(candidate.KeyID) == "" || strings.TrimSpace(candidate.Audience) == "" ||
		candidate.IssuedAt.IsZero() || !candidate.ExpiresAt.After(candidate.IssuedAt) {
		return fmt.Errorf("reserve replay state: incomplete authenticity or validity binding")
	}
	return nil
}

func validateStoredEntry(current entry) error {
	if err := validateStoredIdentity(current); err != nil {
		return err
	}
	if err := validateStoredBinding(current); err != nil {
		return err
	}
	if current.State != stateAccepted && current.State != stateTerminal && current.State != stateOutcomeUnknown {
		return fmt.Errorf("decode replay ledger: invalid state")
	}
	if current.State == stateTerminal && len(current.TerminalDigest) != sha256.Size*2 {
		return fmt.Errorf("decode replay ledger: terminal digest missing")
	}
	return nil
}

func validateStoredIdentity(current entry) error {
	if _, err := uuid.Parse(current.CommandID); err != nil || len(current.NonceDigest) != sha256.Size*2 {
		return fmt.Errorf("decode replay ledger: invalid command or nonce digest")
	}
	if _, err := hex.DecodeString(current.NonceDigest); err != nil {
		return fmt.Errorf("decode replay ledger: malformed nonce digest")
	}
	return nil
}

func validateStoredBinding(current entry) error {
	if strings.TrimSpace(current.KeyID) == "" || strings.TrimSpace(current.Audience) == "" ||
		current.IssuedAt.IsZero() || !current.ExpiresAt.After(current.IssuedAt) || current.RetentionUntil.IsZero() {
		return fmt.Errorf("decode replay ledger: incomplete entry binding")
	}
	return nil
}

func replayStateError(current state) error {
	if current == stateOutcomeUnknown {
		return agent.ErrCommandOutcomeUnknown
	}
	return agent.ErrCommandReplay
}
