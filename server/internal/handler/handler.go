package handler

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"

	"github.com/codevault-llc/xenomorph/internal/database"
	"github.com/codevault-llc/xenomorph/internal/shared"
	"github.com/codevault-llc/xenomorph/pkg/encryption"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/utils"
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

// Reads a message from the connection, extracting the message type, flags, message ID, and payload.
// If the flags indicate that the payload is compressed, it decompresses the payload.
func (h Handler) ReadMessage(conn net.Conn) (msgType byte, flags byte, msgID uint32, payload []byte, err error) {
	header := make([]byte, 10)
	if _, err = io.ReadFull(conn, header); err != nil {
		return
	}

	totalLen := binary.BigEndian.Uint32(header[0:])
	msgType = header[4]
	flags = header[5]
	msgID = binary.BigEndian.Uint32(header[6:])

	payload = make([]byte, totalLen-10)
	if _, err = io.ReadFull(conn, payload); err != nil {
		return
	}

	if flags&0x1 != 0 {
		payload, err = utils.Decompress(payload)
	}

	// decrypt the payload
	client, _ := h.Server.GetClientFromAddr(conn.RemoteAddr())

	var uuid string
	if client != nil {
		uuid = client.UUID
	}

	_, privateKey, _ := h.Server.GetCassandra().GetClientEssentials(uuid)
	if privateKey != "" { 
		decryptedMessage, err := encryption.RSADecryptBytes(privateKey, payload)
		if err != nil {
			logger.GetLogger().Error("Failed to decrypt message", zap.Error(err), zap.String("t", string(payload)))
			return 0, 0, 0, nil, fmt.Errorf("failed to decrypt message: %w", err)
		}
		
		payload = decryptedMessage
	}
	
	logger.GetLogger().Debug("Read message", zap.String("type", string(msgType)), zap.Uint32("msgID", msgID), zap.Int("payloadLength", len(payload)))
	return
}

// Sends a message to the connection with the specified type, flags, message ID, and payload.
// If the flags indicate that the payload should be compressed, it compresses the payload before sending.
// The message header consists of the total length (4 bytes), message type (1 byte),
func (h Handler) SendMessage(conn net.Conn, msgType byte, flags byte, msgID uint32, payload []byte) error {
	// encrypt the payload
	client, _ := h.Server.GetClientFromAddr(conn.RemoteAddr())
	var uuid string
	if client != nil {
		uuid = client.UUID
	}

	publicKey, _, _ := h.Server.GetCassandra().GetClientEssentials(uuid)
	if publicKey != "" {
		encryptedMessage, err := encryption.RSAEncryptBytes(publicKey, payload)
		if err != nil {
			logger.GetLogger().Error("Failed to encrypt message", zap.Error(err), zap.String("t", string(payload)))
			return fmt.Errorf("failed to encrypt message: %w", err)
		}
		payload = encryptedMessage
	}
	
	if flags&0x1 != 0 { // compression flag
		payload = utils.Compress(payload)
	}

	totalLen := 10 + len(payload) // 4+1+1+4 = 10 header bytes
	header := make([]byte, 10)
	binary.BigEndian.PutUint32(header[0:], uint32(totalLen))
	header[4] = msgType
	header[5] = flags
	binary.BigEndian.PutUint32(header[6:], msgID)

	_, err := conn.Write(append(header, payload...))
	return err
}
