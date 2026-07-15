package agentquic

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestHandshakeAdmissionBoundsIncompleteAndPrefixRate(t *testing.T) {
	t.Parallel()

	config := testConfig()
	config.MaximumIncompleteHandshakes = 1
	config.SourcePrefixRatePerSecond = 1
	config.SourcePrefixBurst = 1
	metrics := &Metrics{}
	admission := newHandshakeAdmission(config, metrics)
	now := time.Unix(100, 0)
	admission.now = func() time.Time { return now }
	address := &net.UDPAddr{IP: net.ParseIP("192.0.2.10"), Port: 1234}

	firstContext, err := admission.connectionContext(context.Background(), address)
	if err != nil {
		t.Fatalf("first admission failed: %v", err)
	}
	if _, err := admission.connectionContext(context.Background(), address); err == nil {
		t.Fatal("incomplete-handshake cap was not enforced")
	}
	ticketFromContext(firstContext).release()
	if _, err := admission.connectionContext(context.Background(), address); err == nil {
		t.Fatal("source-prefix rate was not enforced")
	}
	now = now.Add(time.Second)
	refilledContext, err := admission.connectionContext(context.Background(), address)
	if err != nil {
		t.Fatalf("refilled admission failed: %v", err)
	}
	ticketFromContext(refilledContext).release()
}

func TestSourcePrefixGroupsNetworkObservation(t *testing.T) {
	t.Parallel()

	first := sourcePrefix(&net.UDPAddr{IP: net.ParseIP("203.0.113.5"), Port: 1})
	second := sourcePrefix(&net.UDPAddr{IP: net.ParseIP("203.0.113.200"), Port: 2})
	if first != second {
		t.Fatalf("same /24 produced %q and %q", first, second)
	}
}
