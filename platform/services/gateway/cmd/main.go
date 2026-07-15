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
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/agentquic"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/config"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/keyservice"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/sdk"
)

const maximumConcurrentGatewayServices = 3

func run() error {
	if err := sdk.InitLogger(""); err != nil {
		return fmt.Errorf("logger initialization: %w", err)
	}

	cfg, err := config.LoadFromEnv()
	if err != nil {
		return fmt.Errorf("invalid gateway configuration: %w", err)
	}

	keys, err := openKeyService(cfg)
	if err != nil {
		return err
	}

	defer closeKeyService(keys)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	return serveGateway(ctx, cancel, cfg, keys)
}

func serveGateway(ctx context.Context, cancel context.CancelFunc, cfg config.GatewayConfig, keys *keyservice.Service) error {
	signingKey, err := setupCommandSigner(ctx, cfg, keys)
	if err != nil {
		return err
	}

	natsBroker, err := broker.New(cfg.NATSURL)
	if err != nil {
		return fmt.Errorf("NATS connection: %w", err)
	}
	defer natsBroker.Close()

	monitor := activity.NewMonitor(cfg.ActivityOfflineAfter)

	if err := activity.StartStream(ctx, natsBroker, monitor, cfg.ActivitySweepInterval); err != nil {
		return fmt.Errorf("activity monitoring setup: %w", err)
	}

	srv, queue, err := buildGatewayServer(cfg, signingKey, keys, natsBroker, monitor)
	if err != nil {
		return err
	}

	quicListener, err := buildAgentQUICListener(cfg, srv, queue, signingKey)
	if err != nil {
		return err
	}

	serviceFailures := make(chan error, maximumConcurrentGatewayServices)
	startHTTPServers(ctx, cfg, srv, serviceFailures)
	startAgentQUICService(ctx, cfg, quicListener, serviceFailures)

	return waitForShutdown(cancel, serviceFailures)
}

func waitForShutdown(cancel context.CancelFunc, serviceFailures <-chan error) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	defer signal.Stop(quit)
	select {
	case <-quit:
		cancel()
		return nil
	case serviceError := <-serviceFailures:
		cancel()
		return serviceError
	}
}

func startAgentQUICService(ctx context.Context, cfg config.GatewayConfig, listener *agentquic.Listener, failures chan<- error) {
	if listener == nil {
		return
	}

	go func() {
		slog.Info("agent QUIC listener starting", "addr", cfg.AgentQUIC.Address)

		if err := listener.Run(ctx); err != nil {
			failures <- fmt.Errorf("agent QUIC listener: %w", err)
		}
	}()
}

func openKeyService(cfg config.GatewayConfig) (*keyservice.Service, error) {
	keys, err := keyservice.New(keyservice.Config{
		ProviderName: cfg.CryptoProvider, AllowedModuleVersions: cfg.CryptoModuleVersions,
		Certificate: cfg.CryptoCertificate, SecurityPolicy: cfg.CryptoSecurityPolicy,
		AllowedOperatingEnvironments: cfg.CryptoEnvironments,
	})
	if err != nil {
		return nil, fmt.Errorf("cryptographic provider setup: %w", err)
	}

	provider := keys.Provider()
	slog.Info("cryptographic provider ready",
		"provider", provider.Name, "module_version", provider.ModuleVersion,
		"certificate", provider.Certificate, "operating_environment", provider.OperatingEnvironment,
	)

	return keys, nil
}

func closeKeyService(keys *keyservice.Service) {
	if err := keys.Close(); err != nil {
		slog.Error("cryptographic provider shutdown failed", "error", err)
	}
}

func main() {
	if err := run(); err != nil {
		slog.Error("gateway startup failed", "error", err)
		os.Exit(1)
	}
}
