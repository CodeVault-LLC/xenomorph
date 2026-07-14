package agent

import (
	"fmt"
	"testing"
)

func TestBuildEndpointAttestationWithoutExtendedData(t *testing.T) {
	payload := BuildEndpointAttestation(false, func() (string, error) {
		return "edge-host-1", nil
	}, func() (string, error) {
		return "/tmp/no-home-needed", nil
	})

	if payload.Hostname != "edge-host-1" {
		t.Fatalf("expected hostname edge-host-1, got %q", payload.Hostname)
	}
	if payload.RequiresAttestation {
		t.Fatal("expected RequiresAttestation=false")
	}
	if len(payload.Browsers) != 0 {
		t.Fatalf("expected no browsers for non-extended payload, got %d", len(payload.Browsers))
	}
	if len(payload.InstalledApplications) != 0 {
		t.Fatalf("expected no installed apps for non-extended payload, got %d", len(payload.InstalledApplications))
	}
}

func TestBuildEndpointAttestationFallsBackHostname(t *testing.T) {
	payload := BuildEndpointAttestation(true, func() (string, error) {
		return "", fmt.Errorf("hostname unavailable")
	}, func() (string, error) {
		return "/tmp/nonexistent", nil
	})

	if payload.Hostname != unknownHostname {
		t.Fatalf("expected fallback hostname %q, got %q", unknownHostname, payload.Hostname)
	}
}
