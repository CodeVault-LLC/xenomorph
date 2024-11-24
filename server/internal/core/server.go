package core

import (
	"fmt"
	"net"
	"sync"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/internal/database"
	"github.com/codevault-llc/xenomorph/internal/shared"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

type Server struct {
	Port              string
	Listener          net.Listener
	Clients           map[string]*common.ClientListData
	mu                sync.Mutex
	BotController     shared.BotController
	MessageController shared.MessageController
	Handler           shared.HandlerController
	Cassandra         shared.CassandraController
}

func NewServer(port string) *Server {
	cassandra := database.NewCassandra()

	return &Server{
		Port:      port,
		Clients:   make(map[string]*common.ClientListData),
		Cassandra: cassandra,
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

func (s *Server) RegisterClient(uuid string, data *common.ClientListData) (*common.ClientListData, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Clients[uuid] = data

	publicKey, err := s.Cassandra.RegisterClient(uuid)
	if err != nil {
		logger.Log.Error("Failed to register client in Cassandra", zap.Error(err))
		return nil, "", err
	}

	return data, publicKey, nil
}

func (s *Server) UpdateClient(uuid string, data *common.ClientData) (*common.ClientData, error) {
	if err := s.Cassandra.UpdateClient(uuid, data); err != nil {
		logger.Log.Error("Failed to update client in Cassandra", zap.Error(err))
		return nil, err
	}

	return data, nil
}

func (s *Server) GetClientByAddress(addr net.Addr) *common.ClientData {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, client := range s.Clients {
		if client.Addr.String() == addr.String() {
			clientData, err := s.Cassandra.GetClient(client.UUID)
			if err != nil {
				logger.Log.Error("Failed to get client from Cassandra", zap.Error(err))
				return nil
			}

			return &clientData
		}
	}

	return nil
}

func (s *Server) GetClientByUUID(uuid string) *common.ClientData {
	clientData, err := s.Cassandra.GetClient(uuid)
	if err != nil {
		logger.Log.Error("Failed to get client from Cassandra", zap.Error(err))
		return nil
	}

	return &clientData
}

func (s *Server) ClientCheck(uuid string) bool {
	exist, err := s.Cassandra.ClientExists(uuid)
	if err != nil {
		logger.Log.Error("Failed to check if client exists in Cassandra", zap.Error(err))
		return false
	}

	return exist
}

func (s *Server) GetClientInitialConnection(uuid string) (*common.ClientListData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	client, ok := s.Clients[uuid]
	if !ok {
		return nil, fmt.Errorf("client not found")
	}

	return client, nil
}

func (s *Server) GetClientInitialConnectionFromAddr(addr net.Addr) (*common.ClientListData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, client := range s.Clients {
		if client.Addr.String() == addr.String() {
			return client, nil
		}
	}

	return nil, fmt.Errorf("client not found")
}

func (s *Server) GetHandler() shared.HandlerController {
	return s.Handler
}

func (s *Server) GetCassandra() shared.CassandraController {
	return s.Cassandra
}
