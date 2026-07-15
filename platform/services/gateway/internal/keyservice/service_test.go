package keyservice

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
)

type staticNonceAllocator struct {
	nonce []byte
	err   error
}

func (a staticNonceAllocator) NextNonce(_ context.Context, _ Metadata) ([]byte, error) {
	return append([]byte(nil), a.nonce...), a.err
}

func readyTestService() *Service {
	return &Service{provider: ProviderInfo{ApprovedMode: true}}
}

func TestGenerateDEKSealAndOpen(t *testing.T) {
	t.Parallel()

	service := readyTestService()

	key, err := service.GenerateDEK(context.Background(), DEKScope{
		Purpose: "event", SecurityDomain: "internal", Direction: DirectionGatewayToBroker,
	}, 1, DEKPolicy{InvocationLimit: 2, ByteLimit: 64, RotationInterval: time.Hour}, staticNonceAllocator{nonce: make([]byte, 12)})
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}

	metadata := key.Metadata()
	authenticated := AuthenticatedContext{
		ProtocolVersion: 1, GatewayID: "gateway-1", SenderID: "gateway-1", RecipientID: "broker-1",
		SecurityDomain: "internal", SessionID: "session-1", Sequence: 1, EventID: "event-1",
		IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute),
		Direction: DirectionGatewayToBroker, Operation: "publish", KeyID: metadata.ID, Algorithm: metadata.Algorithm,
	}

	protected, err := key.Seal(context.Background(), []byte("payload"), authenticated)
	if err != nil {
		t.Fatalf("Seal() error = %v", err)
	}

	plaintext, err := key.Open(protected, authenticated)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if string(plaintext) != "payload" {
		t.Fatalf("Open() = %q, want payload", plaintext)
	}
}

func TestDEKRetiresAfterNonceAllocationFailure(t *testing.T) {
	t.Parallel()

	service := readyTestService()

	key, err := service.GenerateDEK(context.Background(), DEKScope{
		Purpose: "event", SecurityDomain: "internal", Direction: DirectionGatewayToBroker,
	}, 1, DEKPolicy{InvocationLimit: 1, ByteLimit: 64, RotationInterval: time.Hour}, staticNonceAllocator{err: errors.New("uncertain allocation")})
	if err != nil {
		t.Fatalf("GenerateDEK() error = %v", err)
	}

	metadata := key.Metadata()

	authenticated := AuthenticatedContext{
		ProtocolVersion: 1, GatewayID: "gateway-1", SenderID: "gateway-1", RecipientID: "broker-1",
		SecurityDomain: "internal", SessionID: "session-1", Sequence: 1, EventID: "event-1",
		IssuedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute),
		Direction: DirectionGatewayToBroker, Operation: "publish", KeyID: metadata.ID, Algorithm: metadata.Algorithm,
	}
	if _, err := key.Seal(context.Background(), []byte("payload"), authenticated); err == nil {
		t.Fatal("Seal() error = nil, want allocation failure")
	}

	if key.Metadata().State != StateRetired {
		t.Fatalf("key state = %q, want %q", key.Metadata().State, StateRetired)
	}
}

func TestCommandSignerLifecycle(t *testing.T) {
	service := readyTestService()

	signer, err := service.GenerateCommandSigner(context.Background(), 1)
	if err != nil {
		t.Fatalf("GenerateCommandSigner() error = %v", err)
	}

	if err := signer.Activate(); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}

	now := time.Now().UTC()

	envelope := commandauth.Envelope{
		ProtocolVersion: commandauth.ProtocolVersion, CommandID: "command-1", AudienceAgentID: "agent-1",
		Type: "support.notice", RequestedBy: "operator-1", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
		Nonce: "nonce-1", Reason: "test", KeyID: signer.KeyID(),
	}
	if err := signer.SignCommand(&envelope); err != nil {
		t.Fatalf("SignCommand() error = %v", err)
	}

	if err := commandauth.Verify(envelope, signer.PublicKey()); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}

	signer.Revoke()

	if err := signer.SignCommand(&envelope); err == nil {
		t.Fatal("SignCommand() error = nil after revocation")
	}
}

func TestServiceCloseFailsClosed(t *testing.T) {
	t.Parallel()

	service := readyTestService()
	if err := service.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if err := service.Ready(); err == nil {
		t.Fatal("Ready() error = nil after close")
	}
}

func TestValidateProviderRejectsUnapprovedIdentity(t *testing.T) {
	t.Parallel()

	provider := ProviderInfo{
		Name: "provider", ModuleVersion: "version", Certificate: "certificate",
		SecurityPolicy: "policy", OperatingEnvironment: "linux/amd64", ApprovedMode: false,
	}
	config := Config{
		ProviderName: "provider", AllowedModuleVersions: []string{"version"}, Certificate: "certificate",
		SecurityPolicy: "policy", AllowedOperatingEnvironments: []string{"linux/amd64"},
	}

	if err := validateProvider(config, provider); err == nil {
		t.Fatal("validateProvider() error = nil for unapproved provider")
	}
}
