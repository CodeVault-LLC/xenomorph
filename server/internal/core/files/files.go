package files

import (
	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/internal/database"
)

// FileClient is a client that can upload and download files.
type FileClient struct {
	server            common.ServerController
	messageController common.MessageController
}

// NewFileClient creates a new file client.
func NewFileClient(server common.ServerController, messageController common.MessageController) *FileClient {
	err := database.InitAWS()
	if err != nil {
		panic(err)
	}

	return &FileClient{
		server:            server,
		messageController: messageController,
	}
}
