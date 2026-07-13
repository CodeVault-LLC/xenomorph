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
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/config"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/sdk"
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

	monitor := activity.NewMonitor(cfg.ActivityOfflineAfter)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := activity.StartStream(ctx, natsBroker, monitor, cfg.ActivitySweepInterval); err != nil {
		return fmt.Errorf("activity monitoring setup: %w", err)
	}

	srv, err := buildGatewayServer(cfg, natsBroker, monitor)
	if err != nil {
		return err
	}
	startHTTPServers(ctx, cfg, srv)

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
