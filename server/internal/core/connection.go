package core

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

const (
	HeaderSize = 10 // Example: 4 bytes for type, 6 bytes for payload size
)

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	clientAddr := conn.RemoteAddr().String()
	userData := s.GetClientByAddress(conn.RemoteAddr())
	logger.Log.Info("Client connected", zap.String("address", clientAddr))

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
			s.MessageController.HandleConnection(userData.UUID, message, &conn)
			continue
		}

		s.MessageController.HandleReceiveMessage(userData.UUID, message, &conn)
	}
}

func (s *Server) readChunkedMessage(conn net.Conn) (*common.Message, error) {
	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Parse the message type and size from the header
	messageType := string(header[:4])
	totalSize := int(binary.BigEndian.Uint32(header[4:8]))

	switch messageType {
	case "JSON":
		body := make([]byte, totalSize)
		if _, err := io.ReadFull(conn, body); err != nil {
			return nil, fmt.Errorf("failed to read JSON body: %w", err)
		}

		fullMessage := strings.Replace(string(body), "END_OF_MESSAGE", "", 1)
		var msg common.Message
		if err := json.Unmarshal([]byte(fullMessage), &msg); err != nil {
			logger.Log.Error("Failed to parse JSON message", zap.Error(err), zap.String("message", fullMessage))
			return nil, fmt.Errorf("failed to parse JSON message: %w", err)
		}

		return &msg, nil

	case "FILE":
		fileData := make([]byte, totalSize)
		if _, err := io.ReadFull(conn, fileData); err != nil {
			return nil, fmt.Errorf("failed to read file chunk: %w", err)
		}

		userData := s.GetClientByAddress(conn.RemoteAddr())
		if userData == nil {
			return nil, errors.New("user not found")
		}

		if err := s.MessageController.HandleFileChunk(userData.UUID, fileData, &conn); err != nil {
			return nil, fmt.Errorf("failed to handle file chunk: %w", err)
		}

		return nil, nil // No common.Message for file chunks

	default:
		return nil, fmt.Errorf("unsupported message type: %s", messageType)
	}
}
