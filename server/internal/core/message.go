package core

import (
	"encoding/json"
	"fmt"

	"github.com/codevault-llc/xenomorph/internal/common"
)

func (s *Server) SendMessage(uuid string, message common.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	client, exists := s.Clients[uuid]
	if !exists {
		return fmt.Errorf("client with UUID %s does not exist", uuid)
	}

	// Send the message to the client and a END_OF_MESSAGE delimiter
	messageAsString, err := json.Marshal(message)
	if err != nil {
		return err
	}

	_, err = client.Socket.Write(append(messageAsString, []byte("END_OF_MESSAGE")...))
	if err != nil {
		return err
	}

	return nil
}
