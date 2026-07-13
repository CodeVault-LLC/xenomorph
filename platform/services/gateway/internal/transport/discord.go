package transport

import (
	"context"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider/discord"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/sdk"
)

// interactionRespond is a helper that sends an embed response to a Discord
// interaction. Returns nil when discordPoster is nil.
func (s *Server) interactionRespond(ctx context.Context, interaction *discordgo.Interaction, embed map[string]any) error {
	if s.discordPoster == nil {
		return nil
	}
	return s.discordPoster.RespondInteraction(ctx, interaction.ID, interaction.Token, embed)
}

// extractInteractionOption extracts a typed option value from a Discord
// interaction's application command data. Returns the option value and true
// when found, or zero value and false when not present.
func extractInteractionOption(data discordgo.ApplicationCommandInteractionData, name string) (discordgo.ApplicationCommandInteractionDataOption, bool) {
	for _, opt := range data.Options {
		if opt.Name == name {
			return *opt, true
		}
	}
	return discordgo.ApplicationCommandInteractionDataOption{}, false
}

// HandleDiscordInteraction routes a Discord slash command interaction to the
// appropriate handler based on the command name. Unknown commands return a
// help hint embed.
func (s *Server) HandleDiscordInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID string) error {
	if s.discordPoster == nil {
		return nil
	}

	traceID := sdk.TraceIDFromContext(ctx)

	data := interaction.ApplicationCommandData()

	switch data.Name {
	case "help":
		return s.interactionRespond(ctx, interaction, discord.BuildHelpEmbed(traceID))
	case "status":
		return s.handleDiscordStatusInteraction(ctx, interaction, agentID, traceID)
	case "screenshot":
		return s.handleDiscordScreenshotInteraction(ctx, interaction, agentID, traceID)
	case "ping":
		return s.handleDiscordPingInteraction(ctx, interaction, agentID, traceID)
	case "clean":
		return s.handleDiscordCleanInteraction(ctx, interaction, agentID, traceID)
	default:
		return s.interactionRespond(ctx, interaction, discord.BuildUnknownCommandEmbed(data.Name, traceID))
	}
}

// handleDiscordStatusInteraction responds with the agent's current
// online/offline status as an embed.
func (s *Server) handleDiscordStatusInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID, traceID string) error {
	if s.statusProvider == nil {
		return s.interactionRespond(ctx, interaction, discord.BuildStatusProviderUnavailableEmbed(traceID))
	}

	snapshot, ok := s.statusProvider.Snapshot(agentID)
	if !ok {
		return s.interactionRespond(ctx, interaction, discord.BuildStatusUnknownEmbed(agentID, traceID))
	}

	return s.interactionRespond(ctx, interaction, discord.BuildStatusEmbed(snapshot, traceID))
}

// handleDiscordScreenshotInteraction enqueues a screenshot request for the
// agent and responds with an embed confirmation.
func (s *Server) handleDiscordScreenshotInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID, traceID string) error {
	if s.commandQueue == nil {
		return s.interactionRespond(ctx, interaction, discord.BuildQueueNotAvailableEmbed(traceID))
	}

	userName := ""
	if interaction.Member != nil && interaction.Member.User != nil {
		userName = interaction.Member.User.Username
	} else if interaction.User != nil {
		userName = interaction.User.Username
	}

	cmd := &command.Envelope{
		Type:        string(CommandTypeRequestScreenshot),
		RequestedBy: userName,
		Reason:      fmt.Sprintf("Screenshot requested by %s via Discord", userName),
	}
	if err := s.commandQueue.Enqueue(agentID, cmd); err != nil {
		return fmt.Errorf("enqueue signed screenshot command: %w", err)
	}

	return s.interactionRespond(ctx, interaction, discord.BuildScreenshotQueuedEmbed(agentID, cmd.CommandID, traceID))
}

// handleDiscordPingInteraction responds with bot latency and optionally
// agent information when the command was issued from an agent's category
// channel.
func (s *Server) handleDiscordPingInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID, traceID string) error {
	botLatency := time.Duration(0)
	if s.discordPoster != nil {
		_, _, err := s.discordPoster.GetBotUser(ctx)
		if err == nil {
			botLatency = time.Duration(0)
		}
	}

	var agentSnapshot *provider.AgentSnapshot
	agentOnline := false
	if s.statusProvider != nil {
		snapshot, ok := s.statusProvider.Snapshot(agentID)
		if ok {
			agentSnapshot = &snapshot
			agentOnline = snapshot.IsOnline
		}
	}

	return s.interactionRespond(ctx, interaction, discord.BuildPingEmbed(botLatency, agentOnline, agentSnapshot, traceID))
}

// handleDiscordCleanInteraction cleans up agent logs and messages. When the
// remove_channels option is set to true, all of the agent's Discord channels
// are also deleted.
func (s *Server) handleDiscordCleanInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID, traceID string) error {
	if s.discordPoster == nil {
		return nil
	}

	removeChannels := false
	data := interaction.ApplicationCommandData()
	if opt, ok := extractInteractionOption(data, "remove_channels"); ok {
		removeChannels = opt.BoolValue()
	}

	if removeChannels {
		sets := s.discordPoster.AllChannelSets()
		if set, ok := sets[agentID]; ok {
			if set.CommandsID != "" {
				_ = s.discordPoster.DeleteChannel(ctx, set.CommandsID)
			}
			if set.LogsID != "" {
				_ = s.discordPoster.DeleteChannel(ctx, set.LogsID)
			}
			if set.UploadsID != "" {
				_ = s.discordPoster.DeleteChannel(ctx, set.UploadsID)
			}
			if set.CategoryID != "" {
				_ = s.discordPoster.DeleteChannel(ctx, set.CategoryID)
			}
		}
	}

	return s.interactionRespond(ctx, interaction, discord.BuildCleanEmbed(agentID, removeChannels, traceID))
}
