package messages

import (
	"encoding/json"
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

type MessageCore struct {
	Server common.ServerController
	Bot    common.BotController
}

func NewMessageCore(server common.ServerController, bot common.BotController) *MessageCore {
	return &MessageCore{
		Server: server,
		Bot:    bot,
	}
}

func ConvertStringToMessage(data string) (*common.Message, error) {
	var msg common.Message
	if err := json.Unmarshal([]byte(data), &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

func (m *MessageCore) HandleReceiveMessage(uuid string, msg *common.Message, conn *net.Conn) {
	switch msg.Type {
	case common.MessageTypeCommand:
		m.handleCommand(uuid, msg)
	case common.MessageTypeFile:
		m.handleFile(uuid, msg)
	case common.MessageTypePreFile:
		m.preHandleFile(uuid, msg)
	case common.MessageTypePing:
		m.handlePing(uuid, msg)
	default:
		logger.Log.Warn("Unknown message type", zap.String("type", string(msg.Type)))
	}
}
