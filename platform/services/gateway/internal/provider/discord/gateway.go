// Package discord implements a Discord notification provider and a Gateway
// WebSocket listener for receiving and routing !-commands from Discord.
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

// CommandHandler is the interface for handling a parsed Discord !-command.
// The transport.Server implements this interface to route commands to the
// correct agent handler (help, status, screenshot).
type CommandHandler interface {
	HandleDiscordCommand(ctx context.Context, agentID, channelID, command string, args []string, userName string) error
}

// GatewayListener uses a Discord Gateway WebSocket connection to receive
// messages in real time, eliminating REST polling and the rate-limit
// problems that come with it.
//
// Architecture:
//   - discordgo.Session manages the WebSocket connection (auto-reconnect,
//     heartbeats, resume) and provides a rate-limit-aware REST client.
//   - MESSAGE_CREATE events are pushed over the WebSocket as they happen.
//   - A periodically-refreshed channel cache maps channelID -> agentID
//     so only commands channels are processed.
//
// Security: the listener only processes messages from channels whose topic
// contains "kind=commands". This prevents command injection through non-command
// channels. Bot messages and the listener's own messages are always ignored.
type GatewayListener struct {
	session *discordgo.Session
	handler CommandHandler
	guildID string

	channelIDtoAgent map[string]string
	cacheTime        time.Time
	cacheMu          sync.Mutex
	cacheTTL         time.Duration
}

// NewGatewayListener creates a GatewayListener with the given bot token,
// guild ID, and command handler. The handler must be non-nil; the function
// panics when handler is nil.
//
// The bot token must be prefixed with "Bot " by the caller (or the raw token
// is accepted and prefixed internally). The intents GuildMessages and
// MessageContent are required for command processing.
func NewGatewayListener(token, guildID string, handler CommandHandler) (*GatewayListener, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("discordgo session creation failed: %w", err)
	}

	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	gl := &GatewayListener{
		session:          dg,
		handler:          handler,
		guildID:          guildID,
		channelIDtoAgent: make(map[string]string),
		cacheTTL:         channelCacheTTL,
	}

	dg.AddHandler(gl.onMessageCreate)

	return gl, nil
}

// Start connects to the Discord Gateway and begins receiving events. The
// connection is established synchronously; event handling runs in the
// background via the discordgo session's internal goroutines. The channel
// cache refresher goroutine is tied to ctx cancellation.
//
// The cache is seeded synchronously before Start returns so the first
// command does not race against an empty cache. The cache is refreshed
// every cacheTTL until ctx is cancelled.
func (gl *GatewayListener) Start(ctx context.Context) error {
	if err := gl.session.Open(); err != nil {
		return fmt.Errorf("discord gateway open failed: %w", err)
	}
	slog.Info("Discord Gateway connected")
	gl.refreshChannelCache()

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

// onMessageCreate handles incoming Discord messages. It filters out bot
// messages, messages outside commands channels, and messages that do not
// start with "!". Parsed commands are forwarded to the handler.
func (gl *GatewayListener) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}
	if m.Author.ID == s.State.User.ID {
		return
	}

	agentID, ok := gl.lookupChannel(m.ChannelID)
	if !ok {
		return
	}

	cmd, args := parseCommand(m.Content)
	if cmd == "" {
		return
	}

	slog.Info("discord command received",
		"agent_id", agentID,
		"channel_id", m.ChannelID,
		"command", cmd,
		"user", m.Author.Username,
	)

	if err := gl.handler.HandleDiscordCommand(context.Background(), agentID, m.ChannelID, cmd, args, m.Author.Username); err != nil {
		slog.Error("discord command handling failed",
			"command", cmd,
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

// parseCommand extracts the !-command and arguments from a Discord message.
// Returns an empty command string when the message does not start with "!".
func parseCommand(content string) (string, []string) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "!") {
		return "", nil
	}

	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return "", nil
	}

	cmd := strings.ToLower(strings.TrimPrefix(parts[0], "!"))
	return cmd, parts[1:]
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
