package core

import (
	"encoding/json"
	"errors"
	"io"
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	clientAddr := conn.RemoteAddr().String()

	logger.GetLogger().Info("Client connected", zap.String("address", clientAddr))

	for {
		msgType, flags, msgID, payload, err := s.Handler.ReadMessage(conn)
		logger.GetLogger().Debug("Received message from client",
			zap.String("address", clientAddr),
			zap.ByteString("payload", payload),
			zap.Uint8("msgType", msgType),
			zap.Uint8("flags", flags),
			zap.Uint32("msgID", msgID),
		)
		
		if err != nil {
			if errors.Is(err, io.EOF) {
				logger.GetLogger().Info("Client disconnected", zap.String("address", clientAddr))
				break
			}

			logger.GetLogger().Error("Error reading message from client", zap.Error(err))

			break
		}

		if msgType == 0 {
			logger.GetLogger().Error("Received empty message from client", zap.String("address", clientAddr))
			continue
		}

		// Handle registration
		if msgType == common.MsgConnect {
			if len(payload) == 0 {
				logger.GetLogger().Error("Received empty payload for MsgConnect", zap.String("address", clientAddr))
				continue
			}

			if len(payload) != 36 {
				logger.GetLogger().Error("Received invalid UUID length for MsgConnect", zap.String("address", clientAddr), zap.Int("length", len(payload)))
				continue
			}

			// Check if the client is already registered
			s.mu.Lock()
			if _, exists := s.Clients[string(payload)]; exists {
				s.mu.Unlock()
				logger.GetLogger().Info("Client already registered", zap.String("uuid", string(payload)), zap.String("address", clientAddr))
				continue
			}
			s.mu.Unlock()

			// Register the client
			data := &common.ClientListData{
				UUID:     string(payload),
				Addr:     conn.RemoteAddr(),
				Socket: conn,
			}

			registeredData, publicKey, err := s.RegisterClient(string(payload), data)
			if err != nil {
				logger.GetLogger().Error("Failed to register client", zap.Error(err), zap.String("address", clientAddr))
				continue
			}

				
		}

		if msgType == common.MsgRegistration {
			_, err := s.MessageController.HandleConnection("", payload, &conn)
			if err != nil {
				logger.GetLogger().Error("Failed to handle connection", zap.Error(err))
				continue
			}

			continue
		}

		// parse the payload into a command
		command := &common.Command{}
		err = json.Unmarshal(payload, command)
		if err != nil {
			logger.GetLogger().Error("Failed to unmarshal command", zap.Error(err), zap.ByteString("payload", payload))
			continue
		}

		userData, _ := s.GetClientFromAddr(conn.RemoteAddr())

		var uuid string
		if userData != nil {
			uuid = userData.UUID
		}

		s.MessageController.HandleReceiveMessage(uuid, command, &conn)
	}
}