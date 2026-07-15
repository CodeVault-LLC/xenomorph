package config

import (
	"strings"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	t.Parallel()

	valid := Config{
		Environment: "production", ImplementationVersion: "test",
		QUICEndpoint: "gateway.internal:8444", ServerName: "gateway.internal",
		ClientCertificateFile: "client.crt", ClientPrivateKeyFile: "client.key", CAFile: "ca.crt",
		CommandVerificationKeyFile: "command.pub", ReplayLedgerFile: "ledger.json", ReplayAuthenticationKeyFile: "ledger.key",
		HeartbeatInterval: 15 * time.Second, OperationTimeout: 10 * time.Second,
		QUICHandshakeTimeout: 5 * time.Second, QUICIdleTimeout: 45 * time.Second, QUICKeepAlive: 10 * time.Second,
		ReconnectMinimumBackoff: time.Second, ReconnectMaximumBackoff: 30 * time.Second,
	}

	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "IP server name", mutate: func(config *Config) { config.ServerName = "192.0.2.1" }},
		{name: "localhost production", mutate: func(config *Config) { config.ServerName = "localhost" }},
		{name: "fast heartbeat", mutate: func(config *Config) { config.HeartbeatInterval = time.Second }},
		{name: "invalid keepalive", mutate: func(config *Config) { config.QUICKeepAlive = config.QUICIdleTimeout }},
		{name: "invalid operation timeout", mutate: func(config *Config) { config.OperationTimeout = 0 }},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			config := valid
			test.mutate(&config)

			if err := config.Validate(); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestLoadRejectsLegacyHTTPTransportConfiguration(t *testing.T) {
	t.Setenv("AGENT_TRANSPORT_MODE", "http")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "QUIC-only") {
		t.Fatalf("Load error = %v, want QUIC-only configuration error", err)
	}
}
