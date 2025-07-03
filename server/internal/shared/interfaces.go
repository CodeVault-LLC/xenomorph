package shared

import (
	"net"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/internal/common"
)

// BotController defines methods that the Server can call on the Bot.
type BotController interface {
	SendMessageToChannel(channelID, message string) error
	SendEmbedToChannel(channelID, message string, embed *discordgo.MessageEmbed) error
	GenerateUser(data *common.ClientData) error
	GetChannelFromUser(uuid string, channelName string) string
	GetChannelFromName(channelName string) string
	AddServerController(server ServerController)
}

// ServerController defines methods that the Bot can call on the Server.
type ServerController interface {
	RegisterClient(uuid string, data *common.ClientListData) (*common.ClientListData, string, error)
	UpdateClient(uuid string, data *common.ClientData) (*common.ClientData, error)
	GetClientFromAddr(addr net.Addr) (*common.ClientListData, error)
	GetClient(uuid string) (*common.ClientListData, error)
	GetHandler() HandlerController
	GetCassandra() CassandraController
}

type MessageController interface {
	HandleReceiveMessage(uuid string, msg *common.Command, conn *net.Conn)
	HandleConnection(uuid string, payload []byte, conn *net.Conn) (*common.ClientData, error)
	PreHandleFile(uuid string, msg *common.FileData)
	HandleConnect(uuid string, msg *common.Command, conn *net.Conn) error
}

type HandlerController interface {
	ReadMessage(conn net.Conn) (msgType byte, flags byte, msgID uint32, payload []byte, err error)
	SendMessage(conn net.Conn, msgType byte, flags byte, msgID uint32, payload []byte) error
	
	// Shorthand for sending a file upload message
	FileUpload(conn net.Conn, filename []byte, file []byte) error
}

type CassandraController interface {
	UpdateClient(uuid string, data *common.ClientData) error
	GetClient(uuid string) (common.ClientData, error)
	ClientExists(uuid string) (bool, error)
	RegisterClient(uuid string) (string, error)
	GetClientEssentials(uuid string) (string, string, error)
	InsertFile(uuid string, data common.FileData) error
	Close()
}
