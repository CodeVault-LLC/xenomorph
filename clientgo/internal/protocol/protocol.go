package protocol

import (
	"encoding/json"

	"github.com/codevault-llc/xenomorph-client/internal/secure"
)

const (
	TypeConnect    = "connect"
	TypeConnection = "connection"
	TypePing       = "ping"
	TypeFile       = "file"
)

type Message struct {
	Type     string      `json:"type"`
	JSONData interface{} `json:"json_data"`
}

func NewMessage(msgType string, data interface{}) Message {
	return Message{Type: msgType, JSONData: data}
}

func (m Message) EncryptAndSerialize(sec *secure.Sec) ([]byte, error) {
	plain, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	return sec.Encrypt(plain)
}

func ParseMessage(sec *secure.Sec, data []byte) (Message, error) {
	decrypted, err := sec.Decrypt(data)
	if err != nil {
		return Message{}, err
	}

	var msg Message
	err = json.Unmarshal(decrypted, &msg)
	if err != nil {
		return Message{}, err
	}
	
	return msg, nil
}
