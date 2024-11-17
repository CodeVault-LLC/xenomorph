package bot

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/internal/core"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

type Handler struct {
	server common.ServerController
}

func NewHandler(server common.ServerController) *Handler {
	return &Handler{
		server: server,
	}
}

func (h *Handler) HandleMessage(m *discordgo.MessageCreate, s *discordgo.Session) {
	content := m.Content

	// Use categories or channel name as an identifier for client UUIDs
	clientUUID := h.getClientUUID(s, m.ChannelID)
	client, exists := h.server.(*core.Server).Clients[clientUUID]
	if !exists {
		logger.Log.Info("Discord user is not registered", zap.String("discord_user_id", m.Author.ID))
		return
	}

	if strings.HasPrefix(content, "!") {
		command := strings.TrimPrefix(content, "!")
		if err := h.server.SendMessage(client.UUID, common.Message{
			Type: common.MessageTypeCommand,
			Data: command,
		}); err != nil {
			logger.Log.Error("Failed to send command to client", zap.Error(err))
		}
	}
}

// Retrieve client UUID from Discord channel or category
func (h *Handler) getClientUUID(s *discordgo.Session, channelID string) string {
	channel, err := s.State.Channel(channelID)
	if err != nil {
		logger.Log.Error("Failed to retrieve channel information", zap.Error(err))
		return ""
	}

	if category, err := s.State.Channel(channel.ParentID); err == nil {
		return category.Name
	}
	return channel.Name
}
