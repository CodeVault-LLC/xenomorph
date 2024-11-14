package core

import (
	"encoding/json"
	"io"
	"net"
	"strings"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()
	message, err := s.readMessage(conn)
	if err != nil {
		logger.Log.Error("Failed to handle message", zap.Error(err))
		return
	}

	logger.Log.Info("Received message", zap.String("message", message))
	var data common.ClientData
	if err := json.Unmarshal([]byte(message), &data); err != nil {
		logger.Log.Error("Failed to decode client data", zap.Error(err))
		return
	}

	data.Socket = conn
	data.Addr = conn.RemoteAddr()
	s.RegisterClient(&data)
}

func (s *Server) readMessage(conn net.Conn) (string, error) {
	var messageBuilder strings.Builder
	buffer := make([]byte, 8192)

	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Log.Error("Error reading from connection", zap.Error(err))
			return "", err
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
