// Package commandauth defines and verifies the integrity-protected command
// envelope exchanged by the gateway and agent. The gateway authors every
// signed field. Operator-authored payload bytes remain untrusted input and
// require command-specific validation before use.
package commandauth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

// ProtocolVersion is the command signing contract version.
const ProtocolVersion = 1

// KeyID returns the SHA-256 fingerprint of an RSA public key.
func KeyID(publicKey *rsa.PublicKey) (string, error) {
	if publicKey == nil {
		return "", fmt.Errorf("command verification key is nil")
	}

	encoded, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("encode command verification key: %w", err)
	}

	fingerprint := sha256.Sum256(encoded)

	return fmt.Sprintf("sha256:%x", fingerprint), nil
}

// Envelope is the versioned, gateway-authored command wire contract.
type Envelope struct {
	ProtocolVersion int             `json:"protocol_version"`
	CommandID       string          `json:"command_id"`
	AudienceAgentID string          `json:"audience_agent_id"`
	Type            string          `json:"type"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	RequestedBy     string          `json:"requested_by"`
	IssuedAt        time.Time       `json:"issued_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	Nonce           string          `json:"nonce"`
	Reason          string          `json:"reason"`
	KeyID           string          `json:"key_id"`
	Signature       string          `json:"signature"`
}

type unsignedEnvelope struct {
	ProtocolVersion int             `json:"protocol_version"`
	CommandID       string          `json:"command_id"`
	AudienceAgentID string          `json:"audience_agent_id"`
	Type            string          `json:"type"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	RequestedBy     string          `json:"requested_by"`
	IssuedAt        time.Time       `json:"issued_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	Nonce           string          `json:"nonce"`
	Reason          string          `json:"reason"`
	KeyID           string          `json:"key_id"`
}

// Sign applies an RSA-PSS SHA-256 signature to an envelope.
func Sign(envelope *Envelope, privateKey *rsa.PrivateKey) error {
	if privateKey == nil {
		return fmt.Errorf("command signing key is nil")
	}

	return SignWithSigner(envelope, privateKey)
}

// SignWithSigner applies an RSA-PSS SHA-256 signature using an opaque signing
// capability. The signer may be backed by an in-process key, HSM, or KMS, but
// its public key must be RSA so command verification remains interoperable.
func SignWithSigner(envelope *Envelope, signer crypto.Signer) error {
	if envelope == nil {
		return fmt.Errorf("command envelope is nil")
	}

	if signer == nil || isNilSigner(signer) {
		return fmt.Errorf("command signing key is nil")
	}

	if _, ok := signer.Public().(*rsa.PublicKey); !ok {
		return fmt.Errorf("command signing key must be RSA")
	}

	digest, err := digestEnvelope(*envelope)
	if err != nil {
		return err
	}

	signature, err := signer.Sign(rand.Reader, digest, &rsa.PSSOptions{
		SaltLength: rsa.PSSSaltLengthEqualsHash,
		Hash:       crypto.SHA256,
	})
	if err != nil {
		return fmt.Errorf("sign command envelope: %w", err)
	}

	envelope.Signature = base64.RawURLEncoding.EncodeToString(signature)

	return nil
}

func isNilSigner(signer crypto.Signer) bool {
	value := reflect.ValueOf(signer)
	return value.Kind() == reflect.Pointer && value.IsNil()
}

// Verify checks an envelope's RSA-PSS SHA-256 signature.
func Verify(envelope Envelope, publicKey *rsa.PublicKey) error {
	if publicKey == nil {
		return fmt.Errorf("command verification key is nil")
	}

	signature, err := base64.RawURLEncoding.DecodeString(envelope.Signature)
	if err != nil {
		return fmt.Errorf("decode command signature: %w", err)
	}

	digest, err := digestEnvelope(envelope)
	if err != nil {
		return err
	}

	if err := rsa.VerifyPSS(publicKey, crypto.SHA256, digest, signature, nil); err != nil {
		return fmt.Errorf("verify command signature: %w", err)
	}

	return nil
}

func digestEnvelope(envelope Envelope) ([]byte, error) {
	encoded, err := json.Marshal(unsignedEnvelope{
		ProtocolVersion: envelope.ProtocolVersion,
		CommandID:       envelope.CommandID,
		AudienceAgentID: envelope.AudienceAgentID,
		Type:            envelope.Type,
		Payload:         envelope.Payload,
		RequestedBy:     envelope.RequestedBy,
		IssuedAt:        envelope.IssuedAt.UTC(),
		ExpiresAt:       envelope.ExpiresAt.UTC(),
		Nonce:           envelope.Nonce,
		Reason:          envelope.Reason,
		KeyID:           envelope.KeyID,
	})
	if err != nil {
		return nil, fmt.Errorf("encode command envelope: %w", err)
	}

	digest := sha256.Sum256(encoded)

	return digest[:], nil
}
