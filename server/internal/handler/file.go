package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/internal/database"
	"github.com/codevault-llc/xenomorph/pkg/encryption"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

const (
	fileUploadSize = 1024 * 1024
)

// FileUpload handles the file upload process.
func (h Handler) FileUpload(conn net.Conn, header common.Header) (*common.Message, error) {
	metadataBuf := make([]byte, header.TotalSize)
	if _, err := io.ReadFull(conn, metadataBuf); err != nil {
		return nil, fmt.Errorf("failed to read file metadata: %w", err)
	}

	var metadata common.FileData
	if err := json.Unmarshal(metadataBuf, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse file metadata: %w", err)
	}

	userData, err := h.Server.GetClientFromAddr(conn.RemoteAddr())
	if err != nil {
		logger.GetLogger().Error("Failed to get client data", zap.Error(err))
		return nil, err
	}

	if userData == nil {
		return nil, fmt.Errorf("user data not found for address: %s", conn.RemoteAddr())
	}

	h.Message.PreHandleFile(userData.UUID, &metadata)

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

	client, _ := h.Server.GetClientFromAddr(conn.RemoteAddr())

	var uuid string
	if client != nil {
		uuid = client.UUID
	}

	privateKey, _ := h.Server.GetCassandra().GetClientEssentials(uuid)

	fileData := fileBuf
	if privateKey != "" {
		fileData, err = encryption.RSADecryptBytes(privateKey, fileData)
		if err != nil {
			logger.GetLogger().Error("Failed to decrypt message", zap.Error(err), zap.String("t", string(fileBuf)))
			return nil, fmt.Errorf("failed to decrypt message: %w", err)
		}
	}

	bucketID := database.GenerateBucketName(metadata.FileExtension)
	err = database.UploadFileChunks("content-bucket", bucketID, fileData, fileUploadSize)

	if err != nil {
		return nil, fmt.Errorf("failed to upload file chunks: %w", err)
	}

	message := common.Message{
		Type: common.MessageTypeFile,
		Data: bucketID,
	}

	metadata.BucketID = bucketID

	err = h.Server.GetCassandra().InsertFile(userData.UUID, metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to insert file: %w", err)
	}

	logger.GetLogger().Info("File upload completed", zap.String("file", metadata.FileName), zap.String("user", userData.UUID))

	return &message, nil
}
