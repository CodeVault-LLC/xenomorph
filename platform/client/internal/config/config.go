// Package config owns the immutable client profile compiled into each agent
// artifact. It does not load production settings from the host, assert gateway
// identity, or authorize transport authentication.
package config

import (
	"fmt"
	"net"
	"runtime"
	"strings"
	"time"
)

const (
	minimumHeartbeat = 10 * time.Second
	maximumHeartbeat = 30 * time.Second
)

// Config is the immutable client transport, credential-reference, cadence, and
// local security-state contract selected when the artifact is generated.
type Config struct {
	Environment                 string
	ImplementationVersion       string
	TargetOS                    string
	TargetArchitecture          string
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

// Load returns the profile compiled into this artifact and fails closed when
// generation did not provide a complete profile.
func Load() (Config, error) {
	config := generatedConfig()
	if !developmentBuild && config.Environment != "production" {
		return Config{}, fmt.Errorf("load compiled client profile: production environment is required")
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("load compiled client profile: %w", err)
	}

	return config, nil
}

// Validate enforces the fixed transport and bounded retry policy.
func (config Config) Validate() error {
	validators := []func() error{
		config.validateVersion,
		config.validateTarget,
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

func (config Config) validateTarget() error {
	if config.TargetOS != runtime.GOOS || config.TargetArchitecture != runtime.GOARCH {
		return fmt.Errorf("validate client profile: target %s/%s does not match this binary", config.TargetOS, config.TargetArchitecture)
	}

	return nil
}

func (config Config) validateVersion() error {
	if strings.TrimSpace(config.Environment) == "" {
		return fmt.Errorf("validate client profile: environment is required")
	}
	if version := strings.TrimSpace(config.ImplementationVersion); version == "" || len(version) > 64 {
		return fmt.Errorf("validate client profile: implementation version must contain 1 to 64 bytes")
	}

	return nil
}

func (config Config) validateEndpoints() error {
	if _, _, err := net.SplitHostPort(config.QUICEndpoint); err != nil {
		return fmt.Errorf("validate client profile: QUIC endpoint requires host and port: %w", err)
	}
	if strings.TrimSpace(config.ServerName) == "" || net.ParseIP(config.ServerName) != nil ||
		(strings.EqualFold(config.ServerName, "localhost") && config.Environment == "production") {
		return fmt.Errorf("validate client profile: TLS server name must be a non-localhost DNS name")
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
			return fmt.Errorf("validate client profile: %s path is required", path.name)
		}
	}

	return nil
}

func (config Config) validateTiming() error {
	if config.HeartbeatInterval < minimumHeartbeat || config.HeartbeatInterval > maximumHeartbeat {
		return fmt.Errorf("validate client profile: heartbeat interval must be between 10s and 30s")
	}
	if config.OperationTimeout <= 0 || config.QUICHandshakeTimeout < time.Second || config.QUICIdleTimeout <= config.HeartbeatInterval {
		return fmt.Errorf("validate client profile: operation, handshake, or idle timeout is invalid")
	}
	if config.QUICKeepAlive <= 0 || config.QUICKeepAlive >= config.QUICIdleTimeout/2 {
		return fmt.Errorf("validate client profile: QUIC keepalive must be below half the idle timeout")
	}
	if config.ReconnectMinimumBackoff <= 0 || config.ReconnectMaximumBackoff < config.ReconnectMinimumBackoff {
		return fmt.Errorf("validate client profile: reconnect backoff range is invalid")
	}

	return nil
}
