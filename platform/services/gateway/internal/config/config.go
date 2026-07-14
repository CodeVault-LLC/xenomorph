// Package config loads and validates gateway runtime configuration from
// environment variables. This package owns the configuration schema and
// default values. All configuration is read once at startup and remains
// immutable for the lifetime of the process.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultNATSURL         string        = "nats://localhost:4222"
	defaultGatewayAddr     string        = ":8443"
	defaultGatewayCertPath string        = "../../infrastructure/certs"
	defaultDashboardAddr   string        = "127.0.0.1:8080"
	defaultOfflineAfter    time.Duration = 30 * time.Second
	defaultSweepInterval   time.Duration = 5 * time.Second
	defaultStatePath       string        = "./data"
	defaultDashboardOrigin string        = "https://localhost:5173"
	defaultCryptoProvider  string        = "go-cryptographic-module"
	defaultFIPSModule      string        = "v1.0.0-c2097c7c"
	defaultCMVPCertificate string        = "CMVP-5247"
	defaultSecurityPolicy  string        = "Go Cryptographic Module v1.0.0 Security Policy"
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
	cfg := GatewayConfig{
		NATSURL:               stringFromEnv("NATS_URL", defaultNATSURL),
		ListenAddr:            stringFromEnv("GATEWAY_ADDR", defaultGatewayAddr),
		CertPath:              certPath,
		DashboardAddr:         stringFromEnv("DASHBOARD_ADDR", defaultDashboardAddr),
		ActivityOfflineAfter:  offlineAfter,
		ActivitySweepInterval: sweepInterval,
		StatePath:             stringFromEnv("GATEWAY_STATE_PATH", defaultStatePath),
		FileOperatorID:        stringFromEnv("FILE_OPERATOR_ID", "internal-website"),
		DashboardOrigin:       stringFromEnv("DASHBOARD_ALLOWED_ORIGIN", defaultDashboardOrigin),
		CommandSigningKeyPath: stringFromEnv("COMMAND_SIGNING_KEY_PATH", filepath.Join(certPath, "command-signing.key")),
		CommandPublicKeyPath:  stringFromEnv("COMMAND_PUBLIC_KEY_PATH", filepath.Join(certPath, "command-signing.pub")),
		CryptoProvider:        stringFromEnv("CRYPTO_PROVIDER", defaultCryptoProvider),
		CryptoModuleVersions:  listFromEnv("CRYPTO_ALLOWED_MODULE_VERSIONS", []string{defaultFIPSModule}),
		CryptoCertificate:     stringFromEnv("CRYPTO_PROVIDER_CERTIFICATE", defaultCMVPCertificate),
		CryptoSecurityPolicy:  stringFromEnv("CRYPTO_SECURITY_POLICY", defaultSecurityPolicy),
		CryptoEnvironments:    listFromEnv("CRYPTO_ALLOWED_ENVIRONMENTS", nil),
	}

	if cfg.ActivityOfflineAfter <= 0 {
		return GatewayConfig{}, fmt.Errorf("ACTIVITY_OFFLINE_AFTER must be positive, got %v", cfg.ActivityOfflineAfter)
	}
	if cfg.ActivitySweepInterval <= 0 {
		return GatewayConfig{}, fmt.Errorf("ACTIVITY_SWEEP_INTERVAL must be positive, got %v", cfg.ActivitySweepInterval)
	}
	return cfg, nil
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
