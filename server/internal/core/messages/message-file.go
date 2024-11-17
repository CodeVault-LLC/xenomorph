package messages

import (
	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/lib"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

var validFileTransferIDs = make(map[string]bool)

func (m *MessageCore) preHandleFile(uuid string, _ *common.Message) {
	fileTransferID := lib.GenerateID()
	validFileTransferIDs[fileTransferID] = true
	err := m.Server.SendMessage(uuid, common.Message{
		Type: common.MessageTypePreFile,
		Data: fileTransferID,
	})

	if err != nil {
		logger.Log.Error("Failed to send file transfer id", zap.Error(err))
	}
}

func (m *MessageCore) handleFile(uuid string, msg *common.Message) {
	// We need to handle somethings here.
}
