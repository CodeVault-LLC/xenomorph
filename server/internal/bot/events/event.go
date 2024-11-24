package events

import (
	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/internal/shared"
)

// Event contains the Discord session for all event handlers to use.
type Event struct {
	Session *discordgo.Session
	Server  shared.ServerController
}

// NewEvent initializes a new Event instance.
func NewEvent(session *discordgo.Session) *Event {
	return &Event{
		Session: session,
	}
}
