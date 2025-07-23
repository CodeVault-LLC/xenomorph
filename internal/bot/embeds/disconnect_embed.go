package embeds

import (
	"github.com/bwmarrin/discordgo"
	"github.com/codevault-llc/xenomorph/pkg/types"
)

func DisconnectEmbed(data types.DisconnectData) *discordgo.MessageEmbed {
		color := 0xFF3333

		// Create a description regarding the reason (sigterm = shutdown, sigint = ctrl+c, etc.)
		extraDesc := ""
		switch data.Reason {
		case types.ShutdownSigInt.String():
			extraDesc = "The client was terminated by the user (Ctrl+C)."
		case types.ShutdownSigTerm.String():
			extraDesc = "The client was terminated by the system (SIGTERM)."
		case types.ShutdownManual.String():
			extraDesc = "The client was manually disconnected."
		case types.ShutdownSystemSleep.String():
			extraDesc = "The system is going to sleep, disconnecting the client."
		case types.ShutdownNetworkLoss.String():
			extraDesc = "The client lost network connection."
		case types.ShutdownServerClosed.String():
			extraDesc = "The server closed the connection."
		case types.ShutdownError.String():
			extraDesc = "An error occurred, causing the client to disconnect."
		default:
			extraDesc = "The client disconnected for an unknown reason."
	}

    embed := &discordgo.MessageEmbed{
        Title:      "Xenomorph **`[Disconnect]`**",
        Color:       color,
        Fields: []*discordgo.MessageEmbedField{
            {
                Name:  "__**Reason**__",
                Value: Codeblock(data.Reason + " - " + extraDesc),
            },
            {
                Name:  "__**Uptime**__",
                Value: Codeblock(data.Uptime),
            },
            {
                Name:  "__**Host**__",
                Value: Codeblock(data.Hostname),
            },
        },
    }

    return embed
}
