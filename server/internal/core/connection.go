package core

import (
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
			s.MessageController.HandleConnection("", message, &conn)
			continue
		}

		s.MessageController.HandleReceiveMessage(userData.UUID, message, &conn)
	}
}

func (s *Server) readChunkedMessage(conn net.Conn) (*common.Message, error) {
	header := make([]byte, 1024) // 1KB buffer
	headerN, err := conn.Read(header)
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	headChunk := string(header[:headerN])

	var head common.Header
	if err := json.Unmarshal([]byte(headChunk), &head); err != nil {
		logger.Log.Error("Failed to parse JSON header", zap.Error(err), zap.String("header", string(header)))
		return nil, fmt.Errorf("failed to parse JSON header: %w", err)
	}

	logger.Log.Info("Received message", zap.Any("header", head))

	switch head.Type {
	case "JSON":
		var messageBuilder strings.Builder
		body := make([]byte, 8192) // 8KB buffer

		for {
			n, err := conn.Read(body)
			if err != nil {
				if err == io.EOF {
					break
				}

				return nil, fmt.Errorf("failed to read body chunk: %w", err)
			}

			chunk := string(body[:n])
			if strings.Contains(chunk, "END_OF_MESSAGE") {
				chunk = strings.Replace(chunk, "END_OF_MESSAGE", "", -1)
				messageBuilder.WriteString(chunk)
				break
			}

			messageBuilder.WriteString(chunk)
		}

		bodyChunk := messageBuilder.String()

		var msg common.Message
		if err := json.Unmarshal([]byte(bodyChunk), &msg); err != nil {
			logger.Log.Error("Failed to parse JSON message", zap.Error(err), zap.String("message", string(body)))
			return nil, fmt.Errorf("failed to parse JSON message: %w", err)
		}

		return &msg, nil

	case "FILE":
		metadata := make([]byte, 1024) // 1KB buffer
		metadataN, err := conn.Read(metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to read header: %w", err)
		}

		metadataChunk := string(metadata[:metadataN])

		var metadat common.FileData
		if err := json.Unmarshal([]byte(metadataChunk), &head); err != nil {
			logger.Log.Error("Failed to parse JSON file metadata", zap.Error(err), zap.String("metadata", string(metadata)))
			return nil, fmt.Errorf("failed to parse JSON metadata: %w", err)
		}

		userData := s.GetClientByAddress(conn.RemoteAddr())
		if userData == nil {
			return nil, fmt.Errorf("failed to get user data for address: %s", conn.RemoteAddr().String())
		}

		s.MessageController.PreHandleFile(userData.UUID, &metadat)

		var messageBuilder strings.Builder
		body := make([]byte, 8192) // 8KB buffer

		for {
			n, err := conn.Read(body)
			if err != nil {
				if err == io.EOF {
					break
				}

				return nil, fmt.Errorf("failed to read body chunk: %w", err)
			}

			chunk := string(body[:n])
			if strings.Contains(chunk, "END_OF_MESSAGE") {
				chunk = strings.Replace(chunk, "END_OF_MESSAGE", "", -1)
				messageBuilder.WriteString(chunk)
				s.MessageController.HandleFileChunk(userData.UUID, []byte(chunk), &conn)
				break
			}

			s.MessageController.HandleFileChunk(userData.UUID, []byte(chunk), &conn)
			messageBuilder.WriteString(chunk)
		}

		msg := &common.Message{
			Type: common.MessageTypeFile,
		}

		return msg, nil

	default:
		return nil, fmt.Errorf("unsupported message type: %s", head.Type)
	}
}
