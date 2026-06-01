package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultNATSURL         = "nats://localhost:4222"
	defaultGatewayAddr     = ":8443"
	defaultGatewayCertPath = "../../infrastructure/certs"
	defaultOfflineAfter    = 30 * time.Second
	defaultSweepInterval   = 5 * time.Second
)

// GatewayConfig controls runtime wiring for ingress and outbound providers.
type GatewayConfig struct {
	NATSURL               string
	ListenAddr            string
	CertPath              string
	NotifyProviders       []string
	DiscordBotToken       string
	DiscordGuildID        string
	DiscordAPIBaseURL     string
	ActivityOfflineAfter  time.Duration
	ActivitySweepInterval time.Duration
}

// LoadFromEnv loads runtime configuration with safe defaults.
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
		NotifyProviders:       splitCSV(os.Getenv("NOTIFY_PROVIDERS")),
		DiscordBotToken:       strings.TrimSpace(os.Getenv("DISCORD_BOT_TOKEN")),
		DiscordGuildID:        strings.TrimSpace(os.Getenv("DISCORD_GUILD_ID")),
		DiscordAPIBaseURL:     strings.TrimSpace(os.Getenv("DISCORD_API_BASE_URL")),
		ActivityOfflineAfter:  offlineAfter,
		ActivitySweepInterval: sweepInterval,
	}

	if cfg.ActivityOfflineAfter <= 0 {
		return GatewayConfig{}, fmt.Errorf("ACTIVITY_OFFLINE_AFTER must be > 0")
	}
	if cfg.ActivitySweepInterval <= 0 {
		return GatewayConfig{}, fmt.Errorf("ACTIVITY_SWEEP_INTERVAL must be > 0")
	}

	return cfg, nil
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

	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid duration %q: %w", key, raw, err)
	}
	return parsed, nil
}

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
