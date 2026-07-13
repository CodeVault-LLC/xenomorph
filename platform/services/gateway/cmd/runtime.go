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
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider/discord"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/transport"
)

func buildGatewayServer(cfg config.GatewayConfig, natsBroker *broker.NATS, notifier *provider.Fanout, discordProvider *discord.Provider, monitor *activity.Monitor) (*transport.Server, error) {
	signingKey, signingKeyID, err := command.LoadRSASigningKey(filepath.Join(cfg.CertPath, "server.key"))
	if err != nil {
		return nil, fmt.Errorf("command signing setup: %w", err)
	}
	queue, err := command.NewQueue(signingKey, signingKeyID)
	if err != nil {
		return nil, fmt.Errorf("command queue setup: %w", err)
	}
	fileService, err := buildFileWorkspace(cfg, queue)
	if err != nil {
		return nil, err
	}
	var discordPoster provider.DiscordPoster
	if discordProvider != nil {
		discordPoster = discordProvider
	}
	server := transport.NewServer(natsBroker, notifier, queue, discordPoster, monitor)
	server.ConfigureFileWorkspace(fileService, cfg.FileOperatorID)
	server.ConfigureDashboardOrigin(cfg.DashboardOrigin)
	return server, nil
}

func startDiscordGateway(ctx context.Context, cfg config.GatewayConfig, server *transport.Server, discordProvider *discord.Provider) error {
	if discordProvider == nil {
		return nil
	}
	listener, err := discord.NewGatewayListener(cfg.DiscordBotToken, cfg.DiscordGuildID, server, discordProvider)
	if err != nil {
		return fmt.Errorf("discord gateway listener creation: %w", err)
	}
	if err := listener.Start(ctx); err != nil {
		return fmt.Errorf("discord gateway start: %w", err)
	}
	return nil
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
