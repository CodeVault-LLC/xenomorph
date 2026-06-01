package main

import (
	"context"
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

func main() {
	if err := sdk.InitLogger(""); err != nil {
		slog.Error("logger initialization failed", "error", err)
		os.Exit(1)
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		slog.Error("invalid gateway configuration", "error", err)
		os.Exit(1)
	}

	natsBroker, err := broker.New(cfg.NATSURL)
	if err != nil {
		slog.Error("NATS connection failed", "error", err)
		os.Exit(1)
	}
	defer natsBroker.Close()
	slog.Info("connected to NATS JetStream")

	notifier, discordProvider, err := buildNotifier(cfg)
	if err != nil {
		slog.Error("notification provider setup failed", "error", err)
		os.Exit(1)
	}

	monitor := activity.NewMonitor(cfg.ActivityOfflineAfter, notifier)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := activity.StartStream(ctx, natsBroker, monitor, cfg.ActivitySweepInterval); err != nil {
		slog.Error("activity monitoring setup failed", "error", err)
		os.Exit(1)
	}

	cmdQueue := command.NewQueue()

	var discordPoster provider.DiscordPoster = discordProvider

	srv := transport.NewServer(natsBroker, notifier, cmdQueue, discordPoster, monitor)

	if discordProvider != nil {
		gateway, err := discord.NewGatewayListener(cfg.DiscordBotToken, cfg.DiscordGuildID, srv)
		if err != nil {
			slog.Error("Discord gateway listener creation failed", "error", err)
			os.Exit(1)
		}
		if err := gateway.Start(ctx); err != nil {
			slog.Error("Discord gateway start failed", "error", err)
			os.Exit(1)
		}
		slog.Info("Discord command listener started")
	}

	go func() {
		slog.Info("gateway server starting", "addr", cfg.ListenAddr, "tls", "mTLS required")
		if err := srv.Run(cfg.ListenAddr, cfg.CertPath); err != nil {
			slog.Error("gateway server terminated with error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	cancel()
	slog.Info("gateway shutdown completed")
}
