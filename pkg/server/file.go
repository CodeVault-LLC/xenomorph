package server

import (
	"os"

	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"go.uber.org/zap"
)

func UploadFile(filePath string, msgID uint32, client types.ClientController) error {
	file, err := os.Open(filePath)
		if err != nil {
			logger.L().Error("Failed to open file", zap.Error(err))
			return err
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
		logger.L().Info("File upload completed", zap.String("file", fi.Name()), zap.Int64("size", fi.Size()))
		return nil
}

func UploadFileFromBuffer(fileBuffer []byte, fileName string, msgID uint32, client types.ClientController) error {
	if len(fileBuffer) == 0 {
		logger.L().Error("File buffer is empty")
		return nil
	}

	metadata := types.FileMetadata{
		ID:    msgID,
		Name:  fileName,
		Size:  int64(len(fileBuffer)),
		Direction: "upload",
	}

	client.Send(types.MsgFileStart, 0, msgID, []byte(metadata.ToJSON()))

	chunkSize := 4096
	for i := 0; i < len(fileBuffer); i += chunkSize {
		end := i + chunkSize
		if end > len(fileBuffer) {
			end = len(fileBuffer)
		}
		chunk := fileBuffer[i:end]
		if len(chunk) == 0 {
			continue
		}

		if len(chunk) < chunkSize {
			chunk = append(chunk, make([]byte, chunkSize-len(chunk))...)
		}

		client.Send(types.MsgFileChunk, 0, msgID, chunk)
	}

	fileEndMsg := types.FileEnd{
		ID: msgID,
	}

	client.Send(types.MsgFileEnd, 0, msgID, []byte(fileEndMsg.ToJSON()))
	logger.L().Info("File upload from buffer completed", zap.String("file", fileName), zap.Int64("size", int64(len(fileBuffer))))
	return nil
}