package agentquic

import (
	"math"
	"testing"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
	"github.com/codevault-llc/xenomorph/platform/shared/wire"
)

func TestLogEntryUsesFixedRegistryIncludingFallbackAudit(t *testing.T) {
	t.Parallel()
	entry, err := logEntryFromAgent(agent.LogEntryPayload{
		Level: "WARN", Component: "client.runtime", Message: "event=quic_network_fallback",
	})
	if err != nil {
		t.Fatalf("encode fallback audit event: %v", err)
	}
	if entry.Level != uint64(wire.LogLevelWarn) || entry.Component != uint64(wire.LogComponentRuntime) ||
		entry.EventCode != uint64(wire.LogEventQUICNetworkFallback) || entry.Detail != "" {
		t.Fatalf("unexpected fixed log entry: %#v", entry)
	}
	if _, err := logEntryFromAgent(agent.LogEntryPayload{
		Level: "INFO", Component: "client.runtime", Message: "raw unregistered text",
	}); err == nil {
		t.Fatal("unregistered diagnostic text was accepted")
	}
}

func TestRatioPartsPerMillionIsCanonicalAndBounded(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		value float64
		want  uint64
	}{
		{name: "NaN", value: math.NaN(), want: 0},
		{name: "negative", value: -1, want: 0},
		{name: "half", value: 0.5, want: 500_000},
		{name: "one", value: 1, want: partsPerMillion},
		{name: "above one", value: 2, want: partsPerMillion},
	}
	for _, test := range tests {
		if got := ratioPartsPerMillion(test.value); got != test.want {
			t.Errorf("%s conversion = %d, want %d", test.name, got, test.want)
		}
	}
}

func TestOperationIDBindsDomainAudienceAndPayload(t *testing.T) {
	t.Parallel()
	payload := []byte("canonical body")
	first := operationIDForPayload("attestation", "agent-a", payload)
	if first == [16]byte{} || first != operationIDForPayload("attestation", "agent-a", payload) {
		t.Fatal("operation identifier is not stable and nonzero")
	}
	if first == operationIDForPayload("attestation", "agent-b", payload) ||
		first == operationIDForPayload("other", "agent-a", payload) ||
		first == operationIDForPayload("attestation", "agent-a", []byte("different")) {
		t.Fatal("operation identifier is not bound to its complete scope")
	}
}
