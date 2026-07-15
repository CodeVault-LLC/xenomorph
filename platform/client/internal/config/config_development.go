//go:build development

// Development-only profile. This file is intentionally excluded from every
// production build; production profile selection cannot be changed at runtime.
package config

import "time"

func generatedConfig() Config {
	return Config{
		Environment: "development", ImplementationVersion: "development",
		QUICEndpoint: "localhost:8444", ServerName: "localhost",
		ClientCertificateFile:       "../infrastructure/certs/client.crt",
		ClientPrivateKeyFile:        "../infrastructure/certs/client.key",
		CAFile:                      "../infrastructure/certs/ca.crt",
		CommandVerificationKeyFile:  "../infrastructure/certs/command-signing.pub",
		ReplayLedgerFile:            ".xenomorph/command-replay-ledger.json",
		ReplayAuthenticationKeyFile: ".xenomorph/command-replay.key",
		HeartbeatInterval:           15 * time.Second, OperationTimeout: 10 * time.Second,
		QUICHandshakeTimeout: 5 * time.Second, QUICIdleTimeout: 45 * time.Second,
		QUICKeepAlive: 10 * time.Second, ReconnectMinimumBackoff: time.Second,
		ReconnectMaximumBackoff: 30 * time.Second,
	}
}
