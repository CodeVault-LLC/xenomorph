// Package activity tracks agent online/offline state from authenticated
// heartbeat events. This package owns the liveness state machine and
// transition notification logic.
package activity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
)

// Monitor derives online/offline transitions from authenticated heartbeat
// traffic. Presence state is held in memory and is not persisted across
// gateway restarts.
//
// The Monitor owns two operations:
//   - ProcessHeartbeat updates the last-seen timestamp for an agent and
//     emits an online notification when the agent was previously offline or
//     when its last-seen exceeded the offlineAfter threshold.
//   - Sweep iterates all tracked agents and emits offline notifications for
//     those whose last-seen exceeds offlineAfter.
type Monitor struct {
	notify       *provider.Fanout
	offlineAfter time.Duration
	now          func() time.Time

	mu     sync.Mutex
	online map[string]presence
	all    map[string]clientRecord
}

// presence holds agent liveness state in memory.
type presence struct {
	lastSeen time.Time
	hostname string
}

// ClientSnapshot is a read-only all-time view of an authenticated agent known
// to the gateway during the current process lifetime.
type ClientSnapshot struct {
	AgentID               string    `json:"agent_id"`
	Hostname              string    `json:"hostname"`
	ClientIP              string    `json:"client_ip"`
	OSVersion             string    `json:"os_version"`
	CPULoad               float64   `json:"cpu_load"`
	RAMUsage              float64   `json:"ram_usage"`
	UptimeSeconds         uint64    `json:"uptime_seconds"`
	CPUModel              string    `json:"cpu_model"`
	CPUCores              int32     `json:"cpu_cores"`
	CPUThreads            int32     `json:"cpu_threads"`
	TotalRAMBytes         uint64    `json:"total_ram_bytes"`
	GPUDevices            []string  `json:"gpu_devices"`
	NetworkName           string    `json:"network_name"`
	NetworkAddresses      []string  `json:"network_addresses"`
	KernelVersion         string    `json:"kernel_version"`
	CPUFrequencyMHz       uint64    `json:"cpu_frequency_mhz"`
	NetworkOnline         bool      `json:"network_online"`
	NetworkLinkSpeedMbps  uint64    `json:"network_link_speed_mbps"`
	NetworkType           string    `json:"network_type"`
	TotalStorageBytes     uint64    `json:"total_storage_bytes"`
	AvailableStorageBytes uint64    `json:"available_storage_bytes"`
	NetworkSSID           string    `json:"network_ssid"`
	FirstSeen             time.Time `json:"first_seen"`
	LastSeen              time.Time `json:"last_seen"`
	LastOnline            time.Time `json:"last_online"`
	IsOnline              bool      `json:"is_online"`
}

// clientRecord stores the all-time presence metadata for a gateway-authenticated
// agent. Identity and IP are gateway-authored. Hostname, OS, CPU, and RAM are
// client-authored telemetry labels and are not used as identity evidence.
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
	networkSSID           string
	firstSeen             time.Time
	lastSeen              time.Time
	lastOnline            time.Time
	isOnline              bool
}

// NewMonitor creates a Monitor that emits offline notifications when an
// agent has not been seen for offlineAfter duration. The notifier fanout
// receives ActivityEvent notifications for each transition.
//
// When notify is nil, the monitor tracks state internally but produces no
// outbound notifications. This is valid for deployments without notification
// providers.
func NewMonitor(offlineAfter time.Duration, notify *provider.Fanout) *Monitor {
	if notify == nil {
		notify = provider.NewFanout(nil)
	}

	return &Monitor{
		notify:       notify,
		offlineAfter: offlineAfter,
		now:          time.Now,
		online:       make(map[string]presence),
		all:          make(map[string]clientRecord),
	}
}

// ProcessHeartbeat updates liveness state for an agent based on a
// gateway-authenticated envelope. An online notification is emitted when the
// agent was previously unknown or when its last-seen exceeded the
// offlineAfter threshold.
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
		networkSSID = hb.GetNetworkSsid()
	}

	eventTime := m.now()
	if envelope.Timestamp != nil {
		eventTime = envelope.Timestamp.AsTime()
	}

	var shouldNotifyOnline bool

	m.mu.Lock()
	state, exists := m.online[agentID]
	if !exists || eventTime.Sub(state.lastSeen) > m.offlineAfter {
		shouldNotifyOnline = true
	}
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
	record.networkSSID = networkSSID
	record.lastSeen = eventTime
	record.lastOnline = eventTime
	record.isOnline = true
	m.all[agentID] = record
	m.mu.Unlock()

	if shouldNotifyOnline {
		return m.notify.Notify(ctx, provider.ActivityEvent{
			AgentID:    agentID,
			Hostname:   hostname,
			OccurredAt: eventTime,
			Status:     provider.StatusOnline,
			Source:     "heartbeat",
		})
	}

	return nil
}

// Sweep checks every tracked agent and emits offline notifications for those
// whose last-seen exceeds offlineAfter. Stale agents are removed from the
// tracking map. Errors from individual notification deliveries are collected
// and returned as a joined error.
//
// Call Sweep periodically from a ticker. The sweep interval should be
// shorter than offlineAfter to ensure timely offline detection.
func (m *Monitor) Sweep(ctx context.Context) error {
	now := m.now()

	type staleAgent struct {
		agentID  string
		hostname string
	}

	stale := make([]staleAgent, 0)

	m.mu.Lock()
	for agentID, state := range m.online {
		if now.Sub(state.lastSeen) > m.offlineAfter {
			stale = append(stale, staleAgent{agentID: agentID, hostname: state.hostname})
			delete(m.online, agentID)
			record, ok := m.all[agentID]
			if ok {
				record.isOnline = false
				m.all[agentID] = record
			}
		}
	}
	m.mu.Unlock()

	var errs []error
	for _, agent := range stale {
		err := m.notify.Notify(ctx, provider.ActivityEvent{
			AgentID:    agent.agentID,
			Hostname:   agent.hostname,
			OccurredAt: now,
			Status:     provider.StatusOffline,
			Source:     "heartbeat-timeout",
		})
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Snapshot returns a point-in-time view of an agent's presence state.
// Returns false when the agent is not currently tracked (never seen or
// expired by Sweep).
func (m *Monitor) Snapshot(agentID string) (provider.AgentSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.online[agentID]
	if !ok {
		return provider.AgentSnapshot{}, false
	}

	return provider.AgentSnapshot{
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
			NetworkSSID:           record.networkSSID,
			FirstSeen:             record.firstSeen,
			LastSeen:              record.lastSeen,
			LastOnline:            record.lastOnline,
			IsOnline:              record.isOnline,
		})
	}

	return snapshots
}
