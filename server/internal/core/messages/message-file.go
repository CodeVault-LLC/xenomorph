package messages

import (
	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/embeds"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

// Make a object which should include the users submitted files grouped up by their uuid.
var usersSubmittedFiles = make(map[string]common.FileData)

func (m *MessageCore) PreHandleFile(uuid string, metadata *common.FileData) {
	if _, ok := usersSubmittedFiles[uuid]; ok {
		logger.GetLogger().Warn("User already has a file in progress", zap.String("uuid", uuid))
		return
	}

	if metadata.FileName == "" || metadata.FileSize <= 0 {
		logger.GetLogger().Error("Invalid file metadata", zap.String("uuid", uuid), zap.Any("metadata", metadata))
		return
	}

	usersSubmittedFiles[uuid] = *metadata
	logger.GetLogger().Info("Accepted file metadata", zap.String("uuid", uuid), zap.String("file", metadata.FileName))
}

func (m *MessageCore) handleFile(uuid string, msg *common.Message) {
	if _, ok := usersSubmittedFiles[uuid]; !ok {
		logger.GetLogger().Warn("No file in progress for user", zap.String("uuid", uuid))
		return
	}

	channel := m.Bot.GetChannelFromUser(uuid, "info")
	fileData := usersSubmittedFiles[uuid]

	embed := embeds.FileEmbed(&fileData, msg)
	err := m.Bot.SendEmbedToChannel(channel, "", &embed)

	if err != nil {
		logger.GetLogger().Error("Failed to send file embed to channel", zap.Error(err))
		return
	}

	delete(usersSubmittedFiles, uuid)
}
