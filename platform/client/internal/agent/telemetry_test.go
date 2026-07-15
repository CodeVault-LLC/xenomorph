package agent

import (
	"fmt"
	"testing"
)

func TestBuildHeartbeatPayloadUsesProvidedHostname(t *testing.T) {
	payload := BuildHeartbeatPayload(func() (string, error) {
		return "edge-host-01", nil
	})

	if payload.Hostname != "edge-host-01" {
		t.Fatalf("expected hostname edge-host-01, got %q", payload.Hostname)
	}

	if payload.OsVersion == "" {
		t.Fatal("expected non-empty os version")
	}

	if payload.CPULoad < 0 || payload.CPULoad > 1 {
		t.Fatalf("expected cpu load ratio in [0,1], got %f", payload.CPULoad)
	}

	if payload.RAMUsage < 0 || payload.RAMUsage > 1 {
		t.Fatalf("expected ram usage ratio in [0,1], got %f", payload.RAMUsage)
	}
}

func TestBuildHeartbeatPayloadFallsBackWhenProviderErrors(t *testing.T) {
	payload := BuildHeartbeatPayload(func() (string, error) {
		return "", fmt.Errorf("hostname unavailable")
	})

	if payload.Hostname != unknownHostname {
		t.Fatalf("expected fallback hostname %q, got %q", unknownHostname, payload.Hostname)
	}
}

func TestBuildHeartbeatPayloadFallsBackWhenProviderReturnsWhitespace(t *testing.T) {
	payload := BuildHeartbeatPayload(func() (string, error) {
		return "   ", nil
	})

	if payload.Hostname != unknownHostname {
		t.Fatalf("expected fallback hostname %q, got %q", unknownHostname, payload.Hostname)
	}
}

func TestClampRatioBoundsValues(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{name: "negative", in: -0.4, want: 0},
		{name: "valid", in: 0.42, want: 0.42},
		{name: "over", in: 1.7, want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampRatio(tt.in); got != tt.want {
				t.Fatalf("expected %f, got %f", tt.want, got)
			}
		})
	}
}
