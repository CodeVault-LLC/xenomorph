package agentquic

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
	sharedidentity "github.com/codevault-llc/xenomorph/platform/shared/identity"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const integrationTimeout = 5 * time.Second

type integrationIngressSink struct {
	receipts chan IngressReceipt
}

func (sink *integrationIngressSink) CommitAgentMessage(
	_ context.Context,
	receipt IngressReceipt,
	_ IngressMessage,
) (IngressResult, error) {
	sink.receipts <- receipt
	return IngressResult{Status: wire.AcknowledgementAccepted, Commit: wire.CommitPublished, Retry: wire.RetryNever}, nil
}

type integrationCommandSource struct{}

func (integrationCommandSource) WaitDispatch(ctx context.Context, _ string) (*commandauth.Envelope, error) {
	<-ctx.Done()
	return nil, nil
}

func (integrationCommandSource) MarkOutcomeUnknown(string, string) error { return nil }

func TestListenerMutualTLSAndHeartbeatCommit(t *testing.T) { //nolint:cyclop // This test verifies the complete handshake-to-commit trust path.
	credentials := newIntegrationCredentials(t)
	sink := &integrationIngressSink{receipts: make(chan IngressReceipt, 1)}
	listener, address, cancel := startIntegrationListener(t, credentials, sink)
	defer cancel()

	connection := dialIntegrationClient(t, address, credentials.clientTLS("localhost", wire.ALPN))
	defer closeIntegrationConnection(connection)
	control, codec := negotiateIntegrationControl(t, connection, wire.ClientHello{
		MinimumMinor: 0, MaximumMinor: 0, ImplementationVersion: "integration-test",
		Platform: uint64(wire.PlatformLinux), Architecture: uint64(wire.ArchitectureAMD64),
		ClientInstanceNonce: [16]byte{1},
	})

	events, err := connection.OpenUniStreamSync(testContext(t))
	if err != nil {
		t.Fatalf("open event stream: %v", err)
	}
	if err := wire.WritePreamble(events, wire.StreamEvents); err != nil {
		t.Fatalf("write event preamble: %v", err)
	}
	eventCodec := integrationCodec(t, wire.StreamEvents)
	body, err := (wire.Heartbeat{Hostname: "agent-host", OSVersion: "linux"}).MarshalBinary()
	if err != nil {
		t.Fatalf("encode heartbeat: %v", err)
	}
	if err := eventCodec.WriteFrame(events, wire.Frame{
		Header: wire.FrameHeader{Type: wire.MessageHeartbeat, SchemaRevision: 1, Flags: wire.FlagAckRequired, Sequence: 2},
		Body:   body,
	}); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}

	acknowledgement := readIntegrationFrame(t, codec, control)
	if acknowledgement.Header.Type != wire.MessageMessageAck ||
		acknowledgement.Header.CorrelationSequence != 2 {
		t.Fatalf("unexpected acknowledgement header: %#v", acknowledgement.Header)
	}
	select {
	case receipt := <-sink.receipts:
		certificate, err := x509.ParseCertificate(credentials.clientCertificate.Certificate[0])
		if err != nil {
			t.Fatalf("parse client certificate: %v", err)
		}
		expectedAgentID, err := sharedidentity.AgentIDFromCertificate(certificate)
		if err != nil {
			t.Fatalf("derive expected agent identity: %v", err)
		}
		if receipt.AgentID != expectedAgentID || receipt.SessionID == [16]byte{} || receipt.MessageType != wire.MessageHeartbeat {
			t.Fatalf("unexpected authenticated receipt: %#v", receipt)
		}
	case <-time.After(integrationTimeout):
		t.Fatal("timed out waiting for heartbeat commit")
	}
	if listener.Metrics().Snapshot().DecodedFrames == 0 {
		t.Fatal("expected decoded frame metric")
	}
}

func TestListenerRejectsUnauthenticatedAndIncompatiblePeers(t *testing.T) {
	credentials := newIntegrationCredentials(t)
	sink := &integrationIngressSink{receipts: make(chan IngressReceipt, 1)}
	_, address, cancel := startIntegrationListener(t, credentials, sink)
	defer cancel()

	tests := []struct {
		name      string
		tlsConfig *tls.Config
		versions  []quic.Version
	}{
		{name: "missing client certificate", tlsConfig: credentials.clientTLSWithoutCertificate("localhost", wire.ALPN)},
		{name: "untrusted client CA", tlsConfig: credentials.untrustedClientTLS("localhost", wire.ALPN)},
		{name: "expired client certificate", tlsConfig: credentials.invalidValidityClientTLS("localhost", wire.ALPN, time.Now().Add(-2*time.Hour), time.Now().Add(-time.Hour))},
		{name: "not yet valid client certificate", tlsConfig: credentials.invalidValidityClientTLS("localhost", wire.ALPN, time.Now().Add(time.Hour), time.Now().Add(2*time.Hour))},
		{name: "wrong server name", tlsConfig: credentials.clientTLS("wrong.invalid", wire.ALPN)},
		{name: "wrong ALPN", tlsConfig: credentials.clientTLS("localhost", "unsupported-agent/1")},
		{name: "unsupported QUIC version", tlsConfig: credentials.clientTLS("localhost", wire.ALPN), versions: []quic.Version{quic.Version2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := &quic.Config{Versions: test.versions, HandshakeIdleTimeout: time.Second}
			if len(configuration.Versions) == 0 {
				configuration.Versions = []quic.Version{quic.Version1}
			}
			ctx, stop := context.WithTimeout(context.Background(), 2*time.Second)
			defer stop()
			connection, err := quic.DialAddr(ctx, address, test.tlsConfig, configuration)
			if err == nil {
				err = probeRejectedConnection(connection)
				closeIntegrationConnection(connection)
			}
			if err == nil {
				t.Fatal("incompatible peer unexpectedly completed the handshake")
			}
		})
	}
	select {
	case receipt := <-sink.receipts:
		t.Fatalf("application handler observed unauthenticated input: %#v", receipt)
	default:
	}
}

func probeRejectedConnection(connection *quic.Conn) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream, err := connection.OpenStreamSync(ctx)
	if err != nil {
		return err
	}
	if err := wire.WritePreamble(stream, wire.StreamControl); err != nil {
		return err
	}
	body, err := (wire.ClientHello{MinimumMinor: 0, MaximumMinor: 0, ImplementationVersion: "rejection-probe",
		Platform: uint64(wire.PlatformLinux), Architecture: uint64(wire.ArchitectureAMD64), ClientInstanceNonce: [16]byte{1}}).MarshalBinary()
	if err != nil {
		return err
	}
	specification, _ := wire.SpecificationForStream(wire.StreamControl)
	codec, err := wire.NewFrameCodec(specification.MaximumFrameBytes)
	if err != nil {
		return err
	}
	if err := codec.WriteFrame(stream, wire.Frame{Header: wire.FrameHeader{
		Type: wire.MessageClientHello, SchemaRevision: 1, Sequence: 1,
	}, Body: body}); err != nil {
		return err
	}
	if err := stream.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}
	_, err = codec.ReadFrame(stream)
	return err
}

func TestListenerRejectsUnsupportedApplicationVersion(t *testing.T) {
	credentials := newIntegrationCredentials(t)
	sink := &integrationIngressSink{receipts: make(chan IngressReceipt, 1)}
	_, address, cancel := startIntegrationListener(t, credentials, sink)
	defer cancel()
	connection := dialIntegrationClient(t, address, credentials.clientTLS("localhost", wire.ALPN))
	defer closeIntegrationConnection(connection)

	stream, err := connection.OpenStreamSync(testContext(t))
	if err != nil {
		t.Fatalf("open control stream: %v", err)
	}
	if err := wire.WritePreamble(stream, wire.StreamControl); err != nil {
		t.Fatalf("write control preamble: %v", err)
	}
	hello := wire.ClientHello{MinimumMinor: 1, MaximumMinor: 1, ImplementationVersion: "future-client",
		Platform: uint64(wire.PlatformLinux), Architecture: uint64(wire.ArchitectureAMD64), ClientInstanceNonce: [16]byte{1}}
	body, err := hello.MarshalBinary()
	if err != nil {
		t.Fatalf("encode client hello: %v", err)
	}
	codec := integrationCodec(t, wire.StreamControl)
	if err := codec.WriteFrame(stream, wire.Frame{Header: wire.FrameHeader{
		Type: wire.MessageClientHello, SchemaRevision: 1, Sequence: 1,
	}, Body: body}); err != nil {
		t.Fatalf("write client hello: %v", err)
	}
	if _, err := codec.ReadFrame(stream); err == nil {
		t.Fatal("unsupported application version unexpectedly negotiated")
	}
}

type integrationCredentials struct {
	directory         string
	caCertificate     *x509.Certificate
	caPrivateKey      ed25519.PrivateKey
	serverCertificate tls.Certificate
	clientCertificate tls.Certificate
	serverRoots       *x509.CertPool
}

func newIntegrationCredentials(t *testing.T) integrationCredentials {
	t.Helper()
	directory := t.TempDir()
	caCertificate, caPrivateKey := createIntegrationCA(t, "integration-ca")
	serverCertificate := createIntegrationLeaf(t, caCertificate, caPrivateKey, "gateway", true, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	clientCertificate := createIntegrationLeaf(t, caCertificate, caPrivateKey, "agent", false, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	roots := x509.NewCertPool()
	roots.AddCert(caCertificate)
	writeIntegrationCertificate(t, filepath.Join(directory, "ca.crt"), caCertificate.Raw)
	writeIntegrationTLSKeyPair(t, directory, "server", serverCertificate)
	writeIntegrationSecret(t, filepath.Join(directory, "reset.key"), 1)
	writeIntegrationSecret(t, filepath.Join(directory, "token.key"), 2)
	return integrationCredentials{directory: directory, caCertificate: caCertificate, caPrivateKey: caPrivateKey,
		serverCertificate: serverCertificate, clientCertificate: clientCertificate, serverRoots: roots}
}

func (credentials integrationCredentials) clientTLS(serverName, alpn string) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: credentials.serverRoots,
		ServerName: serverName, NextProtos: []string{alpn}, Certificates: []tls.Certificate{credentials.clientCertificate}}
}

func (credentials integrationCredentials) clientTLSWithoutCertificate(serverName, alpn string) *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: credentials.serverRoots,
		ServerName: serverName, NextProtos: []string{alpn}}
}

func (credentials integrationCredentials) untrustedClientTLS(serverName, alpn string) *tls.Config {
	untrustedCA, untrustedKey := createIntegrationCAForCredentials("untrusted-ca")
	untrustedClient := createIntegrationLeafForCredentials(untrustedCA, untrustedKey, "untrusted-agent", false,
		time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	configuration := credentials.clientTLS(serverName, alpn)
	configuration.Certificates = []tls.Certificate{untrustedClient}
	return configuration
}

func (credentials integrationCredentials) invalidValidityClientTLS(
	serverName string,
	alpn string,
	notBefore time.Time,
	notAfter time.Time,
) *tls.Config {
	certificate := createIntegrationLeafForCredentials(
		credentials.caCertificate, credentials.caPrivateKey, "invalid-validity-agent", false, notBefore, notAfter,
	)
	configuration := credentials.clientTLS(serverName, alpn)
	configuration.Certificates = []tls.Certificate{certificate}
	return configuration
}

func startIntegrationListener(
	t *testing.T,
	credentials integrationCredentials,
	sink IngressSink,
) (*Listener, string, context.CancelFunc) {
	t.Helper()
	config := testConfig()
	config.Address = "127.0.0.1:0"
	config.ServerCertificateFile = filepath.Join(credentials.directory, "server.crt")
	config.ServerPrivateKeyFile = filepath.Join(credentials.directory, "server.key")
	config.ClientCAFile = filepath.Join(credentials.directory, "ca.crt")
	config.StatelessResetKeyFile = filepath.Join(credentials.directory, "reset.key")
	config.TokenGeneratorKeyFile = filepath.Join(credentials.directory, "token.key")
	config.ControlStreamTimeout = time.Second
	listener, err := NewListener(config, sink, integrationCommandSource{}, "integration-command-key")
	if err != nil {
		t.Fatalf("create listener: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errorsChannel := make(chan error, 1)
	go func() { errorsChannel <- listener.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case runErr := <-errorsChannel:
			if runErr != nil && !errors.Is(runErr, context.Canceled) {
				t.Errorf("listener shutdown: %v", runErr)
			}
		case <-time.After(integrationTimeout):
			t.Error("listener did not shut down")
		}
	})
	deadline := time.NewTimer(integrationTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if address := listener.Address(); address != nil {
			return listener, address.String(), cancel
		}
		select {
		case <-deadline.C:
			t.Fatal("listener did not bind UDP address")
		case <-ticker.C:
		}
	}
}

func dialIntegrationClient(t *testing.T, address string, tlsConfig *tls.Config) *quic.Conn {
	t.Helper()
	connection, err := quic.DialAddr(testContext(t), address, tlsConfig, &quic.Config{
		Versions: []quic.Version{quic.Version1}, HandshakeIdleTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("dial integration listener: %v", err)
	}
	return connection
}

func negotiateIntegrationControl(t *testing.T, connection *quic.Conn, hello wire.ClientHello) (*quic.Stream, wire.FrameCodec) {
	t.Helper()
	stream, err := connection.OpenStreamSync(testContext(t))
	if err != nil {
		t.Fatalf("open control stream: %v", err)
	}
	if err := wire.WritePreamble(stream, wire.StreamControl); err != nil {
		t.Fatalf("write control preamble: %v", err)
	}
	body, err := hello.MarshalBinary()
	if err != nil {
		t.Fatalf("encode client hello: %v", err)
	}
	codec := integrationCodec(t, wire.StreamControl)
	if err := codec.WriteFrame(stream, wire.Frame{Header: wire.FrameHeader{
		Type: wire.MessageClientHello, SchemaRevision: 1, Sequence: 1,
	}, Body: body}); err != nil {
		t.Fatalf("write client hello: %v", err)
	}
	frame := readIntegrationFrame(t, codec, stream)
	if frame.Header.Type != wire.MessageServerHello {
		t.Fatalf("expected server hello, got %d", frame.Header.Type)
	}
	return stream, codec
}

func integrationCodec(t *testing.T, kind wire.StreamKind) wire.FrameCodec {
	t.Helper()
	specification, exists := wire.SpecificationForStream(kind)
	if !exists {
		t.Fatalf("missing stream specification %d", kind)
	}
	codec, err := wire.NewFrameCodec(specification.MaximumFrameBytes)
	if err != nil {
		t.Fatalf("create frame codec: %v", err)
	}
	return codec
}

func readIntegrationFrame(t *testing.T, codec wire.FrameCodec, stream *quic.Stream) wire.Frame {
	t.Helper()
	if err := stream.SetReadDeadline(time.Now().Add(integrationTimeout)); err != nil {
		t.Fatalf("set stream deadline: %v", err)
	}
	frame, err := codec.ReadFrame(stream)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	return frame
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), integrationTimeout)
	t.Cleanup(cancel)
	return ctx
}

func closeIntegrationConnection(connection *quic.Conn) {
	_ = connection.CloseWithError(0, "test complete")
}

func createIntegrationCA(t *testing.T, commonName string) (*x509.Certificate, ed25519.PrivateKey) {
	t.Helper()
	certificate, privateKey := createIntegrationCAForCredentials(commonName)
	return certificate, privateKey
}

func createIntegrationCAForCredentials(commonName string) (*x509.Certificate, ed25519.PrivateKey) {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: commonName},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true,
		BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	encoded, _ := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	certificate, _ := x509.ParseCertificate(encoded)
	return certificate, privateKey
}

func createIntegrationLeaf(
	t *testing.T,
	caCertificate *x509.Certificate,
	caPrivateKey ed25519.PrivateKey,
	commonName string,
	server bool,
	notBefore time.Time,
	notAfter time.Time,
) tls.Certificate {
	t.Helper()
	return createIntegrationLeafForCredentials(caCertificate, caPrivateKey, commonName, server, notBefore, notAfter)
}

func createIntegrationLeafForCredentials(
	caCertificate *x509.Certificate,
	caPrivateKey ed25519.PrivateKey,
	commonName string,
	server bool,
	notBefore time.Time,
	notAfter time.Time,
) tls.Certificate {
	publicKey, privateKey, _ := ed25519.GenerateKey(rand.Reader)
	extendedUsage := []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	template := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: commonName},
		NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: extendedUsage}
	if server {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.DNSNames = []string{"localhost"}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	encoded, _ := x509.CreateCertificate(rand.Reader, template, caCertificate, publicKey, caPrivateKey)
	return tls.Certificate{Certificate: [][]byte{encoded}, PrivateKey: privateKey}
}

func writeIntegrationTLSKeyPair(t *testing.T, directory, name string, certificate tls.Certificate) {
	t.Helper()
	writeIntegrationCertificate(t, filepath.Join(directory, name+".crt"), certificate.Certificate[0])
	encodedKey, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	writeIntegrationPEM(t, filepath.Join(directory, name+".key"), "PRIVATE KEY", encodedKey, 0o600)
}

func writeIntegrationCertificate(t *testing.T, path string, encoded []byte) {
	t.Helper()
	writeIntegrationPEM(t, path, "CERTIFICATE", encoded, 0o600)
}

func writeIntegrationPEM(t *testing.T, path, blockType string, encoded []byte, mode os.FileMode) {
	t.Helper()
	data := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: encoded})
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatalf("write integration credential: %v", err)
	}
}

func writeIntegrationSecret(t *testing.T, path string, value byte) {
	t.Helper()
	data := make([]byte, secretKeyBytes)
	for index := range data {
		data[index] = value
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(data)), 0o600); err != nil {
		t.Fatalf("write integration transport key: %v", err)
	}
}
