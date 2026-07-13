// Package agent implements the remote support client runtime.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
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

// PutChunk uploads one checksum-bound chunk through the mTLS gateway plane.
func (a *Agent) PutChunk(ctx context.Context, transferID, token string, index int, data []byte) error {
	endpoint := a.transferChunkURL(transferID, index)
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build transfer chunk upload: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/octet-stream")
	response, err := a.client.Do(request)
	if err != nil {
		return fmt.Errorf("upload transfer chunk: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("upload transfer chunk rejected: status %d", response.StatusCode)
	}
	return nil
}

// GetChunk downloads one checksum-bound chunk through the mTLS gateway plane.
func (a *Agent) GetChunk(ctx context.Context, transferID, token string, index int, expectedSize int64) ([]byte, error) {
	if expectedSize <= 0 || expectedSize > 4<<20 {
		return nil, fmt.Errorf("transfer chunk size is outside limit")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, a.transferChunkURL(transferID, index), nil)
	if err != nil {
		return nil, fmt.Errorf("build transfer chunk download: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := a.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download transfer chunk: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download transfer chunk rejected: status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, expectedSize+1))
	if err != nil {
		return nil, fmt.Errorf("read transfer chunk: %w", err)
	}
	if int64(len(data)) != expectedSize {
		return nil, fmt.Errorf("transfer chunk size mismatch")
	}
	return data, nil
}

// Finalize asks the gateway to verify the complete staged transfer object.
func (a *Agent) Finalize(ctx context.Context, transferID, token string) error {
	endpoint := a.gatewayURL + "/files/transfers/" + url.PathEscape(transferID) + "/finalize"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build transfer finalization: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := a.client.Do(request)
	if err != nil {
		return fmt.Errorf("finalize transfer: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("transfer finalization rejected: status %d", response.StatusCode)
	}
	return nil
}

func (a *Agent) transferChunkURL(transferID string, index int) string {
	return a.gatewayURL + "/files/transfers/" + url.PathEscape(transferID) + "/chunks/" + strconv.Itoa(index)
}

// Stage1AuthResult contains the gateway response to the initial authentication.
type Stage1AuthResult struct {
	EventID    string
	IsNewAgent bool
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

// LogEntryPayload is client-authored diagnostic information submitted to the
// gateway for dashboard visibility.
type LogEntryPayload struct {
	Level     string `json:"level"`
	Message   string `json:"message"`
	Component string `json:"component"`
}

// Agent manages communication with the gateway server.
type Agent struct {
	client     *http.Client
	gatewayURL string
}

// New creates an Agent with the given HTTP client and gateway URL.
func New(client *http.Client, gatewayURL string) *Agent {
	return &Agent{
		client:     client,
		gatewayURL: gatewayURL,
	}
}

// SendHeartbeat submits a heartbeat to the gateway by reusing the auth flow.
func (a *Agent) SendHeartbeat() error {
	_, err := a.Authenticate()
	return err
}

// Authenticate performs stage-1 authentication with the gateway.
func (a *Agent) Authenticate() (Stage1AuthResult, error) {
	payload := BuildHeartbeatPayload(nil)

	data, err := json.Marshal(payload)
	if err != nil {
		return Stage1AuthResult{}, err
	}

	req, err := http.NewRequest(http.MethodPost, a.gatewayURL+"/ingest/heartbeat", bytes.NewBuffer(data))
	if err != nil {
		return Stage1AuthResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return Stage1AuthResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 202 && resp.StatusCode != 200 {
		return Stage1AuthResult{}, fmt.Errorf("gateway rejected heartbeat: status %d", resp.StatusCode)
	}

	var ack struct {
		EventID    string `json:"event_id"`
		IsNewAgent bool   `json:"is_new_agent"`
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, heartbeatResponseSize))
	if readErr != nil {
		return Stage1AuthResult{}, fmt.Errorf("read heartbeat response: %w", readErr)
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &ack); err != nil {
			return Stage1AuthResult{}, fmt.Errorf("decode heartbeat response: %w", err)
		}
	}

	return Stage1AuthResult{EventID: ack.EventID, IsNewAgent: ack.IsNewAgent}, nil
}

// SendEntryReport submits the stage-2 onboarding payload to the gateway.
func (a *Agent) SendEntryReport(payload EntryPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, a.gatewayURL+"/ingest/entry", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway rejected entry report: status %d", resp.StatusCode)
	}

	return nil
}

// PollNextCommand fetches the next pending command from the gateway queue.
func (a *Agent) PollNextCommand() (*CommandEnvelope, error) {
	req, err := http.NewRequest(http.MethodGet, a.gatewayURL+"/commands/next", nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("command poll failed: status %d", resp.StatusCode)
	}

	var cmd CommandEnvelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, commandResponseSize)).Decode(&cmd); err != nil {
		return nil, fmt.Errorf("decode command payload: %w", err)
	}

	return &cmd, nil
}

// SendCommandResult submits the command execution result to the gateway.
func (a *Agent) SendCommandResult(payload CommandResultPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, a.gatewayURL+"/commands/result", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("command result rejected: status %d", resp.StatusCode)
	}

	return nil
}

// SendLogEntry submits a bounded client diagnostic log to the gateway.
func (a *Agent) SendLogEntry(payload LogEntryPayload) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, a.gatewayURL+"/ingest/logs", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("log entry rejected: status %d", resp.StatusCode)
	}

	return nil
}
