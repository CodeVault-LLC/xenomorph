// Package discord implements a Discord notification provider and a Gateway
// WebSocket listener for receiving and routing slash commands from Discord.
package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
)

const channelCacheTTL = 60 * time.Second

// InteractionHandler is the interface for handling a parsed Discord slash
// command interaction. The transport.Server implements this interface to route
// commands to the correct agent handler (help, status, screenshot, ping, clean).
type InteractionHandler interface {
	HandleDiscordInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID string) error
}

// GatewayListener uses a Discord Gateway WebSocket connection to receive
// messages and interactions in real time, eliminating REST polling and the
// rate-limit problems that come with it.
//
// Architecture:
//   - discordgo.Session manages the WebSocket connection (auto-reconnect,
//     heartbeats, resume) and provides a rate-limit-aware REST client.
//   - INTERACTION_CREATE events are pushed over the WebSocket as they happen.
//   - A periodically-refreshed channel cache maps channelID -> agentID
//     so only commands channels are processed.
//
// Security: the listener only processes interactions from channels whose topic
// contains "kind=commands". This prevents command injection through non-command
// channels.
type GatewayListener struct {
	session   *discordgo.Session
	handler   InteractionHandler
	guildID   string
	provider  *Provider
	channelID string

	channelIDtoAgent map[string]string
	cacheTime        time.Time
	cacheMu          sync.Mutex
	cacheTTL         time.Duration
}

// NewGatewayListener creates a GatewayListener with the given bot token,
// guild ID, command handler, and provider. The handler must be non-nil; the
// function panics when handler is nil.
//
// The bot token must be prefixed with "Bot " by the caller (or the raw token
// is accepted and prefixed internally). The intents GuildMessages and
// MessageContent are required for command processing.
func NewGatewayListener(token, guildID string, handler InteractionHandler, provider *Provider) (*GatewayListener, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("discordgo session creation failed: %w", err)
	}

	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	gl := &GatewayListener{
		session:          dg,
		handler:          handler,
		guildID:          guildID,
		provider:         provider,
		channelIDtoAgent: make(map[string]string),
		cacheTTL:         channelCacheTTL,
	}

	dg.AddHandler(gl.onInteractionCreate)

	return gl, nil
}

// Start connects to the Discord Gateway and begins receiving events. The
// connection is established synchronously; event handling runs in the
// background via the discordgo session's internal goroutines. The channel
// cache refresher goroutine is tied to ctx cancellation.
//
// The cache is seeded synchronously before Start returns so the first
// interaction does not race against an empty cache. The cache is refreshed
// every cacheTTL until ctx is cancelled.
//
// Slash commands are registered on startup for the configured guild.
func (gl *GatewayListener) Start(ctx context.Context) error {
	if err := gl.session.Open(); err != nil {
		return fmt.Errorf("discord gateway open failed: %w", err)
	}
	slog.Info("Discord Gateway connected")
	gl.refreshChannelCache()

	if err := gl.registerSlashCommands(ctx); err != nil {
		slog.Error("slash command registration failed", "error", err)
	}

	go func() {
		ticker := time.NewTicker(gl.cacheTTL)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				_ = gl.session.Close()
				return
			case <-ticker.C:
				gl.refreshChannelCache()
			}
		}
	}()

	return nil
}

// registerSlashCommands registers all slash commands for the guild.
func (gl *GatewayListener) registerSlashCommands(ctx context.Context) error {
	if gl.provider == nil {
		return fmt.Errorf("provider not available for command registration")
	}

	commands := []guildCommand{
		{
			Name:        "help",
			Description: "Show all available commands",
		},
		{
			Name:        "status",
			Description: "Show agent connection status",
		},
		{
			Name:        "screenshot",
			Description: "Request a screenshot from the agent",
		},
		{
			Name:        "ping",
			Description: "Check bot latency and agent status",
		},
		{
			Name:        "clean",
			Description: "Clean up agent logs and messages",
			Options: []map[string]any{
				{
					"name":        "remove_channels",
					"description": "Also delete the agent's Discord channels",
					"type":        5,
					"required":    false,
				},
			},
		},
	}

	if err := gl.provider.RegisterGuildCommands(ctx, commands); err != nil {
		return fmt.Errorf("register guild commands: %w", err)
	}

	slog.Info("slash commands registered")
	return nil
}

// onInteractionCreate handles incoming Discord interactions. It filters out
// non-command interactions, looks up the agent ID from the channel cache,
// and forwards the interaction to the handler.
func (gl *GatewayListener) onInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	if i.Member == nil && i.User == nil {
		return
	}

	channelID := i.ChannelID
	if channelID == "" {
		if i.GuildID != "" {
			channelID = i.ChannelID
		}
	}

	agentID, ok := gl.lookupChannel(channelID)
	if !ok {
		return
	}

	cmdName := i.ApplicationCommandData().Name

	slog.Info("discord slash command received",
		"agent_id", agentID,
		"channel_id", channelID,
		"command", cmdName,
		"interaction_id", i.ID,
	)

	if err := gl.handler.HandleDiscordInteraction(context.Background(), i.Interaction, agentID); err != nil {
		slog.Error("discord interaction handling failed",
			"command", cmdName,
			"agent_id", agentID,
			"error", err,
		)
	}
}

// lookupChannel returns the agent ID associated with the given Discord
// channel ID. Returns false when the channel is not a known commands channel.
func (gl *GatewayListener) lookupChannel(channelID string) (string, bool) {
	gl.cacheMu.Lock()
	defer gl.cacheMu.Unlock()
	id, ok := gl.channelIDtoAgent[channelID]
	return id, ok
}

// refreshChannelCache fetches the guild's channel list and rebuilds the
// channel-to-agent mapping from channel topics. Only text channels whose
// topic contains "kind=commands" are included in the cache.
//
// The channel topic format is:
//
//	xenomorph agent_id=<agentID> kind=commands
//
// This is set by the Discord provider when creating the commands channel.
func (gl *GatewayListener) refreshChannelCache() {
	channels, err := gl.session.GuildChannels(gl.guildID)
	if err != nil {
		slog.Error("Discord guild channel list fetch failed", "error", err)
		return
	}

	gl.cacheMu.Lock()
	defer gl.cacheMu.Unlock()

	cleared := len(gl.channelIDtoAgent)
	gl.channelIDtoAgent = make(map[string]string, len(channels))
	for _, ch := range channels {
		if ch.Type != discordgo.ChannelTypeGuildText {
			continue
		}
		if !strings.Contains(ch.Topic, "kind=commands") {
			continue
		}
		agentID := extractAgentIDFromTopic(ch.Topic)
		if agentID == "" {
			continue
		}
		gl.channelIDtoAgent[ch.ID] = agentID
	}
	gl.cacheTime = time.Now()

	slog.Info("Discord channel cache refreshed",
		"total_channels", len(channels),
		"commands_channels", len(gl.channelIDtoAgent),
		"entries_replaced", cleared,
	)
}

// extractAgentIDFromTopic parses the "agent_id=" value from a Discord
// channel topic. Returns an empty string when the topic does not contain
// a valid agent_id field.
func extractAgentIDFromTopic(topic string) string {
	for _, part := range strings.Fields(topic) {
		if strings.HasPrefix(part, "agent_id=") {
			return strings.TrimPrefix(part, "agent_id=")
		}
	}
	return ""
}
