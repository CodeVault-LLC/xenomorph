// Package config owns immutable QUIC agent runtime configuration loaded at
// startup. It does not own gateway identity or transport authentication.
package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultCertificatePath = "../infrastructure/certs"
	defaultQUICEndpoint    = "localhost:8444"
	defaultHeartbeat       = 15 * time.Second
	defaultOperation       = 10 * time.Second
	defaultHandshake       = 5 * time.Second
	defaultIdleTimeout     = 45 * time.Second
	defaultKeepAlive       = 10 * time.Second
	defaultMaximumBackoff  = 30 * time.Second
	minimumHeartbeat       = 10 * time.Second
	maximumHeartbeat       = 30 * time.Second
)

// Config is the immutable client transport, credential, cadence, and state contract.
type Config struct {
	Environment                 string
	ImplementationVersion       string
	QUICEndpoint                string
	ServerName                  string
	ClientCertificateFile       string
	ClientPrivateKeyFile        string
	CAFile                      string
	CommandVerificationKeyFile  string
	ReplayLedgerFile            string
	ReplayAuthenticationKeyFile string
	HeartbeatInterval           time.Duration
	OperationTimeout            time.Duration
	QUICHandshakeTimeout        time.Duration
	QUICIdleTimeout             time.Duration
	QUICKeepAlive               time.Duration
	ReconnectMinimumBackoff     time.Duration
	ReconnectMaximumBackoff     time.Duration
}

// Load reads and validates agent configuration from environment variables.
func Load() (Config, error) {
	if err := rejectLegacyTransportConfiguration(); err != nil {
		return Config{}, err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("load client config: resolve home directory: %w", err)
	}

	certificatePath := stringFromEnv("AGENT_CERT_PATH", defaultCertificatePath)
	statePath := stringFromEnv("AGENT_STATE_PATH", filepath.Join(home, ".xenomorph"))
	config := Config{
		Environment:                 stringFromEnv("AGENT_ENVIRONMENT", "development"),
		ImplementationVersion:       stringFromEnv("AGENT_IMPLEMENTATION_VERSION", "development"),
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

	if err := config.Validate(); err != nil {
		return Config{}, err
	}

	return config, nil
}

func rejectLegacyTransportConfiguration() error {
	legacyKeys := []string{
		"AGENT_TRANSPORT_MODE",
		"AGENT_GATEWAY_URL",
		"AGENT_HTTP_TIMEOUT",
		"AGENT_HTTP_FALLBACK_UNTIL",
	}
	for _, key := range legacyKeys {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return fmt.Errorf("load client config: %s is no longer supported; the agent transport is QUIC-only", key)
		}
	}

	return nil
}

// Validate enforces secure transport and bounded retry policy.
func (config Config) Validate() error {
	validators := []func() error{
		config.validateVersion,
		config.validateEndpoints,
		config.validatePaths,
		config.validateTiming,
	}
	for _, validate := range validators {
		if err := validate(); err != nil {
			return err
		}
	}

	return nil
}

func (config Config) validateVersion() error {
	if version := strings.TrimSpace(config.ImplementationVersion); version == "" || len(version) > 64 {
		return fmt.Errorf("validate client config: implementation version must contain 1 to 64 bytes")
	}

	return nil
}

func (config Config) validateEndpoints() error {
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

	if config.OperationTimeout <= 0 || config.QUICHandshakeTimeout < time.Second || config.QUICIdleTimeout <= config.HeartbeatInterval {
		return fmt.Errorf("validate client config: operation, handshake, or idle timeout is invalid")
	}

	if config.QUICKeepAlive <= 0 || config.QUICKeepAlive >= config.QUICIdleTimeout/2 {
		return fmt.Errorf("validate client config: QUIC keepalive must be below half the idle timeout")
	}

	if config.ReconnectMinimumBackoff <= 0 || config.ReconnectMaximumBackoff < config.ReconnectMinimumBackoff {
		return fmt.Errorf("validate client config: reconnect backoff range is invalid")
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
		{key: "AGENT_OPERATION_TIMEOUT", fallback: defaultOperation, target: &config.OperationTimeout},
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
