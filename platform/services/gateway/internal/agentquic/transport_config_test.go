package agentquic

import (
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	t.Parallel()

	valid := testConfig()
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "missing address", mutate: func(config *Config) { config.Address = "" }},
		{name: "missing reset key path", mutate: func(config *Config) { config.StatelessResetKeyFile = "" }},
		{name: "short handshake", mutate: func(config *Config) { config.HandshakeIdleTimeout = time.Millisecond }},
		{name: "keepalive above half idle", mutate: func(config *Config) { config.KeepAlivePeriod = config.MaximumIdleTimeout }},
		{name: "heartbeat below policy", mutate: func(config *Config) { config.HeartbeatInterval = time.Second }},
		{name: "stream window ordering", mutate: func(config *Config) { config.InitialStreamReceiveWindow = config.MaximumStreamReceiveWindow + 1 }},
		{name: "registry below sessions", mutate: func(config *Config) { config.MaximumRegistryEntries = config.MaximumActiveSessions - 1 }},
		{name: "transfer above stream limit", mutate: func(config *Config) { config.ConcurrentTransferStreams = 9 }},
		{name: "diagnostic files unbounded", mutate: func(config *Config) {
			config.EnableTransportDiagnostics = true
			config.TransportDiagnosticFileLimit = 0
		}},
		{name: "diagnostic bytes below minimum", mutate: func(config *Config) {
			config.EnableTransportDiagnostics = true
			config.TransportDiagnosticByteLimit = minimumDiagnosticBytes - 1
		}},
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

func testConfig() Config {
	return Config{
		Address:                      "127.0.0.1:0",
		ServerCertificateFile:        "server.crt",
		ServerPrivateKeyFile:         "server.key",
		ClientCAFile:                 "ca.crt",
		StatelessResetKeyFile:        "reset.key",
		TokenGeneratorKeyFile:        "token.key",
		HandshakeIdleTimeout:         5 * time.Second,
		MaximumIdleTimeout:           45 * time.Second,
		KeepAlivePeriod:              10 * time.Second,
		ControlStreamTimeout:         5 * time.Second,
		TransferStreamIOTimeout:      60 * time.Second,
		MediaFrameTimeout:            10 * time.Second,
		DrainTimeout:                 5 * time.Second,
		InitialStreamReceiveWindow:   256 << 10,
		MaximumStreamReceiveWindow:   4 << 20,
		InitialConnectionWindow:      512 << 10,
		MaximumConnectionWindow:      16 << 20,
		MaximumIncomingStreams:       8,
		MaximumIncomingUniStreams:    4,
		MaximumIncompleteHandshakes:  16,
		MaximumActiveSessions:        32,
		MaximumRegistryEntries:       64,
		SourcePrefixRatePerSecond:    10,
		SourcePrefixBurst:            20,
		MaximumSourcePrefixes:        128,
		MaximumClientChainBytes:      16 << 10,
		MaximumClientChainDepth:      4,
		ConcurrentTransferStreams:    4,
		HeartbeatInterval:            15 * time.Second,
		EventFrameMaximum:            256 << 10,
		TransportDiagnosticFileLimit: 8,
		TransportDiagnosticByteLimit: 64 << 20,
	}
}
