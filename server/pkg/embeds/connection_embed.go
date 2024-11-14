package embeds

import (
	"strconv"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/internal/common"
)

func ConnectionEmbed(data *common.ClientData) discordgo.MessageEmbed {
	return discordgo.MessageEmbed{
		Title: "Xenomorph **`[Connection]`**",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "Computer Info",
				Inline: true,
				Value: "__**Computer Info**__\n" + Codeblock(DisplayFieldList([]Field{
					{Name: "Computer Name", Value: data.ComputerName},
					{Name: "Computer OS", Value: data.ComputerOS},
					{Name: "Computer Version", Value: data.ComputerVersion},
					{Name: "Total Memory", Value: strconv.FormatUint(data.TotalMemory, 10)},
					{Name: "Up Time", Value: data.UpTime},
					{Name: "UUID", Value: data.UUID},
					{Name: "CPU", Value: data.CPU},
					{Name: "GPU", Value: data.GPU},
					{Name: "UAC", Value: strconv.FormatBool(data.UAC)},
					{Name: "Anti Virus", Value: data.AntiVirus},
				})),
			},
		},
		Color: 0x00ff00,
	}
}
