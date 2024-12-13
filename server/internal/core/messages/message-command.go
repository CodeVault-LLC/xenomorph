package messages

import (
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (m *MessageCore) handleCommand(_ string, msg *common.Message, conn *net.Conn) {
	client, err := m.Server.GetClientFromAddr((*conn).RemoteAddr())
	if err != nil {
		logger.GetLogger().Error("Failed to get client data", zap.Error(err))
		return
	}

	mainChannel := m.Bot.GetChannelFromUser(client.UUID, "main")
	if mainChannel == "" {
		logger.GetLogger().Error("Failed to get main channel ID")
		return
	}

	err = m.Bot.SendMessageToChannel(mainChannel, string(*msg.JSONData))
	if err != nil {
		logger.GetLogger().Error("Failed to send command message to channel", zap.Error(err))
		return
	}
}
