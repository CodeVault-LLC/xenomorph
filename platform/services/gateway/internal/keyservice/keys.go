package keyservice

import (
	"context"
	"crypto"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/mlkem"
	"crypto/rsa"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sync"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
)

const maxDerivedKeyBytes = 64

const absoluteDEKInvocationLimit uint64 = 1 << 32

const maxAuthenticatedFieldBytes = 256

// DEKPolicy contains the risk-derived operating limits enforced for one DEK.
// InvocationLimit must be lower than or equal to the SP 800-38D fail-safe
// ceiling; deployments should configure a substantially lower value.
type DEKPolicy struct {
	InvocationLimit  uint64
	ByteLimit        uint64
	RotationInterval time.Duration
}

// DEKScope prevents a key from crossing purpose, tenant or security-domain,
// and traffic-direction boundaries.
type DEKScope struct {
	Purpose        string
	SecurityDomain string
	Direction      Direction
}

// NonceAllocator durably allocates one unique 96-bit nonce for a DEK. An
// implementation must use a strongly consistent prefix and counter store and
// must never return an uncertain or previously allocated value.
type NonceAllocator interface {
	NextNonce(ctx context.Context, key Metadata) ([]byte, error)
}

// ProtectedData contains one AES-GCM nonce and its ciphertext with appended
// authentication tag. It contains no private key material.
type ProtectedData struct {
	Nonce      []byte
	Ciphertext []byte
}

// Direction is a closed cryptographic traffic direction. Each direction and
// security domain requires an independent DEK.
type Direction string

const (
	// DirectionGatewayToAgent protects gateway-authored agent traffic.
	DirectionGatewayToAgent Direction = "gateway-to-agent"
	// DirectionAgentToGateway protects authenticated client-authored traffic.
	DirectionAgentToGateway Direction = "agent-to-gateway"
	// DirectionGatewayToBroker protects gateway-authored broker traffic.
	DirectionGatewayToBroker Direction = "gateway-to-broker"
	// DirectionBrokerToGateway protects authenticated broker traffic.
	DirectionBrokerToGateway Direction = "broker-to-gateway"
)

// AuthenticatedContext is the canonical, server-authored context bound to
// every protected application message. Sender and recipient identities come
// from authenticated gateway state, never from client claims.
type AuthenticatedContext struct {
	ProtocolVersion uint32    `json:"protocol_version"`
	GatewayID       string    `json:"gateway_id"`
	SenderID        string    `json:"sender_id"`
	RecipientID     string    `json:"recipient_id"`
	SecurityDomain  string    `json:"security_domain"`
	SessionID       string    `json:"session_id"`
	Sequence        uint64    `json:"sequence"`
	EventID         string    `json:"event_id"`
	IssuedAt        time.Time `json:"issued_at"`
	ExpiresAt       time.Time `json:"expires_at"`
	Direction       Direction `json:"direction"`
	Operation       string    `json:"operation"`
	KeyID           string    `json:"key_id"`
	Algorithm       string    `json:"algorithm"`
}

// DEK is an opaque AES-256-GCM key handle. Nonces must be exactly 96 bits and
// supplied by a durable allocator that guarantees uniqueness for this key.
type DEK struct {
	mu              sync.RWMutex
	metadata        Metadata
	aead            cipher.AEAD
	allocator       NonceAllocator
	invocations     uint64
	bytesSealed     uint64
	invocationLimit uint64
	byteLimit       uint64
}

// Metadata returns the non-secret server-authored key metadata.
func (key *DEK) Metadata() Metadata { key.mu.RLock(); defer key.mu.RUnlock(); return key.metadata }

// Seal allocates a unique nonce and encrypts plaintext with authenticated
// protocol context. Allocation uncertainty permanently retires this handle.
func (key *DEK) Seal(ctx context.Context, plaintext []byte, authenticated AuthenticatedContext) (ProtectedData, error) {
	key.mu.Lock()
	defer key.mu.Unlock()
	if key.metadata.State != StateActive || key.aead == nil {
		return ProtectedData{}, fmt.Errorf("DEK is not active")
	}
	if ctx == nil || ctx.Err() != nil {
		return ProtectedData{}, fmt.Errorf("DEK operation context is invalid")
	}
	additionalData, err := key.authenticatedData(authenticated)
	if err != nil {
		return ProtectedData{}, err
	}
	plaintextBytes := uint64(len(plaintext))
	if key.operatingLimitReached(plaintextBytes) {
		key.metadata.State = StateRetired
		return ProtectedData{}, fmt.Errorf("DEK operating limit reached")
	}
	nonce, err := key.allocator.NextNonce(ctx, key.metadata)
	if err != nil || len(nonce) != key.aead.NonceSize() {
		key.metadata.State = StateRetired
		return ProtectedData{}, fmt.Errorf("allocate DEK nonce")
	}
	ciphertext := key.aead.Seal(nil, nonce, plaintext, additionalData)
	key.invocations++
	key.bytesSealed += plaintextBytes
	return ProtectedData{Nonce: append([]byte(nil), nonce...), Ciphertext: ciphertext}, nil
}

func (key *DEK) operatingLimitReached(plaintextBytes uint64) bool {
	if key.metadata.NotAfter.IsZero() || !time.Now().UTC().Before(key.metadata.NotAfter) {
		return true
	}
	if key.invocations >= key.invocationLimit || key.bytesSealed >= key.byteLimit {
		return true
	}
	return plaintextBytes > key.byteLimit-key.bytesSealed
}

// MakeVerifyOnly ends encryption while retaining decryption for a bounded
// overlap controlled by the key manager.
func (key *DEK) MakeVerifyOnly() error {
	key.mu.Lock()
	defer key.mu.Unlock()
	if key.metadata.State != StateActive {
		return fmt.Errorf("only an active DEK can become verify-only")
	}
	key.metadata.State = StateVerifyOnly
	return nil
}

// Revoke immediately prohibits encryption and decryption with this handle.
func (key *DEK) Revoke() {
	key.mu.Lock()
	defer key.mu.Unlock()
	key.metadata.State = StateRevoked
}

// Open decrypts ciphertext using authenticated protocol context.
func (key *DEK) Open(protected ProtectedData, authenticated AuthenticatedContext) ([]byte, error) {
	key.mu.RLock()
	defer key.mu.RUnlock()
	if (key.metadata.State != StateActive && key.metadata.State != StateVerifyOnly) || key.aead == nil {
		return nil, fmt.Errorf("DEK is not available for decryption")
	}
	if len(protected.Nonce) != key.aead.NonceSize() {
		return nil, fmt.Errorf("DEK nonce or authenticated context is invalid")
	}
	additionalData, err := key.authenticatedData(authenticated)
	if err != nil {
		return nil, err
	}
	plaintext, err := key.aead.Open(nil, protected.Nonce, protected.Ciphertext, additionalData)
	if err != nil {
		return nil, fmt.Errorf("decrypt protected data")
	}
	return plaintext, nil
}

func (key *DEK) authenticatedData(authenticated AuthenticatedContext) ([]byte, error) {
	if authenticated.KeyID != key.metadata.ID || authenticated.Algorithm != key.metadata.Algorithm {
		return nil, fmt.Errorf("authenticated key context does not match DEK")
	}
	if authenticated.SecurityDomain != key.metadata.SecurityDomain || authenticated.Direction != key.metadata.Direction {
		return nil, fmt.Errorf("authenticated scope does not match DEK")
	}
	if err := validateAuthenticatedContext(authenticated); err != nil {
		return nil, err
	}
	authenticated.IssuedAt = authenticated.IssuedAt.UTC()
	authenticated.ExpiresAt = authenticated.ExpiresAt.UTC()
	encoded, err := json.Marshal(authenticated)
	if err != nil {
		return nil, fmt.Errorf("encode authenticated context: %w", err)
	}
	return encoded, nil
}

func validateAuthenticatedContext(authenticated AuthenticatedContext) error {
	fields := []string{
		authenticated.GatewayID, authenticated.SenderID, authenticated.RecipientID,
		authenticated.SecurityDomain, authenticated.SessionID, authenticated.EventID,
		string(authenticated.Direction), authenticated.Operation, authenticated.KeyID, authenticated.Algorithm,
	}
	if authenticated.ProtocolVersion == 0 || authenticated.Sequence == 0 {
		return fmt.Errorf("authenticated protocol version and sequence are required")
	}
	if authenticated.IssuedAt.IsZero() || !authenticated.ExpiresAt.After(authenticated.IssuedAt) {
		return fmt.Errorf("authenticated validity window is invalid")
	}
	if !validDirection(authenticated.Direction) {
		return fmt.Errorf("authenticated direction is invalid")
	}
	for _, field := range fields {
		if field == "" || len(field) > maxAuthenticatedFieldBytes {
			return fmt.Errorf("authenticated context field is invalid")
		}
	}
	return nil
}

func validDirection(direction Direction) bool {
	switch direction {
	case DirectionGatewayToAgent, DirectionAgentToGateway, DirectionGatewayToBroker, DirectionBrokerToGateway:
		return true
	default:
		return false
	}
}

// Destroy makes the in-process DEK handle permanently unusable.
func (key *DEK) Destroy() {
	key.mu.Lock()
	defer key.mu.Unlock()
	key.aead = nil
	key.metadata.State = StateDestroyed
}

// P384Key is an opaque ephemeral P-384 private-key handle.
type P384Key struct {
	mu         sync.RWMutex
	metadata   Metadata
	privateKey *ecdh.PrivateKey
}

// Metadata returns the non-secret server-authored key metadata.
func (key *P384Key) Metadata() Metadata { key.mu.RLock(); defer key.mu.RUnlock(); return key.metadata }

// PublicKey returns the standardized uncompressed P-384 public-key encoding.
func (key *P384Key) PublicKey() []byte {
	key.mu.RLock()
	defer key.mu.RUnlock()
	if key.privateKey == nil {
		return nil
	}
	return append([]byte(nil), key.privateKey.PublicKey().Bytes()...)
}

// Derive derives one context-bound traffic key from a validated P-384 peer
// key and immediately destroys the ephemeral private handle.
func (key *P384Key) Derive(peerPublicKey, salt, contextInfo []byte, length int) ([]byte, error) {
	key.mu.Lock()
	defer key.mu.Unlock()
	if key.metadata.State != StateActive || key.privateKey == nil {
		return nil, fmt.Errorf("P-384 key is not active")
	}
	if len(salt) == 0 || len(contextInfo) == 0 || length <= 0 || length > maxDerivedKeyBytes {
		return nil, fmt.Errorf("P-384 derivation context is invalid")
	}
	peer, err := ecdh.P384().NewPublicKey(peerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("parse P-384 peer public key: %w", err)
	}
	secret, err := key.privateKey.ECDH(peer)
	key.privateKey = nil
	key.metadata.State = StateDestroyed
	if err != nil {
		return nil, fmt.Errorf("derive P-384 shared secret: %w", err)
	}
	defer clear(secret)
	derived, err := hkdf.Key(sha512.New384, secret, salt, string(contextInfo), length)
	if err != nil {
		return nil, fmt.Errorf("derive P-384 traffic key: %w", err)
	}
	return derived, nil
}

// Destroy makes the ephemeral P-384 handle permanently unusable.
func (key *P384Key) Destroy() {
	key.mu.Lock()
	defer key.mu.Unlock()
	key.privateKey = nil
	key.metadata.State = StateDestroyed
}

// MLKEM768Key is an opaque ephemeral ML-KEM-768 decapsulation-key handle.
type MLKEM768Key struct {
	mu         sync.Mutex
	metadata   Metadata
	privateKey *mlkem.DecapsulationKey768
}

// Metadata returns the non-secret server-authored key metadata.
func (key *MLKEM768Key) Metadata() Metadata {
	key.mu.Lock()
	defer key.mu.Unlock()
	return key.metadata
}

// EncapsulationKey returns the standardized FIPS 203 public-key encoding.
func (key *MLKEM768Key) EncapsulationKey() []byte {
	key.mu.Lock()
	defer key.mu.Unlock()
	if key.privateKey == nil {
		return nil
	}
	return append([]byte(nil), key.privateKey.EncapsulationKey().Bytes()...)
}

// DecapsulateAndDerive decapsulates one ciphertext, derives a context-bound
// traffic key, and immediately destroys the ephemeral private handle.
func (key *MLKEM768Key) DecapsulateAndDerive(ciphertext, salt, contextInfo []byte, length int) ([]byte, error) {
	key.mu.Lock()
	defer key.mu.Unlock()
	if key.metadata.State != StateActive || key.privateKey == nil {
		return nil, fmt.Errorf("ML-KEM-768 key is not active")
	}
	if len(salt) == 0 || len(contextInfo) == 0 || length <= 0 || length > maxDerivedKeyBytes {
		return nil, fmt.Errorf("ML-KEM-768 derivation context is invalid")
	}
	secret, err := key.privateKey.Decapsulate(ciphertext)
	key.privateKey = nil
	key.metadata.State = StateDestroyed
	if err != nil {
		return nil, fmt.Errorf("decapsulate ML-KEM-768 ciphertext")
	}
	defer clear(secret)
	derived, err := hkdf.Key(sha512.New384, secret, salt, string(contextInfo), length)
	if err != nil {
		return nil, fmt.Errorf("derive ML-KEM-768 traffic key: %w", err)
	}
	return derived, nil
}

// Destroy makes the ephemeral ML-KEM-768 handle permanently unusable.
func (key *MLKEM768Key) Destroy() {
	key.mu.Lock()
	defer key.mu.Unlock()
	key.privateKey = nil
	key.metadata.State = StateDestroyed
}

// CommandSigner is an opaque gateway command-signing capability.
type CommandSigner struct {
	mu       sync.RWMutex
	metadata Metadata
	signer   crypto.Signer
}

// Activate permits signing after the public verification key has been
// published. Only a preactive key can be activated.
func (signer *CommandSigner) Activate() error {
	signer.mu.Lock()
	defer signer.mu.Unlock()
	if signer.signer == nil || signer.metadata.State != StatePreactive {
		return fmt.Errorf("only a preactive command signing key can be activated")
	}
	if !time.Now().UTC().Before(signer.metadata.NotAfter) {
		return fmt.Errorf("command signing key has expired")
	}
	signer.metadata.State = StateActive
	return nil
}

// MakeVerifyOnly ends signing while retaining the public-key lifecycle record
// for a bounded verification overlap.
func (signer *CommandSigner) MakeVerifyOnly() error {
	signer.mu.Lock()
	defer signer.mu.Unlock()
	if signer.metadata.State != StateActive {
		return fmt.Errorf("only an active command signing key can become verify-only")
	}
	signer.metadata.State = StateVerifyOnly
	return nil
}

// Revoke immediately prohibits all use of the command signing key.
func (signer *CommandSigner) Revoke() {
	signer.mu.Lock()
	defer signer.mu.Unlock()
	signer.metadata.State = StateRevoked
}

// Ready reports whether the signing key is active and inside its validity
// window.
func (signer *CommandSigner) Ready() error {
	signer.mu.RLock()
	defer signer.mu.RUnlock()
	if signer.signer == nil || signer.metadata.State != StateActive {
		return fmt.Errorf("command signing key is not active")
	}
	if signer.metadata.NotAfter.IsZero() || !time.Now().UTC().Before(signer.metadata.NotAfter) {
		return fmt.Errorf("command signing key has expired")
	}
	return nil
}

// Metadata returns the non-secret server-authored key metadata.
func (signer *CommandSigner) Metadata() Metadata {
	signer.mu.RLock()
	defer signer.mu.RUnlock()
	return signer.metadata
}

// KeyID returns the gateway-authored fingerprint of the verification key.
func (signer *CommandSigner) KeyID() string {
	signer.mu.RLock()
	defer signer.mu.RUnlock()
	return signer.metadata.ID
}

// PublicKey returns the RSA command-verification key.
func (signer *CommandSigner) PublicKey() *rsa.PublicKey {
	signer.mu.RLock()
	defer signer.mu.RUnlock()
	if signer.signer == nil {
		return nil
	}
	key, _ := signer.signer.Public().(*rsa.PublicKey)
	if key == nil {
		return nil
	}
	return &rsa.PublicKey{N: new(big.Int).Set(key.N), E: key.E}
}

// SignCommand signs a gateway-authored command envelope with RSA-PSS.
func (signer *CommandSigner) SignCommand(envelope *commandauth.Envelope) error {
	signer.mu.RLock()
	defer signer.mu.RUnlock()
	if signer.metadata.State != StateActive || signer.signer == nil || !time.Now().UTC().Before(signer.metadata.NotAfter) {
		return fmt.Errorf("command signing key is not active")
	}
	return commandauth.SignWithSigner(envelope, signer.signer)
}

// Destroy releases the in-process private-key reference and prohibits signing.
func (signer *CommandSigner) Destroy() {
	signer.mu.Lock()
	defer signer.mu.Unlock()
	signer.signer = nil
	signer.metadata.State = StateDestroyed
}

type opaqueSigner interface {
	Public() crypto.PublicKey
	Sign(io.Reader, []byte, crypto.SignerOpts) ([]byte, error)
}

var _ opaqueSigner = (*rsa.PrivateKey)(nil)
