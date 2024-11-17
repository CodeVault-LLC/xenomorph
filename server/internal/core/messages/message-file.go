package messages

import (
	"encoding/json"
	"os"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/lib"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

var validFileTransferIDs = make(map[string]common.FileData)

func (m *MessageCore) preHandleFile(uuid string, msg *common.Message) {
	dataBytes, err := json.Marshal(msg.JsonData)
	if err != nil {
		logger.Log.Error("Failed to marshal data to JSON", zap.Error(err))
		return
	}

	var fileData common.FileData
	if err := json.Unmarshal(dataBytes, &fileData); err != nil {
		logger.Log.Error("Failed to unmarshal data to FileData", zap.Error(err))
		return
	}

	fileTransferID := lib.GenerateID()
	validFileTransferIDs[fileTransferID] = fileData

	err = m.Server.SendMessage(uuid, common.Message{
		Type: common.MessageTypePreFile,
		Data: fileTransferID,
	})
	if err != nil {
		logger.Log.Error("Failed to send message to client", zap.Error(err))
		delete(validFileTransferIDs, fileTransferID)
		return
	}
}

func (m *MessageCore) handleFile(uuid string, msg *common.Message) {
	dataBytes, err := json.Marshal(msg.JsonData)
	if err != nil {
		logger.Log.Error("Failed to marshal data to JSON", zap.Error(err))
		return
	}

	var fileData common.FileDataChunk
	if err := json.Unmarshal(dataBytes, &fileData); err != nil {
		logger.Log.Error("Failed to unmarshal data to FileDataChunk", zap.Error(err))
		return
	}

	file, ok := validFileTransferIDs[fileData.ID]
	if !ok {
		logger.Log.Error("Invalid file transfer ID")
		return
	}

	if fileData.End {
		delete(validFileTransferIDs, fileData.ID)
		return
	}

	f, err := os.OpenFile("files/"+uuid+"/"+file.FileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		logger.Log.Error("Failed to open file", zap.Error(err))
		return
	}
	defer f.Close()

	_, err = f.Write([]byte(msg.Data))
	if err != nil {
		logger.Log.Error("Failed to write to file", zap.Error(err))
		return
	}
}
