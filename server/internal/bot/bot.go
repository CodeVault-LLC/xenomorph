package bot

import (
	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/config"
	"github.com/codevault-llc/xenomorph/internal/bot/events"
	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/internal/shared"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

type Bot struct {
	Session *discordgo.Session
	Events  *events.Event
}

// NewBot initializes the bot with a reference to the server through the handler.
func NewBot(token string) (*Bot, error) {
	session, err := discordgo.New("Bot " + token)
	if err != nil {
		logger.GetLogger().Error("Failed to create Discord session", zap.Error(err))
		return nil, err
	}

	session.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuildMessageReactions | discordgo.IntentsGuilds | discordgo.IntentsGuildMembers | discordgo.IntentsGuildIntegrations | discordgo.IntentsGuildWebhooks | discordgo.IntentsGuildInvites | discordgo.IntentsGuildVoiceStates | discordgo.IntentsGuildPresences | discordgo.IntentsGuildMessageTyping | discordgo.IntentsDirectMessages | discordgo.IntentsDirectMessageReactions | discordgo.IntentsDirectMessageTyping
	eventHandlers := events.NewEvent(session)

	bot := &Bot{
		Session: session,
		Events:  eventHandlers,
	}

	session.AddHandler(eventHandlers.OnReady)
	session.AddHandler(eventHandlers.OnInteractionCreate)

	return bot, nil
}

func (b *Bot) AddServerController(server shared.ServerController) {
	b.Events.Server = server
}

// SendMessageToChannel allows the server to send messages to a Discord channel.
func (b *Bot) SendMessageToChannel(channelID, message string) error {
	_, err := b.Session.ChannelMessageSend(channelID, message)
	if err != nil {
		logger.GetLogger().Error("Failed to send message to Discord channel", zap.String("channel_id", channelID), zap.Error(err))
	}

	return err
}

func (b *Bot) SendEmbedToChannel(channelID, _ string, embed *discordgo.MessageEmbed) error {
	_, err := b.Session.ChannelMessageSendEmbed(channelID, embed)
	if err != nil {
		logger.GetLogger().Error("Failed to send embed to Discord channel", zap.String("channel_id", channelID), zap.Error(err))
	}

	return err
}

func (b *Bot) GenerateUser(data *common.ClientData) error {
	categoryID := b.GetCategoryID(data.UUID)
	if categoryID != "" {
		return nil
	}

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
		logger.GetLogger().Error("Failed to create category", zap.Error(err))
		return err
	}

	mainTextChannel.ParentID = categoryOutput.ID
	_, err = b.Session.GuildChannelCreateComplex(config.ConfigInstance.DiscordGuild, *mainTextChannel)

	if err != nil {
		logger.GetLogger().Error("Failed to create channel", zap.Error(err))
		return err
	}

	infoTextChannel.ParentID = categoryOutput.ID
	_, err = b.Session.GuildChannelCreateComplex(config.ConfigInstance.DiscordGuild, *infoTextChannel)

	if err != nil {
		logger.GetLogger().Error("Failed to create channel", zap.Error(err))
		return err
	}

	return nil
}

func (b *Bot) GetCategoryID(uuid string) string {
	for _, guild := range b.Session.State.Guilds {
		for _, channel := range guild.Channels {
			if channel.Type == discordgo.ChannelTypeGuildCategory && channel.Name == uuid {
				return channel.ID
			}
		}
	}

	return ""
}

func (b *Bot) GetChannelFromUser(uuid string, channelName string) string {
	logger := logger.GetLogger()

	for _, guild := range b.Session.State.Guilds {
		for _, channel := range guild.Channels {
			if channel.ParentID == "" {
				continue
			}

			parentChannel, err := b.Session.Channel(channel.ParentID)
			if err != nil {
				logger.Error("Failed to get parent channel", zap.Error(err))
				continue
			}

			if parentChannel.Name == uuid {
				if channel.Name == channelName {
					return channel.ID
				}
			}
		}
	}

	return ""
}

func (b *Bot) GetChannelFromName(channelName string) string {
	for _, guild := range b.Session.State.Guilds {
		for _, channel := range guild.Channels {
			if channel.Name == channelName {
				return channel.ID
			}
		}
	}

	return ""
}

func (b *Bot) RegisterCommands(session *discordgo.Session, guildID string, isProduction bool) error {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "ls",
			Description: "List the contents of a directory.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "directory",
					Description: "The directory to list.",
					Required:    false,
				},
			},
		},
		{
			Name:        "terminal",
			Description: "Open a terminal session.",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "command",
					Description: "The command to run.",
					Required:    true,
				},
			},
		},
	}

	for _, cmd := range commands {
		if isProduction {
			_, err := session.ApplicationCommandCreate(session.State.User.ID, "", cmd)
			if err != nil {
				logger.GetLogger().Error("Failed to register global command", zap.String("name", cmd.Name), zap.Error(err))
				return err
			}
		} else {
			_, err := session.ApplicationCommandCreate(session.State.User.ID, guildID, cmd)
			if err != nil {
				logger.GetLogger().Error("Failed to register guild command", zap.String("name", cmd.Name), zap.String("guildID", guildID), zap.Error(err))
				return err
			}
		}
	}

	return nil
}

func (b *Bot) CleanupCommands(session *discordgo.Session) error {
	commands, err := session.ApplicationCommands(session.State.User.ID, "")
	if err != nil {
		return err
	}

	for _, cmd := range commands {
		err := session.ApplicationCommandDelete(session.State.User.ID, "", cmd.ID)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *Bot) Run() error {
	err := b.Session.Open()
	if err != nil {
		logger.GetLogger().Error("Failed to open Discord session", zap.Error(err))
		return err
	}

	err = b.CleanupCommands(b.Session)
	if err != nil {
		logger.GetLogger().Error("Failed to cleanup commands", zap.Error(err))
		return err
	}

	isProduction := config.ConfigInstance.AppEnv == "production"

	err = b.RegisterCommands(b.Session, config.ConfigInstance.DiscordGuild, isProduction)
	if err != nil {
		logger.GetLogger().Error("Failed to register commands", zap.Error(err))
		return err
	}

	return nil
}
