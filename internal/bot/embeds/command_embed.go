package embeds

import (
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/pkg/types"
)

const (
	CommandResponseColor = 0x2C8BFF
)

func CommandResponseEmbed(response *types.CommandResponse, serverDuration time.Duration) *discordgo.MessageEmbed {
	fields := []*discordgo.MessageEmbedField{}

	// Command Output Field(s)
	if response.Output != "" {
		outputFields := SplitField("__**Command Output**__", Codeblock(response.Output))
		fields = append(fields, outputFields...)
	} else {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  "__**Command Output**__",
			Value: "No output returned",
		})
	}

	// Error field (if any)
	if response.Error != "" {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:  "__**Error**__",
			Value: response.Error,
		})
	}

	// Duration Field
	fields = append(fields, &discordgo.MessageEmbedField{
		Name: "__**Timing**__",
		Value: Codeblock(DisplayFieldList([]Field{
			{
				Name:  "Execution Time",
				Value: (time.Duration(response.Duration) * time.Millisecond).Truncate(time.Millisecond).String(),
			},
			{
				Name:  "Server Duration",
				Value: serverDuration.Truncate(time.Millisecond).String(),
			},
		})),
	})

	return &discordgo.MessageEmbed{
		Title:  "Xenomorph **`[Command]`**",
		Color:  CommandResponseColor,
		Fields: fields,
	}
}
