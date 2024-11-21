package core

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/encryption"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	clientAddr := conn.RemoteAddr().String()
	userData := s.GetClientByAddress(conn.RemoteAddr())

	if userData == nil {
		privateKey, err := encryption.GenerateRSAKeys()
		if err != nil {
			logger.Log.Error("Failed to generate RSA keys", zap.Error(err))
			return
		}

		data, err := s.RegisterClient(&common.ClientData{
			Addr:       conn.RemoteAddr(),
			PrivateKey: *privateKey,
			Socket:     conn,
		})

		if err != nil {
			logger.Log.Error("Failed to register client", zap.Error(err))
			return
		}
		userData = data
		logger.Log.Info("Client connected", zap.String("address", clientAddr))

		/*_, err = conn.Write([]byte(fmt.Sprintf("%v", privateKey.PublicKey)))
		if err != nil {
			logger.Log.Error("Failed to send unauthorized message to client", zap.Error(err))
		}
		return*/
	}

	for {
		message, err := s.readChunkedMessage(conn)
		if err != nil {
			if errors.Is(err, io.EOF) {
				logger.Log.Info("Client disconnected", zap.String("address", clientAddr))
				break
			}
			logger.Log.Error("Error reading message from client", zap.Error(err))
			break
		}

		if message == nil {
			logger.Log.Warn("Received nil message from client")
			continue
		}

		if message.Type == common.MessageTypeConnection {
			data, err := s.MessageController.HandleConnection("", message, &conn)
			if err != nil {
				logger.Log.Error("Failed to handle connection", zap.Error(err))
				continue
			}

			userData = data
			continue
		}

		if userData == nil {
			logger.Log.Warn("User data not found for address", zap.String("address", clientAddr))
			continue
		}

		s.MessageController.HandleReceiveMessage(userData.UUID, message, &conn)
	}
}

func (s *Server) readChunkedMessage(conn net.Conn) (*common.Message, error) {
	headerSizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, headerSizeBuf); err != nil {
		return nil, fmt.Errorf("failed to read header size: %w", err)
	}

	headerSize := int(binary.BigEndian.Uint32(headerSizeBuf))

	// Step 2: Read the header based on its size
	headerBuf := make([]byte, headerSize)
	if _, err := io.ReadFull(conn, headerBuf); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	var header common.Header
	if err := json.Unmarshal(headerBuf, &header); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	switch header.Type {
	case "JSON":
		bodyBuf := make([]byte, header.TotalSize)
		if _, err := io.ReadFull(conn, bodyBuf); err != nil {
			return nil, fmt.Errorf("failed to read body: %w", err)
		}

		// Parse JSON message
		var message common.Message
		if err := json.Unmarshal(bodyBuf, &message); err != nil {
			return nil, fmt.Errorf("failed to parse JSON message: %w", err)
		}

		return &message, nil

	case "FILE":
		return s.FileController.FileUpload(conn, header)
	default:
		return nil, fmt.Errorf("unsupported message type: %s", header.Type)
	}
}
