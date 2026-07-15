package agentquic

import (
	"fmt"
	"strings"
	"time"
)

const (
	minimumHandshakeTimeout = time.Second
	minimumIdleTimeout      = 10 * time.Second
	minimumControlTimeout   = time.Second
	minimumReceiveWindow    = 64 << 10
	maximumReceiveWindow    = 64 << 20
	minimumDiagnosticBytes  = 1 << 20
	maximumDiagnosticBytes  = 16 << 30
	maximumDiagnosticFiles  = 1024
)

// Config is the immutable gateway QUIC listener and resource profile.
type Config struct {
	Address                      string
	ServerCertificateFile        string
	ServerPrivateKeyFile         string
	ClientCAFile                 string
	StatelessResetKeyFile        string
	TokenGeneratorKeyFile        string
	HandshakeIdleTimeout         time.Duration
	MaximumIdleTimeout           time.Duration
	KeepAlivePeriod              time.Duration
	ControlStreamTimeout         time.Duration
	TransferStreamIOTimeout      time.Duration
	MediaFrameTimeout            time.Duration
	DrainTimeout                 time.Duration
	InitialStreamReceiveWindow   uint64
	MaximumStreamReceiveWindow   uint64
	InitialConnectionWindow      uint64
	MaximumConnectionWindow      uint64
	MaximumIncomingStreams       int64
	MaximumIncomingUniStreams    int64
	MaximumIncompleteHandshakes  int
	MaximumActiveSessions        int
	MaximumRegistryEntries       int
	SourcePrefixRatePerSecond    float64
	SourcePrefixBurst            int
	MaximumSourcePrefixes        int
	MaximumClientChainBytes      int
	MaximumClientChainDepth      int
	RequireAddressValidation     bool
	ConcurrentTransferStreams    uint16
	HeartbeatInterval            time.Duration
	EventFrameMaximum            uint32
	EnableTransportDiagnostics   bool
	TransportDiagnosticDirectory string
	TransportDiagnosticFileLimit uint32
	TransportDiagnosticByteLimit uint64
}

// Validate rejects an incomplete or internally inconsistent production profile.
func (config Config) Validate() error {
	validators := []func() error{
		config.validateRequiredValues,
		config.validateTimeouts,
		func() error { return validateReceiveWindows(config) },
		config.validateConnectionBounds,
		config.validateAdmissionBounds,
		config.validateApplicationBounds,
		config.validateDiagnostics,
	}
	for _, validate := range validators {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

func (config Config) validateRequiredValues() error {
	for name, value := range map[string]string{
		"address": config.Address, "server certificate": config.ServerCertificateFile,
		"server private key": config.ServerPrivateKeyFile, "client CA": config.ClientCAFile,
		"stateless reset key": config.StatelessResetKeyFile, "token generator key": config.TokenGeneratorKeyFile,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("validate QUIC config: %s file is required", name)
		}
	}
	return nil
}

func (config Config) validateTimeouts() error {
	if config.HandshakeIdleTimeout < minimumHandshakeTimeout || config.MaximumIdleTimeout < minimumIdleTimeout ||
		config.ControlStreamTimeout < minimumControlTimeout || config.DrainTimeout <= 0 {
		return fmt.Errorf("validate QUIC config: timeout outside secure minimum")
	}
	if err := config.validateOperationTimeouts(); err != nil {
		return err
	}
	return config.validateLivenessTimeouts()
}

func (config Config) validateLivenessTimeouts() error {
	if config.KeepAlivePeriod <= 0 || config.KeepAlivePeriod >= config.MaximumIdleTimeout/2 {
		return fmt.Errorf("validate QUIC config: keepalive must be positive and below half the idle timeout")
	}
	if config.HeartbeatInterval < 10*time.Second || config.HeartbeatInterval > 30*time.Second ||
		config.MaximumIdleTimeout <= config.HeartbeatInterval {
		return fmt.Errorf("validate QUIC config: heartbeat and idle policy are inconsistent")
	}
	return nil
}

func (config Config) validateOperationTimeouts() error {
	if config.TransferStreamIOTimeout < minimumControlTimeout || config.MediaFrameTimeout < minimumControlTimeout {
		return fmt.Errorf("validate QUIC config: operation timeout outside secure minimum")
	}
	return nil
}

func (config Config) validateConnectionBounds() error {
	if config.MaximumIncomingStreams < 1 || config.MaximumIncomingUniStreams < 2 ||
		config.MaximumIncompleteHandshakes < 1 || config.MaximumActiveSessions < 1 {
		return fmt.Errorf("validate QUIC config: stream, handshake, or session bound is invalid")
	}
	return nil
}

func (config Config) validateAdmissionBounds() error {
	if config.MaximumRegistryEntries < config.MaximumActiveSessions {
		return fmt.Errorf("validate QUIC config: registry bound is below session capacity")
	}
	if config.SourcePrefixRatePerSecond <= 0 || config.SourcePrefixBurst < 1 || config.MaximumSourcePrefixes < 1 {
		return fmt.Errorf("validate QUIC config: source-prefix admission bound is invalid")
	}
	if config.MaximumClientChainBytes < 1024 || config.MaximumClientChainDepth < 1 {
		return fmt.Errorf("validate QUIC config: client certificate-chain bound is invalid")
	}
	return nil
}

func (config Config) validateApplicationBounds() error {
	if int64(config.ConcurrentTransferStreams) > config.MaximumIncomingStreams || config.EventFrameMaximum < 4096 {
		return fmt.Errorf("validate QUIC config: application stream or frame bound is invalid")
	}
	return nil
}

func (config Config) validateDiagnostics() error {
	if !config.EnableTransportDiagnostics {
		return nil
	}
	if strings.TrimSpace(config.TransportDiagnosticDirectory) == "" {
		return fmt.Errorf("validate QUIC config: diagnostic directory is required when diagnostics are enabled")
	}
	if config.TransportDiagnosticFileLimit == 0 || config.TransportDiagnosticFileLimit > maximumDiagnosticFiles ||
		config.TransportDiagnosticByteLimit < minimumDiagnosticBytes || config.TransportDiagnosticByteLimit > maximumDiagnosticBytes {
		return fmt.Errorf("validate QUIC config: diagnostic retention bound is invalid")
	}
	return nil
}

func validateReceiveWindows(config Config) error {
	windows := []uint64{
		config.InitialStreamReceiveWindow,
		config.MaximumStreamReceiveWindow,
		config.InitialConnectionWindow,
		config.MaximumConnectionWindow,
	}
	for _, window := range windows {
		if window < minimumReceiveWindow || window > maximumReceiveWindow {
			return fmt.Errorf("validate QUIC config: receive window outside bounded range")
		}
	}
	if config.InitialStreamReceiveWindow > config.MaximumStreamReceiveWindow ||
		config.InitialConnectionWindow > config.MaximumConnectionWindow ||
		config.MaximumStreamReceiveWindow > config.MaximumConnectionWindow {
		return fmt.Errorf("validate QUIC config: receive-window ordering is invalid")
	}
	return nil
}
