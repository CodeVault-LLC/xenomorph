package events

import (
	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/internal/common"
	"github.com/codevault-llc/xenomorph/pkg/logger"
	"go.uber.org/zap"
)

func (e *Event) OnInteractionCreate(session *discordgo.Session, event *discordgo.InteractionCreate) {
	switch event.Type {
	case discordgo.InteractionApplicationCommand:
		e.handleCommandInteraction(session, event)
	}
}

func (e *Event) handleCommandInteraction(session *discordgo.Session, event *discordgo.InteractionCreate) {
	channel, err := session.State.Channel(event.ChannelID)
	if err != nil {
		logger.GetLogger().Error("Failed to get channel", zap.Error(err))
		return
	}

	category, err := session.State.Channel(channel.ParentID)
	if err != nil {
		logger.GetLogger().Error("Failed to get category", zap.Error(err))
		return
	}

	if category == nil {
		logger.GetLogger().Info("Command invoked outside a valid category", zap.String("channel", channel.Name))
		return
	}

	categoryName := category.Name
	connectionClient, ok := e.Server.GetClient(categoryName)
	if ok != nil {
		session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "It seems like the client is not connected. Please try again later.",
			},
		},
		)

		return
	}

	session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Your request has been received and is being processed.",
		},
	})

	data := event.ApplicationCommandData()
	arguments := data.Options

	var argumentValues []string
	for _, arg := range arguments {
		argumentValues = append(argumentValues, arg.StringValue())
	}

	err = e.Server.GetHandler().SendMessage(connectionClient.Socket, &common.Message{
		Type:      common.MessageTypeCommand,
		Data:      data.Name,
		Arguments: &argumentValues,
	})

	if err != nil {
		logger.GetLogger().Error("Failed to send command message", zap.Error(err))
		session.ChannelMessageSend(channel.ID, "An error occurred while sending the command. Please try again.")
		return
	}
}
