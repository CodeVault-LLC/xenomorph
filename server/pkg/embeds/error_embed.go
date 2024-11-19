package embeds

import (
	"github.com/bwmarrin/discordgo"
)

type ErrorEntry struct {
	Level string `json:"level"` // e.g., "info", "error"
	Ts    string `json:"ts"`    // Unix timestamp with fractional seconds
	Msg   string `json:"msg"`   // Log message
}

func ErrorEmbed(data *ErrorEntry) discordgo.MessageEmbed {
	messageEmbed := discordgo.MessageEmbed{
		Title: "Xenomorph **`[Error]`**",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "__**Error Info**__",
				Value: Codeblock(DisplayFieldList([]Field{
					{Name: "Level", Value: data.Level},
					{Name: "Timestamp", Value: data.Ts},
					{Name: "Message", Value: data.Msg},
				})),
			},
		},
		Color: 0xFF3333,
	}

	return messageEmbed
}
