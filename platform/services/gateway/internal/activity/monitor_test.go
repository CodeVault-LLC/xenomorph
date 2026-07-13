package activity

import (
	"context"
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
	envelope.Security.ClientIp = "192.0.2.10"
	envelope.GetHeartbeat().OsVersion = "linux-test"
	envelope.GetHeartbeat().CpuLoad = 0.42
	envelope.GetHeartbeat().RamUsage = 0.73
	envelope.GetHeartbeat().UptimeSeconds = 1234
	envelope.GetHeartbeat().CpuModel = "test cpu"
	envelope.GetHeartbeat().CpuCores = 4
	envelope.GetHeartbeat().CpuThreads = 8
	envelope.GetHeartbeat().TotalRamBytes = 16 * 1024 * 1024 * 1024
	envelope.GetHeartbeat().GpuDevices = []string{"0x10de 0x2684"}
	envelope.GetHeartbeat().NetworkName = "eth0"
	envelope.GetHeartbeat().NetworkAddresses = []string{"192.0.2.20/24"}
	envelope.GetHeartbeat().KernelVersion = "6.9.0-test"
	envelope.GetHeartbeat().CpuFrequencyMhz = 3200
	envelope.GetHeartbeat().NetworkOnline = true
	envelope.GetHeartbeat().NetworkLinkSpeedMbps = 1000
	envelope.GetHeartbeat().NetworkType = "ethernet"
	envelope.GetHeartbeat().TotalStorageBytes = 512 * 1024 * 1024 * 1024
	envelope.GetHeartbeat().AvailableStorageBytes = 128 * 1024 * 1024 * 1024
	envelope.GetHeartbeat().UsedStorageBytes = 384 * 1024 * 1024 * 1024
	envelope.GetHeartbeat().StorageUsage = 0.75
	envelope.GetHeartbeat().StorageInodeUsage = 0.15
	envelope.GetHeartbeat().StorageDevice = "/dev/nvme0n1p2"
	envelope.GetHeartbeat().StorageFilesystem = "ext4"
	envelope.GetHeartbeat().StorageMountpoint = "/"
	envelope.GetHeartbeat().StorageModel = "Example NVMe"
	envelope.GetHeartbeat().StorageType = "solid-state"
	envelope.GetHeartbeat().ApplicationTypes = []*pb.ApplicationTypeUsage{
		{Category: "Development", Count: 12},
		{Category: "Browsers", Count: 4},
	}
	envelope.GetHeartbeat().NetworkSsid = "office-wifi"

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
	if clients[0].UptimeSeconds != 1234 {
		t.Fatalf("expected uptime to be recorded, got %d", clients[0].UptimeSeconds)
	}
	if clients[0].CPUModel != "test cpu" || clients[0].CPUCores != 4 || clients[0].CPUThreads != 8 {
		t.Fatalf("expected CPU details to be recorded, got model=%q cores=%d threads=%d", clients[0].CPUModel, clients[0].CPUCores, clients[0].CPUThreads)
	}
	if clients[0].TotalRAMBytes != 16*1024*1024*1024 {
		t.Fatalf("expected total RAM to be recorded, got %d", clients[0].TotalRAMBytes)
	}
	if len(clients[0].GPUDevices) != 1 || clients[0].GPUDevices[0] != "0x10de 0x2684" {
		t.Fatalf("expected GPU devices to be recorded, got %#v", clients[0].GPUDevices)
	}
	if clients[0].NetworkName != "eth0" || len(clients[0].NetworkAddresses) != 1 || clients[0].NetworkAddresses[0] != "192.0.2.20/24" {
		t.Fatalf("expected network details to be recorded, got name=%q addresses=%#v", clients[0].NetworkName, clients[0].NetworkAddresses)
	}
	if clients[0].KernelVersion != "6.9.0-test" {
		t.Fatalf("expected kernel version to be recorded, got %q", clients[0].KernelVersion)
	}
	if clients[0].CPUFrequencyMHz != 3200 {
		t.Fatalf("expected CPU frequency to be recorded, got %d", clients[0].CPUFrequencyMHz)
	}
	if !clients[0].NetworkOnline || clients[0].NetworkLinkSpeedMbps != 1000 || clients[0].NetworkType != "ethernet" {
		t.Fatalf("expected network link details to be recorded, got online=%t speed=%d type=%q", clients[0].NetworkOnline, clients[0].NetworkLinkSpeedMbps, clients[0].NetworkType)
	}
	if clients[0].TotalStorageBytes != 512*1024*1024*1024 || clients[0].AvailableStorageBytes != 128*1024*1024*1024 {
		t.Fatalf("expected storage details to be recorded, got total=%d available=%d", clients[0].TotalStorageBytes, clients[0].AvailableStorageBytes)
	}
	if clients[0].UsedStorageBytes != 384*1024*1024*1024 || clients[0].StorageUsage != 0.75 || clients[0].StorageInodeUsage != 0.15 {
		t.Fatalf("expected storage utilization to be recorded, got used=%d usage=%f inode_usage=%f", clients[0].UsedStorageBytes, clients[0].StorageUsage, clients[0].StorageInodeUsage)
	}
	if clients[0].StorageDevice != "/dev/nvme0n1p2" || clients[0].StorageFilesystem != "ext4" || clients[0].StorageMountpoint != "/" {
		t.Fatalf("expected filesystem details to be recorded, got device=%q filesystem=%q mountpoint=%q", clients[0].StorageDevice, clients[0].StorageFilesystem, clients[0].StorageMountpoint)
	}
	if clients[0].StorageModel != "Example NVMe" || clients[0].StorageType != "solid-state" || len(clients[0].ApplicationTypes) != 2 {
		t.Fatalf("expected cached disk inventory to be recorded, got model=%q type=%q application_types=%#v", clients[0].StorageModel, clients[0].StorageType, clients[0].ApplicationTypes)
	}
	if clients[0].NetworkSSID != "office-wifi" {
		t.Fatalf("expected wireless network name to be recorded, got %q", clients[0].NetworkSSID)
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
