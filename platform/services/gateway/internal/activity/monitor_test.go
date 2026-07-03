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

func testSetup() (*captureProvider, *Monitor, *pb.EventEnvelope, *time.Time) {
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

	return capture, monitor, envelope, &now
}

func setNow(t *time.Time, envelope *pb.EventEnvelope, monitor *Monitor, d time.Duration) {
	*t = t.Add(d)
	monitor.now = func() time.Time { return *t }
	envelope.Timestamp = timestamppb.New(*t)
}

func TestMonitorFirstHeartbeatTriggersOnline(t *testing.T) {
	capture, monitor, envelope, _ := testSetup()
	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("first heartbeat failed: %v", err)
	}
	if len(capture.events) != 1 {
		t.Fatalf("expected 1 online notification, got %d", len(capture.events))
	}
	if capture.events[0].Status != provider.StatusOnline {
		t.Fatalf("expected online status, got %s", capture.events[0].Status)
	}
}

func TestMonitorDuplicateHeartbeatNoDuplicateNotification(t *testing.T) {
	capture, monitor, envelope, now := testSetup()
	setNow(now, envelope, monitor, time.Second)
	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("duplicate heartbeat failed: %v", err)
	}
	if len(capture.events) != 1 {
		t.Fatalf("expected no duplicate online notification, got %d", len(capture.events))
	}
}

func TestMonitorSweepDetectsOffline(t *testing.T) {
	capture, monitor, envelope, now := testSetup()
	setNow(now, envelope, monitor, time.Second)

	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("heartbeat before sweep failed: %v", err)
	}

	setNow(now, envelope, monitor, 3*time.Second)
	if err := monitor.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep failed: %v", err)
	}
	if len(capture.events) != 2 {
		t.Fatalf("expected offline notification, got %d events", len(capture.events))
	}
	if capture.events[1].Status != provider.StatusOffline {
		t.Fatalf("expected offline status, got %s", capture.events[1].Status)
	}
}

func TestMonitorHeartbeatAfterOfflineRecoversOnline(t *testing.T) {
	capture, monitor, envelope, now := testSetup()
	setNow(now, envelope, monitor, time.Second)

	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("first heartbeat failed: %v", err)
	}

	setNow(now, envelope, monitor, 3*time.Second)
	if err := monitor.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep failed: %v", err)
	}

	setNow(now, envelope, monitor, time.Second)
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

func TestMonitorListClientsKeepsOfflineClients(t *testing.T) {
	_, monitor, envelope, now := testSetup()
	envelope.Security.ClientIp = "192.0.2.10"
	envelope.GetHeartbeat().OsVersion = "linux-test"
	envelope.GetHeartbeat().CpuLoad = 0.42
	envelope.GetHeartbeat().RamUsage = 0.73

	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}

	clients := monitor.ListClients()
	if len(clients) != 1 {
		t.Fatalf("expected one client, got %d", len(clients))
	}
	if !clients[0].IsOnline {
		t.Fatal("expected client to be online after heartbeat")
	}
	if clients[0].ClientIP != "192.0.2.10" {
		t.Fatalf("expected client IP to be recorded, got %q", clients[0].ClientIP)
	}
	if clients[0].OSVersion != "linux-test" {
		t.Fatalf("expected OS version to be recorded, got %q", clients[0].OSVersion)
	}

	setNow(now, envelope, monitor, 3*time.Second)
	if err := monitor.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep failed: %v", err)
	}

	clients = monitor.ListClients()
	if len(clients) != 1 {
		t.Fatalf("expected offline client to remain in all-time list, got %d", len(clients))
	}
	if clients[0].IsOnline {
		t.Fatal("expected client to be offline after sweep")
	}
	if !clients[0].LastOnline.Equal(clients[0].LastSeen) {
		t.Fatalf("expected last_online to match last heartbeat; last_online=%v last_seen=%v", clients[0].LastOnline, clients[0].LastSeen)
	}
}
