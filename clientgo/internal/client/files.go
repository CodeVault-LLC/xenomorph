package client

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/codevault-llc/xenomorph-client/internal/protocol"
	"github.com/codevault-llc/xenomorph-client/pkg/logger"
	"go.uber.org/zap"
)

func (c *Client) SendFile(filePath string, tags []string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, _ := file.Stat()
	meta := map[string]interface{}{
		"file_name": filepath.Base(filePath),
		"file_size": info.Size(),
		"tags":      tags,
	}
	
	metadata, _ := json.Marshal(meta)
	buf := make([]byte, 4096)
	var content []byte
	for {
		n, err := file.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		if n == 0 {
			break
		}
		content = append(content, buf[:n]...)
	}
	
	fileMsg := protocol.NewMessage(protocol.TypeFile, map[string]interface{}{
		"metadata": string(metadata),
		"content":  string(content),
	})
	
	c.Send(fileMsg)
	logger.GetLogger().Info("File sent", zap.String("file", filePath), zap.Int64("size", info.Size()), zap.Strings("tags", tags))
	return nil
}
