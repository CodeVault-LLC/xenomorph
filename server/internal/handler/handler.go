package handler

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/internal/database"
	"github.com/codevault-llc/xenomorph/internal/shared"
	"github.com/codevault-llc/xenomorph/pkg/encryption"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

type Handler struct {
	Server  shared.ServerController
	Message shared.MessageController
}

func NewHandler(message shared.MessageController) *Handler {
	err := database.InitAWS()
	if err != nil {
		panic(err)
	}

	return &Handler{
		Message: message,
	}
}

const (
	headerSize = 4
)

// ReadChunkedHeader reads a chunked header from the connection.
func (h Handler) ReadChunkedHeader(conn net.Conn) (*common.Header, error) {
	headerSizeBuf := make([]byte, headerSize)
	if _, err := io.ReadFull(conn, headerSizeBuf); err != nil {
		return nil, fmt.Errorf("failed to read header size: %w", err)
	}

	headerSize := int(binary.BigEndian.Uint32(headerSizeBuf))

	headerBuf := make([]byte, headerSize)
	if _, err := io.ReadFull(conn, headerBuf); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	var header common.Header
	if err := json.Unmarshal(headerBuf, &header); err != nil {
		return nil, fmt.Errorf("failed to parse header: %w", err)
	}

	return &header, nil
}

// ReadChunkedMessage reads a chunked message from the connection.
func (h Handler) ReadChunkedMessage(conn net.Conn, totalSize int) (*common.Message, error) {
	messageBuf := make([]byte, totalSize)
	if _, err := io.ReadFull(conn, messageBuf); err != nil {
		return nil, fmt.Errorf("failed to read message: %w", err)
	}

	client, _ := h.Server.GetClientFromAddr(conn.RemoteAddr())

	var uuid string
	if client != nil {
		uuid = client.UUID
	}

	privateKey, _ := h.Server.GetCassandra().GetClientEssentials(uuid)
	if privateKey != "" {
		decryptedMessage, err := encryption.RSADecryptBytes(privateKey, messageBuf)
		if err != nil {
			logger.GetLogger().Error("Failed to decrypt message", zap.Error(err), zap.String("t", string(messageBuf)))
			return nil, fmt.Errorf("failed to decrypt message: %w", err)
		}

		var message common.Message
		if err := json.Unmarshal(decryptedMessage, &message); err != nil {
			return nil, fmt.Errorf("failed to parse message: %w", err)
		}

		return &message, nil
	}

	var message common.Message
	if err := json.Unmarshal(messageBuf, &message); err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	return &message, nil
}

func (h Handler) SendMessage(conn net.Conn, message *common.Message) error {
	messageAsString, err := json.Marshal(message)
	if err != nil {
		return err
	}

	_, err = conn.Write(append(messageAsString, []byte("END_OF_MESSAGE")...))
	if err != nil {
		return err
	}

	return nil
}
