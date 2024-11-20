package core

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

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

			userData = s.GetClientByAddress(conn.RemoteAddr())
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

	logger.Log.Info("Received header buffer", zap.ByteString("headerSizeBuf", headerSizeBuf))
	logger.Log.Info("Received header size", zap.Int("size", int(binary.BigEndian.Uint32(headerSizeBuf))))
	headerSize := int(binary.BigEndian.Uint32(headerSizeBuf))
	logger.Log.Info("Reading header", zap.Int("size", headerSize))

	// Step 2: Read the header based on its size
	headerBuf := make([]byte, headerSize)
	if _, err := io.ReadFull(conn, headerBuf); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}
	logger.Log.Info("Received header", zap.ByteString("header", headerBuf))

	var header common.Header
	if err := json.Unmarshal(headerBuf, &header); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}
	logger.Log.Info("Parsed header", zap.Any("header", header))

	switch header.Type {
	case "JSON":
		bodyBuf := make([]byte, header.TotalSize)
		if _, err := io.ReadFull(conn, bodyBuf); err != nil {
			return nil, fmt.Errorf("failed to read body: %w", err)
		}
		logger.Log.Info("Received body", zap.ByteString("body", bodyBuf))

		// Parse JSON message
		var message common.Message
		if err := json.Unmarshal(bodyBuf, &message); err != nil {
			return nil, fmt.Errorf("failed to parse JSON message: %w", err)
		}
		logger.Log.Info("Parsed message", zap.Any("message", message))

		return &message, nil

	case "FILE":
		return s.handleFileUpload(conn, header)
	default:
		return nil, fmt.Errorf("unsupported message type: %s", header.Type)
	}
}

func (s *Server) handleFileUpload(conn net.Conn, header common.Header) (*common.Message, error) {
	// Step 3: Read file metadata
	metadataBuf := make([]byte, header.TotalSize)
	if _, err := io.ReadFull(conn, metadataBuf); err != nil {
		return nil, fmt.Errorf("failed to read file metadata: %w", err)
	}

	var metadata common.FileData
	if err := json.Unmarshal(metadataBuf, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse file metadata: %w", err)
	}

	userData := s.GetClientByAddress(conn.RemoteAddr())
	if userData == nil {
		return nil, fmt.Errorf("user data not found for address: %s", conn.RemoteAddr())
	}

	s.MessageController.PreHandleFile(userData.UUID, &metadata)

	// Prepare file storage
	filePath := fmt.Sprintf("./files/%s/%s", userData.UUID, metadata.FileName)
	if err := os.MkdirAll(fmt.Sprintf("./files/%s", userData.UUID), 0755); err != nil {
		return nil, fmt.Errorf("failed to create file directory: %w", err)
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for writing: %w", err)
	}
	defer file.Close()

	// Since we have not added anything with the file yet lets just make it not do chunks
	// Step 4: Read file data
	fileBuf := make([]byte, metadata.FileSize)
	if _, err := io.ReadFull(conn, fileBuf); err != nil {
		return nil, fmt.Errorf("failed to read file data: %w", err)
	}

	if _, err := file.Write(fileBuf); err != nil {
		return nil, fmt.Errorf("failed to write file data: %w", err)
	}

	logger.Log.Info("File upload completed", zap.String("file", metadata.FileName), zap.String("user", userData.UUID))

	return &common.Message{Type: common.MessageTypeFile}, nil
}
