package messages

import (
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/internal/shared"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

type MessageCore struct {
	Server shared.ServerController
	Bot    shared.BotController
}

func NewMessageCore(server shared.ServerController, bot shared.BotController) *MessageCore {
	return &MessageCore{
		Server: server,
		Bot:    bot,
	}
}

func (m *MessageCore) HandleReceiveMessage(uuid string, msg *common.Message, conn *net.Conn) {
	switch msg.Type {
	case common.MessageTypeConnect:
		err := m.HandleConnect(uuid, msg, conn)
		if err != nil {
			logger.Log.Error("Failed to handle connect message", zap.Error(err))
		}
	case common.MessageTypeCommand:
		m.handleCommand(uuid, msg, conn)
	case common.MessageTypeFile:
		m.handleFile(uuid, msg)
	case common.MessageTypePing:
		m.handlePing(uuid, msg)
	default:
		logger.Log.Warn("Unknown message type", zap.Any("message", msg))
	}
}
