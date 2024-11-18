package messages

import (
	"encoding/json"
	"net"
	"os"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

var usersSubmittedFiles = make(map[string]common.FileData)

func (m *MessageCore) preHandleFile(uuid string, msg *common.Message) {
	if _, ok := usersSubmittedFiles[uuid]; ok {
		logger.Log.Warn("User already has a file in progress", zap.String("uuid", uuid))
		return
	}

	dataBytes, err := json.Marshal(msg.JsonData)
	if err != nil {
		logger.Log.Error("Failed to marshal data to JSON", zap.Error(err))
		return
	}

	var fileData common.FileData
	if err := json.Unmarshal(dataBytes, &fileData); err != nil {
		logger.Log.Error("Failed to unmarshal data to ClientData", zap.Error(err))
		return
	}

	usersSubmittedFiles[uuid] = fileData
	logger.Log.Info("Received file data", zap.String("uuid", uuid))
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

	logger.Log.Info("Wrote file chunk", zap.String("uuid", uuid))

	fileInfo, err := file.Stat()
	if err != nil {
		logger.Log.Error("Error getting file info", zap.Error(err))
		return err
	}

	if fileInfo.Size() == usersSubmittedFiles[uuid].FileSize {
		logger.Log.Info("File transfer complete", zap.String("uuid", uuid))
		delete(usersSubmittedFiles, uuid)
	}

	return nil
}
