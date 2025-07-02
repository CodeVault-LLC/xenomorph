package command

import (
	"github.com/codevault-llc/xenomorph-client/internal/protocol"
	"github.com/codevault-llc/xenomorph-client/pkg/logger"
	"go.uber.org/zap"
)

// This command is used to list files or directories in the current working directory.
func initLsCommand() {
	err := commandHandler.AddCommand("ls", func(msg protocol.Message) {
		logger.GetLogger().Info("Handling 'ls' command", zap.Any("data", msg.JSONData))
	})
	
	if err != nil {
		logger.GetLogger().Error("Failed to register 'ls' command handler", zap.Error(err))
	}
}