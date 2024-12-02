package common

import "encoding/json"

type MessageType string

const (
	MessageTypeConnect    MessageType = "connect"
	MessageTypeHandshake  MessageType = "handshake"
	MessageTypeAck        MessageType = "ack"
	MessageTypeInitialize MessageType = "initialize"
	MessageTypeValidation MessageType = "validation"

	MessageTypeConnection MessageType = "connection"

	MessageTypeCommand MessageType = "command"
	MessageTypePing    MessageType = "ping"

	// File
	MessageTypePreFile MessageType = "prefile"
	MessageTypeFile    MessageType = "file"
)

// Message represents a message sent between the Bot and the Server.
type Message struct {
	Type      MessageType      `json:"type"`
	Data      string           `json:"data"`
	Arguments *[]string        `json:"arguments"`
	JSONData  *json.RawMessage `json:"json_data"`
	Tags      *[]string        `json:"tags"`
}
