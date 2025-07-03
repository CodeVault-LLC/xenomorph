package command

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"go.uber.org/zap"
)

type CommandHandler interface {
	AddCommand(cmd string, handler func(conn net.Conn, msgID uint32, command types.Command)) error
	Handle(conn net.Conn, msgID uint32, payload []byte)
}

type SimpleHandler struct {
	mu       sync.RWMutex
	commands map[string]func(conn net.Conn, msgID uint32, command types.Command)
}

var commandHandler CommandHandler

func NewHandler() CommandHandler {
	commandHandler = &SimpleHandler{
		commands: make(map[string]func(conn net.Conn, msgID uint32, command types.Command)),
	}

	return commandHandler
}

func (h *SimpleHandler) AddCommand(cmd string, handler func(conn net.Conn, msgID uint32, command types.Command)) error {
	cmd = strings.TrimSpace(strings.ToLower(cmd))

	if cmd == "" {
		logger.L().Error("Command cannot be empty")
		return fmt.Errorf("command cannot be empty")
	}

	if handler == nil {
		logger.L().Error("Handler function cannot be nil", zap.String("command", cmd))
		return fmt.Errorf("handler function cannot be nil")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.commands[cmd]; exists {
		logger.L().Warn("Overwriting existing command handler", zap.String("command", cmd))
	}

	h.commands[cmd] = handler
	logger.L().Info("Command registered", zap.String("command", cmd))
	return nil
}

func (h *SimpleHandler) Handle(conn net.Conn, msgID uint32, payload []byte) {
	if conn == nil {
		logger.L().Error("Connection is nil, cannot handle command")
		return
	}

	msg, err := h.parseCommand(payload)
	if err != nil {
		logger.L().Error("Failed to parse command", zap.Error(err))
		return
	}

	h.mu.RLock()
	handler, exists := h.commands[strings.ToLower(msg.Name)]
	h.mu.RUnlock()

	if !exists {
		logger.L().Error("Command handler not found", zap.String("command", msg.Name))
		return
	}

	handler(conn, msgID, *msg)
}

func (h *SimpleHandler) parseCommand(payload []byte) (*types.Command, error) {
	var cmd types.Command
	if err := json.Unmarshal(payload, &cmd); err != nil {
		logger.L().Error("Failed to parse command", zap.Error(err))
		return nil, fmt.Errorf("failed to parse command: %w", err)
	}

	if cmd.Name == "" {
		logger.L().Error("Command name is empty")
		return nil, fmt.Errorf("command name cannot be empty")
	}

	return &cmd, nil
}

func GetHandler() CommandHandler{
	if commandHandler == nil {
		logger.L().Error("Command handler is not initialized")
		return nil
	}
	
	return commandHandler
}
