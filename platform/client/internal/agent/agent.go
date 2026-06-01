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
	Hostname  string  `json:"hostname"`
	OsVersion string  `json:"os_version"`
	CPULoad   float64 `json:"cpu_load"`
	RAMUsage  float64 `json:"ram_usage"`
}

// Stage1AuthResult contains the gateway response to the initial authentication.
type Stage1AuthResult struct {
	EventID    string
	IsNewAgent bool
}

// CommandEnvelope is a command received from the gateway for local execution.
type CommandEnvelope struct {
	CommandID   string          `json:"command_id"`
	Type        string          `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	RequestedBy string          `json:"requested_by"`
	IssuedAt    time.Time       `json:"issued_at"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Reason      string          `json:"reason"`
	Signature   string          `json:"signature"`
}

// CommandResultPayload is sent to the gateway after command execution.
type CommandResultPayload struct {
	CommandID      string    `json:"command_id"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	Reason         string    `json:"reason"`
	UserApproved   bool      `json:"user_approved"`
	DisconnectNow  bool      `json:"disconnect_now"`
	RespondedAt    time.Time `json:"responded_at"`
	ClientHostname string    `json:"client_hostname"`
	OutputData     []byte    `json:"output_data,omitempty"`
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

	req, err := http.NewRequest("POST", a.gatewayURL+"/ingest/heartbeat", bytes.NewBuffer(data))
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

	req, err := http.NewRequest("POST", a.gatewayURL+"/ingest/entry", bytes.NewBuffer(data))
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
	req, err := http.NewRequest("GET", a.gatewayURL+"/commands/next", nil)
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

	req, err := http.NewRequest("POST", a.gatewayURL+"/commands/result", bytes.NewBuffer(data))
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
