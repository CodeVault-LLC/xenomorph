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
	"github.com/codevault-llc/xenomorph/platform/client/internal/agentquic"
	clientconfig "github.com/codevault-llc/xenomorph/platform/client/internal/config"
	clientfs "github.com/codevault-llc/xenomorph/platform/client/internal/filesystem"
	"github.com/codevault-llc/xenomorph/platform/client/internal/replay"
	"github.com/codevault-llc/xenomorph/platform/shared/commandauth"
	sharedidentity "github.com/codevault-llc/xenomorph/platform/shared/identity"
)

const commandVerificationKeyBits = 3072

type controlTransport interface {
	Authenticate() (agent.DeviceAuthResult, error)
	SendHeartbeat() error
	SubmitAttestation(agent.EndpointAttestation) error
	PollNextCommand() (*agent.CommandEnvelope, error)
	SendCommandResult(agent.CommandResultPayload) error
	SendLogEntry(agent.LogEntryPayload) error
}

type appContext struct {
	httpClient        *http.Client
	gatewayURL        string
	tlsConfig         *tls.Config
	transport         controlTransport
	httpAgent         *agent.Agent
	quicClient        *agentquic.Client
	streamer          *screenStreamer
	validator         *agent.CommandValidator
	heartbeatInterval time.Duration
	transferPlane     clientfs.TransferPlane
}

func setupApp() (*appContext, error) {
	if err := removeLegacyRuntimeState(); err != nil {
		return nil, err
	}

	runtimeConfig, err := clientconfig.Load()
	if err != nil {
		return nil, err
	}

	tlsConfig, clientCertificate, err := loadClientTLS(runtimeConfig)
	if err != nil {
		return nil, err
	}

	httpClient := newHTTPClient(runtimeConfig.HTTPTimeout, tlsConfig)
	httpAgent := agent.New(httpClient, runtimeConfig.GatewayURL)

	audience, err := sharedidentity.AgentIDFromCertificate(clientCertificate)
	if err != nil {
		return nil, fmt.Errorf("derive command audience: %w", err)
	}

	ac := &appContext{
		httpClient:        httpClient,
		gatewayURL:        runtimeConfig.GatewayURL,
		tlsConfig:         tlsConfig,
		httpAgent:         httpAgent,
		streamer:          newScreenStreamer(runtimeConfig.GatewayURL, tlsConfig),
		heartbeatInterval: runtimeConfig.HeartbeatInterval,
	}

	validator, err := newCommandValidator(runtimeConfig, audience)
	if err != nil {
		return nil, fmt.Errorf("initialize command validator: %w", err)
	}

	ac.validator = validator
	if err := selectControlTransport(ac, runtimeConfig, audience); err != nil {
		return nil, err
	}

	ac.streamer.quicClient = ac.quicClient

	return ac, nil
}

func loadClientTLS(runtimeConfig clientconfig.Config) (*tls.Config, *x509.Certificate, error) {
	certificate, err := tls.LoadX509KeyPair(runtimeConfig.ClientCertificateFile, runtimeConfig.ClientPrivateKeyFile)
	if err != nil {
		return nil, nil, fmt.Errorf("load client certs: %w", err)
	}

	if len(certificate.Certificate) == 0 {
		return nil, nil, fmt.Errorf("load client certs: identity certificate is missing")
	}

	clientCertificate, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		return nil, nil, fmt.Errorf("parse client identity certificate: %w", err)
	}

	caData, err := os.ReadFile(filepath.Clean(runtimeConfig.CAFile))
	if err != nil {
		return nil, nil, fmt.Errorf("read CA cert: %w", err)
	}

	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caData) {
		return nil, nil, fmt.Errorf("read CA cert: no certificates found")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate}, RootCAs: caPool,
		ServerName: runtimeConfig.ServerName, MinVersion: tls.VersionTLS13,
		CurvePreferences: []tls.CurveID{tls.CurveP384},
	}

	return tlsConfig, clientCertificate, nil
}

func newHTTPClient(timeout time.Duration, tlsConfig *tls.Config) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &http.Transport{TLSClientConfig: tlsConfig.Clone()},
	}
}

func newCommandValidator(runtimeConfig clientconfig.Config, audience string) (*agent.CommandValidator, error) {
	verificationKey, keyID, err := loadCommandVerificationKey(runtimeConfig.CommandVerificationKeyFile)
	if err != nil {
		return nil, err
	}

	replayLedger, err := replay.Open(runtimeConfig.ReplayLedgerFile, runtimeConfig.ReplayAuthenticationKeyFile)
	if err != nil {
		return nil, fmt.Errorf("initialize command replay ledger: %w", err)
	}

	return agent.NewCommandValidatorWithReplayLedger(verificationKey, keyID, audience, replayLedger)
}

func selectControlTransport(ac *appContext, runtimeConfig clientconfig.Config, audience string) error {
	if runtimeConfig.TransportMode == clientconfig.TransportHTTP {
		ac.transport = ac.httpAgent
		ac.transferPlane = ac.httpAgent

		return nil
	}

	quicClient, err := agentquic.New(runtimeConfig, ac.tlsConfig, audience, ac.validator.KeyID())
	if err != nil {
		return err
	}

	startContext, cancel := context.WithTimeout(context.Background(), runtimeConfig.QUICHandshakeTimeout)
	err = quicClient.Start(startContext)

	cancel()

	if err == nil {
		ac.quicClient = quicClient
		ac.transport = quicClient
		ac.transferPlane = quicClient

		return nil
	}

	quicClient.Close()

	if runtimeConfig.TransportMode != clientconfig.TransportQUICFirst ||
		agentquic.IsSecurityFailure(err) || !runtimeConfig.HTTPFallbackUntil.After(time.Now().UTC()) {
		return fmt.Errorf("establish required QUIC transport: %w", err)
	}

	ac.transport = ac.httpAgent
	ac.transferPlane = ac.httpAgent
	_ = ac.httpAgent.SendLogEntry(agent.LogEntryPayload{
		Level: "WARN", Component: "client.runtime", Message: "event=quic_network_fallback",
	})

	return nil
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

func authenticateDevice(ac *appContext) (bool, error) {
	auth, err := ac.transport.Authenticate()
	if err != nil {
		return false, fmt.Errorf("authentication failed: %w", err)
	}

	return auth.RequiresAttestation, nil
}

func attestEndpoint(ac *appContext, requiresAttestation bool) error {
	if !requiresAttestation {
		return nil
	}

	attestation := agent.BuildEndpointAttestation(requiresAttestation, nil, nil)
	if err := ac.transport.SubmitAttestation(attestation); err != nil {
		return fmt.Errorf("endpoint attestation failed: %w", err)
	}

	return nil
}

func processCommand(ac *appContext, cmd *agent.CommandEnvelope) error {
	ctx, cancel := context.WithDeadline(context.Background(), cmd.ExpiresAt)
	defer cancel()

	decision, err := agent.HandleCommandWithTransferPlane(ctx, *cmd, ac.validator, ac.transferPlane)
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

	if err := ac.transport.SendCommandResult(decision.Result); err != nil {
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
	if ac == nil || ac.transport == nil {
		return
	}
	// A logging failure is intentionally not persisted or returned to prevent
	// recursive diagnostics and client-side data retention.
	_ = ac.transport.SendLogEntry(agent.LogEntryPayload{
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

	if ac.quicClient != nil {
		ac.quicClient.Close()
	}
}
