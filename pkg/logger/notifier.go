package logger

import "github.com/bwmarrin/discordgo"

type BotNotifier interface {
	GetChannelFromName(name string) string
	SendEmbedToChannel(channelID, msg string, embed *discordgo.MessageEmbed) error
}