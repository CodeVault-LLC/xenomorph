// Package agent implements the remote support client runtime.
package agent

import (
	"encoding/json"
	"time"
)

// HeartbeatPayload is sent to the gateway with each authentication request.
type HeartbeatPayload struct {
	Hostname              string                 `json:"hostname"`
	OsVersion             string                 `json:"os_version"`
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
}

// DeviceAuthResult contains the gateway response to device authentication.
type DeviceAuthResult struct {
	EventID             string
	RequiresAttestation bool
}

// CommandEnvelope is a command received from the gateway for local execution.
type CommandEnvelope struct {
	ProtocolVersion int             `json:"protocol_version"`
	CommandID       string          `json:"command_id"`
	AudienceAgentID string          `json:"audience_agent_id"`
	Type            CommandType     `json:"type"`
	Payload         json.RawMessage `json:"payload"`
	RequestedBy     string          `json:"requested_by"`
	IssuedAt        time.Time       `json:"issued_at"`
	ExpiresAt       time.Time       `json:"expires_at"`
	Nonce           string          `json:"nonce"`
	Reason          string          `json:"reason"`
	KeyID           string          `json:"key_id"`
	Signature       string          `json:"signature"`
	// AcknowledgePersistence is transport-owned and excluded from the signed
	// envelope. The validator invokes it only after durable replay reservation.
	AcknowledgePersistence func() error `json:"-"`
}

// CommandResultPayload is sent to the gateway after command execution.
type CommandResultPayload struct {
	CommandID                string          `json:"command_id"`
	Type                     CommandType     `json:"type"`
	Status                   CommandStatus   `json:"status"`
	Reason                   string          `json:"reason"`
	RespondedAt              time.Time       `json:"responded_at"`
	ClientHostname           string          `json:"client_hostname"`
	OutputData               []byte          `json:"output_data,omitempty"`
	TerminalSessionID        string          `json:"terminal_session_id,omitempty"`
	TerminalShell            string          `json:"terminal_shell,omitempty"`
	TerminalCommand          string          `json:"terminal_command,omitempty"`
	TerminalWorkingDirectory string          `json:"terminal_working_directory,omitempty"`
	TerminalExitCode         int             `json:"terminal_exit_code,omitempty"`
	Result                   json.RawMessage `json:"result,omitempty"`
}

// LogEntryPayload is client-authored operational metadata submitted directly
// to the authenticated gateway. It must never contain telemetry payloads,
// command payloads, terminal output, screenshots, credentials, or error text.
type LogEntryPayload struct {
	Level     string `json:"level"`
	Message   string `json:"message"`
	Component string `json:"component"`
}
