package core

import (
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	clientAddr := conn.RemoteAddr().String()

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
			_, err := s.MessageController.HandleConnection("", message, &conn)
			if err != nil {
				logger.Log.Error("Failed to handle connection", zap.Error(err))
				continue
			}

			continue
		}

		userData, _ := s.GetClientFromAddr(conn.RemoteAddr())
		var uuid string
		if userData != nil {
			uuid = userData.UUID
		}

		s.MessageController.HandleReceiveMessage(uuid, message, &conn)
	}
}

func (s *Server) readChunkedMessage(conn net.Conn) (*common.Message, error) {
	header, err := s.Handler.ReadChunkedHeader(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	switch header.Type {
	case "JSON":
		msg, err := s.Handler.ReadChunkedMessage(conn, header.TotalSize)
		if err != nil {
			return nil, fmt.Errorf("failed to read message: %w", err)
		}

		return msg, nil

	case "FILE":
		file, err := s.Handler.FileUpload(conn, *header)
		if err != nil {
			return nil, fmt.Errorf("failed to upload file: %w", err)
		}

		return file, nil

	default:
		return nil, fmt.Errorf("unsupported message type: %s", header.Type)
	}
}
