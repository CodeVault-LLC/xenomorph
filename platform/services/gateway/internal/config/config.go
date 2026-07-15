// Package config loads and validates gateway runtime configuration from
// environment variables. This package owns the configuration schema and
// default values. All configuration is read once at startup and remains
// immutable for the lifetime of the process.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/agentquic"
)

const (
	defaultNATSURL                 string        = "nats://localhost:4222"
	defaultGatewayAddr             string        = ":8443"
	defaultGatewayCertPath         string        = "../../infrastructure/certs"
	defaultDashboardAddr           string        = "127.0.0.1:8080"
	defaultOfflineAfter            time.Duration = 30 * time.Second
	defaultSweepInterval           time.Duration = 5 * time.Second
	defaultStatePath               string        = "./data"
	defaultDashboardOrigin         string        = "https://localhost:5173"
	defaultCryptoProvider          string        = "go-cryptographic-module"
	defaultFIPSModule              string        = "v1.0.0-c2097c7c"
	defaultCMVPCertificate         string        = "CMVP-5247"
	defaultSecurityPolicy          string        = "Go Cryptographic Module v1.0.0 Security Policy"
	defaultQUICAddress             string        = ":8444"
	defaultQUICHandshake                         = 5 * time.Second
	defaultQUICIdle                              = 45 * time.Second
	defaultQUICKeepAlive                         = 10 * time.Second
	defaultQUICControl                           = 5 * time.Second
	defaultQUICTransferIO                        = 60 * time.Second
	defaultQUICMediaFrame                        = 10 * time.Second
	defaultQUICDrain                             = 5 * time.Second
	defaultQUICHeartbeat                         = 15 * time.Second
	defaultInitialStreamWindow     uint64        = 256 << 10
	defaultMaximumStreamWindow     uint64        = 4 << 20
	defaultInitialConnectionWindow uint64        = 512 << 10
	defaultMaximumConnectionWindow uint64        = 16 << 20
	defaultMaximumBidiStreams      int64         = 8
	defaultMaximumUniStreams       int64         = 4
	defaultMaximumHandshakes                     = 128
	defaultMaximumSessions                       = 1_000
	defaultMaximumRegistryEntries                = 2_000
	defaultSourceRate                            = 5.0
	defaultSourceBurst                           = 20
	defaultMaximumSourcePrefixes                 = 4_096
	defaultMaximumClientChainBytes               = 32 << 10
	defaultMaximumClientChainDepth               = 5
	defaultMaximumTransfers        uint64        = 4
	defaultEventFrameMaximum       uint64        = 256 << 10
	defaultDiagnosticFileLimit     uint64        = 8
	defaultDiagnosticByteLimit     uint64        = 64 << 20
)

// GatewayConfig controls runtime wiring for gateway and dashboard services.
// Cryptographic operating environments require an explicit deployment
// allowlist; other fields have safe development defaults.
type GatewayConfig struct {
	NATSURL               string
	ListenAddr            string
	CertPath              string
	DashboardAddr         string
	ActivityOfflineAfter  time.Duration
	ActivitySweepInterval time.Duration
	StatePath             string
	FileOperatorID        string
	DashboardOrigin       string
	CommandSigningKeyPath string
	CommandPublicKeyPath  string
	CryptoProvider        string
	CryptoModuleVersions  []string
	CryptoCertificate     string
	CryptoSecurityPolicy  string
	CryptoEnvironments    []string
	AgentQUICEnabled      bool
	AgentQUIC             agentquic.Config
}

// LoadFromEnv reads configuration from environment variables with safe
// defaults. Returns an error when a required variable is unset or when a
// duration variable cannot be parsed.
//
// Duration variables (parsed via time.ParseDuration):
//   - ACTIVITY_OFFLINE_AFTER (default: 30s)
//   - ACTIVITY_SWEEP_INTERVAL (default: 5s)
//
// Duration values must be positive (>0).
//
// String variables:
//   - NATS_URL (default: nats://localhost:4222)
//   - GATEWAY_ADDR (default: :8443)
//   - GATEWAY_CERT_PATH (default: ../../infrastructure/certs)
//   - DASHBOARD_ADDR (default: 127.0.0.1:8080)
//   - GATEWAY_STATE_PATH (default: ./data)
//   - FILE_OPERATOR_ID (audit source label; default: internal-website)
//   - DASHBOARD_ALLOWED_ORIGIN (default: https://localhost:5173)
//   - COMMAND_SIGNING_KEY_PATH (default: <GATEWAY_CERT_PATH>/command-signing.key)
//   - COMMAND_PUBLIC_KEY_PATH (default: <GATEWAY_CERT_PATH>/command-signing.pub)
//   - CRYPTO_PROVIDER (default: go-cryptographic-module)
//   - CRYPTO_ALLOWED_MODULE_VERSIONS (default: v1.0.0-c2097c7c)
//   - CRYPTO_PROVIDER_CERTIFICATE (default: CMVP-5247)
//   - CRYPTO_SECURITY_POLICY (pinned provider policy identity)
//   - CRYPTO_ALLOWED_ENVIRONMENTS (required comma-separated GOOS/GOARCH allowlist)
func LoadFromEnv() (GatewayConfig, error) {
	offlineAfter, err := durationFromEnv("ACTIVITY_OFFLINE_AFTER", defaultOfflineAfter)
	if err != nil {
		return GatewayConfig{}, err
	}

	sweepInterval, err := durationFromEnv("ACTIVITY_SWEEP_INTERVAL", defaultSweepInterval)
	if err != nil {
		return GatewayConfig{}, err
	}
	certPath := stringFromEnv("GATEWAY_CERT_PATH", defaultGatewayCertPath)
	statePath := stringFromEnv("GATEWAY_STATE_PATH", defaultStatePath)
	agentQUIC, agentQUICEnabled, err := loadAgentQUICConfig(certPath, statePath)
	if err != nil {
		return GatewayConfig{}, err
	}

	cfg := GatewayConfig{
		NATSURL:               stringFromEnv("NATS_URL", defaultNATSURL),
		ListenAddr:            stringFromEnv("GATEWAY_ADDR", defaultGatewayAddr),
		CertPath:              certPath,
		DashboardAddr:         stringFromEnv("DASHBOARD_ADDR", defaultDashboardAddr),
		ActivityOfflineAfter:  offlineAfter,
		ActivitySweepInterval: sweepInterval,
		StatePath:             statePath,
		FileOperatorID:        stringFromEnv("FILE_OPERATOR_ID", "internal-website"),
		DashboardOrigin:       stringFromEnv("DASHBOARD_ALLOWED_ORIGIN", defaultDashboardOrigin),
		CommandSigningKeyPath: stringFromEnv("COMMAND_SIGNING_KEY_PATH", filepath.Join(certPath, "command-signing.key")),
		CommandPublicKeyPath:  stringFromEnv("COMMAND_PUBLIC_KEY_PATH", filepath.Join(certPath, "command-signing.pub")),
		CryptoProvider:        stringFromEnv("CRYPTO_PROVIDER", defaultCryptoProvider),
		CryptoModuleVersions:  listFromEnv("CRYPTO_ALLOWED_MODULE_VERSIONS", []string{defaultFIPSModule}),
		CryptoCertificate:     stringFromEnv("CRYPTO_PROVIDER_CERTIFICATE", defaultCMVPCertificate),
		CryptoSecurityPolicy:  stringFromEnv("CRYPTO_SECURITY_POLICY", defaultSecurityPolicy),
		CryptoEnvironments:    listFromEnv("CRYPTO_ALLOWED_ENVIRONMENTS", nil),
		AgentQUICEnabled:      agentQUICEnabled,
		AgentQUIC:             agentQUIC,
	}

	if cfg.ActivityOfflineAfter <= 0 {
		return GatewayConfig{}, fmt.Errorf("ACTIVITY_OFFLINE_AFTER must be positive, got %v", cfg.ActivityOfflineAfter)
	}
	if cfg.ActivitySweepInterval <= 0 {
		return GatewayConfig{}, fmt.Errorf("ACTIVITY_SWEEP_INTERVAL must be positive, got %v", cfg.ActivitySweepInterval)
	}
	if err := cfg.AgentQUIC.Validate(); err != nil {
		return GatewayConfig{}, err
	}
	return cfg, nil
}

func loadAgentQUICConfig(certPath, statePath string) (agentquic.Config, bool, error) {
	enabled, err := boolFromEnv("AGENT_QUIC_ENABLED", false)
	if err != nil {
		return agentquic.Config{}, false, err
	}
	retry, err := boolFromEnv("AGENT_QUIC_REQUIRE_RETRY", false)
	if err != nil {
		return agentquic.Config{}, false, err
	}
	diagnostics, err := boolFromEnv("AGENT_QUIC_DIAGNOSTICS_ENABLED", false)
	if err != nil {
		return agentquic.Config{}, false, err
	}
	config := agentquic.Config{
		Address:                      stringFromEnv("AGENT_QUIC_ADDR", defaultQUICAddress),
		ServerCertificateFile:        stringFromEnv("AGENT_QUIC_SERVER_CERT_FILE", filepath.Join(certPath, "server.crt")),
		ServerPrivateKeyFile:         stringFromEnv("AGENT_QUIC_SERVER_KEY_FILE", filepath.Join(certPath, "server.key")),
		ClientCAFile:                 stringFromEnv("AGENT_QUIC_CLIENT_CA_FILE", filepath.Join(certPath, "ca.crt")),
		StatelessResetKeyFile:        stringFromEnv("AGENT_QUIC_STATELESS_RESET_KEY_FILE", filepath.Join(statePath, "secrets", "quic-reset.key")),
		TokenGeneratorKeyFile:        stringFromEnv("AGENT_QUIC_TOKEN_KEY_FILE", filepath.Join(statePath, "secrets", "quic-token.key")),
		RequireAddressValidation:     retry,
		EnableTransportDiagnostics:   diagnostics,
		TransportDiagnosticDirectory: stringFromEnv("AGENT_QUIC_DIAGNOSTIC_PATH", filepath.Join(statePath, "qlog")),
	}
	if err := loadAgentQUICTimeouts(&config); err != nil {
		return agentquic.Config{}, false, err
	}
	if err := loadAgentQUICLimits(&config); err != nil {
		return agentquic.Config{}, false, err
	}
	return config, enabled, nil
}

func loadAgentQUICTimeouts(config *agentquic.Config) error {
	values := []struct {
		key      string
		fallback time.Duration
		target   *time.Duration
	}{
		{key: "AGENT_QUIC_HANDSHAKE_TIMEOUT", fallback: defaultQUICHandshake, target: &config.HandshakeIdleTimeout},
		{key: "AGENT_QUIC_IDLE_TIMEOUT", fallback: defaultQUICIdle, target: &config.MaximumIdleTimeout},
		{key: "AGENT_QUIC_KEEPALIVE", fallback: defaultQUICKeepAlive, target: &config.KeepAlivePeriod},
		{key: "AGENT_QUIC_CONTROL_TIMEOUT", fallback: defaultQUICControl, target: &config.ControlStreamTimeout},
		{key: "AGENT_QUIC_TRANSFER_IO_TIMEOUT", fallback: defaultQUICTransferIO, target: &config.TransferStreamIOTimeout},
		{key: "AGENT_QUIC_MEDIA_FRAME_TIMEOUT", fallback: defaultQUICMediaFrame, target: &config.MediaFrameTimeout},
		{key: "AGENT_QUIC_DRAIN_TIMEOUT", fallback: defaultQUICDrain, target: &config.DrainTimeout},
		{key: "AGENT_QUIC_HEARTBEAT_INTERVAL", fallback: defaultQUICHeartbeat, target: &config.HeartbeatInterval},
	}
	for _, value := range values {
		parsed, err := durationFromEnv(value.key, value.fallback)
		if err != nil {
			return err
		}
		*value.target = parsed
	}
	return nil
}

func loadAgentQUICLimits(config *agentquic.Config) error {
	loaders := []func(*agentquic.Config) error{
		loadAgentQUICWindows,
		loadAgentQUICSessions,
		loadAgentQUICAdmission,
		loadAgentQUICApplicationLimits,
		loadAgentQUICDiagnosticLimits,
	}
	for _, load := range loaders {
		if err := load(config); err != nil {
			return err
		}
	}
	return nil
}

func loadAgentQUICWindows(config *agentquic.Config) error {
	var err error
	if config.InitialStreamReceiveWindow, err = uint64FromEnv("AGENT_QUIC_INITIAL_STREAM_WINDOW", defaultInitialStreamWindow); err != nil {
		return err
	}
	if config.MaximumStreamReceiveWindow, err = uint64FromEnv("AGENT_QUIC_MAX_STREAM_WINDOW", defaultMaximumStreamWindow); err != nil {
		return err
	}
	if config.InitialConnectionWindow, err = uint64FromEnv("AGENT_QUIC_INITIAL_CONNECTION_WINDOW", defaultInitialConnectionWindow); err != nil {
		return err
	}
	if config.MaximumConnectionWindow, err = uint64FromEnv("AGENT_QUIC_MAX_CONNECTION_WINDOW", defaultMaximumConnectionWindow); err != nil {
		return err
	}
	return nil
}

func loadAgentQUICSessions(config *agentquic.Config) error {
	var err error
	if config.MaximumIncomingStreams, err = int64FromEnv("AGENT_QUIC_MAX_BIDI_STREAMS", defaultMaximumBidiStreams); err != nil {
		return err
	}
	if config.MaximumIncomingUniStreams, err = int64FromEnv("AGENT_QUIC_MAX_UNI_STREAMS", defaultMaximumUniStreams); err != nil {
		return err
	}
	if config.MaximumIncompleteHandshakes, err = intFromEnv("AGENT_QUIC_MAX_HANDSHAKES", defaultMaximumHandshakes); err != nil {
		return err
	}
	if config.MaximumActiveSessions, err = intFromEnv("AGENT_QUIC_MAX_SESSIONS", defaultMaximumSessions); err != nil {
		return err
	}
	if config.MaximumRegistryEntries, err = intFromEnv("AGENT_QUIC_MAX_REGISTRY_ENTRIES", defaultMaximumRegistryEntries); err != nil {
		return err
	}
	return nil
}

func loadAgentQUICAdmission(config *agentquic.Config) error {
	var err error
	if config.SourcePrefixRatePerSecond, err = float64FromEnv("AGENT_QUIC_SOURCE_RATE", defaultSourceRate); err != nil {
		return err
	}
	if config.SourcePrefixBurst, err = intFromEnv("AGENT_QUIC_SOURCE_BURST", defaultSourceBurst); err != nil {
		return err
	}
	if config.MaximumSourcePrefixes, err = intFromEnv("AGENT_QUIC_MAX_SOURCE_PREFIXES", defaultMaximumSourcePrefixes); err != nil {
		return err
	}
	if config.MaximumClientChainBytes, err = intFromEnv("AGENT_QUIC_MAX_CLIENT_CHAIN_BYTES", defaultMaximumClientChainBytes); err != nil {
		return err
	}
	if config.MaximumClientChainDepth, err = intFromEnv("AGENT_QUIC_MAX_CLIENT_CHAIN_DEPTH", defaultMaximumClientChainDepth); err != nil {
		return err
	}
	return nil
}

func loadAgentQUICApplicationLimits(config *agentquic.Config) error {
	transferStreams, err := uint64FromEnv("AGENT_QUIC_MAX_TRANSFERS", defaultMaximumTransfers)
	if err != nil || transferStreams > 64 {
		return fmt.Errorf("AGENT_QUIC_MAX_TRANSFERS must be an integer in [0,64]")
	}
	config.ConcurrentTransferStreams = uint16(transferStreams)
	eventMaximum, err := uint64FromEnv("AGENT_QUIC_EVENT_FRAME_MAX", defaultEventFrameMaximum)
	if err != nil || eventMaximum > uint64(^uint32(0)) {
		return fmt.Errorf("AGENT_QUIC_EVENT_FRAME_MAX is outside uint32 range")
	}
	config.EventFrameMaximum = uint32(eventMaximum)
	return nil
}

func loadAgentQUICDiagnosticLimits(config *agentquic.Config) error {
	fileLimit, err := uint64FromEnv("AGENT_QUIC_DIAGNOSTIC_FILE_LIMIT", defaultDiagnosticFileLimit)
	if err != nil || fileLimit > uint64(^uint32(0)) {
		return fmt.Errorf("AGENT_QUIC_DIAGNOSTIC_FILE_LIMIT is outside uint32 range")
	}
	config.TransportDiagnosticFileLimit = uint32(fileLimit)
	byteLimit, err := uint64FromEnv("AGENT_QUIC_DIAGNOSTIC_BYTE_LIMIT", defaultDiagnosticByteLimit)
	if err != nil {
		return err
	}
	config.TransportDiagnosticByteLimit = byteLimit
	return nil
}

// listFromEnv reads a comma-separated allowlist, trims each value, and
// discards empty entries. The fallback is copied so configuration remains
// immutable to callers.
func listFromEnv(key string, fallback []string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return append([]string(nil), fallback...)
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			values = append(values, value)
		}
	}
	return values
}

// stringFromEnv reads a string from the environment with a fallback default.
// Empty strings after trimming whitespace trigger the fallback.
func stringFromEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

// durationFromEnv reads and parses a duration from the environment.
// Returns the fallback when the variable is unset. Returns an error when
// the value cannot be parsed via time.ParseDuration.
func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q: %w", key, raw, err)
	}
	return parsed, nil
}

func boolFromEnv(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s: invalid boolean %q: %w", key, raw, err)
	}
	return value, nil
}

func uint64FromEnv(key string, fallback uint64) (uint64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid unsigned integer %q: %w", key, raw, err)
	}
	return value, nil
}

func int64FromEnv(key string, fallback int64) (int64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q: %w", key, raw, err)
	}
	return value, nil
}

func intFromEnv(key string, fallback int) (int, error) {
	value, err := int64FromEnv(key, int64(fallback))
	if err != nil {
		return 0, err
	}
	converted := int(value)
	if int64(converted) != value {
		return 0, fmt.Errorf("%s: integer is outside platform range", key)
	}
	return converted, nil
}

func float64FromEnv(key string, fallback float64) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid number %q: %w", key, raw, err)
	}
	return value, nil
}
