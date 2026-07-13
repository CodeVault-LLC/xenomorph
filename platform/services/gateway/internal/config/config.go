// Package config loads and validates gateway runtime configuration from
// environment variables. This package owns the configuration schema and
// default values. All configuration is read once at startup and remains
// immutable for the lifetime of the process.
package config

import (
	"fmt"
	"os"
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
)

// GatewayConfig controls runtime wiring for ingress and outbound providers.
// Every field has a safe default except provider-specific credentials, which
// return an error when required but unset.
type GatewayConfig struct {
	NATSURL               string
	ListenAddr            string
	CertPath              string
	DashboardAddr         string
	NotifyProviders       []string
	DiscordBotToken       string
	DiscordGuildID        string
	DiscordAPIBaseURL     string
	ActivityOfflineAfter  time.Duration
	ActivitySweepInterval time.Duration
	StatePath             string
	FileOperatorID        string
	DashboardOrigin       string
}

// LoadFromEnv reads configuration from environment variables with safe
// defaults. Returns an error when a required variable is unset or when a
// duration variable cannot be parsed.
//
// Required variables (no default, must be set when the corresponding
// provider is enabled):
//   - DISCORD_BOT_TOKEN (when "discord" is in NOTIFY_PROVIDERS)
//   - DISCORD_GUILD_ID (when "discord" is in NOTIFY_PROVIDERS)
//
// Duration variables (parsed via time.ParseDuration):
//   - ACTIVITY_OFFLINE_AFTER (default: 30s)
//   - ACTIVITY_SWEEP_INTERVAL (default: 5s)
//
// Duration values must be positive (>0).
//
// List variables (comma-separated):
//   - NOTIFY_PROVIDERS (e.g. "discord" or "discord,slack")
//
// String variables:
//   - NATS_URL (default: nats://localhost:4222)
//   - GATEWAY_ADDR (default: :8443)
//   - GATEWAY_CERT_PATH (default: ../../infrastructure/certs)
//   - DASHBOARD_ADDR (default: 127.0.0.1:8080)
//   - DISCORD_API_BASE_URL (default: https://discord.com/api/v10)
//   - GATEWAY_STATE_PATH (default: ./data)
//   - FILE_OPERATOR_ID (audit source label; default: internal-website)
//   - DASHBOARD_ALLOWED_ORIGIN (default: https://localhost:5173)
func LoadFromEnv() (GatewayConfig, error) {
	offlineAfter, err := durationFromEnv("ACTIVITY_OFFLINE_AFTER", defaultOfflineAfter)
	if err != nil {
		return GatewayConfig{}, err
	}

	sweepInterval, err := durationFromEnv("ACTIVITY_SWEEP_INTERVAL", defaultSweepInterval)
	if err != nil {
		return GatewayConfig{}, err
	}

	cfg := GatewayConfig{
		NATSURL:               stringFromEnv("NATS_URL", defaultNATSURL),
		ListenAddr:            stringFromEnv("GATEWAY_ADDR", defaultGatewayAddr),
		CertPath:              stringFromEnv("GATEWAY_CERT_PATH", defaultGatewayCertPath),
		DashboardAddr:         stringFromEnv("DASHBOARD_ADDR", defaultDashboardAddr),
		NotifyProviders:       splitCSV(os.Getenv("NOTIFY_PROVIDERS")),
		DiscordBotToken:       strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		DiscordGuildID:        strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID")),
		DiscordAPIBaseURL:     strings.TrimSpace(os.Getenv("DISCORD_API_BASE_URL")),
		ActivityOfflineAfter:  offlineAfter,
		ActivitySweepInterval: sweepInterval,
		StatePath:             stringFromEnv("GATEWAY_STATE_PATH", defaultStatePath),
		FileOperatorID:        stringFromEnv("FILE_OPERATOR_ID", "internal-website"),
		DashboardOrigin:       stringFromEnv("DASHBOARD_ALLOWED_ORIGIN", defaultDashboardOrigin),
	}

	if cfg.ActivityOfflineAfter <= 0 {
		return GatewayConfig{}, fmt.Errorf("ACTIVITY_OFFLINE_AFTER must be positive, got %v", cfg.ActivityOfflineAfter)
	}
	if cfg.ActivitySweepInterval <= 0 {
		return GatewayConfig{}, fmt.Errorf("ACTIVITY_SWEEP_INTERVAL must be positive, got %v", cfg.ActivitySweepInterval)
	}
	return cfg, nil
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

// splitCSV splits a comma-separated string into a lowercased, trimmed
// string slice. Empty parts are discarded. Returns nil for empty input.
func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		item := strings.ToLower(strings.TrimSpace(part))
		if item != "" {
			result = append(result, item)
		}
	}

	return result
}
