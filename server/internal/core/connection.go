package core

import (
	"io"
	"net"
	"strings"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/internal/core/messages"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	clientAddr := conn.RemoteAddr().String()
	logger.Log.Info("Client connected", zap.String("address", clientAddr))

	for {
		message, err := s.readMessage(conn)
		if err != nil {
			if err == io.EOF {
				logger.Log.Info("Client disconnected", zap.String("address", clientAddr))
				break
			}
			logger.Log.Error("Error reading message from client", zap.Error(err))
			break
		}

		if strings.TrimSpace(message) == "" {
			logger.Log.Warn("Empty message received", zap.String("address", clientAddr))
			continue
		}

		convertedMessage, err := messages.ConvertStringToMessage(message)
		if err != nil {
			logger.Log.Error("Failed to convert message to struct", zap.Error(err))
			continue
		}

		if convertedMessage.Type == common.MessageTypeConnection {
			s.MessageController.HandleConnection(clientAddr, convertedMessage, &conn)
			continue
		}

		client := s.GetClientByAddress(conn.RemoteAddr())
		if client == nil {
			logger.Log.Warn("Client not found", zap.String("address", clientAddr))
			continue
		}

		s.MessageController.HandleReceiveMessage(client.UUID, convertedMessage, &conn)
	}
}

func (s *Server) readMessage(conn net.Conn) (string, error) {
	var messageBuilder strings.Builder
	buffer := make([]byte, 8192)

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				logger.Log.Info("Connection read timed out")
				return "", io.EOF
			}
			if err == io.EOF {
				logger.Log.Info("Connection closed by client")
				return "", err
			}
			logger.Log.Error("Error reading from connection", zap.Error(err))
			return "", err
		}

		// Handle zero-byte reads
		if n == 0 {
			logger.Log.Info("Connection appears to be closed by client")
			return "", io.EOF
		}

		chunk := string(buffer[:n])
		if strings.Contains(chunk, "END_OF_MESSAGE") {
			chunk = strings.Replace(chunk, "END_OF_MESSAGE", "", -1)
			messageBuilder.WriteString(chunk)
			break
		}
		messageBuilder.WriteString(chunk)
	}

	return messageBuilder.String(), nil
}
