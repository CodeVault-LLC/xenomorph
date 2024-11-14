package core

import "fmt"

func (s *Server) SendCommand(uuid string, command string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	client, exists := s.Clients[uuid]
	if !exists {
		return fmt.Errorf("client with UUID %s does not exist", uuid)
	}

	_, err := client.Socket.Write([]byte(command))
	return err
}
