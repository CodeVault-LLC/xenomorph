package main

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
)

const (
	GatewayURL = "https://localhost:8443"
	CertPath   = "../infrastructure/certs"
)

func main() {
	cert, err := tls.LoadX509KeyPair(CertPath+"/client.crt", CertPath+"/client.key")
	if err != nil {
		log.Fatalf("❌ Failed to load client certs: %v", err)
	}

	caCert, err := os.ReadFile(CertPath + "/ca.crt")
	if err != nil {
		log.Fatalf("❌ Failed to read CA cert: %v", err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS13,
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	a := agent.New(httpClient, GatewayURL)
	disconnectOnDeny := agent.LoadDisconnectOnDenyFromEnv()
	statePath, err := agent.DefaultStatePath()
	if err != nil {
		log.Fatalf("❌ Failed to resolve runtime state path: %v", err)
	}
	runtimeState, err := agent.LoadRuntimeState(statePath)
	if err != nil {
		log.Printf("⚠️ Failed to load runtime state, continuing with defaults: %v", err)
	}

	log.Println("✅ Agent initialized. Stage 1 authentication...")
	stage1, err := a.Authenticate()
	if err != nil {
		log.Fatalf("❌ Stage 1 authentication failed: %v", err)
	}
	log.Printf("✅ Stage 1 complete. event_id=%s is_new_agent=%t", stage1.EventID, stage1.IsNewAgent)

	sendExtendedEntry := stage1.IsNewAgent || !runtimeState.OnboardingSent
	entry := agent.BuildEntryPayload(sendExtendedEntry, nil, nil)

	log.Printf("✅ Stage 2 entry report (extended=%t)...", sendExtendedEntry)
	if err := a.SendEntryReport(entry); err != nil {
		log.Printf("⚠️ Stage 2 entry report failed: %v", err)
	} else {
		runtimeState.OnboardingSent = true
		if err := agent.SaveRuntimeState(statePath, runtimeState); err != nil {
			log.Printf("⚠️ Failed to persist runtime state: %v", err)
		}
		log.Printf("✅ Stage 2 complete")
	}

	log.Println("✅ Stage 3 command loop active")
	log.Printf("🔐 Command safety policy: disconnect_on_deny=%t", disconnectOnDeny)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("💓 Stage 3 heartbeat and command poll")

		if err := a.SendHeartbeat(); err != nil {
			log.Printf("⚠️ Heartbeat failed: %v", err)
		} else {
			log.Printf("👍 Heartbeat acknowledged")
		}

		cmd, err := a.PollNextCommand()
		if err != nil {
			log.Printf("⚠️ Command poll failed: %v", err)
			continue
		}

		if cmd == nil {
			continue
		}

		log.Printf("📥 Received command command_id=%s type=%s", cmd.CommandID, cmd.Type)

		decision, err := agent.HandleCommandWithConsent(*cmd, agent.ZenityApprover{}, disconnectOnDeny)
		if err != nil {
			log.Printf("⚠️ Command handling failed: %v", err)
			continue
		}

		if err := a.SendCommandResult(decision.Result); err != nil {
			log.Printf("⚠️ Failed to submit command result: %v", err)
		}

		if decision.DisconnectNow {
			log.Printf("⛔ Disconnecting due to command denial policy command_id=%s", cmd.CommandID)
			return
		}
	}
}
