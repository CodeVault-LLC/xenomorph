package messages

import (
	"encoding/json"
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/embeds"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (m *MessageCore) HandleConnection(_ string, msg *common.Message, conn *net.Conn) (*common.ClientData, error) {
	dataBytes, err := json.Marshal(msg.JSONData)
	if err != nil {
		logger.GetLogger().Error("Failed to marshal data to JSON", zap.Error(err))
		return nil, err
	}

	var updatedClientData common.ClientData
	if err := json.Unmarshal(dataBytes, &updatedClientData); err != nil {
		logger.GetLogger().Error("Failed to unmarshal data to ClientData", zap.Error(err))
	}

	clientData, err := m.Server.GetClientFromAddr((*conn).RemoteAddr())
	if err != nil {
		logger.GetLogger().Error("Failed to get client data", zap.Error(err))
		return nil, err
	}

	data, _ := m.Server.UpdateClient(clientData.UUID, &updatedClientData)
	err = m.Bot.GenerateUser(data)

	if err != nil {
		logger.GetLogger().Error("Failed to generate user", zap.Error(err))
	}

	embed := embeds.ConnectionEmbed(data)
	err = m.Bot.SendEmbedToChannel(m.Bot.GetChannelFromUser(data.UUID, "info"), "", &embed)

	if err != nil {
		logger.GetLogger().Error("Failed to send message to channel", zap.Error(err))
	}

	return &updatedClientData, nil
}
