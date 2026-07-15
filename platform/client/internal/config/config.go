// Package config owns immutable agent runtime configuration loaded at startup.
// It does not own gateway identity, transport authentication, or fallback
// decisions after a security failure; those remain enforced by the transport.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultCertificatePath = "../infrastructure/certs"
	defaultGatewayURL      = "https://localhost:8443"
	defaultQUICEndpoint    = "localhost:8444"
	defaultHeartbeat       = 15 * time.Second
	defaultHTTPTimeout     = 10 * time.Second
	defaultHandshake       = 5 * time.Second
	defaultIdleTimeout     = 45 * time.Second
	defaultKeepAlive       = 10 * time.Second
	defaultMaximumBackoff  = 30 * time.Second
	minimumHeartbeat       = 10 * time.Second
	maximumHeartbeat       = 30 * time.Second
)

// TransportMode controls the explicit rollout authority for agent message families.
type TransportMode string

const (
	// TransportHTTP uses only the bounded legacy HTTPS agent plane.
	TransportHTTP TransportMode = "http"
	// TransportQUIC requires QUIC and never downgrades to HTTPS.
	TransportQUIC TransportMode = "quic"
	// TransportQUICFirst permits an expiring network-only HTTPS rollout fallback.
	TransportQUICFirst TransportMode = "quic-first"
)

// Config is the immutable client transport, credential, cadence, and state contract.
type Config struct {
	Environment                 string
	ImplementationVersion       string
	TransportMode               TransportMode
	GatewayURL                  string
	QUICEndpoint                string
	ServerName                  string
	ClientCertificateFile       string
	ClientPrivateKeyFile        string
	CAFile                      string
	CommandVerificationKeyFile  string
	ReplayLedgerFile            string
	ReplayAuthenticationKeyFile string
	HeartbeatInterval           time.Duration
	HTTPTimeout                 time.Duration
	QUICHandshakeTimeout        time.Duration
	QUICIdleTimeout             time.Duration
	QUICKeepAlive               time.Duration
	ReconnectMinimumBackoff     time.Duration
	ReconnectMaximumBackoff     time.Duration
	HTTPFallbackUntil           time.Time
}

// Load reads and validates agent configuration from environment variables.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("load client config: resolve home directory: %w", err)
	}
	certificatePath := stringFromEnv("AGENT_CERT_PATH", defaultCertificatePath)
	statePath := stringFromEnv("AGENT_STATE_PATH", filepath.Join(home, ".xenomorph"))
	mode := TransportMode(stringFromEnv("AGENT_TRANSPORT_MODE", string(TransportHTTP)))
	config := Config{
		Environment:                 stringFromEnv("AGENT_ENVIRONMENT", "development"),
		ImplementationVersion:       stringFromEnv("AGENT_IMPLEMENTATION_VERSION", "development"),
		TransportMode:               mode,
		GatewayURL:                  stringFromEnv("AGENT_GATEWAY_URL", defaultGatewayURL),
		QUICEndpoint:                stringFromEnv("AGENT_QUIC_ENDPOINT", defaultQUICEndpoint),
		ServerName:                  stringFromEnv("AGENT_TLS_SERVER_NAME", "localhost"),
		ClientCertificateFile:       stringFromEnv("AGENT_CLIENT_CERT_FILE", filepath.Join(certificatePath, "client.crt")),
		ClientPrivateKeyFile:        stringFromEnv("AGENT_CLIENT_KEY_FILE", filepath.Join(certificatePath, "client.key")),
		CAFile:                      stringFromEnv("AGENT_CA_FILE", filepath.Join(certificatePath, "ca.crt")),
		CommandVerificationKeyFile:  stringFromEnv("AGENT_COMMAND_KEY_FILE", filepath.Join(certificatePath, "command-signing.pub")),
		ReplayLedgerFile:            stringFromEnv("AGENT_REPLAY_LEDGER_FILE", filepath.Join(statePath, "command-replay-ledger.json")),
		ReplayAuthenticationKeyFile: stringFromEnv("AGENT_REPLAY_AUTH_KEY_FILE", filepath.Join(statePath, "command-replay.key")),
	}
	if err := loadDurations(&config); err != nil {
		return Config{}, err
	}
	if raw := strings.TrimSpace(os.Getenv("AGENT_HTTP_FALLBACK_UNTIL")); raw != "" {
		fallbackUntil, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return Config{}, fmt.Errorf("AGENT_HTTP_FALLBACK_UNTIL: invalid RFC3339 time %q: %w", raw, err)
		}
		config.HTTPFallbackUntil = fallbackUntil.UTC()
	}
	if err := config.Validate(time.Now().UTC()); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate enforces secure transport and bounded retry policy.
func (config Config) Validate(now time.Time) error {
	validators := []func() error{
		config.validateModeAndVersion,
		config.validateEndpoints,
		config.validatePaths,
		config.validateTiming,
		func() error { return config.validateFallback(now) },
	}
	for _, validate := range validators {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

func (config Config) validateModeAndVersion() error {
	if config.TransportMode != TransportHTTP && config.TransportMode != TransportQUIC && config.TransportMode != TransportQUICFirst {
		return fmt.Errorf("validate client config: unsupported transport mode %q", config.TransportMode)
	}
	if version := strings.TrimSpace(config.ImplementationVersion); version == "" || len(version) > 64 {
		return fmt.Errorf("validate client config: implementation version must contain 1 to 64 bytes")
	}
	return nil
}

func (config Config) validateEndpoints() error {
	gatewayURL, err := url.Parse(config.GatewayURL)
	if err != nil || gatewayURL.Scheme != "https" || gatewayURL.Host == "" || gatewayURL.User != nil {
		return fmt.Errorf("validate client config: gateway URL must be an HTTPS origin without user information")
	}
	if _, _, err := net.SplitHostPort(config.QUICEndpoint); err != nil {
		return fmt.Errorf("validate client config: QUIC endpoint requires host and port: %w", err)
	}
	if strings.TrimSpace(config.ServerName) == "" || net.ParseIP(config.ServerName) != nil {
		return fmt.Errorf("validate client config: TLS server name must be an explicit DNS name")
	}
	if config.Environment == "production" && strings.EqualFold(config.ServerName, "localhost") {
		return fmt.Errorf("validate client config: production TLS server name cannot be localhost")
	}
	return nil
}

func (config Config) validatePaths() error {
	paths := []struct{ name, value string }{
		{name: "client certificate", value: config.ClientCertificateFile},
		{name: "client private key", value: config.ClientPrivateKeyFile},
		{name: "CA", value: config.CAFile},
		{name: "command verification key", value: config.CommandVerificationKeyFile},
		{name: "replay ledger", value: config.ReplayLedgerFile},
		{name: "replay authentication key", value: config.ReplayAuthenticationKeyFile},
	}
	for _, path := range paths {
		if strings.TrimSpace(path.value) == "" {
			return fmt.Errorf("validate client config: %s path is required", path.name)
		}
	}
	return nil
}

func (config Config) validateTiming() error {
	if config.HeartbeatInterval < minimumHeartbeat || config.HeartbeatInterval > maximumHeartbeat {
		return fmt.Errorf("validate client config: heartbeat interval must be between 10s and 30s")
	}
	if config.HTTPTimeout <= 0 || config.QUICHandshakeTimeout < time.Second || config.QUICIdleTimeout <= config.HeartbeatInterval {
		return fmt.Errorf("validate client config: HTTP, handshake, or idle timeout is invalid")
	}
	if config.QUICKeepAlive <= 0 || config.QUICKeepAlive >= config.QUICIdleTimeout/2 {
		return fmt.Errorf("validate client config: QUIC keepalive must be below half the idle timeout")
	}
	if config.ReconnectMinimumBackoff <= 0 || config.ReconnectMaximumBackoff < config.ReconnectMinimumBackoff {
		return fmt.Errorf("validate client config: reconnect backoff range is invalid")
	}
	return nil
}

func (config Config) validateFallback(now time.Time) error {
	if config.TransportMode == TransportQUICFirst && (config.HTTPFallbackUntil.IsZero() || !config.HTTPFallbackUntil.After(now)) {
		return fmt.Errorf("validate client config: quic-first requires a future HTTP fallback expiry")
	}
	return nil
}

func loadDurations(config *Config) error {
	values := []struct {
		key      string
		fallback time.Duration
		target   *time.Duration
	}{
		{key: "AGENT_HEARTBEAT_INTERVAL", fallback: defaultHeartbeat, target: &config.HeartbeatInterval},
		{key: "AGENT_HTTP_TIMEOUT", fallback: defaultHTTPTimeout, target: &config.HTTPTimeout},
		{key: "AGENT_QUIC_HANDSHAKE_TIMEOUT", fallback: defaultHandshake, target: &config.QUICHandshakeTimeout},
		{key: "AGENT_QUIC_IDLE_TIMEOUT", fallback: defaultIdleTimeout, target: &config.QUICIdleTimeout},
		{key: "AGENT_QUIC_KEEPALIVE", fallback: defaultKeepAlive, target: &config.QUICKeepAlive},
		{key: "AGENT_RECONNECT_MIN_BACKOFF", fallback: time.Second, target: &config.ReconnectMinimumBackoff},
		{key: "AGENT_RECONNECT_MAX_BACKOFF", fallback: defaultMaximumBackoff, target: &config.ReconnectMaximumBackoff},
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

func stringFromEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func durationFromEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q: %w", key, raw, err)
	}
	return value, nil
}
