package messages

import (
	"net"
	"os"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/embeds"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

var usersSubmittedFiles = make(map[string]common.FileData)

func (m *MessageCore) PreHandleFile(uuid string, msg *common.FileData) {
	if _, ok := usersSubmittedFiles[uuid]; ok {
		logger.Log.Warn("User already has a file in progress", zap.String("uuid", uuid))
		return
	}

	usersSubmittedFiles[uuid] = *msg
	logger.Log.Info("Received file data", zap.String("uuid", uuid))
}

func (m *MessageCore) handleFile(uuid string, _ *common.Message) {
	if _, ok := usersSubmittedFiles[uuid]; !ok {
		logger.Log.Warn("No file in progress for user", zap.String("uuid", uuid))
		return
	}

	channel := m.Bot.GetChannelID(uuid, "info")
	fileData := usersSubmittedFiles[uuid]

	embed := embeds.FileEmbed(&fileData)
	err := m.Bot.SendEmbedToChannel(channel, "", &embed)
	if err != nil {
		logger.Log.Error("Failed to send file embed to channel", zap.Error(err))
		return
	}

	delete(usersSubmittedFiles, uuid)
}

func (m *MessageCore) HandleFileChunk(uuid string, fileData []byte, conn *net.Conn) error {
	if _, ok := usersSubmittedFiles[uuid]; !ok {
		logger.Log.Warn("No file in progress for user", zap.String("uuid", uuid))
		return nil
	}

	filePath := "./files/" + uuid + "/" + usersSubmittedFiles[uuid].FileName

	file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Log.Error("Error opening file", zap.Error(err))
		return err
	}
	defer file.Close()

	if _, err := file.Write(fileData); err != nil {
		logger.Log.Error("Error writing to file", zap.Error(err))
		return err
	}

	return nil
}
