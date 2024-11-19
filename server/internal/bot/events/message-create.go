package events

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

const (
	COMMAND_PREFIX = "!"
)

// OnMessageCreate handles the bot's "message create" event.
func (e *Event) OnMessageCreate(session *discordgo.Session, event *discordgo.MessageCreate) {
	// Ignore all messages from a bot.
	if event.Author.Bot {
		return
	}

	// Ignore all messages that don't start with the command prefix.
	if event.Content[0:len(COMMAND_PREFIX)] != COMMAND_PREFIX {
		return
	}

	// Split the message into the command and arguments.
	command := event.Content[len(COMMAND_PREFIX):]
	args := []string{}

	// If the command has arguments, split them into a slice.
	if len(command) > 0 {
		args = strings.Split(command, " ")
		command = args[0]
		args = args[1:]
	}

	channel, err := session.State.Channel(event.ChannelID)
	if err != nil {
		logger.Log.Error("Failed to get channel", zap.Error(err))
		return
	}

	category, err := session.State.Channel(channel.ParentID)
	if err != nil {
		logger.Log.Error("Failed to get category", zap.Error(err))
		return
	}

	if category == nil {
		logger.Log.Info("Message sent outside of category", zap.String("channel", channel.Name))
		return
	}

	categoryName := category.Name

	client := e.Server.GetClientByUUID(categoryName)

	if client == nil {
		logger.Log.Info("Client not found", zap.String("category", categoryName))
		return
	}

	// Handle the command.
	e.Server.SendMessage(client.UUID, common.Message{
		Type:      common.MessageTypeCommand,
		Data:      command,
		Arguments: &args,
		JsonData:  nil,
	})
}
