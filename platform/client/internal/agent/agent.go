package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type HeartbeatPayload struct {
	Hostname  string  `json:"hostname"`
	OsVersion string  `json:"os_version"`
	CpuLoad   float64 `json:"cpu_load"`
	RamUsage  float64 `json:"ram_usage"`
}

type Stage1AuthResult struct {
	EventID    string
	IsNewAgent bool
}

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

type Agent struct {
	client     *http.Client
	gatewayURL string
}

func New(client *http.Client, gatewayURL string) *Agent {
	return &Agent{
		client:     client,
		gatewayURL: gatewayURL,
	}
}

func (a *Agent) SendHeartbeat() error {
	_, err := a.Authenticate()
	return err
}

func (a *Agent) Authenticate() (Stage1AuthResult, error) {
	payload := BuildHeartbeatPayload(nil)

	data, err := json.Marshal(payload)
	if err != nil {
		return Stage1AuthResult{}, err
	}

	log.Printf("➡️ Sending Heartbeat: %s", string(data))

	req, err := http.NewRequest("POST", a.gatewayURL+"/ingest/heartbeat", bytes.NewBuffer(data))
	if err != nil {
		return Stage1AuthResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return Stage1AuthResult{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 202 && resp.StatusCode != 200 {
		return Stage1AuthResult{}, fmt.Errorf("gateway rejected heartbeat: status %d", resp.StatusCode)
	}

	var ack struct {
		EventID    string `json:"event_id"`
		IsNewAgent bool   `json:"is_new_agent"`
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway rejected entry report: status %d", resp.StatusCode)
	}

	return nil
}

func (a *Agent) PollNextCommand() (*CommandEnvelope, error) {
	req, err := http.NewRequest("GET", a.gatewayURL+"/commands/next", nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("command poll failed: status %d", resp.StatusCode)
	}

	var cmd CommandEnvelope
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8192)).Decode(&cmd); err != nil {
		return nil, fmt.Errorf("decode command payload: %w", err)
	}

	if cmd.CommandID == "" {
		log.Printf("⚠️ command poll returned empty command id")
	}

	return &cmd, nil
}

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("command result rejected: status %d", resp.StatusCode)
	}

	return nil
}
