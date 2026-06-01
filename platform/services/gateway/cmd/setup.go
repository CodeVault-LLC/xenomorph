package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/config"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider/discord"
)

// buildNotifier constructs the notification provider fanout from the gateway
// configuration and returns the Discord provider separately for direct use by
// the command queue bridge and screenshot forwarding path.
//
// Providers are selected from cfg.NotifyProviders. Each name is
// case-insensitively matched against the supported provider registry.
// Supported provider names:
//   - "discord"
//
// When no providers are configured, buildNotifier returns a no-op fanout and
// a nil Discord provider pointer. The gateway continues to operate without
// outbound notifications; heartbeat activity remains internal.
//
// The returned Discord provider pointer is nil whenever Discord is not listed
// in NotifyProviders or when Discord configuration is incomplete. Callers
// must guard against nil before dereferencing the pointer.
func buildNotifier(cfg config.GatewayConfig) (*provider.Fanout, *discord.Provider, error) {
	if len(cfg.NotifyProviders) == 0 {
		slog.Info("no outbound notification providers configured; heartbeat activity remains internal")
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
			slog.Info("notification provider enabled", "provider", "discord")

		default:
			return nil, nil, fmt.Errorf("unsupported notification provider: %q", name)
		}
	}

	if err := preflightProviders(providers); err != nil {
		return nil, nil, err
	}

	return provider.NewFanout(providers), discordProvider, nil
}

// preflightProviders validates every provider that implements
// PreflightChecker by calling its PreflightCheck method with a 10-second
// timeout context.
//
// Providers that do not implement PreflightChecker are silently skipped. A
// failed preflight is fatal at startup: preflightProviders returns the first
// validation error without attempting remaining providers.
//
// The 10-second timeout is a hard deadline for each provider's preflight.
func preflightProviders(providers []provider.Provider) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, p := range providers {
		checker, ok := p.(provider.PreflightChecker)
		if !ok {
			continue
		}

		if err := checker.PreflightCheck(ctx); err != nil {
			return fmt.Errorf("provider %q preflight check failed: %w", p.Name(), err)
		}

		slog.Info("provider preflight passed", "provider", p.Name())
	}

	return nil
}
