package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"runtime"
)

type HeartbeatPayload struct {
	Hostname  string  `json:"hostname"`
	OsVersion string  `json:"os_version"`
	CpuLoad   float64 `json:"cpu_load"`
	RamUsage  float64 `json:"ram_usage"`
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
	payload := HeartbeatPayload{
		Hostname:  "laptop-edge-01",
		OsVersion: runtime.GOOS + "/" + runtime.GOARCH,
		CpuLoad:   15.5,
		RamUsage:  42.0,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	log.Printf("➡️ Sending Heartbeat: %s", string(data))

	req, err := http.NewRequest("POST", a.gatewayURL+"/ingest/heartbeat", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 202 && resp.StatusCode != 200 {
		return fmt.Errorf("gateway rejected heartbeat: status %d", resp.StatusCode)
	}

	return nil
}
