package command

import (
	"fmt"
	"strings"
	"sync"

	"github.com/codevault-llc/xenomorph-client/internal/protocol"
	"github.com/codevault-llc/xenomorph-client/pkg/logger"
	"go.uber.org/zap"
)


type CommandHandler interface {
	AddCommand(cmd string, handler func(msg protocol.Message)) error
	Handle(msg protocol.Message)
}

type SimpleHandler struct {
	mu       sync.RWMutex
	commands map[string]func(msg protocol.Message)
}

var commandHandler CommandHandler

func NewHandler() CommandHandler {
	commandHandler = &SimpleHandler{
		commands: make(map[string]func(msg protocol.Message)),
	}

	return commandHandler
}

func (h *SimpleHandler) AddCommand(cmd string, handler func(msg protocol.Message)) error {
	cmd = strings.TrimSpace(strings.ToLower(cmd))

	if cmd == "" {
		logger.GetLogger().Error("Command cannot be empty")
		return fmt.Errorf("command cannot be empty")
	}

	if handler == nil {
		logger.GetLogger().Error("Handler function cannot be nil", zap.String("command", cmd))
		return fmt.Errorf("handler function cannot be nil")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if _, exists := h.commands[cmd]; exists {
		logger.GetLogger().Warn("Overwriting existing command handler", zap.String("command", cmd))
	}

	h.commands[cmd] = handler
	logger.GetLogger().Info("Command registered", zap.String("command", cmd))
	return nil
}

func (h *SimpleHandler) Handle(msg protocol.Message) {
	logger.GetLogger().Info("Received message", zap.String("type", msg.Type), zap.Any("data", msg.JSONData))

	h.mu.RLock()
	handler, exists := h.commands[strings.ToLower(msg.Type)]
	h.mu.RUnlock()

	if !exists {
		logger.GetLogger().Warn("No handler for command", zap.String("type", msg.Type))
		return
	}

	handler(msg)
}

func GetHandler() CommandHandler{
	if commandHandler == nil {
		logger.GetLogger().Error("Command handler is not initialized")
		return nil
	}
	
	return commandHandler
}
