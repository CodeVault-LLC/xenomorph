package agentquic

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"testing"
	"time"

	quic "github.com/quic-go/quic-go"

	clientconfig "github.com/codevault-llc/xenomorph/platform/client/internal/config"
)

func TestNewRequiresStrictTLSProfile(t *testing.T) {
	t.Parallel()
	config := validClientConfig()
	valid := &tls.Config{MinVersion: tls.VersionTLS13, ServerName: config.ServerName,
		RootCAs: x509.NewCertPool(), Certificates: []tls.Certificate{{Certificate: [][]byte{{1}}}}}
	if _, err := New(config, valid, "agent-id", "command-key"); err != nil {
		t.Fatalf("strict TLS profile rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*tls.Config)
	}{
		{name: "TLS below 1.3", mutate: func(profile *tls.Config) { profile.MinVersion = tls.VersionTLS12 }},
		{name: "insecure verification", mutate: func(profile *tls.Config) { profile.InsecureSkipVerify = true }}, //nolint:gosec // The test proves this prohibited setting is rejected.
		{name: "server name mismatch", mutate: func(profile *tls.Config) { profile.ServerName = "other.internal" }},
		{name: "missing roots", mutate: func(profile *tls.Config) { profile.RootCAs = nil }},
		{name: "missing client certificate", mutate: func(profile *tls.Config) { profile.Certificates = nil }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile := valid.Clone()
			test.mutate(profile)
			if _, err := New(config, profile, "agent-id", "command-key"); err == nil {
				t.Fatal("insecure TLS profile was accepted")
			}
		})
	}
}

func TestSecurityFailureClassification(t *testing.T) {
	t.Parallel()
	if !IsSecurityFailure(ErrSecurityFailure) || !IsSecurityFailure(x509.UnknownAuthorityError{}) ||
		!IsSecurityFailure(&quic.TransportError{ErrorCode: minimumQUICCryptoErrorCode}) {
		t.Fatal("security failure was not classified as downgrade-prohibited")
	}
	if IsSecurityFailure(errors.New("network unreachable")) {
		t.Fatal("ordinary network failure was classified as a security failure")
	}
}

func validClientConfig() clientconfig.Config {
	return clientconfig.Config{
		Environment: "test", ImplementationVersion: "test", TransportMode: clientconfig.TransportQUIC,
		GatewayURL: "https://gateway.internal:8443", QUICEndpoint: "gateway.internal:8444",
		ServerName: "gateway.internal", ClientCertificateFile: "client.crt", ClientPrivateKeyFile: "client.key",
		CAFile: "ca.crt", CommandVerificationKeyFile: "command.pub", ReplayLedgerFile: "ledger.json",
		ReplayAuthenticationKeyFile: "ledger.key", HeartbeatInterval: 15 * time.Second, HTTPTimeout: 10 * time.Second,
		QUICHandshakeTimeout: 5 * time.Second, QUICIdleTimeout: 45 * time.Second, QUICKeepAlive: 10 * time.Second,
		ReconnectMinimumBackoff: time.Second, ReconnectMaximumBackoff: time.Minute,
	}
}

func TestReconnectJitterRemainsBounded(t *testing.T) {
	t.Parallel()
	base := 10 * time.Second
	minimum := base - base/jitterFractionDenominator
	maximum := base + base/jitterFractionDenominator
	for range 100 {
		value := jitter(base)
		if value < minimum || value > maximum {
			t.Fatalf("jitter %s outside [%s, %s]", value, minimum, maximum)
		}
	}
}

func TestRemovePendingResponseReleasesAcknowledgementState(t *testing.T) {
	t.Parallel()

	session := &clientSession{pending: make(map[uint64]chan eventResponse)}
	first := make(chan eventResponse, 1)
	second := make(chan eventResponse, 1)
	session.registerPending(1, first)
	session.registerPending(2, second)
	session.removePendingResponse(first)
	if _, exists := session.pending[1]; exists {
		t.Fatal("timed-out acknowledgement remained pending")
	}
	if session.pending[2] != second {
		t.Fatal("unrelated acknowledgement state was removed")
	}
}
