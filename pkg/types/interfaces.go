package types

import (
	"net"

	"github.com/bwmarrin/discordgo"
)

// BotController defines methods that the Server can call on the Bot.
type BotController interface {
	SendMessageToChannel(channelID, message string) error
	SendEmbedToChannel(channelID, message string, embed *discordgo.MessageEmbed) error
	GenerateUser(data *RegistrationData) error
	GetChannelFromUser(uuid string, channelName string) string
	GetChannelFromName(channelName string) string
	AddServerController(server ServerController)
}

// ServerController defines methods that the Bot can call on the Server.
type ServerController interface {
	RegisterClient(uuid string, data *ClientDataLite) (*ClientDataLite, string, error)
	UpdateClient(uuid string, data *RegistrationData) (*RegistrationData, error)
	GetClientFromAddr(addr net.Addr) (*ClientDataLite, error)
	GetClient(uuid string) (*ClientDataLite, error)
	GetHandler() HandlerController
	GetCassandra() CassandraController
}

type MessageController interface {
	HandleReceiveMessage(uuid string, msg *Command, conn *net.Conn)
	HandleConnection(uuid string, payload []byte, conn *net.Conn) (*RegistrationData, error)
	//PreHandleFile(uuid string, msg *FileData)
	HandleConnect(uuid string, msg *Command, conn *net.Conn) error
}

type HandlerController interface {
	ReadMessage(conn net.Conn) (msgType byte, flags byte, msgID uint32, payload []byte, err error)
	SendMessage(conn net.Conn, msgType byte, flags byte, msgID uint32, payload []byte) error
	
	// Shorthand for sending a file upload message
	//FileUpload(conn net.Conn, filename []byte, file []byte) error
}

type CassandraController interface {
	UpdateClient(uuid string, data *RegistrationData) error
	GetClient(uuid string) (RegistrationData, error)
	ClientExists(uuid string) (bool, error)
	RegisterClient(uuid string) (string, error)
	GetClientEssentials(uuid string) (string, string, error)
	//InsertFile(uuid string, data FileData) error
	Close()
}
