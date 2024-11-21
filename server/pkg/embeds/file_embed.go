package embeds

import (
	"strconv"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/internal/common"
)

func FileEmbed(data *common.FileData, msg *common.Message) discordgo.MessageEmbed {
	messageEmbed := discordgo.MessageEmbed{
		Title: "📂 Xenomorph **`[Files]`**",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "__**File Info**__",
				Value: Codeblock(DisplayFieldList([]Field{
					{Name: "File Name", Value: data.FileName},
					{Name: "File Size", Value: strconv.FormatUint(uint64(data.FileSize), 10)},
					{Name: "File Type", Value: data.FileType},
				})),
			},
		},
		Color: 0x1E3E62,
	}

	return messageEmbed
}
