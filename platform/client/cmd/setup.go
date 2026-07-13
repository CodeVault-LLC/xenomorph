package main

import (
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/codevault-llc/xenomorph/platform/client/internal/agent"
	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
	sharedidentity "github.com/codevault-llc/xenomorph/platform/shared/identity"
)

const (
	gatewayURL    string        = "https://localhost:8443"
	certPath      string        = "../infrastructure/certs"
	clientTimeout time.Duration = 10 * time.Second
)

type appContext struct {
	httpClient *http.Client
	gatewayURL string
	tlsConfig  *tls.Config
	statePath  string
	runtimeSt  agent.RuntimeState
	ag         *agent.Agent
	streamer   *screenStreamer
	validator  *agent.CommandValidator
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

	statePath, err := agent.DefaultStatePath()
	if err != nil {
		return nil, fmt.Errorf("resolve state path: %w", err)
	}

	runtimeState, err := agent.LoadRuntimeState(statePath)
	if err != nil {
		runtimeState = agent.RuntimeState{}
	}

	clientCertificate, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse client identity certificate: %w", err)
	}
	audience, err := sharedidentity.AgentIDFromCertificate(clientCertificate)
	if err != nil {
		return nil, fmt.Errorf("derive command audience: %w", err)
	}
	verificationKey, keyID, err := loadCommandVerificationKey(filepath.Join(certPath, "server.crt"))
	if err != nil {
		return nil, err
	}

	ac := &appContext{
		httpClient: httpClient,
		gatewayURL: gatewayURL,
		tlsConfig:  tlsConfig,
		statePath:  statePath,
		runtimeSt:  runtimeState,
		ag:         a,
		streamer:   newScreenStreamer(gatewayURL, tlsConfig),
	}
	validator, err := agent.NewCommandValidator(verificationKey, keyID, audience, runtimeState.SeenCommandNonces, func(nonce string) error {
		ac.runtimeSt.RecordCommandNonce(nonce)
		return agent.SaveRuntimeState(ac.statePath, ac.runtimeSt)
	})
	if err != nil {
		return nil, fmt.Errorf("initialize command validator: %w", err)
	}
	ac.validator = validator
	return ac, nil
}

func loadCommandVerificationKey(path string) (*rsa.PublicKey, string, error) {
	encoded, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, "", fmt.Errorf("read command verification certificate: %w", err)
	}
	block, _ := pem.Decode(encoded)
	if block == nil {
		return nil, "", fmt.Errorf("decode command verification certificate: invalid PEM data")
	}
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("parse command verification certificate: %w", err)
	}
	publicKey, ok := certificate.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, "", fmt.Errorf("parse command verification certificate: RSA key required")
	}
	keyID, err := commandauth.KeyID(publicKey)
	if err != nil {
		return nil, "", err
	}
	return publicKey, keyID, nil
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

func processCommand(ac *appContext, cmd *agent.CommandEnvelope) error {
	decision, err := agent.HandleCommand(*cmd, ac.validator)
	if err != nil {
		return fmt.Errorf("command handling failed: %w", err)
	}

	if decision.Result.Status == agent.CommandStatusExecuted {
		switch cmd.Type {
		case agent.CommandTypeStartScreenStream:
			if err := ac.streamer.Start(cmd.Payload); err != nil {
				decision.Result.Status = "rejected"
				decision.Result.Reason = err.Error()
			} else {
				decision.Result.Reason = "screen stream started"
			}
		case agent.CommandTypeStopScreenStream:
			ac.streamer.Stop()
			decision.Result.Reason = "screen stream stopped"
		}
	}

	if err := ac.ag.SendCommandResult(decision.Result); err != nil {
		return fmt.Errorf("command result submission failed: %w", err)
	}

	reportClientLog(ac, "INFO", "client.command", fmt.Sprintf("command_id=%s type=%s status=%s reason=%s", decision.Result.CommandID, decision.Result.Type, decision.Result.Status, decision.Result.Reason))

	return nil
}

func reportClientLog(ac *appContext, level, component, message string) {
	if ac == nil || ac.ag == nil {
		return
	}
	_ = ac.ag.SendLogEntry(agent.LogEntryPayload{
		Level:     level,
		Component: component,
		Message:   message,
	})
}

func shutdown(ac *appContext) {
	if ac == nil {
		return
	}
	ac.streamer.Stop()
}
