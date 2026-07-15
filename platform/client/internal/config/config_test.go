package config

import (
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	valid := Config{
		Environment: "production", ImplementationVersion: "test", TransportMode: TransportQUIC, GatewayURL: "https://gateway.internal:8443",
		QUICEndpoint: "gateway.internal:8444", ServerName: "gateway.internal",
		ClientCertificateFile: "client.crt", ClientPrivateKeyFile: "client.key", CAFile: "ca.crt",
		CommandVerificationKeyFile: "command.pub", ReplayLedgerFile: "ledger.json", ReplayAuthenticationKeyFile: "ledger.key",
		HeartbeatInterval: 15 * time.Second, HTTPTimeout: 10 * time.Second,
		QUICHandshakeTimeout: 5 * time.Second, QUICIdleTimeout: 45 * time.Second, QUICKeepAlive: 10 * time.Second,
		ReconnectMinimumBackoff: time.Second, ReconnectMaximumBackoff: 30 * time.Second,
	}

	if err := valid.Validate(now); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "HTTP URL", mutate: func(config *Config) { config.GatewayURL = "http://gateway.internal" }},
		{name: "IP server name", mutate: func(config *Config) { config.ServerName = "192.0.2.1" }},
		{name: "localhost production", mutate: func(config *Config) { config.ServerName = "localhost" }},
		{name: "fast heartbeat", mutate: func(config *Config) { config.HeartbeatInterval = time.Second }},
		{name: "invalid keepalive", mutate: func(config *Config) { config.QUICKeepAlive = config.QUICIdleTimeout }},
		{name: "expired fallback", mutate: func(config *Config) { config.TransportMode = TransportQUICFirst; config.HTTPFallbackUntil = now }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			config := valid
			test.mutate(&config)

			if err := config.Validate(now); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}
