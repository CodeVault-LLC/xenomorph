package command

import (
	"net"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"go.uber.org/zap"
)

// This command is used to list files or directories in the current working directory.
func initLsCommand() {
	err := commandHandler.AddCommand("ls", func(conn net.Conn, msgID uint32, command types.Command) {
		logger.L().Info("Received 'ls' command", zap.String("command", command.Name), zap.Strings("args", command.Args))

		// make the command ls, and get the response
		
	})
	
	if err != nil {
		logger.L().Error("Failed to register 'ls' command handler", zap.Error(err))
	}
}