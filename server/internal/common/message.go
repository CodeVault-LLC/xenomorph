package common

import "encoding/json"

type MessageType string

const (
	MessageTypeConnection MessageType = "CONNECTION"

	MessageTypeCommand MessageType = "COMMAND"
	MessageTypePing    MessageType = "PING"

	// File
	MessageTypePreFile MessageType = "PREFILE"
	MessageTypeFile    MessageType = "FILE"
)

// Message represents a message sent between the Bot and the Server.
type Message struct {
	Type      MessageType      `json:"type"`
	Data      string           `json:"data"`
	Arguments *[]string        `json:"arguments"`
	JsonData  *json.RawMessage `json:"json_data"`
}
