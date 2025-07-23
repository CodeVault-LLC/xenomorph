package embeds

import (
	"strconv"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/pkg/types"
)

const (
	FileColor = 0x1E3E62
)

func FileEmbed(data *types.File) discordgo.MessageEmbed {
	var fileSize uint64
	if data.Size < 0 {
		fileSize = 0
	} else {
		fileSize = uint64(data.Size)
	}

	messageEmbed := discordgo.MessageEmbed{
		Title: "📂 Xenomorph **`[Files]`**",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "__**File Info**__",
				Value: Codeblock(DisplayFieldList([]Field{
					{Name: "File Name", Value: data.Name},
					{Name: "File Size", Value: strconv.FormatUint(fileSize, 10)},
					{Name: "File Type", Value: data.FileType},
				})),
			},
		},
		Color: FileColor,
	}

	return messageEmbed
}
