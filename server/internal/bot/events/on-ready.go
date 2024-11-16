package events

import (
	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/pkg/logger"
)

// OnReady handles the bot's "ready" event.
func (e *Event) OnReady(session *discordgo.Session, event *discordgo.Ready) {
	logger.Log.Info("Bot is online and ready!")
}
