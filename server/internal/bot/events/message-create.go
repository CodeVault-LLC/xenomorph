package events

import (
	"strings"

	"github.com/bwmarrin/discordgo"
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
}
