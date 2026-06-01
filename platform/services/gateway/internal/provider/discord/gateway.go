package discord

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
)

// GatewayListener uses a Discord Gateway WebSocket connection to receive
// messages in real time, eliminating REST polling and the rate-limit
// problems that come with it.
//
// Architecture:
//   - discordgo.Session manages the WebSocket connection (auto-reconnect,
//     heartbeats, resume) and provides a rate-limit-aware REST client.
//   - MESSAGE_CREATE events are pushed over the WebSocket as they happen.
//   - A periodically-refreshed channel cache maps channelID -> agentID
//     so we only handle messages in commands channels.
type GatewayListener struct {
	session *discordgo.Session
	handler provider.DiscordCommandHandler
	guildID string

	channelIDtoAgent map[string]string
	cacheTime        time.Time
	cacheMu          sync.Mutex
	cacheTTL         time.Duration
}

func NewGatewayListener(token, guildID string, handler provider.DiscordCommandHandler) (*GatewayListener, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("discordgo create: %w", err)
	}

	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	gl := &GatewayListener{
		session:          dg,
		handler:          handler,
		guildID:          guildID,
		channelIDtoAgent: make(map[string]string),
		cacheTTL:         60 * time.Second,
	}

	dg.AddHandler(gl.onMessageCreate)

	return gl, nil
}

// Start connects to the Discord Gateway and begins receiving events.
// It returns once the connection is established; event handling runs
// in the background. The cache refresher goroutine is tied to ctx.
func (gl *GatewayListener) Start(ctx context.Context) error {
	if err := gl.session.Open(); err != nil {
		return fmt.Errorf("discordgo open: %w", err)
	}
	log.Println("✅ Discord Gateway connected (real-time events)")

	// Seed the channel cache before the first command arrives.
	gl.refreshChannelCache()

	go func() {
		ticker := time.NewTicker(gl.cacheTTL)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				gl.session.Close()
				return
			case <-ticker.C:
				gl.refreshChannelCache()
			}
		}
	}()

	return nil
}

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

	log.Printf("📥 discord command agent=%s channel=%s cmd=%s user=%s",
		agentID, m.ChannelID, cmd, m.Author.Username)

	if err := gl.handler.HandleDiscordCommand(context.Background(), agentID, m.ChannelID, cmd, args, m.Author.Username); err != nil {
		log.Printf("⚠️ discord gateway: handle !%s: %v", cmd, err)
	}
}

func (gl *GatewayListener) lookupChannel(channelID string) (string, bool) {
	gl.cacheMu.Lock()
	defer gl.cacheMu.Unlock()
	id, ok := gl.channelIDtoAgent[channelID]
	return id, ok
}

func (gl *GatewayListener) refreshChannelCache() {
	channels, err := gl.session.GuildChannels(gl.guildID)
	if err != nil {
		log.Printf("⚠️ discord gateway: fetch guild channels: %v", err)
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

	log.Printf("🔁 discord channel cache refreshed: %d channels (%d commands), replaced %d entries",
		len(channels), len(gl.channelIDtoAgent), cleared)
}

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

func extractAgentIDFromTopic(topic string) string {
	for _, part := range strings.Fields(topic) {
		if strings.HasPrefix(part, "agent_id=") {
			return strings.TrimPrefix(part, "agent_id=")
		}
	}
	return ""
}
