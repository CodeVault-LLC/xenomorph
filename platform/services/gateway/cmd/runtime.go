package main

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/activity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/config"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/keyservice"
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

func buildGatewayServer(cfg config.GatewayConfig, signingKey command.Signer, keys *keyservice.Service, natsBroker *broker.NATS, monitor *activity.Monitor) (*transport.Server, error) {
	queue, err := command.NewQueueWithSigner(signingKey)
	if err != nil {
		return nil, fmt.Errorf("command queue setup: %w", err)
	}
	fileService, err := buildFileWorkspace(cfg, queue)
	if err != nil {
		return nil, err
	}
	server := transport.NewServer(natsBroker, queue, monitor)
	server.ConfigureFileWorkspace(fileService, cfg.FileOperatorID)
	server.ConfigureDashboardOrigin(cfg.DashboardOrigin)
	server.ConfigureReadiness(keys)
	return server, nil
}

func samePath(first, second string) bool {
	firstAbsolute, firstErr := filepath.Abs(filepath.Clean(first))
	secondAbsolute, secondErr := filepath.Abs(filepath.Clean(second))
	return firstErr == nil && secondErr == nil && firstAbsolute == secondAbsolute
}

func startHTTPServers(ctx context.Context, cfg config.GatewayConfig, server *transport.Server) {
	go func() {
		if err := server.Run(cfg.ListenAddr, cfg.CertPath); err != nil {
			slog.Error("gateway server terminated with error", "error", err)
		}
	}()
	go func() {
		slog.Info("dashboard API server starting", "addr", cfg.DashboardAddr)
		if err := transport.RunDashboard(ctx, cfg.DashboardAddr, cfg.CertPath, server.DashboardRuntime()); err != nil {
			slog.Error("dashboard server terminated with error", "error", err)
		}
	}()
}
