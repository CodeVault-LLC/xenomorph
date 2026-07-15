package agentquic

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
	"runtime"
	"sync"
	"time"

	quic "github.com/quic-go/quic-go"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
	clientconfig "github.com/codevault-llc/xenomorph/platform/client/internal/config"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const commandQueueDepth = 32

const (
	clientInitialStreamWindow     = 256 << 10
	clientMaximumStreamWindow     = 4 << 20
	clientInitialConnectionWindow = 512 << 10
	clientMaximumConnectionWindow = 16 << 20
	clientMaximumStreams          = 8
	clientMaximumUniStreams       = 4
	reconnectBackoffMultiplier    = 2
	jitterFractionDenominator     = 5
	minimumQUICCryptoErrorCode    = quic.TransportErrorCode(0x100)
	maximumQUICCryptoErrorCode    = quic.TransportErrorCode(0x1ff)
)

// Client supervises one active authenticated QUIC session and bounded command queue.
type Client struct {
	config        clientconfig.Config
	tlsConfig     *tls.Config
	audience      string
	commandKeyID  string
	instanceNonce [16]byte

	mu        sync.Mutex
	session   *clientSession
	changed   chan struct{}
	fatal     error
	cancel    context.CancelFunc
	waiter    sync.WaitGroup
	commands  chan *agent.CommandEnvelope
	transfers transferRegistry
}

// New validates the exact TLS profile and constructs a stopped supervisor.
func New(config clientconfig.Config, tlsConfig *tls.Config, audience, commandKeyID string) (*Client, error) {
	if err := config.Validate(time.Now().UTC()); err != nil {
		return nil, fmt.Errorf("create QUIC client: %w", err)
	}

	if err := validateClientTLSProfile(config, tlsConfig); err != nil {
		return nil, err
	}

	if audience == "" || commandKeyID == "" {
		return nil, fmt.Errorf("create QUIC client: local command audience and verification key ID are required")
	}

	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("create QUIC client instance nonce: %w", err)
	}

	quicTLS := tlsConfig.Clone()
	quicTLS.NextProtos = []string{wire.ALPN}

	return &Client{
		config: config, tlsConfig: quicTLS, audience: audience, commandKeyID: commandKeyID, instanceNonce: nonce,
		changed: make(chan struct{}), commands: make(chan *agent.CommandEnvelope, commandQueueDepth),
		transfers: newTransferRegistry(),
	}, nil
}

func validateClientTLSProfile(config clientconfig.Config, tlsConfig *tls.Config) error {
	if tlsConfig == nil || tlsConfig.MinVersion != tls.VersionTLS13 || tlsConfig.InsecureSkipVerify ||
		tlsConfig.ServerName != config.ServerName || tlsConfig.RootCAs == nil || len(tlsConfig.Certificates) != 1 {
		return fmt.Errorf("create QUIC client: strict TLS 1.3 credentials and server name are required")
	}

	return nil
}

// Start launches reconnect supervision and waits for the first negotiated session.
func (client *Client) Start(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("start QUIC client: context is nil")
	}

	client.mu.Lock()
	if client.cancel != nil {
		client.mu.Unlock()
		return fmt.Errorf("start QUIC client: supervisor already started")
	}

	lifecycleContext, cancel := context.WithCancel(context.WithoutCancel(ctx))
	client.cancel = cancel
	client.waiter.Add(1)

	go client.supervise(lifecycleContext)
	client.mu.Unlock()
	_, err := client.waitSession(ctx)

	return err
}

// Close stops reconnect supervision and all active lane workers.
func (client *Client) Close() {
	if client == nil {
		return
	}

	client.mu.Lock()
	cancel := client.cancel
	client.cancel = nil
	client.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	client.waiter.Wait()
}

// Authenticate sends the first authoritative heartbeat on the active session.
func (client *Client) Authenticate() (agent.DeviceAuthResult, error) {
	if err := client.SendHeartbeat(); err != nil {
		return agent.DeviceAuthResult{}, err
	}

	return agent.DeviceAuthResult{RequiresAttestation: true}, nil
}

// SendHeartbeat sends one bounded telemetry sample and waits for broker publication.
func (client *Client) SendHeartbeat() error {
	ctx, cancel := context.WithTimeout(context.Background(), client.config.HTTPTimeout)
	defer cancel()

	session, err := client.waitSession(ctx)
	if err != nil {
		return err
	}

	message := heartbeatFromAgent(agent.BuildHeartbeatPayload(nil))

	body, err := message.MarshalBinary()
	if err != nil {
		return fmt.Errorf("encode QUIC heartbeat: %w", err)
	}

	err = session.sendEvent(ctx, eventRequest{messageType: wire.MessageHeartbeat, flags: wire.FlagAckRequired, body: body, traffic: trafficTelemetry})

	return err
}

// SubmitAttestation sends endpoint inventory under a stable client operation identifier.
func (client *Client) SubmitAttestation(payload agent.EndpointAttestation) error {
	ctx, cancel := context.WithTimeout(context.Background(), client.config.HTTPTimeout)
	defer cancel()

	session, err := client.waitSession(ctx)
	if err != nil {
		return err
	}

	message := attestationFromAgent(payload)

	body, err := message.MarshalBinary()
	if err != nil {
		return fmt.Errorf("encode QUIC attestation: %w", err)
	}

	err = session.sendEvent(ctx, eventRequest{
		messageType: wire.MessageAttestation, flags: wire.FlagAckRequired | wire.FlagHasOperationID,
		operationID: operationIDForPayload("attestation", client.audience, body), body: body, traffic: trafficTelemetry,
	})

	return err
}

// SendLogEntry sends fixed-code diagnostic metadata without retrying on failure.
func (client *Client) SendLogEntry(payload agent.LogEntryPayload) error {
	ctx, cancel := context.WithTimeout(context.Background(), client.config.HTTPTimeout)
	defer cancel()

	session, err := client.waitSession(ctx)
	if err != nil {
		return err
	}

	message, err := logEntryFromAgent(payload)
	if err != nil {
		return err
	}

	body, err := message.MarshalBinary()
	if err != nil {
		return fmt.Errorf("encode QUIC log entry: %w", err)
	}

	err = session.sendEvent(ctx, eventRequest{messageType: wire.MessageLogEntry, flags: wire.FlagAckRequired, body: body, traffic: trafficLog})

	return err
}

// PollNextCommand blocks until the gateway pushes a signed command or supervision fails.
func (client *Client) PollNextCommand() (*agent.CommandEnvelope, error) {
	for {
		client.mu.Lock()
		fatal := client.fatal
		changed := client.changed
		client.mu.Unlock()

		if fatal != nil {
			return nil, fatal
		}
		select {
		case command := <-client.commands:
			return command, nil
		case <-changed:
		}
	}
}

// SendCommandResult submits a terminal client-authored result under the command ID.
func (client *Client) SendCommandResult(payload agent.CommandResultPayload) error {
	ctx, cancel := context.WithTimeout(context.Background(), client.config.HTTPTimeout)
	defer cancel()

	session, err := client.waitSession(ctx)
	if err != nil {
		return err
	}

	message, err := commandResultFromAgent(payload)
	if err != nil {
		return fmt.Errorf("encode QUIC command result metadata: %w", err)
	}

	if err := wire.ValidateCommandResult(message); err != nil {
		return fmt.Errorf("validate QUIC command result: %w", err)
	}

	body, err := message.MarshalBinary()
	if err != nil {
		return fmt.Errorf("encode QUIC command result: %w", err)
	}

	operation, err := parseOperationID(payload.CommandID)
	if err != nil {
		return err
	}

	err = session.sendEvent(ctx, eventRequest{
		messageType: wire.MessageCommandResult,
		flags:       wire.FlagAckRequired | wire.FlagHasOperationID | wire.FlagSensitive | wire.FlagEndOperation,
		operationID: operation, body: body, traffic: trafficResult,
	})

	return err
}

func (client *Client) supervise(ctx context.Context) {
	defer client.waiter.Done()
	backoff := client.config.ReconnectMinimumBackoff

	for ctx.Err() == nil {
		session, err := client.dialSession(ctx)
		if err != nil {
			if isSecurityFailure(err) {
				client.setFatal(fmt.Errorf("%w: %v", ErrSecurityFailure, err))
				return
			}

			if !waitBackoff(ctx, jitter(backoff)) {
				return
			}

			backoff = min(backoff*reconnectBackoffMultiplier, client.config.ReconnectMaximumBackoff)

			continue
		}

		backoff = client.config.ReconnectMinimumBackoff
		client.setSession(session)
		err = session.run(ctx, client.commands)
		client.clearSession(session)

		if errors.Is(err, ErrSecurityFailure) || errors.Is(err, ErrSessionReplaced) {
			client.setFatal(err)
			return
		}

		if ctx.Err() == nil && !waitBackoff(ctx, jitter(backoff)) {
			return
		}
	}
}

func (client *Client) dialSession(ctx context.Context) (*clientSession, error) {
	dialContext, cancel := context.WithTimeout(ctx, client.config.QUICHandshakeTimeout)
	defer cancel()

	connection, err := quic.DialAddr(dialContext, client.config.QUICEndpoint, client.tlsConfig, &quic.Config{
		Versions: []quic.Version{quic.Version1}, HandshakeIdleTimeout: client.config.QUICHandshakeTimeout,
		MaxIdleTimeout: client.config.QUICIdleTimeout, KeepAlivePeriod: client.config.QUICKeepAlive,
		InitialStreamReceiveWindow: clientInitialStreamWindow, MaxStreamReceiveWindow: clientMaximumStreamWindow,
		InitialConnectionReceiveWindow: clientInitialConnectionWindow, MaxConnectionReceiveWindow: clientMaximumConnectionWindow,
		MaxIncomingStreams: clientMaximumStreams, MaxIncomingUniStreams: clientMaximumUniStreams,
		Allow0RTT: false, EnableDatagrams: false,
	})
	if err != nil {
		return nil, fmt.Errorf("dial QUIC gateway: %w", err)
	}

	state := connection.ConnectionState()
	if state.Used0RTT || state.Version != quic.Version1 || state.TLS.Version != tls.VersionTLS13 ||
		state.TLS.NegotiatedProtocol != wire.ALPN || len(state.TLS.VerifiedChains) == 0 {
		_ = connection.CloseWithError(quic.ApplicationErrorCode(wire.ApplicationAuthState), "authenticated session required")
		return nil, fmt.Errorf("validate QUIC gateway transport state: %w", ErrSecurityFailure)
	}

	session, err := negotiateSession(dialContext, connection, client.hello())
	if err != nil {
		if closeErr := connection.CloseWithError(quic.ApplicationErrorCode(wire.ApplicationVersion), "negotiation failed"); closeErr != nil {
			return nil, fmt.Errorf("negotiate QUIC session: %v; close connection: %w", err, closeErr)
		}

		return nil, err
	}

	if session.serverHello.CommandVerificationKeyID != client.commandKeyID {
		_ = connection.CloseWithError(quic.ApplicationErrorCode(wire.ApplicationAuthState), "command verification key mismatch")
		return nil, fmt.Errorf("validate server command key: %w", ErrSecurityFailure)
	}

	session.transfers = &client.transfers

	return session, nil
}

func (client *Client) hello() wire.ClientHello {
	return wire.ClientHello{
		MinimumMinor: 0, MaximumMinor: uint64(wire.ProtocolMinor), Features: 0,
		ImplementationVersion: client.config.ImplementationVersion,
		Platform:              uint64(platform()), Architecture: uint64(architecture()), ClientInstanceNonce: client.instanceNonce,
	}
}

func (client *Client) waitSession(ctx context.Context) (*clientSession, error) {
	for {
		client.mu.Lock()
		session := client.session
		fatal := client.fatal
		changed := client.changed
		client.mu.Unlock()

		if fatal != nil {
			return nil, fatal
		}

		if session != nil {
			return session, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

func (client *Client) setSession(session *clientSession) {
	client.mu.Lock()
	client.session = session
	client.notifyChangedLocked()
	client.mu.Unlock()
}

func (client *Client) clearSession(session *clientSession) {
	client.mu.Lock()
	if client.session == session {
		client.session = nil
		client.notifyChangedLocked()
	}
	client.mu.Unlock()
}

func (client *Client) setFatal(err error) {
	client.mu.Lock()
	client.fatal = err
	client.notifyChangedLocked()
	client.mu.Unlock()
}

func (client *Client) notifyChangedLocked() {
	close(client.changed)
	client.changed = make(chan struct{})
}

func isSecurityFailure(err error) bool {
	var certificateError *tls.CertificateVerificationError

	var hostnameError x509.HostnameError

	var authorityError x509.UnknownAuthorityError

	var versionError *quic.VersionNegotiationError

	var transportError *quic.TransportError
	cryptoError := errors.As(err, &transportError) && transportError.ErrorCode >= minimumQUICCryptoErrorCode &&
		transportError.ErrorCode <= maximumQUICCryptoErrorCode

	return errors.As(err, &certificateError) || errors.As(err, &hostnameError) ||
		errors.As(err, &authorityError) || errors.As(err, &versionError) || cryptoError || errors.Is(err, ErrSecurityFailure)
}

// IsSecurityFailure reports failures for which retrying another transport
// would weaken authenticated transport policy.
func IsSecurityFailure(err error) bool {
	return isSecurityFailure(err)
}

func waitBackoff(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func jitter(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}

	span := base / jitterFractionDenominator

	value, err := rand.Int(rand.Reader, big.NewInt(int64(span*2+1)))
	if err != nil {
		return base
	}

	return base - span + time.Duration(value.Int64())
}

func platform() wire.Platform {
	switch runtime.GOOS {
	case "linux":
		return wire.PlatformLinux
	case "darwin":
		return wire.PlatformMacOS
	case "windows":
		return wire.PlatformWindows
	default:
		return 0
	}
}

func architecture() wire.Architecture {
	switch runtime.GOARCH {
	case "amd64":
		return wire.ArchitectureAMD64
	case "arm64":
		return wire.ArchitectureARM64
	default:
		return 0
	}
}
