package config

import (
	"runtime"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	t.Parallel()

	valid := Config{
		Environment: "production", ImplementationVersion: "test",
		TargetOS: runtime.GOOS, TargetArchitecture: runtime.GOARCH,
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
		{name: "wrong target", mutate: func(config *Config) { config.TargetArchitecture = "unsupported" }},
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

func TestLoadFailsWithoutGeneratedProfile(t *testing.T) {
	if developmentBuild {
		t.Skip("development build supplies a compile-time fixture profile")
	}

	t.Setenv("AGENT_QUIC_ENDPOINT", "attacker.example:8444")

	if _, err := Load(); err == nil {
		t.Fatal("Load() succeeded without a generated profile")
	}
}
