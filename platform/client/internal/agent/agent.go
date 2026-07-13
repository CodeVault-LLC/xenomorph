// Package agent implements the remote support client runtime.
package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HeartbeatPayload is sent to the gateway with each authentication request.
type HeartbeatPayload struct {
	Hostname              string   `json:"hostname"`
	OsVersion             string   `json:"os_version"`
	CPULoad               float64  `json:"cpu_load"`
	RAMUsage              float64  `json:"ram_usage"`
	UptimeSeconds         uint64   `json:"uptime_seconds"`
	CPUModel              string   `json:"cpu_model"`
	CPUCores              int32    `json:"cpu_cores"`
	CPUThreads            int32    `json:"cpu_threads"`
	TotalRAMBytes         uint64   `json:"total_ram_bytes"`
	GPUDevices            []string `json:"gpu_devices"`
	NetworkName           string   `json:"network_name"`
	NetworkAddresses      []string `json:"network_addresses"`
	KernelVersion         string   `json:"kernel_version"`
	CPUFrequencyMHz       uint64   `json:"cpu_frequency_mhz"`
	NetworkOnline         bool     `json:"network_online"`
	NetworkLinkSpeedMbps  uint64   `json:"network_link_speed_mbps"`
	NetworkType           string   `json:"network_type"`
	TotalStorageBytes     uint64   `json:"total_storage_bytes"`
	AvailableStorageBytes uint64   `json:"available_storage_bytes"`
	NetworkSSID           string   `json:"network_ssid"`
}

// Stage1AuthResult contains the gateway response to the initial authentication.
type Stage1AuthResult struct {
	EventID    string
	IsNewAgent bool
}

// CommandEnvelope is a command received from the gateway for local execution.
type CommandEnvelope struct {
	CommandID   string          `json:"command_id"`
	Type        CommandType     `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	RequestedBy string          `json:"requested_by"`
	IssuedAt    time.Time       `json:"issued_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Reason      string          `json:"reason"`
	Signature   string          `json:"signature"`
}

// CommandResultPayload is sent to the gateway after command execution.
type CommandResultPayload struct {
	CommandID                string        `json:"command_id"`
	Type                     CommandType   `json:"type"`
	Status                   CommandStatus `json:"status"`
	Reason                   string        `json:"reason"`
	RespondedAt              time.Time     `json:"responded_at"`
	ClientHostname           string        `json:"client_hostname"`
	OutputData               []byte        `json:"output_data,omitempty"`
	TerminalSessionID        string        `json:"terminal_session_id,omitempty"`
	TerminalShell            string        `json:"terminal_shell,omitempty"`
	TerminalCommand          string        `json:"terminal_command,omitempty"`
	TerminalWorkingDirectory string        `json:"terminal_working_directory,omitempty"`
	TerminalExitCode         int           `json:"terminal_exit_code,omitempty"`
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
