package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
)

const (
	gatewayURL    = "https://localhost:8443"
	certPath      = "../infrastructure/certs"
	clientTimeout = 10 * time.Second
)

type appContext struct {
	httpClient *http.Client
	gatewayURL string
	statePath  string
	runtimeSt  agent.RuntimeState
	ag         *agent.Agent
	disconnect bool
}

func setupApp() (*appContext, error) {
	cert, err := tls.LoadX509KeyPair(certPath+"/client.crt", certPath+"/client.key")
	if err != nil {
		return nil, fmt.Errorf("load client certs: %w", err)
	}

	caCert, err := os.ReadFile(certPath + "/ca.crt")
	if err != nil {
		return nil, fmt.Errorf("read CA cert: %w", err)
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
		Timeout: clientTimeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	a := agent.New(httpClient, gatewayURL)
	disconnectOnDeny := agent.LoadDisconnectOnDenyFromEnv()

	statePath, err := agent.DefaultStatePath()
	if err != nil {
		return nil, fmt.Errorf("resolve state path: %w", err)
	}

	runtimeState, err := agent.LoadRuntimeState(statePath)
	if err != nil {
		runtimeState = agent.RuntimeState{}
	}

	return &appContext{
		httpClient: httpClient,
		gatewayURL: gatewayURL,
		statePath:  statePath,
		runtimeSt:  runtimeState,
		ag:         a,
		disconnect: disconnectOnDeny,
	}, nil
}

func stage1Auth(ac *appContext) (bool, error) {
	stage1, err := ac.ag.Authenticate()
	if err != nil {
		return false, fmt.Errorf("authentication failed: %w", err)
	}
	return stage1.IsNewAgent, nil
}

func stage2Entry(ac *appContext, isNewAgent bool) error {
	if !isNewAgent && ac.runtimeSt.OnboardingSent {
		return nil
	}

	entry := agent.BuildEntryPayload(isNewAgent, nil, nil)
	if err := ac.ag.SendEntryReport(entry); err != nil {
		return fmt.Errorf("entry report failed: %w", err)
	}

	ac.runtimeSt.OnboardingSent = true
	if err := agent.SaveRuntimeState(ac.statePath, ac.runtimeSt); err != nil {
		return fmt.Errorf("persist state failed: %w", err)
	}

	return nil
}

func processCommand(ac *appContext, cmd *agent.CommandEnvelope) (bool, error) {
	decision, err := agent.HandleCommandWithConsent(*cmd, nil, ac.disconnect)
	if err != nil {
		return false, fmt.Errorf("command handling failed: %w", err)
	}

	if err := ac.ag.SendCommandResult(decision.Result); err != nil {
		return false, fmt.Errorf("command result submission failed: %w", err)
	}

	if decision.DisconnectNow {
		return true, nil
	}

	return false, nil
}

func shutdown(ac *appContext) {
	removeStateFiles(ac.statePath)
}

func removeStateFiles(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	_ = os.Remove(path)
	_ = os.RemoveAll(strings.TrimSuffix(path, "agent-state.json"))
}
