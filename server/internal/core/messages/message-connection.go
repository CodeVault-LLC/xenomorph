package messages

import (
	"encoding/json"
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (m *MessageCore) HandleConnection(_ string, msg *common.Message, conn *net.Conn) {
	logger.Log.Info("Processing connection message", zap.Any("data", msg.Data))

	// Convert msg.Data to JSON string (if necessary)
	dataBytes, err := json.Marshal(msg.Data)
	if err != nil {
		logger.Log.Error("Failed to marshal data to JSON", zap.Error(err))
		return
	}

	// Unmarshal into common.ClientData
	var clientData common.ClientData
	if err := json.Unmarshal(dataBytes, &clientData); err != nil {
		logger.Log.Error("Failed to unmarshal data to ClientData", zap.Error(err))
		return
	}

	// Add socket and address information
	clientData.Socket = *conn
	clientData.Addr = (*conn).RemoteAddr()

	// Register the client
	m.Server.RegisterClient(&clientData)
}
