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
