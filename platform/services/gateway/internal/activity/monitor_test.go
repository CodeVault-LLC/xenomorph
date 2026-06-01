package activity

import (
	"context"
	"testing"
	"time"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type captureProvider struct {
	events []provider.ActivityEvent
}

func (c *captureProvider) Name() string {
	return "capture"
}

func (c *captureProvider) Notify(_ context.Context, event provider.ActivityEvent) error {
	c.events = append(c.events, event)
	return nil
}

func TestMonitorOnlineOfflineTransitions(t *testing.T) {
	capture := &captureProvider{}
	fanout := provider.NewFanout([]provider.Provider{capture})
	monitor := NewMonitor(2*time.Second, fanout)

	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	monitor.now = func() time.Time { return now }

	envelope := &pb.EventEnvelope{
		Timestamp: timestamppb.New(now),
		Security:  &pb.SecurityContext{AgentId: "agent-1"},
		Payload:   &pb.EventEnvelope_Heartbeat{Heartbeat: &pb.Heartbeat{Hostname: "edge-01"}},
	}

	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("first heartbeat failed: %v", err)
	}
	if len(capture.events) != 1 {
		t.Fatalf("expected 1 online notification, got %d", len(capture.events))
	}
	if capture.events[0].Status != provider.StatusOnline {
		t.Fatalf("expected online status, got %s", capture.events[0].Status)
	}

	now = now.Add(1 * time.Second)
	envelope.Timestamp = timestamppb.New(now)
	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("second heartbeat failed: %v", err)
	}
	if len(capture.events) != 1 {
		t.Fatalf("expected no duplicate online notification, got %d", len(capture.events))
	}

	now = now.Add(3 * time.Second)
	if err := monitor.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep failed: %v", err)
	}
	if len(capture.events) != 2 {
		t.Fatalf("expected offline notification, got %d events", len(capture.events))
	}
	if capture.events[1].Status != provider.StatusOffline {
		t.Fatalf("expected offline status, got %s", capture.events[1].Status)
	}

	now = now.Add(1 * time.Second)
	envelope.Timestamp = timestamppb.New(now)
	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("heartbeat after offline failed: %v", err)
	}
	if len(capture.events) != 3 {
		t.Fatalf("expected online recovery notification, got %d", len(capture.events))
	}
	if capture.events[2].Status != provider.StatusOnline {
		t.Fatalf("expected online status after recovery, got %s", capture.events[2].Status)
	}
}

func TestMonitorRejectsEnvelopeWithoutAgentID(t *testing.T) {
	monitor := NewMonitor(5*time.Second, provider.NewFanout(nil))
	err := monitor.ProcessHeartbeat(context.Background(), &pb.EventEnvelope{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
