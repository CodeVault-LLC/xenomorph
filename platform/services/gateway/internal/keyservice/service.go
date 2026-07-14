package keyservice

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/fips140"
	"crypto/mlkem"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
)

const (
	dekBytes                = 32
	commandKeyBits          = 3072
	providerProbeDataSize   = 32
	providerProbeRSAKeyBits = 2048
	goProviderName          = "go-cryptographic-module"
	goV100BuildVersion      = "v1.0.0-c2097c7c"
	goV100Certificate       = "CMVP-5247"
	goV100Policy            = "Go Cryptographic Module v1.0.0 Security Policy"
)

// State is a server-authored cryptographic-key lifecycle state.
type State string

const (
	// StatePreactive permits publication but not cryptographic use.
	StatePreactive State = "preactive"
	// StateActive permits new cryptographic operations.
	StateActive State = "active"
	// StateVerifyOnly permits decryption or verification of existing data.
	StateVerifyOnly State = "verify-only"
	// StateRetired prohibits all new cryptographic operations.
	StateRetired State = "retired"
	// StateRevoked marks key material as untrusted.
	StateRevoked State = "revoked"
	// StateDestroyed records that provider-owned key material was destroyed.
	StateDestroyed State = "destroyed"
)

// Config pins the exact approved provider identity accepted at startup.
type Config struct {
	ProviderName                 string
	AllowedModuleVersions        []string
	Certificate                  string
	SecurityPolicy               string
	AllowedOperatingEnvironments []string
}

// ProviderInfo is the bounded, non-secret provider identity observed at
// startup. Every field is authored by the gateway or its build artifact.
type ProviderInfo struct {
	Name                 string
	ModuleVersion        string
	Certificate          string
	SecurityPolicy       string
	OperatingEnvironment string
	ApprovedMode         bool
}

// Metadata describes a gateway-owned key without exposing private material.
type Metadata struct {
	ID             string
	Algorithm      string
	Purpose        string
	SecurityDomain string
	Direction      Direction
	Version        uint64
	State          State
	CreatedAt      time.Time
	NotAfter       time.Time
}

// Service validates the selected provider and gates every generation
// operation on fail-closed readiness.
type Service struct {
	mu       sync.RWMutex
	provider ProviderInfo
	err      error
	closed   bool
	command  *CommandSigner
}

// New validates approved mode, the pinned provider allowlist, and provider
// consistency probes before making key generation available.
func New(cfg Config) (*Service, error) {
	provider := currentProvider()
	service := &Service{provider: provider}
	if err := validateProvider(cfg, provider); err != nil {
		service.fail(err)
		return service, err
	}
	if err := runProviderProbe(); err != nil {
		wrapped := fmt.Errorf("cryptographic provider self-test: %w", err)
		service.fail(wrapped)
		return service, wrapped
	}
	return service, nil
}

// Ready reports whether the provider remains approved and available for new
// cryptographic operations.
func (service *Service) Ready() error {
	service.mu.RLock()
	defer service.mu.RUnlock()
	if service.closed {
		return fmt.Errorf("cryptographic provider is closed")
	}
	if service.err != nil {
		return fmt.Errorf("cryptographic provider is unready: %w", service.err)
	}
	if service.command != nil {
		if err := service.command.Ready(); err != nil {
			return fmt.Errorf("command signing service is unready: %w", err)
		}
	}
	return nil
}

// Provider returns the non-secret provider identity observed at startup.
func (service *Service) Provider() ProviderInfo {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.provider
}

// Close prevents new operations and releases references to in-process
// provider state. Provider-specific implementations may additionally log out
// authenticated HSM or KMS sessions.
func (service *Service) Close() error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.command != nil {
		service.command.Destroy()
		service.command = nil
	}
	service.closed = true
	return nil
}

// GenerateDEK creates an opaque AES-256-GCM data-encryption key. The caller
// must provide a durable distributed nonce allocator before using it.
func (service *Service) GenerateDEK(ctx context.Context, scope DEKScope, version uint64, policy DEKPolicy, allocator NonceAllocator) (*DEK, error) {
	if err := service.beforeOperation(ctx); err != nil {
		return nil, err
	}
	if err := validateDEKRequest(scope, version, policy, allocator); err != nil {
		return nil, err
	}
	key := make([]byte, dekBytes)
	if _, err := rand.Read(key); err != nil {
		service.fail(fmt.Errorf("generate AES-256 key: %w", err))
		clear(key)
		return nil, service.Ready()
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		clear(key)
		service.fail(fmt.Errorf("initialize AES-256: %w", err))
		return nil, service.Ready()
	}
	aead, err := cipher.NewGCM(block)
	clear(key)
	if err != nil {
		service.fail(fmt.Errorf("initialize AES-256-GCM: %w", err))
		return nil, service.Ready()
	}
	metadata, err := newMetadata("AES-256-GCM", scope.Purpose, version, policy.RotationInterval, nil)
	if err != nil {
		service.fail(err)
		return nil, service.Ready()
	}
	metadata.SecurityDomain = scope.SecurityDomain
	metadata.Direction = scope.Direction
	return &DEK{
		metadata: metadata, aead: aead, allocator: allocator,
		invocationLimit: policy.InvocationLimit, byteLimit: policy.ByteLimit,
	}, nil
}

func validateDEKRequest(scope DEKScope, version uint64, policy DEKPolicy, allocator NonceAllocator) error {
	if strings.TrimSpace(scope.Purpose) == "" {
		return fmt.Errorf("DEK purpose is required")
	}
	if strings.TrimSpace(scope.SecurityDomain) == "" {
		return fmt.Errorf("DEK security domain is required")
	}
	if !validDirection(scope.Direction) {
		return fmt.Errorf("DEK direction is invalid")
	}
	if version == 0 {
		return fmt.Errorf("DEK version is required")
	}
	if err := validateDEKPolicy(policy); err != nil {
		return err
	}
	if allocator == nil {
		return fmt.Errorf("durable DEK nonce allocator is required")
	}
	return nil
}

func validateDEKPolicy(policy DEKPolicy) error {
	if policy.RotationInterval <= 0 || policy.RotationInterval > 24*time.Hour {
		return fmt.Errorf("DEK lifetime must be positive and at most 24 hours")
	}
	if policy.InvocationLimit == 0 || policy.InvocationLimit > absoluteDEKInvocationLimit {
		return fmt.Errorf("DEK invocation limit is invalid")
	}
	if policy.ByteLimit == 0 {
		return fmt.Errorf("DEK byte limit is required")
	}
	return nil
}

// GenerateP384 creates an ephemeral P-384 agreement key.
func (service *Service) GenerateP384(ctx context.Context, purpose string) (*P384Key, error) {
	if err := service.beforeOperation(ctx); err != nil {
		return nil, err
	}
	privateKey, err := ecdh.P384().GenerateKey(rand.Reader)
	if err != nil {
		service.fail(fmt.Errorf("generate P-384 key: %w", err))
		return nil, service.Ready()
	}
	metadata, err := newMetadata("ECDH-P384", purpose, 1, 0, privateKey.PublicKey().Bytes())
	if err != nil {
		service.fail(err)
		return nil, service.Ready()
	}
	return &P384Key{metadata: metadata, privateKey: privateKey}, nil
}

// GenerateMLKEM768 creates an ephemeral FIPS 203 ML-KEM-768 decapsulation
// key.
func (service *Service) GenerateMLKEM768(ctx context.Context, purpose string) (*MLKEM768Key, error) {
	if err := service.beforeOperation(ctx); err != nil {
		return nil, err
	}
	privateKey, err := mlkem.GenerateKey768()
	if err != nil {
		service.fail(fmt.Errorf("generate ML-KEM-768 key: %w", err))
		return nil, service.Ready()
	}
	publicKey := privateKey.EncapsulationKey().Bytes()
	metadata, err := newMetadata("ML-KEM-768", purpose, 1, 0, publicKey)
	if err != nil {
		service.fail(err)
		return nil, service.Ready()
	}
	return &MLKEM768Key{metadata: metadata, privateKey: privateKey}, nil
}

// GenerateCommandSigner creates an opaque RSA-PSS command-signing key with a
// maximum 90-day active lifetime.
func (service *Service) GenerateCommandSigner(ctx context.Context, version uint64) (*CommandSigner, error) {
	if err := service.beforeOperation(ctx); err != nil {
		return nil, err
	}
	if version == 0 {
		return nil, fmt.Errorf("command signing key version is required")
	}
	privateKey, err := rsa.GenerateKey(rand.Reader, commandKeyBits)
	if err != nil {
		service.fail(fmt.Errorf("generate command signing key: %w", err))
		return nil, service.Ready()
	}
	if err := privateKey.Validate(); err != nil {
		service.fail(fmt.Errorf("validate command signing key: %w", err))
		return nil, service.Ready()
	}
	keyID, err := commandauth.KeyID(&privateKey.PublicKey)
	if err != nil {
		service.fail(err)
		return nil, service.Ready()
	}
	now := time.Now().UTC()
	metadata := Metadata{
		ID: keyID, Algorithm: "RSA-PSS-SHA256", Purpose: "command-signing",
		Version: version, State: StatePreactive, CreatedAt: now,
		NotAfter: now.Add(90 * 24 * time.Hour),
	}
	return &CommandSigner{metadata: metadata, signer: privateKey}, nil
}

func (service *Service) setCommandSigner(signer *CommandSigner) {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.command = signer
}

func (service *Service) beforeOperation(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("cryptographic operation context is required")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cryptographic operation canceled: %w", err)
	}
	return service.Ready()
}

func (service *Service) fail(err error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.err == nil {
		service.err = err
	}
}

func currentProvider() ProviderInfo {
	moduleVersion := fipsModuleVersion()
	certificate, securityPolicy := certifiedModuleIdentity(moduleVersion)
	return ProviderInfo{
		Name:                 goProviderName,
		ModuleVersion:        moduleVersion,
		Certificate:          certificate,
		SecurityPolicy:       securityPolicy,
		OperatingEnvironment: runtime.GOOS + "/" + runtime.GOARCH,
		ApprovedMode:         fips140.Enabled(),
	}
}

func validateProvider(cfg Config, provider ProviderInfo) error {
	if err := validateProviderConfig(cfg); err != nil {
		return err
	}
	if !provider.ApprovedMode {
		return fmt.Errorf("cryptographic provider is not in approved mode")
	}
	if provider.Name != cfg.ProviderName {
		return fmt.Errorf("cryptographic provider identity is not approved")
	}
	if !slices.Contains(cfg.AllowedModuleVersions, provider.ModuleVersion) {
		return fmt.Errorf("cryptographic module version is not approved")
	}
	if provider.Certificate != cfg.Certificate || provider.SecurityPolicy != cfg.SecurityPolicy {
		return fmt.Errorf("cryptographic module certificate or security policy is not approved")
	}
	if !slices.Contains(cfg.AllowedOperatingEnvironments, provider.OperatingEnvironment) {
		return fmt.Errorf("cryptographic operating environment is not approved")
	}
	return nil
}

func validateProviderConfig(cfg Config) error {
	if strings.TrimSpace(cfg.ProviderName) == "" {
		return fmt.Errorf("cryptographic provider name is required")
	}
	if len(cfg.AllowedModuleVersions) == 0 {
		return fmt.Errorf("cryptographic module allowlist is required")
	}
	if strings.TrimSpace(cfg.Certificate) == "" || strings.TrimSpace(cfg.SecurityPolicy) == "" {
		return fmt.Errorf("cryptographic certificate and security policy are required")
	}
	if len(cfg.AllowedOperatingEnvironments) == 0 {
		return fmt.Errorf("cryptographic operating-environment allowlist is required")
	}
	return nil
}

func certifiedModuleIdentity(moduleVersion string) (string, string) {
	if moduleVersion == goV100BuildVersion {
		return goV100Certificate, goV100Policy
	}
	return "unrecognized", "unrecognized"
}

func fipsModuleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "GOFIPS140" {
			return setting.Value
		}
	}
	return "off"
}

func runProviderProbe() error {
	probe := make([]byte, providerProbeDataSize)
	if _, err := rand.Read(probe); err != nil {
		return fmt.Errorf("random-bit generation: %w", err)
	}
	defer clear(probe)
	privateKey, err := rsa.GenerateKey(rand.Reader, providerProbeRSAKeyBits)
	if err != nil {
		return fmt.Errorf("RSA key generation: %w", err)
	}
	digest := sha256.Sum256(probe)
	signature, err := rsa.SignPSS(rand.Reader, privateKey, crypto.SHA256, digest[:], &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256})
	if err != nil {
		return fmt.Errorf("RSA pairwise sign: %w", err)
	}
	if err := rsa.VerifyPSS(&privateKey.PublicKey, crypto.SHA256, digest[:], signature, &rsa.PSSOptions{SaltLength: rsa.PSSSaltLengthEqualsHash, Hash: crypto.SHA256}); err != nil {
		return fmt.Errorf("RSA pairwise verify: %w", err)
	}
	if err := probeP384(); err != nil {
		return err
	}
	if err := probeMLKEM768(); err != nil {
		return err
	}
	return probeAESGCM(probe)
}

func probeP384() error {
	first, err := ecdh.P384().GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("P-384 first key generation: %w", err)
	}
	second, err := ecdh.P384().GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("P-384 second key generation: %w", err)
	}
	firstSecret, err := first.ECDH(second.PublicKey())
	if err != nil {
		return fmt.Errorf("P-384 first agreement: %w", err)
	}
	defer clear(firstSecret)
	secondSecret, err := second.ECDH(first.PublicKey())
	if err != nil {
		return fmt.Errorf("P-384 second agreement: %w", err)
	}
	defer clear(secondSecret)
	if !bytes.Equal(firstSecret, secondSecret) {
		return fmt.Errorf("P-384 consistency failed")
	}
	return nil
}

func probeMLKEM768() error {
	privateKey, err := mlkem.GenerateKey768()
	if err != nil {
		return fmt.Errorf("ML-KEM-768 key generation: %w", err)
	}
	encapsulatedSecret, ciphertext := privateKey.EncapsulationKey().Encapsulate()
	defer clear(encapsulatedSecret)
	decapsulatedSecret, err := privateKey.Decapsulate(ciphertext)
	if err != nil {
		return fmt.Errorf("ML-KEM-768 decapsulation: %w", err)
	}
	defer clear(decapsulatedSecret)
	if !bytes.Equal(encapsulatedSecret, decapsulatedSecret) {
		return fmt.Errorf("ML-KEM-768 consistency failed")
	}
	return nil
}

func probeAESGCM(plaintext []byte) error {
	key := make([]byte, dekBytes)
	if _, err := rand.Read(key); err != nil {
		return fmt.Errorf("AES key generation: %w", err)
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("AES initialization: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("GCM initialization: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("GCM nonce generation: %w", err)
	}
	ciphertext := aead.Seal(nil, nonce, plaintext, []byte("gateway-provider-probe"))
	opened, err := aead.Open(nil, nonce, ciphertext, []byte("gateway-provider-probe"))
	if err != nil || !bytes.Equal(opened, plaintext) {
		return fmt.Errorf("AES-GCM consistency failed")
	}
	clear(opened)
	clear(ciphertext)
	return nil
}

func newMetadata(algorithm, purpose string, version uint64, lifetime time.Duration, publicKey []byte) (Metadata, error) {
	if strings.TrimSpace(purpose) == "" {
		return Metadata{}, fmt.Errorf("key purpose is required")
	}
	identifier := make([]byte, sha256.Size)
	prefix := "random:"
	if len(publicKey) > 0 {
		digest := sha256.Sum256(publicKey)
		identifier = digest[:]
		prefix = "sha256:"
	} else if _, err := rand.Read(identifier); err != nil {
		return Metadata{}, fmt.Errorf("generate key identifier: %w", err)
	}
	now := time.Now().UTC()
	metadata := Metadata{ID: prefix + hex.EncodeToString(identifier), Algorithm: algorithm, Purpose: purpose, Version: version, State: StateActive, CreatedAt: now}
	if lifetime > 0 {
		metadata.NotAfter = now.Add(lifetime)
	}
	return metadata, nil
}

var _ crypto.Signer = (*rsa.PrivateKey)(nil)
