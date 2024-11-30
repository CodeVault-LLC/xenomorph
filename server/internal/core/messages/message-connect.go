package messages

import (
	"encoding/json"
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (m *MessageCore) HandleConnect(_ string, msg *common.Message, conn *net.Conn) error {
	dataBytes, err := json.Marshal(msg.JsonData)
	if err != nil {
		logger.Log.Error("Failed to marshal data to JSON", zap.Error(err))
		return err
	}

	var connectData common.ConnectData
	if err := json.Unmarshal(dataBytes, &connectData); err != nil {
		logger.Log.Error("Failed to unmarshal data to ConnectData", zap.Error(err))
	}

	if connectData.UUID == "" {
		logger.Log.Error("UUID not found in connection data")
		return err
	}

	clientExists, err := m.Server.GetCassandra().ClientExists(connectData.UUID)
	if err != nil {
		logger.Log.Error("Failed to check if client exists", zap.Error(err))
		return err
	}

	if !clientExists {
		_, publicKey, err := m.Server.RegisterClient(connectData.UUID, &common.ClientListData{
			UUID:   connectData.UUID,
			Addr:   (*conn).RemoteAddr(),
			Socket: *conn,
		})
		if err != nil {
			logger.Log.Error("Failed to register client", zap.Error(err))
			return err
		}

		handshakeData := common.HandshakeData{
			PublicKey: publicKey,
		}

		jsonData, err := json.Marshal(handshakeData)
		if err != nil {
			logger.Log.Error("Failed to marshal data to JSON", zap.Error(err))
			return err
		}

		rawMessage := json.RawMessage(jsonData)

		handshake := common.Message{
			Type:     common.MessageTypeHandshake,
			JsonData: &rawMessage,
		}

		if conn == nil {
			logger.Log.Error("Connection is nil")
		}

		if conn == nil {
			logger.Log.Error("Connection is nil")
			return err
		}

		err = m.Server.GetHandler().SendMessage(*conn, &handshake)
		if err != nil {
			logger.Log.Error("Failed to send handshake message", zap.Error(err))
			return err
		}

		return nil
	}

	// Register the client in the clientListData so we can use it later
	_, _, err = m.Server.RegisterClient(connectData.UUID, &common.ClientListData{
		UUID:   connectData.UUID,
		Addr:   (*conn).RemoteAddr(),
		Socket: *conn,
	})
	if err != nil {
		logger.Log.Error("Failed to register client", zap.Error(err))
		return err
	}

	// Send a ack to the client
	ack := common.Message{
		Type: common.MessageTypeAck,
	}

	err = m.Server.GetHandler().SendMessage(*conn, &ack)
	if err != nil {
		logger.Log.Error("Failed to send ack message", zap.Error(err))
		return err
	}

	return nil
}
