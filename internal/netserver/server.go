package netserver

import (
	"fmt"
	"net"

	"github.com/codevault-llc/xenomorph/internal/bot"
	"github.com/codevault-llc/xenomorph/internal/config"
	"github.com/codevault-llc/xenomorph/internal/database"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

type Server struct {
	Addr     string
	Port 	 	 string
	Listener net.Listener
	Registry *Registry
	Bot 		 *bot.Bot
}

func NewServer(addr string, port string) *Server {
	database.NewClickhouse()
	registry := NewRegistry() 

	botInstance, err := bot.NewBot(config.ConfigInstance.DiscordToken, registry)
	if err != nil {
		panic("Failed to initialize bot: " + err.Error())
	}

	go func() {
		if err := botInstance.Run(); err != nil {
			logger.L().Error("Bot failed to start", zap.Error(err))
		}
	}()

	logger.SetBotNotifier(botInstance)

	return &Server{
		Addr:     addr,
		Port:     port,
		Registry: registry,
		Bot:      botInstance,
	}
}

func (s *Server) Start() error {
	listener, err := net.Listen("tcp", fmt.Sprintf("%s:%s", s.Addr, s.Port))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}
	s.Listener = listener

	logger.L().Info("Server listening", zap.String("addr", s.Addr))

	for {
		conn, err := s.Listener.Accept()
		if err != nil {
			logger.L().Error("accept failed", zap.Error(err))
			continue
		}

		go func() {
			session := NewSession(conn, s.Registry)
			defer s.Registry.Unregister(session.ID)

			if err := session.Handle(); err != nil {
				logger.L().Error("session handler error", zap.Error(err))
			}
		}()
	}
}

func (s *Server) Stop() error {
	if s.Listener != nil {
		if err := s.Listener.Close(); err != nil {
			return fmt.Errorf("failed to close listener: %w", err)
		}
		s.Listener = nil
		logger.L().Info("Server stopped")
	}
	return nil
}
