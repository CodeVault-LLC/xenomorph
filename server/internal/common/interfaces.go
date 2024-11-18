package common

import (
	"encoding/json"
	"net"

	"github.com/bwmarrin/discordgo"
)

// BotController defines methods that the Server can call on the Bot.
type BotController interface {
	SendMessageToChannel(channelID, message string) error
	SendEmbedToChannel(channelID, message string, embed *discordgo.MessageEmbed) error
	GenerateUser(data *ClientData) error
	GetChannelID(uuid string, channelName string) string
}

// ServerController defines methods that the Bot can call on the Server.
type ServerController interface {
	SendMessage(uuid string, command Message) error
	GetClientByAddress(addr net.Addr) *ClientData
	RegisterClient(data *ClientData)
	GetClientByUUID(uuid string) *ClientData
}

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

type MessageController interface {
	HandleReceiveMessage(uuid string, msg *Message, conn *net.Conn)
	HandleConnection(uuid string, msg *Message, conn *net.Conn)
	HandleFileChunk(uuid string, msg []byte, conn *net.Conn) error
}
