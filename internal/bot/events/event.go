package events

import (
	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/pkg/types"
)

// Event contains the Discord session for all event handlers to use.
type Event struct {
	DCSession *discordgo.Session
	Registry types.RegistryController
}

// NewEvent initializes a new Event instance.
func NewEvent(discord_session *discordgo.Session, registry types.RegistryController) *Event {
	return &Event{
		DCSession: discord_session,
		Registry: registry,
	}
}
