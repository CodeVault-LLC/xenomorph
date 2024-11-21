package common

import (
	"net"

	"github.com/bwmarrin/discordgo"
)

// BotController defines methods that the Server can call on the Bot.
type BotController interface {
	SendMessageToChannel(channelID, message string) error
	SendEmbedToChannel(channelID, message string, embed *discordgo.MessageEmbed) error
	GenerateUser(data *ClientData) error
	GetChannelFromUser(uuid string, channelName string) string
	GetChannelFromName(channelName string) string
}

// ServerController defines methods that the Bot can call on the Server.
type ServerController interface {
	SendMessage(uuid string, command Message) error
	GetClientByAddress(addr net.Addr) *ClientData
	RegisterClient(data *ClientData) (*ClientData, error)
	UpdateClient(data *ClientData) (*ClientData, error)
	GetClientByUUID(uuid string) *ClientData
}

type MessageController interface {
	HandleReceiveMessage(uuid string, msg *Message, conn *net.Conn)
	HandleConnection(uuid string, msg *Message, conn *net.Conn) (*ClientData, error)
	PreHandleFile(uuid string, msg *FileData)
}

type FileController interface {
	FileUpload(conn net.Conn, header Header) (*Message, error)
}
