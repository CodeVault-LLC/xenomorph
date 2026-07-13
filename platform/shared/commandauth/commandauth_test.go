package commandauth

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"
)

func TestSignAndVerify(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	envelope := Envelope{
		ProtocolVersion: ProtocolVersion,
		CommandID:       "command-1",
		AudienceAgentID: "agent-1",
		Type:            "support.notice",
		IssuedAt:        time.Unix(100, 0).UTC(),
		ExpiresAt:       time.Unix(200, 0).UTC(),
		Nonce:           "nonce-1",
		KeyID:           "server-1",
	}
	if err := Sign(&envelope, privateKey); err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if err := Verify(envelope, &privateKey.PublicKey); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey() error = %v", err)
	}
	envelope := Envelope{ProtocolVersion: ProtocolVersion, CommandID: "command-1"}
	if err := Sign(&envelope, privateKey); err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	envelope.CommandID = "command-2"
	if err := Verify(envelope, &privateKey.PublicKey); err == nil {
		t.Fatal("Verify() error = nil, want tampering error")
	}
}
