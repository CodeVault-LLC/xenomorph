package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/activity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/config"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider/discord"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/transport"
	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
)

func main() {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("❌ Invalid configuration: %v", err)
	}

	natsBroker, err := broker.New(cfg.NATSURL)
	if err != nil {
		log.Fatalf("❌ Failed to connect to NATS: %v", err)
	}
	defer natsBroker.Close()
	log.Println("✅ Connected to NATS JetStream")

	notifier, discordProvider, err := buildNotifier(cfg)
	if err != nil {
		log.Fatalf("❌ Failed to configure notification providers: %v", err)
	}

	monitor := activity.NewMonitor(cfg.ActivityOfflineAfter, notifier)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := startActivityStream(ctx, natsBroker, monitor, cfg.ActivitySweepInterval); err != nil {
		log.Fatalf("❌ Failed to start activity monitoring: %v", err)
	}

	cmdQueue := command.NewQueue()

	var discordPoster provider.DiscordPoster = discordProvider

	srv := transport.NewServer(natsBroker, notifier, cmdQueue, discordPoster, monitor)

	if discordProvider != nil {
		gateway, err := discord.NewGatewayListener(cfg.DiscordBotToken, cfg.DiscordGuildID, srv)
		if err != nil {
			log.Fatalf("❌ Failed to create Discord gateway: %v", err)
		}
		if err := gateway.Start(ctx); err != nil {
			log.Fatalf("❌ Failed to start Discord gateway: %v", err)
		}
		log.Println("✅ Discord command listener started (Gateway)")
	}

	go func() {
		log.Printf("🔒 Gateway listening on %s (mTLS Required)", cfg.ListenAddr)
		if err := srv.Run(cfg.ListenAddr, cfg.CertPath); err != nil {
			log.Fatalf("❌ Server failed: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	cancel()

	log.Println("Shutting down Gateway...")
}

func buildNotifier(cfg config.GatewayConfig) (*provider.Fanout, *discord.Provider, error) {
	if len(cfg.NotifyProviders) == 0 {
		log.Println("ℹ️ No outbound providers configured; heartbeat activity will remain internal")
		return provider.NewFanout(nil), nil, nil
	}

	var discordProvider *discord.Provider
	providers := make([]provider.Provider, 0, len(cfg.NotifyProviders))
	for _, name := range cfg.NotifyProviders {
		switch strings.ToLower(name) {
		case "discord":
			var err error
			discordProvider, err = discord.New(discord.Config{
				BotToken:   cfg.DiscordBotToken,
				GuildID:    cfg.DiscordGuildID,
				APIBaseURL: cfg.DiscordAPIBaseURL,
			}, nil)
			if err != nil {
				return nil, nil, err
			}
			providers = append(providers, discordProvider)
			log.Println("✅ Discord provider enabled")
		default:
			return nil, nil, fmt.Errorf("unknown provider %q", name)
		}
	}

	if err := preflightProviders(providers); err != nil {
		return nil, nil, err
	}

	return provider.NewFanout(providers), discordProvider, nil
}

func preflightProviders(providers []provider.Provider) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, p := range providers {
		checker, ok := p.(provider.PreflightChecker)
		if !ok {
			continue
		}

		if err := checker.PreflightCheck(ctx); err != nil {
			return fmt.Errorf("provider %q preflight failed: %w", p.Name(), err)
		}
		log.Printf("✅ Provider preflight passed: %s", p.Name())
	}

	return nil
}

func startActivityStream(ctx context.Context, natsBroker *broker.NATS, monitor *activity.Monitor, sweepInterval time.Duration) error {
	_, err := natsBroker.Subscribe("sys.in.default.*.heartbeat", func(msg *nats.Msg) {
		var envelope pb.EventEnvelope
		if err := proto.Unmarshal(msg.Data, &envelope); err != nil {
			log.Printf("⚠️ Failed to decode heartbeat envelope: %v", err)
			return
		}

		if err := monitor.ProcessHeartbeat(ctx, &envelope); err != nil {
			log.Printf("⚠️ Failed to process heartbeat activity: %v", err)
		}
	})
	if err != nil {
		return err
	}

	ticker := time.NewTicker(sweepInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := monitor.Sweep(ctx); err != nil {
					log.Printf("⚠️ Failed activity sweep: %v", err)
				}
			}
		}
	}()

	return nil
}
