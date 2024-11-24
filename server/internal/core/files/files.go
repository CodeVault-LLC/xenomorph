package files

import (
	"github.com/codevault-llc/xenomorph/internal/database"
	"github.com/codevault-llc/xenomorph/internal/shared"
)

// FileClient is a client that can upload and download files.
type FileClient struct {
	server            shared.ServerController
	messageController shared.MessageController
}

// NewFileClient creates a new file client.
func NewFileClient(server shared.ServerController, messageController shared.MessageController) *FileClient {
	err := database.InitAWS()
	if err != nil {
		panic(err)
	}

	return &FileClient{
		server:            server,
		messageController: messageController,
	}
}
