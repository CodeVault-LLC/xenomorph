// Package activity tracks agent online/offline state from authenticated
// heartbeat events. This package owns the liveness state machine and
// transition notification logic.
package activity

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
)

// Monitor derives online/offline presence from authenticated heartbeat traffic.
// Presence state is held in memory and is not persisted across gateway restarts.
//
// The Monitor owns two operations:
//   - ProcessHeartbeat updates the last-seen timestamp for an agent.
//   - Sweep marks stale agents offline when their last-seen exceeds offlineAfter.
type Monitor struct {
	offlineAfter time.Duration
	now          func() time.Time

	mu     sync.Mutex
	online map[string]presence
	all    map[string]clientRecord
}

// AgentSnapshot is a point-in-time view of an agent's presence at the gateway boundary.
type AgentSnapshot struct {
	AgentID  string
	Hostname string
	LastSeen time.Time
	IsOnline bool
}

func clampTelemetryRatio(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func clampTelemetryText(value string, limit int) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, ""))
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func normalizeStorageType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "solid-state":
		return "solid-state"
	case "rotational":
		return "rotational"
	case "fixed", "removable", "network", "optical":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func normalizeApplicationTypes(values []*pb.ApplicationTypeUsage) []ApplicationTypeUsage {
	const maxDetectedApplications uint32 = 200
	counts := make(map[string]uint32, 8)
	total := uint32(0)
	for _, value := range values {
		if total >= maxDetectedApplications || value == nil || !allowedApplicationCategory(value.GetCategory()) {
			continue
		}
		remaining := maxDetectedApplications - total
		if value.GetCount() < remaining {
			remaining = value.GetCount()
		}
		counts[value.GetCategory()] += remaining
		total += remaining
	}
	result := make([]ApplicationTypeUsage, 0, len(counts))
	for category, count := range counts {
		if count > 0 {
			result = append(result, ApplicationTypeUsage{Category: category, Count: count})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Category < result[j].Category
		}
		return result[i].Count > result[j].Count
	})
	return result
}

func normalizeStorageBytes(total, available, used uint64) (uint64, uint64, uint64) {
	const maxStorageBytes uint64 = 1 << 50
	if total > maxStorageBytes {
		total = maxStorageBytes
	}
	if available > total {
		available = total
	}
	if used > total {
		used = total
	}
	return total, available, used
}

func allowedApplicationCategory(category string) bool {
	switch category {
	case "Browsers", "Development", "Communication", "Media", "Games", "Productivity", "Security", "Utilities and other":
		return true
	default:
		return false
	}
}

// presence holds agent liveness state in memory.
type presence struct {
	lastSeen time.Time
	hostname string
}

// ApplicationTypeUsage is a bounded client-authored installed application
// category count. It is telemetry and is never identity evidence.
type ApplicationTypeUsage struct {
	Category string `json:"category"`
	Count    uint32 `json:"count"`
}

// ClientSnapshot is a read-only all-time view of an authenticated agent known
// to the gateway during the current process lifetime.
type ClientSnapshot struct {
	AgentID               string                 `json:"agent_id"`
	Hostname              string                 `json:"hostname"`
	ClientIP              string                 `json:"client_ip"`
	OSVersion             string                 `json:"os_version"`
	CPULoad               float64                `json:"cpu_load"`
	RAMUsage              float64                `json:"ram_usage"`
	UptimeSeconds         uint64                 `json:"uptime_seconds"`
	CPUModel              string                 `json:"cpu_model"`
	CPUCores              int32                  `json:"cpu_cores"`
	CPUThreads            int32                  `json:"cpu_threads"`
	TotalRAMBytes         uint64                 `json:"total_ram_bytes"`
	GPUDevices            []string               `json:"gpu_devices"`
	NetworkName           string                 `json:"network_name"`
	NetworkAddresses      []string               `json:"network_addresses"`
	KernelVersion         string                 `json:"kernel_version"`
	CPUFrequencyMHz       uint64                 `json:"cpu_frequency_mhz"`
	NetworkOnline         bool                   `json:"network_online"`
	NetworkLinkSpeedMbps  uint64                 `json:"network_link_speed_mbps"`
	NetworkType           string                 `json:"network_type"`
	TotalStorageBytes     uint64                 `json:"total_storage_bytes"`
	AvailableStorageBytes uint64                 `json:"available_storage_bytes"`
	UsedStorageBytes      uint64                 `json:"used_storage_bytes"`
	StorageUsage          float64                `json:"storage_usage"`
	StorageInodeUsage     float64                `json:"storage_inode_usage"`
	StorageDevice         string                 `json:"storage_device"`
	StorageFilesystem     string                 `json:"storage_filesystem"`
	StorageMountpoint     string                 `json:"storage_mountpoint"`
	StorageModel          string                 `json:"storage_model"`
	StorageType           string                 `json:"storage_type"`
	StorageReadOnly       bool                   `json:"storage_read_only"`
	ApplicationTypes      []ApplicationTypeUsage `json:"application_types"`
	NetworkSSID           string                 `json:"network_ssid"`
	FirstSeen             time.Time              `json:"first_seen"`
	LastSeen              time.Time              `json:"last_seen"`
	LastOnline            time.Time              `json:"last_online"`
	IsOnline              bool                   `json:"is_online"`
}

// clientRecord stores the all-time presence metadata for a gateway-authenticated
// agent. Identity and IP are gateway-authored. Hostname, OS, CPU, RAM, storage,
// and application inventory are client-authored telemetry labels and are not
// used as identity evidence.
type clientRecord struct {
	agentID               string
	hostname              string
	clientIP              string
	osVersion             string
	cpuLoad               float64
	ramUsage              float64
	uptimeSeconds         uint64
	cpuModel              string
	cpuCores              int32
	cpuThreads            int32
	totalRAMBytes         uint64
	gpuDevices            []string
	networkName           string
	networkAddresses      []string
	kernelVersion         string
	cpuFrequencyMHz       uint64
	networkOnline         bool
	networkLinkSpeedMbps  uint64
	networkType           string
	totalStorageBytes     uint64
	availableStorageBytes uint64
	usedStorageBytes      uint64
	storageUsage          float64
	storageInodeUsage     float64
	storageDevice         string
	storageFilesystem     string
	storageMountpoint     string
	storageModel          string
	storageType           string
	storageReadOnly       bool
	applicationTypes      []ApplicationTypeUsage
	networkSSID           string
	firstSeen             time.Time
	lastSeen              time.Time
	lastOnline            time.Time
	isOnline              bool
}

// NewMonitor creates a Monitor that marks agents offline when no heartbeat
// has been observed for offlineAfter.
func NewMonitor(offlineAfter time.Duration) *Monitor {
	return &Monitor{
		offlineAfter: offlineAfter,
		now:          time.Now,
		online:       make(map[string]presence),
		all:          make(map[string]clientRecord),
	}
}

// ProcessHeartbeat updates liveness state for an agent based on a
// gateway-authenticated envelope.
//
// The envelope must contain a non-empty Security.AgentId. Heartbeats from
// unauthenticated sources must not reach this function; the mTLS middleware
// in the transport layer is the caller's enforcement point.
//
// Returns an error when envelope or envelope.Security is nil or when the
// agent ID is empty.
func (m *Monitor) ProcessHeartbeat(ctx context.Context, envelope *pb.EventEnvelope) error {
	if envelope == nil {
		return fmt.Errorf("heartbeat envelope is nil")
	}
	if envelope.Security == nil {
		return fmt.Errorf("heartbeat envelope missing security context")
	}
	agentID := envelope.Security.AgentId
	if agentID == "" {
		return fmt.Errorf("heartbeat envelope has empty agent id")
	}

	hb := envelope.GetHeartbeat()
	hostname := ""
	osVersion := ""
	cpuLoad := 0.0
	ramUsage := 0.0
	uptimeSeconds := uint64(0)
	cpuModel := ""
	cpuCores := int32(0)
	cpuThreads := int32(0)
	totalRAMBytes := uint64(0)
	var gpuDevices []string
	networkName := ""
	var networkAddresses []string
	kernelVersion := ""
	cpuFrequencyMHz := uint64(0)
	networkOnline := false
	networkLinkSpeedMbps := uint64(0)
	networkType := ""
	totalStorageBytes := uint64(0)
	availableStorageBytes := uint64(0)
	usedStorageBytes := uint64(0)
	storageUsage := 0.0
	storageInodeUsage := 0.0
	storageDevice := ""
	storageFilesystem := ""
	storageMountpoint := ""
	storageModel := ""
	storageType := ""
	storageReadOnly := false
	var applicationTypes []ApplicationTypeUsage
	networkSSID := ""
	if hb != nil {
		hostname = hb.Hostname
		osVersion = hb.OsVersion
		cpuLoad = hb.CpuLoad
		ramUsage = hb.RamUsage
		uptimeSeconds = hb.GetUptimeSeconds()
		cpuModel = hb.GetCpuModel()
		cpuCores = hb.GetCpuCores()
		cpuThreads = hb.GetCpuThreads()
		totalRAMBytes = hb.GetTotalRamBytes()
		gpuDevices = append([]string(nil), hb.GetGpuDevices()...)
		networkName = hb.GetNetworkName()
		networkAddresses = append([]string(nil), hb.GetNetworkAddresses()...)
		kernelVersion = hb.GetKernelVersion()
		cpuFrequencyMHz = hb.GetCpuFrequencyMhz()
		networkOnline = hb.GetNetworkOnline()
		networkLinkSpeedMbps = hb.GetNetworkLinkSpeedMbps()
		networkType = hb.GetNetworkType()
		totalStorageBytes = hb.GetTotalStorageBytes()
		availableStorageBytes = hb.GetAvailableStorageBytes()
		usedStorageBytes = hb.GetUsedStorageBytes()
		storageUsage = clampTelemetryRatio(hb.GetStorageUsage())
		storageInodeUsage = clampTelemetryRatio(hb.GetStorageInodeUsage())
		storageDevice = clampTelemetryText(hb.GetStorageDevice(), 160)
		storageFilesystem = clampTelemetryText(hb.GetStorageFilesystem(), 32)
		storageMountpoint = clampTelemetryText(hb.GetStorageMountpoint(), 260)
		storageModel = clampTelemetryText(hb.GetStorageModel(), 160)
		storageType = normalizeStorageType(hb.GetStorageType())
		storageReadOnly = hb.GetStorageReadOnly()
		applicationTypes = normalizeApplicationTypes(hb.GetApplicationTypes())
		networkSSID = hb.GetNetworkSsid()
	}
	totalStorageBytes, availableStorageBytes, usedStorageBytes = normalizeStorageBytes(
		totalStorageBytes,
		availableStorageBytes,
		usedStorageBytes,
	)

	eventTime := m.now()
	if envelope.Timestamp != nil {
		eventTime = envelope.Timestamp.AsTime()
	}

	m.mu.Lock()
	m.online[agentID] = presence{lastSeen: eventTime, hostname: hostname}
	record, known := m.all[agentID]
	if !known {
		record = clientRecord{
			agentID:   agentID,
			firstSeen: eventTime,
		}
	}
	record.hostname = hostname
	record.clientIP = envelope.Security.ClientIp
	record.osVersion = osVersion
	record.cpuLoad = cpuLoad
	record.ramUsage = ramUsage
	record.uptimeSeconds = uptimeSeconds
	record.cpuModel = cpuModel
	record.cpuCores = cpuCores
	record.cpuThreads = cpuThreads
	record.totalRAMBytes = totalRAMBytes
	record.gpuDevices = gpuDevices
	record.networkName = networkName
	record.networkAddresses = networkAddresses
	record.kernelVersion = kernelVersion
	record.cpuFrequencyMHz = cpuFrequencyMHz
	record.networkOnline = networkOnline
	record.networkLinkSpeedMbps = networkLinkSpeedMbps
	record.networkType = networkType
	record.totalStorageBytes = totalStorageBytes
	record.availableStorageBytes = availableStorageBytes
	record.usedStorageBytes = usedStorageBytes
	record.storageUsage = storageUsage
	record.storageInodeUsage = storageInodeUsage
	record.storageDevice = storageDevice
	record.storageFilesystem = storageFilesystem
	record.storageMountpoint = storageMountpoint
	record.storageModel = storageModel
	record.storageType = storageType
	record.storageReadOnly = storageReadOnly
	record.applicationTypes = applicationTypes
	record.networkSSID = networkSSID
	record.lastSeen = eventTime
	record.lastOnline = eventTime
	record.isOnline = true
	m.all[agentID] = record
	m.mu.Unlock()

	return nil
}

// Sweep checks every tracked agent and marks as offline those whose last-seen
// exceeds offlineAfter. Stale agents are removed from the tracking map.
//
// Call Sweep periodically from a ticker. The sweep interval should be
// shorter than offlineAfter to ensure timely offline detection.
func (m *Monitor) Sweep(_ context.Context) error {
	now := m.now()

	m.mu.Lock()
	for agentID, state := range m.online {
		if now.Sub(state.lastSeen) > m.offlineAfter {
			delete(m.online, agentID)
			record, ok := m.all[agentID]
			if ok {
				record.isOnline = false
				m.all[agentID] = record
			}
		}
	}
	m.mu.Unlock()

	return nil
}

// Snapshot returns a point-in-time view of an agent's presence state.
// Returns false when the agent is not currently tracked (never seen or
// expired by Sweep).
func (m *Monitor) Snapshot(agentID string) (AgentSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.online[agentID]
	if !ok {
		return AgentSnapshot{}, false
	}

	return AgentSnapshot{
		AgentID:  agentID,
		Hostname: p.hostname,
		LastSeen: p.lastSeen,
		IsOnline: true,
	}, true
}

// ListClients returns every authenticated agent observed by the gateway during
// the current process lifetime. The returned slice is a copy and is safe for
// callers to sort or modify.
func (m *Monitor) ListClients() []ClientSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	snapshots := make([]ClientSnapshot, 0, len(m.all))
	for _, record := range m.all {
		snapshots = append(snapshots, ClientSnapshot{
			AgentID:               record.agentID,
			Hostname:              record.hostname,
			ClientIP:              record.clientIP,
			OSVersion:             record.osVersion,
			CPULoad:               record.cpuLoad,
			RAMUsage:              record.ramUsage,
			UptimeSeconds:         record.uptimeSeconds,
			CPUModel:              record.cpuModel,
			CPUCores:              record.cpuCores,
			CPUThreads:            record.cpuThreads,
			TotalRAMBytes:         record.totalRAMBytes,
			GPUDevices:            append([]string(nil), record.gpuDevices...),
			NetworkName:           record.networkName,
			NetworkAddresses:      append([]string(nil), record.networkAddresses...),
			KernelVersion:         record.kernelVersion,
			CPUFrequencyMHz:       record.cpuFrequencyMHz,
			NetworkOnline:         record.networkOnline,
			NetworkLinkSpeedMbps:  record.networkLinkSpeedMbps,
			NetworkType:           record.networkType,
			TotalStorageBytes:     record.totalStorageBytes,
			AvailableStorageBytes: record.availableStorageBytes,
			UsedStorageBytes:      record.usedStorageBytes,
			StorageUsage:          record.storageUsage,
			StorageInodeUsage:     record.storageInodeUsage,
			StorageDevice:         record.storageDevice,
			StorageFilesystem:     record.storageFilesystem,
			StorageMountpoint:     record.storageMountpoint,
			StorageModel:          record.storageModel,
			StorageType:           record.storageType,
			StorageReadOnly:       record.storageReadOnly,
			ApplicationTypes:      append([]ApplicationTypeUsage(nil), record.applicationTypes...),
			NetworkSSID:           record.networkSSID,
			FirstSeen:             record.firstSeen,
			LastSeen:              record.lastSeen,
			LastOnline:            record.lastOnline,
			IsOnline:              record.isOnline,
		})
	}

	return snapshots
}
