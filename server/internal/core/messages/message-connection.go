package messages

import (
	"encoding/json"
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (m *MessageCore) HandleConnection(_ string, msg *common.Message, conn *net.Conn) {
	dataBytes, err := json.Marshal(msg.JsonData)
	if err != nil {
		logger.Log.Error("Failed to marshal data to JSON", zap.Error(err))
		return
	}

	var clientData common.ClientData
	if err := json.Unmarshal(dataBytes, &clientData); err != nil {
		logger.Log.Error("Failed to unmarshal data to ClientData", zap.Error(err))
		return
	}

	clientData.Socket = *conn
	clientData.Addr = (*conn).RemoteAddr()

	// Register the client
	m.Server.RegisterClient(&clientData)
}
