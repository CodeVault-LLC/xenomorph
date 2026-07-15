package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/activity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/agentquic"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/clientbuild"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/config"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/keyservice"
	operationjournal "github.com/codevault-llc/xenomorph/platform/services/gateway/internal/operation"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/transport"
)

func setupCommandSigner(ctx context.Context, cfg config.GatewayConfig, keys *keyservice.Service) (*keyservice.CommandSigner, error) {
	if keyPathConflictsWithTLS(cfg.CommandSigningKeyPath, cfg.CertPath) || keyPathConflictsWithTLS(cfg.CommandPublicKeyPath, cfg.CertPath) {
		return nil, fmt.Errorf("command key paths must not reuse or overwrite TLS artifacts")
	}

	signingKey, err := keys.LoadOrCreateCommandSigner(ctx, cfg.CommandSigningKeyPath, cfg.CommandPublicKeyPath, 1)
	if err != nil {
		return nil, fmt.Errorf("command signing setup: %w", err)
	}

	return signingKey, nil
}

func keyPathConflictsWithTLS(keyPath, certPath string) bool {
	for _, name := range []string{"ca.crt", "server.crt", "server.key"} {
		if samePath(keyPath, filepath.Join(certPath, name)) {
			return true
		}
	}

	return false
}

func buildGatewayServer(cfg config.GatewayConfig, signingKey command.Signer, keys *keyservice.Service, natsBroker *broker.NATS, monitor *activity.Monitor) (*transport.Server, *command.Queue, error) {
	queue, err := command.NewDurableQueueWithSigner(signingKey, filepath.Join(cfg.StatePath, "command-journal.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("command queue setup: %w", err)
	}

	fileService, err := buildFileWorkspace(cfg, queue)
	if err != nil {
		return nil, nil, err
	}

	server := transport.NewServer(natsBroker, queue, monitor)

	clientBuilder, err := clientbuild.New(cfg.ClientBuildSource)
	if err != nil {
		return nil, nil, fmt.Errorf("client artifact builder setup: %w", err)
	}

	server.ConfigureClientBuilder(clientBuilder)

	operationJournal, err := operationjournal.Open(filepath.Join(cfg.StatePath, "operation-journal.json"))
	if err != nil {
		return nil, nil, fmt.Errorf("operation journal setup: %w", err)
	}

	server.ConfigureOperationJournal(operationJournal)
	server.ConfigureFileWorkspace(fileService, cfg.FileOperatorID)
	server.ConfigureDashboardOrigin(cfg.DashboardOrigin)
	server.ConfigureReadiness(keys)

	return server, queue, nil
}

func samePath(first, second string) bool {
	firstAbsolute, firstErr := filepath.Abs(filepath.Clean(first))
	secondAbsolute, secondErr := filepath.Abs(filepath.Clean(second))

	return firstErr == nil && secondErr == nil && firstAbsolute == secondAbsolute
}

func startDashboardService(ctx context.Context, cfg config.GatewayConfig, server *transport.Server, failures chan<- error) {
	go func() {
		slog.Info("dashboard API server starting", "addr", cfg.DashboardAddr)

		if err := transport.RunDashboard(ctx, cfg.DashboardAddr, cfg.CertPath, server.DashboardRuntime()); err != nil {
			failures <- fmt.Errorf("dashboard listener: %w", err)
		}
	}()
}

func buildAgentQUICListener(cfg config.GatewayConfig, server *transport.Server, queue *command.Queue, signer command.Signer) (*agentquic.Listener, error) {
	listener, err := agentquic.NewListener(cfg.AgentQUIC, server, queue, signer.KeyID())
	if err != nil {
		return nil, fmt.Errorf("agent QUIC listener setup: %w", err)
	}

	return listener, nil
}
