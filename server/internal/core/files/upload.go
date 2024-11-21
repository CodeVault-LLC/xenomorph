package files

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/internal/database"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (fc FileClient) FileUpload(conn net.Conn, header common.Header) (*common.Message, error) {
	metadataBuf := make([]byte, header.TotalSize)
	if _, err := io.ReadFull(conn, metadataBuf); err != nil {
		return nil, fmt.Errorf("failed to read file metadata: %w", err)
	}

	var metadata common.FileData
	if err := json.Unmarshal(metadataBuf, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse file metadata: %w", err)
	}

	userData := fc.server.GetClientByAddress(conn.RemoteAddr())
	if userData == nil {
		return nil, fmt.Errorf("user data not found for address: %s", conn.RemoteAddr())
	}

	fc.messageController.PreHandleFile(userData.UUID, &metadata)

	// Prepare file storage
	filePath := fmt.Sprintf("./files/%s/%s", userData.UUID, metadata.FileName)
	if err := os.MkdirAll(fmt.Sprintf("./files/%s", userData.UUID), 0755); err != nil {
		return nil, fmt.Errorf("failed to create file directory: %w", err)
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open file for writing: %w", err)
	}
	defer file.Close()

	fileBuf := make([]byte, metadata.FileSize)
	if _, err := io.ReadFull(conn, fileBuf); err != nil {
		return nil, fmt.Errorf("failed to read file data: %w", err)
	}

	// Get the file extension, if none exists then just make it a txt file
	extension := strings.Split(metadata.FileName, ".")
	if len(extension) == 1 {
		metadata.FileExtension = "txt"
	} else {
		metadata.FileExtension = extension[len(extension)-1]
	}

	bucketId := database.GenerateBucketName(metadata.FileExtension)
	err = database.UploadFileChunks("content-bucket", bucketId, fileBuf, 1024*1024)
	if err != nil {
		return nil, fmt.Errorf("failed to upload file chunks: %w", err)
	}

	message := common.Message{
		Type: common.MessageTypeFile,
		Data: bucketId,
	}

	if _, err := file.Write(fileBuf); err != nil {
		return nil, fmt.Errorf("failed to write file data: %w", err)
	}

	logger.Log.Info("File upload completed", zap.String("file", metadata.FileName), zap.String("user", userData.UUID))
	return &message, nil
}
