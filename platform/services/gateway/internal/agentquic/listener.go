package agentquic

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	quic "github.com/quic-go/quic-go"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/identity"
	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

const connectionIDLength = 16

// CommandSource blocks until the gateway has a signed command for one agent.
type CommandSource interface {
	WaitDispatch(context.Context, string) (*commandauth.Envelope, error)
	MarkOutcomeUnknown(string, string) error
}

// Listener owns one long-lived UDP socket and QUIC transport.
type Listener struct {
	config        Config
	sink          IngressSink
	commands      CommandSource
	commandKeyID  string
	metrics       *Metrics
	admission     *handshakeAdmission
	registry      *sessionRegistry
	active        chan struct{}
	mu            sync.Mutex
	localAddress  net.Addr
	transport     *quic.Transport
	quicListener  *quic.Listener
	sessionWaiter sync.WaitGroup
}

// NewListener validates dependencies and constructs a disabled-until-Run listener.
func NewListener(config Config, sink IngressSink, commands CommandSource, commandKeyID string) (*Listener, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	if sink == nil || commands == nil || commandKeyID == "" {
		return nil, fmt.Errorf("create QUIC listener: ingress sink, command source, and command key ID are required")
	}

	metrics := &Metrics{}

	return &Listener{
		config:       config,
		sink:         sink,
		commands:     commands,
		commandKeyID: commandKeyID,
		metrics:      metrics,
		admission:    newHandshakeAdmission(config, metrics),
		registry:     newSessionRegistry(config.MaximumRegistryEntries, metrics),
		active:       make(chan struct{}, config.MaximumActiveSessions),
	}, nil
}

// Run serves authenticated sessions until context cancellation or listener failure.
func (listener *Listener) Run(ctx context.Context) error {
	if listener == nil {
		return fmt.Errorf("run QUIC listener: listener is nil")
	}

	if ctx == nil {
		return fmt.Errorf("run QUIC listener: context is nil")
	}

	udpAddress, err := net.ResolveUDPAddr("udp", listener.config.Address)
	if err != nil {
		return fmt.Errorf("resolve QUIC UDP address: %w", err)
	}

	udpConnection, err := net.ListenUDP("udp", udpAddress)
	if err != nil {
		return fmt.Errorf("open QUIC UDP listener: %w", err)
	}

	if err := listener.serve(ctx, udpConnection); err != nil {
		return err
	}

	return nil
}

// Address returns the bound UDP address after Run has initialized the transport.
func (listener *Listener) Address() net.Addr {
	listener.mu.Lock()
	defer listener.mu.Unlock()

	return listener.localAddress
}

// Metrics returns the listener's low-cardinality transport counters.
func (listener *Listener) Metrics() *Metrics {
	return listener.metrics
}

func (listener *Listener) serve(ctx context.Context, udpConnection *net.UDPConn) error {
	resetKey, tokenKey, err := loadTransportKeys(listener.config)
	if err != nil {
		if closeErr := udpConnection.Close(); closeErr != nil {
			return fmt.Errorf("%v; close QUIC UDP socket: %w", err, closeErr)
		}

		return err
	}

	tlsConfig, err := buildServerTLSConfig(listener.config, listener.admission, listener.metrics)
	if err != nil {
		if closeErr := udpConnection.Close(); closeErr != nil {
			return fmt.Errorf("%v; close QUIC UDP socket: %w", err, closeErr)
		}

		return err
	}

	transport := &quic.Transport{
		Conn:                udpConnection,
		StatelessResetKey:   resetKey,
		TokenGeneratorKey:   tokenKey,
		VerifySourceAddress: func(net.Addr) bool { return listener.config.RequireAddressValidation },
		ConnectionIDLength:  connectionIDLength,
		ConnContext: func(parent context.Context, info *quic.ClientInfo) (context.Context, error) {
			return listener.admission.connectionContext(parent, info.RemoteAddr)
		},
	}
	quicConfig := listener.quicConfig()

	tracer, err := newTransportDiagnosticTracer(listener.config, listener.metrics)
	if err != nil {
		_ = transport.Close()
		return err
	}

	quicConfig.Tracer = tracer

	quicListener, err := transport.Listen(tlsConfig, quicConfig)
	if err != nil {
		if closeErr := transport.Close(); closeErr != nil {
			return fmt.Errorf("start QUIC listener: %v; close transport: %w", err, closeErr)
		}

		return fmt.Errorf("start QUIC listener: %w", err)
	}

	listener.setRuntime(transport, quicListener)

	defer listener.closeRuntime()

	return listener.acceptLoop(ctx, quicListener)
}

func (listener *Listener) quicConfig() *quic.Config {
	return &quic.Config{
		Versions:                       []quic.Version{quic.Version1},
		HandshakeIdleTimeout:           listener.config.HandshakeIdleTimeout,
		MaxIdleTimeout:                 listener.config.MaximumIdleTimeout,
		InitialStreamReceiveWindow:     listener.config.InitialStreamReceiveWindow,
		MaxStreamReceiveWindow:         listener.config.MaximumStreamReceiveWindow,
		InitialConnectionReceiveWindow: listener.config.InitialConnectionWindow,
		MaxConnectionReceiveWindow:     listener.config.MaximumConnectionWindow,
		MaxIncomingStreams:             listener.config.MaximumIncomingStreams,
		MaxIncomingUniStreams:          listener.config.MaximumIncomingUniStreams,
		KeepAlivePeriod:                listener.config.KeepAlivePeriod,
		Allow0RTT:                      false,
		EnableDatagrams:                false,
	}
}

func (listener *Listener) acceptLoop(ctx context.Context, quicListener *quic.Listener) error {
	for {
		connection, err := quicListener.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}

			return fmt.Errorf("accept QUIC connection: %w", err)
		}
		select {
		case listener.active <- struct{}{}:
			listener.sessionWaiter.Add(1)
			go listener.serveConnection(ctx, connection)
		default:
			listener.metrics.overloadRejected.Add(1)

			if err := connection.CloseWithError(quic.ApplicationErrorCode(wire.ApplicationLimit), "session capacity"); err != nil {
				listener.metrics.rejectedFrames.Add(1)
			}
		}
	}
}

func (listener *Listener) serveConnection(ctx context.Context, connection *quic.Conn) {
	defer func() {
		<-listener.active
		listener.sessionWaiter.Done()
	}()

	agentID, err := listener.authenticateConnection(connection)
	if err != nil {
		return
	}

	candidate, err := listener.negotiateSession(ctx, connection, agentID)
	if err != nil {
		return
	}

	previous, err := listener.registry.install(candidate)
	if err != nil {
		closeConnection(connection, wire.ApplicationLimit, "session registry capacity")
		return
	}

	listener.metrics.activeSessions.Add(1)

	if previous != nil {
		previous.replace(candidate.sessionID)
	}

	candidate.run(ctx)
	listener.registry.remove(candidate)
	listener.metrics.activeSessions.Add(-1)
}

func (listener *Listener) authenticateConnection(connection *quic.Conn) (string, error) {
	state := connection.ConnectionState()
	if state.Used0RTT || state.Version != quic.Version1 || state.TLS.NegotiatedProtocol != wire.ALPN ||
		len(state.TLS.PeerCertificates) == 0 {
		listener.metrics.handshakeRejected.Add(1)
		closeConnection(connection, wire.ApplicationAuthState, "authenticated session required")

		return "", fmt.Errorf("authenticate QUIC connection: invalid TLS or transport state")
	}

	authenticatedAgent, err := identity.FromPeerCertificate(state.TLS.PeerCertificates[0])
	if err != nil {
		listener.metrics.certificateFailed.Add(1)
		closeConnection(connection, wire.ApplicationAuthState, "invalid client identity")

		return "", fmt.Errorf("authenticate QUIC connection: %w", err)
	}

	return authenticatedAgent.ID, nil
}

func (listener *Listener) negotiateSession(ctx context.Context, connection *quic.Conn, agentID string) (*session, error) {
	sessionID, err := newSessionID()
	if err != nil {
		closeConnection(connection, wire.ApplicationInternal, "session initialization failed")
		return nil, err
	}

	candidate, err := newSession(listener, connection, agentID, sessionID)
	if err != nil {
		closeConnection(connection, wire.ApplicationInternal, "session initialization failed")
		return nil, err
	}

	if err := candidate.negotiate(ctx); err != nil {
		closeConnection(connection, errorCode(err), "session negotiation failed")
		return nil, err
	}

	return candidate, nil
}

func (listener *Listener) setRuntime(transport *quic.Transport, quicListener *quic.Listener) {
	listener.mu.Lock()
	listener.transport = transport
	listener.quicListener = quicListener
	listener.localAddress = quicListener.Addr()
	listener.mu.Unlock()
}

func (listener *Listener) closeRuntime() {
	listener.mu.Lock()
	quicListener := listener.quicListener
	transport := listener.transport
	listener.quicListener = nil
	listener.transport = nil
	listener.mu.Unlock()

	if quicListener != nil {
		if err := quicListener.Close(); err != nil {
			slog.Debug("QUIC listener close failed", "error", err)
		}
	}

	listener.sessionWaiter.Wait()

	if transport != nil {
		if err := transport.Close(); err != nil {
			slog.Debug("QUIC transport close failed", "error", err)
		}
	}
}

func newSessionID() ([16]byte, error) {
	var sessionID [16]byte
	if _, err := rand.Read(sessionID[:]); err != nil {
		return sessionID, fmt.Errorf("generate QUIC session ID: %w", err)
	}

	if sessionID == [16]byte{} {
		return sessionID, fmt.Errorf("generate QUIC session ID: zero output")
	}

	return sessionID, nil
}

func closeConnection(connection *quic.Conn, code wire.ApplicationErrorCode, description string) {
	if connection != nil {
		if err := connection.CloseWithError(quic.ApplicationErrorCode(code), description); err != nil {
			slog.Debug("QUIC connection close failed", "code", code, "error", err)
		}
	}
}

func errorCode(err error) wire.ApplicationErrorCode {
	switch {
	case errors.Is(err, wire.ErrLimit):
		return wire.ApplicationLimit
	case errors.Is(err, wire.ErrUnexpectedMessage):
		return wire.ApplicationUnexpectedMessage
	case errors.Is(err, wire.ErrReplay):
		return wire.ApplicationReplay
	default:
		return wire.ApplicationFrameEncoding
	}
}
