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
			continue
		}

		s.MessageController.HandleReceiveMessage(userData.UUID, message, &conn)
	}
}

const (
	maxHeaderSize = 1024 * 1024        // 1MB
	maxBodySize   = 1024 * 1024 * 1024 // 1GB
)

func (s *Server) readChunkedMessage(conn net.Conn) (*common.Message, error) {
	headerSizeBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, headerSizeBuf); err != nil {
		return nil, fmt.Errorf("failed to read header size: %w", err)
	}

	headerSize := int(binary.BigEndian.Uint32(headerSizeBuf))
	if headerSize <= 0 {
		return nil, fmt.Errorf("invalid header size: %d", headerSize)
	}

	logger.Log.Info("Received header size", zap.Int("header_size", headerSize), zap.String("headerSizeBuf", string(headerSizeBuf)))

	if headerSize > maxHeaderSize {
		//return nil, fmt.Errorf("header size too large: %d", headerSize)
	}

	headerBuf := make([]byte, headerSize)
	if _, err := io.ReadFull(conn, headerBuf); err != nil {
		logger.Log.Error("Failed to read header", zap.Error(err), zap.Int("header_size", headerSize), zap.String("headerBuf", string(headerBuf)))
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	var header common.Header
	if err := json.Unmarshal(headerBuf, &header); err != nil {
		logger.Log.Error("Failed to parse header", zap.Error(err), zap.String("headerBuf", string(headerBuf)))
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	if header.TotalSize > maxBodySize {
		return nil, fmt.Errorf("body size too large: %d", header.TotalSize)
	}

	if header.Type == "" {
		return nil, errors.New("missing message type")
	}

	logger.Log.Info("Received header", zap.Any("header", header))

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

	// Step 4: Read and write file chunks
	buf := make([]byte, 4096) // 4KB buffer for efficient transfer
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break // End of file
			}
			return nil, fmt.Errorf("failed to read file chunk: %w", err)
		}

		if n > 0 {
			if _, writeErr := file.Write(buf[:n]); writeErr != nil {
				return nil, fmt.Errorf("failed to write file chunk: %w", writeErr)
			}
		}
	}

	logger.Log.Info("File upload completed", zap.String("file", metadata.FileName), zap.String("user", userData.UUID))

	return &common.Message{Type: common.MessageTypeFile}, nil
}
