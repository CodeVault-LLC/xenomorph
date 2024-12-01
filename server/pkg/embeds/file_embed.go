package embeds

import (
	"strconv"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/internal/common"
)

const (
	FileColor = 0x1E3E62
)

func FileEmbed(data *common.FileData, _ *common.Message) discordgo.MessageEmbed {
	var fileSize uint64
	if data.FileSize < 0 {
		fileSize = 0
	} else {
		fileSize = uint64(data.FileSize)
	}

	messageEmbed := discordgo.MessageEmbed{
		Title: "📂 Xenomorph **`[Files]`**",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "__**File Info**__",
				Value: Codeblock(DisplayFieldList([]Field{
					{Name: "File Name", Value: data.FileName},
					{Name: "File Size", Value: strconv.FormatUint(fileSize, 10)},
					{Name: "File Type", Value: data.FileType},
				})),
			},
		},
		Color: FileColor,
	}

	return messageEmbed
}
