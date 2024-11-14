package bot

import (
	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/config"
	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

type Bot struct {
	Session *discordgo.Session
	Handler *Handler
}

// NewBot initializes the bot with a reference to the server through the handler.
func NewBot(token string, server common.ServerController) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, err
	}

	handler := NewHandler(server)
	bot := &Bot{
		Session: session,
		Handler: handler,
	}

	session.AddHandler(bot.ready)
	session.AddHandler(bot.messageCreate)
	return bot, nil
}

// SendMessageToChannel allows the server to send messages to a Discord channel.
func (b *Bot) SendMessageToChannel(channelID, message string) error {
	_, err := b.Session.ChannelMessageSend(channelID, message)
	if err != nil {
		logger.Log.Error("Failed to send message to Discord channel", zap.String("channel_id", channelID), zap.Error(err))
	}
	return err
}

func (b *Bot) SendEmbedToChannel(channelID, message string, embed *discordgo.MessageEmbed) error {
	_, err := b.Session.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		logger.Log.Error("Failed to send embed to Discord channel", zap.String("channel_id", channelID), zap.Error(err))
	}
	return err
}

func (b *Bot) GenerateUser(data *common.ClientData) error {
	category := &discordgo.GuildChannelCreateData{
		Name: data.UUID,
		Type: discordgo.ChannelTypeGuildCategory,
	}

	mainTextChannel := &discordgo.GuildChannelCreateData{
		Name: "main",
		Type: discordgo.ChannelTypeGuildText,
	}

	infoTextChannel := &discordgo.GuildChannelCreateData{
		Name: "info",
		Type: discordgo.ChannelTypeGuildText,
	}

	categoryOutput, err := b.Session.GuildChannelCreateComplex(config.ConfigInstance.DiscordGuild, *category)
	if err != nil {
		logger.Log.Error("Failed to create category", zap.Error(err))
		return err
	}

	mainTextChannel.ParentID = categoryOutput.ID
	_, err = b.Session.GuildChannelCreateComplex(config.ConfigInstance.DiscordGuild, *mainTextChannel)
	if err != nil {
		logger.Log.Error("Failed to create channel", zap.Error(err))
		return err
	}

	infoTextChannel.ParentID = categoryOutput.ID
	_, err = b.Session.GuildChannelCreateComplex(config.ConfigInstance.DiscordGuild, *infoTextChannel)
	if err != nil {
		logger.Log.Error("Failed to create channel", zap.Error(err))
		return err
	}

	return nil
}

func (b *Bot) GetChannelID(uuid string, channelName string) string {
	for _, guild := range b.Session.State.Guilds {
		for _, channel := range guild.Channels {
			if channel.Name == channelName && channel.ParentID == uuid {
				return channel.ID
			}
		}
	}
	return ""
}

func (b *Bot) ready(s *discordgo.Session, event *discordgo.Ready) {
	logger.Log.Info("Bot is online")
}

func (b *Bot) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	b.Handler.HandleMessage(m, s)
}

func (b *Bot) Run() error {
	return b.Session.Open()
}
