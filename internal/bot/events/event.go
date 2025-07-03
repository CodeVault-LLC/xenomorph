package events

import (
	"github.com/bwmarrin/discordgo"
)

// Event contains the Discord session for all event handlers to use.
type Event struct {
	Session *discordgo.Session
}

// NewEvent initializes a new Event instance.
func NewEvent(session *discordgo.Session) *Event {
	return &Event{
		Session: session,
	}
}
