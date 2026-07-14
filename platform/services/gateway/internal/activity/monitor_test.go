package activity

import (
	"context"
	"reflect"
	"testing"
	"time"

	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func testSetup() (*Monitor, *pb.EventEnvelope, *time.Time) {
	monitor := NewMonitor(2 * time.Second)

	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	monitor.now = func() time.Time { return now }

	envelope := &pb.EventEnvelope{
		Timestamp: timestamppb.New(now),
		Security:  &pb.SecurityContext{AgentId: "agent-1"},
		Payload:   &pb.EventEnvelope_Heartbeat{Heartbeat: &pb.Heartbeat{Hostname: "edge-01"}},
	}

	return monitor, envelope, &now
}

func setNow(t *time.Time, envelope *pb.EventEnvelope, monitor *Monitor, d time.Duration) {
	*t = t.Add(d)
	monitor.now = func() time.Time { return *t }
	envelope.Timestamp = timestamppb.New(*t)
}

func TestMonitorFirstHeartbeatMarksAgentOnline(t *testing.T) {
	monitor, envelope, _ := testSetup()
	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("first heartbeat failed: %v", err)
	}
	snapshot, ok := monitor.Snapshot("agent-1")
	if !ok || !snapshot.IsOnline {
		t.Fatal("expected agent to be online after first heartbeat")
	}
}

func TestMonitorDuplicateHeartbeatKeepsAgentOnline(t *testing.T) {
	monitor, envelope, now := testSetup()
	setNow(now, envelope, monitor, time.Second)
	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("duplicate heartbeat failed: %v", err)
	}
	snapshot, ok := monitor.Snapshot("agent-1")
	if !ok || !snapshot.LastSeen.Equal(*now) {
		t.Fatal("expected latest heartbeat to update online agent state")
	}
}

func TestMonitorSweepDetectsOffline(t *testing.T) {
	monitor, envelope, now := testSetup()
	setNow(now, envelope, monitor, time.Second)

	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("heartbeat before sweep failed: %v", err)
	}

	setNow(now, envelope, monitor, 3*time.Second)
	if err := monitor.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep failed: %v", err)
	}
	if _, ok := monitor.Snapshot("agent-1"); ok {
		t.Fatal("expected stale agent to be removed from online state")
	}
}

func TestMonitorHeartbeatAfterOfflineRecoversOnline(t *testing.T) {
	monitor, envelope, now := testSetup()
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
	if _, ok := monitor.Snapshot("agent-1"); !ok {
		t.Fatal("expected heartbeat after sweep to restore online state")
	}
}

func TestMonitorRejectsEnvelopeWithoutAgentID(t *testing.T) {
	monitor := NewMonitor(5 * time.Second)
	err := monitor.ProcessHeartbeat(context.Background(), &pb.EventEnvelope{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestMonitorListClientsKeepsOfflineClients(t *testing.T) {
	monitor, envelope, now := testSetup()
	populateTelemetry(envelope)

	if err := monitor.ProcessHeartbeat(context.Background(), envelope); err != nil {
		t.Fatalf("heartbeat failed: %v", err)
	}

	clients := monitor.ListClients()
	assertClientTelemetry(t, clients, *now, true)

	setNow(now, envelope, monitor, 3*time.Second)
	if err := monitor.Sweep(context.Background()); err != nil {
		t.Fatalf("sweep failed: %v", err)
	}

	clients = monitor.ListClients()
	assertClientTelemetry(t, clients, now.Add(-3*time.Second), false)
}

func populateTelemetry(envelope *pb.EventEnvelope) {
	envelope.Security.ClientIp = "192.0.2.10"
	envelope.Payload = &pb.EventEnvelope_Heartbeat{Heartbeat: &pb.Heartbeat{
		Hostname: "edge-01", OsVersion: "linux-test", CpuLoad: 0.42, RamUsage: 0.73,
		UptimeSeconds: 1234, CpuModel: "test cpu", CpuCores: 4, CpuThreads: 8,
		TotalRamBytes: 16 * 1024 * 1024 * 1024, GpuDevices: []string{"0x10de 0x2684"},
		NetworkName: "eth0", NetworkAddresses: []string{"192.0.2.20/24"}, KernelVersion: "6.9.0-test",
		CpuFrequencyMhz: 3200, NetworkOnline: true, NetworkLinkSpeedMbps: 1000, NetworkType: "ethernet",
		TotalStorageBytes: 512 * 1024 * 1024 * 1024, AvailableStorageBytes: 128 * 1024 * 1024 * 1024,
		UsedStorageBytes: 384 * 1024 * 1024 * 1024, StorageUsage: 0.75, StorageInodeUsage: 0.15,
		StorageDevice: "/dev/nvme0n1p2", StorageFilesystem: "ext4", StorageMountpoint: "/",
		StorageModel: "Example NVMe", StorageType: storageTypeSolidState, NetworkSsid: "office-wifi",
		ApplicationTypes: []*pb.ApplicationTypeUsage{{Category: "Development", Count: 12}, {Category: "Browsers", Count: 4}},
	}}
}

func assertClientTelemetry(t *testing.T, clients []ClientSnapshot, observedAt time.Time, online bool) {
	t.Helper()
	want := []ClientSnapshot{{
		AgentID: "agent-1", Hostname: "edge-01", ClientIP: "192.0.2.10", OSVersion: "linux-test",
		CPULoad: 0.42, RAMUsage: 0.73, UptimeSeconds: 1234, CPUModel: "test cpu", CPUCores: 4, CPUThreads: 8,
		TotalRAMBytes: 16 * 1024 * 1024 * 1024, GPUDevices: []string{"0x10de 0x2684"},
		NetworkName: "eth0", NetworkAddresses: []string{"192.0.2.20/24"}, KernelVersion: "6.9.0-test",
		CPUFrequencyMHz: 3200, NetworkOnline: true, NetworkLinkSpeedMbps: 1000, NetworkType: "ethernet",
		TotalStorageBytes: 512 * 1024 * 1024 * 1024, AvailableStorageBytes: 128 * 1024 * 1024 * 1024,
		UsedStorageBytes: 384 * 1024 * 1024 * 1024, StorageUsage: 0.75, StorageInodeUsage: 0.15,
		StorageDevice: "/dev/nvme0n1p2", StorageFilesystem: "ext4", StorageMountpoint: "/",
		StorageModel: "Example NVMe", StorageType: storageTypeSolidState,
		ApplicationTypes: []ApplicationTypeUsage{{Category: "Development", Count: 12}, {Category: "Browsers", Count: 4}},
		NetworkSSID:      "office-wifi", FirstSeen: observedAt, LastSeen: observedAt, LastOnline: observedAt, IsOnline: online,
	}}
	if !reflect.DeepEqual(clients, want) {
		t.Fatalf("client telemetry mismatch:\n got: %#v\nwant: %#v", clients, want)
	}
}
