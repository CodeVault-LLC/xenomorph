// Command entry point for the gateway service.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/activity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/config"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider/discord"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/sdk"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/transport"
)

func run() error {
	if err := sdk.InitLogger(""); err != nil {
		return fmt.Errorf("logger initialization: %w", err)
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("invalid gateway configuration: %w", err)
	}

	natsBroker, err := broker.New(cfg.NATSURL)
	if err != nil {
		return fmt.Errorf("NATS connection: %w", err)
	}
	defer natsBroker.Close()

	notifier, discordProvider, err := buildNotifier(cfg)
	if err != nil {
		return fmt.Errorf("notification provider setup: %w", err)
	}

	monitor := activity.NewMonitor(cfg.ActivityOfflineAfter, notifier)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := activity.StartStream(ctx, natsBroker, monitor, cfg.ActivitySweepInterval); err != nil {
		return fmt.Errorf("activity monitoring setup: %w", err)
	}

	cmdQueue := command.NewQueue()

	var discordPoster provider.DiscordPoster
	if discordProvider != nil {
		discordPoster = discordProvider
	}

	srv := transport.NewServer(natsBroker, notifier, cmdQueue, discordPoster, monitor)

	if discordProvider != nil {
		gatewayListener, err := discord.NewGatewayListener(cfg.DiscordBotToken, cfg.DiscordGuildID, srv, discordProvider)
		if err != nil {
			return fmt.Errorf("discord gateway listener creation: %w", err)
		}
		if err := gatewayListener.Start(ctx); err != nil {
			return fmt.Errorf("discord gateway start: %w", err)
		}
	}

	go func() {
		if err := srv.Run(cfg.ListenAddr, cfg.CertPath); err != nil {
			slog.Error("gateway server terminated with error", "error", err)
		}
	}()

	go func() {
		slog.Info("dashboard API server starting", "addr", cfg.DashboardAddr)
		if err := transport.RunDashboard(ctx, cfg.DashboardAddr, srv.DashboardRuntime()); err != nil {
			slog.Error("dashboard server terminated with error", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	return nil
}

func main() {
	if err := run(); err != nil {
		slog.Error("gateway startup failed", "error", err)
		os.Exit(1)
	}
}
