package events

import (
	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/pkg/logger"
)

// OnReady handles the bot's "ready" event.
func (e *Event) OnReady(_ *discordgo.Session, _ *discordgo.Ready) {
	logger.GetLogger().Info("Bot is online and ready!")
}
