package events

import (
	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/internal/common"
)

// Event contains the Discord session for all event handlers to use.
type Event struct {
	Session *discordgo.Session
	Server  common.ServerController
}

// NewEvent initializes a new Event instance.
func NewEvent(session *discordgo.Session, server common.ServerController) *Event {
	return &Event{
		Session: session,
		Server:  server,
	}
}
