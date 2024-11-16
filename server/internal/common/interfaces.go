package common

import (
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
	SendCommand(clientID, command string) error
}
