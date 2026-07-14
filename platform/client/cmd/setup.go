package main

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
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
	gatewayURL                 string        = "https://localhost:8443"
	certPath                   string        = "../infrastructure/certs"
	clientTimeout              time.Duration = 10 * time.Second
	commandVerificationKeyBits               = 3072
)

type appContext struct {
	httpClient *http.Client
	gatewayURL string
	tlsConfig  *tls.Config
	ag         *agent.Agent
	streamer   *screenStreamer
	validator  *agent.CommandValidator
}

func setupApp() (*appContext, error) {
	if err := removeLegacyRuntimeState(); err != nil {
		return nil, err
	}

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
		Certificates:     []tls.Certificate{cert},
		RootCAs:          caCertPool,
		ServerName:       "localhost",
		MinVersion:       tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.CurveP384},
	}

	httpClient := &http.Client{
		Timeout: clientTimeout,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	a := agent.New(httpClient, gatewayURL)

	clientCertificate, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("parse client identity certificate: %w", err)
	}
	audience, err := sharedidentity.AgentIDFromCertificate(clientCertificate)
	if err != nil {
		return nil, fmt.Errorf("derive command audience: %w", err)
	}
	verificationKey, keyID, err := loadCommandVerificationKey(filepath.Join(certPath, "command-signing.pub"))
	if err != nil {
		return nil, err
	}

	ac := &appContext{
		httpClient: httpClient,
		gatewayURL: gatewayURL,
		tlsConfig:  tlsConfig,
		ag:         a,
		streamer:   newScreenStreamer(gatewayURL, tlsConfig),
	}
	validator, err := agent.NewCommandValidator(verificationKey, keyID, audience)
	if err != nil {
		return nil, fmt.Errorf("initialize command validator: %w", err)
	}
	ac.validator = validator
	return ac, nil
}

// removeLegacyRuntimeState removes the pre-stateless client state file. The
// client never recreates this file; failure leaves data at rest and blocks
// startup until the condition is remediated.
func removeLegacyRuntimeState() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory for legacy state cleanup: %w", err)
	}
	return removeLegacyRuntimeStateAt(homeDir)
}

func removeLegacyRuntimeStateAt(homeDir string) error {
	statePath := filepath.Join(homeDir, ".xenomorph", "agent-state.json")
	if err := os.Remove(statePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove legacy runtime state: %w", err)
	}
	return nil
}

func loadCommandVerificationKey(path string) (*rsa.PublicKey, string, error) {
	encoded, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, "", fmt.Errorf("read command verification key: %w", err)
	}
	block, remainder := pem.Decode(encoded)
	if block == nil || len(remainder) != 0 {
		return nil, "", fmt.Errorf("decode command verification key: invalid PEM data")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, "", fmt.Errorf("parse command verification key: %w", err)
	}
	publicKey, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, "", fmt.Errorf("parse command verification key: RSA key required")
	}
	if publicKey.N.BitLen() < commandVerificationKeyBits {
		return nil, "", fmt.Errorf("parse command verification key: RSA key must contain at least 3072 bits")
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
	if !isNewAgent {
		return nil
	}

	entry := agent.BuildEntryPayload(isNewAgent, nil, nil)
	if err := ac.ag.SendEntryReport(entry); err != nil {
		return fmt.Errorf("entry report failed: %w", err)
	}

	return nil
}

func processCommand(ac *appContext, cmd *agent.CommandEnvelope) error {
	ctx, cancel := context.WithDeadline(context.Background(), cmd.ExpiresAt)
	defer cancel()
	decision, err := agent.HandleCommandWithTransferPlane(ctx, *cmd, ac.validator, ac.ag)
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
		reportClientLog(ac, "ERROR", "client.command", "event=command_result_submission_failed")
		return fmt.Errorf("command result submission failed: %w", err)
	}
	reportClientLog(ac, "INFO", "client.command", "event=command_completed")

	return nil
}

// reportClientLog submits operational metadata to the gateway and deliberately
// discards delivery failures. Diagnostic delivery must not alter client
// behavior, and no retry queue or local log file is permitted on the client.
func reportClientLog(ac *appContext, level, component, message string) {
	if ac == nil || ac.ag == nil {
		return
	}
	// A logging failure is intentionally not persisted or returned to prevent
	// recursive diagnostics and client-side data retention.
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
