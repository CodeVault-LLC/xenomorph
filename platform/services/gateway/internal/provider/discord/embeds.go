package discord

import (
	"fmt"
	"strings"
	"time"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
)

const (
	embedColorBlue   = 0x3498DB
	embedColorGreen  = 0x2ECC71
	embedColorRed    = 0xE74C3C
	embedColorYellow = 0xF1C40F
	embedColorGray   = 0x95A5A6

	censoredTraceLen = 8
	footerSep        = " | "
	brandName        = "Xenomorph"
)

// censorTrace returns the first n characters of a trace ID followed by
// asterisks. When the trace ID is shorter than n the full value is returned
// with a censored suffix.
func censorTrace(traceID string) string {
	trimmed := strings.TrimSpace(traceID)
	if trimmed == "" {
		return "n/a"
	}
	if len(trimmed) <= censoredTraceLen {
		return trimmed + "****"
	}
	return trimmed[:censoredTraceLen] + "****"
}

// footerText builds the standard footer string containing the censored trace
// ID and the brand name.
func footerText(traceID string) string {
	return censorTrace(traceID) + footerSep + brandName
}

// BuildHelpEmbed returns an embed listing every registered slash command.
func BuildHelpEmbed(traceID string) map[string]any {
	return map[string]any{
		"title":       "Available Commands",
		"color":       embedColorBlue,
		"description": "All commands use Discord slash command syntax.",
		"fields": []map[string]any{
			{
				"name":   "/help",
				"value":  "Show this help message",
				"inline": true,
			},
			{
				"name":   "/status",
				"value":  "Show agent connection status",
				"inline": true,
			},
			{
				"name":   "/screenshot",
				"value":  "Request a screenshot from the agent",
				"inline": true,
			},
			{
				"name":   "/ping",
				"value":  "Check bot latency and agent status",
				"inline": true,
			},
			{
				"name":   "/clean",
				"value":  "Clean up agent logs and messages",
				"inline": true,
			},
		},
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildStatusEmbed returns an embed showing the agent's current status.
func BuildStatusEmbed(snapshot provider.AgentSnapshot, traceID string) map[string]any {
	statusIndicator := "Online"
	color := embedColorGreen
	if !snapshot.IsOnline {
		statusIndicator = "Offline"
		color = embedColorRed
	}

	ts := snapshot.LastSeen.UTC().Format("Mon Jan 02 2006 15:04:05 UTC")

	return map[string]any{
		"title":       "Agent Status",
		"color":       color,
		"description": fmt.Sprintf("**%s** is currently **%s**", nonEmpty(snapshot.Hostname), strings.ToLower(statusIndicator)),
		"fields": []map[string]any{
			{"name": "Agent ID", "value": "`" + nonEmpty(snapshot.AgentID) + "`", "inline": true},
			{"name": "Hostname", "value": nonEmpty(snapshot.Hostname), "inline": true},
			{"name": "Status", "value": statusIndicator, "inline": true},
			{"name": "Last Seen", "value": ts, "inline": false},
		},
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildStatusUnknownEmbed returns an embed when the agent has never connected.
func BuildStatusUnknownEmbed(agentID, traceID string) map[string]any {
	return map[string]any{
		"title":       "Agent Status",
		"color":       embedColorGray,
		"description": fmt.Sprintf("Agent `%s` has never connected or has been offline for too long.", agentID),
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildScreenshotQueuedEmbed returns an embed confirming a screenshot request
// was enqueued.
func BuildScreenshotQueuedEmbed(agentID, commandID, traceID string) map[string]any {
	return map[string]any{
		"title":       "Screenshot Requested",
		"color":       embedColorBlue,
		"description": fmt.Sprintf("Screenshot request queued for agent `%s`. The agent will execute the command on the next poll cycle.", agentID),
		"fields": []map[string]any{
			{"name": "Command ID", "value": "`" + commandID + "`", "inline": true},
			{"name": "Agent ID", "value": "`" + agentID + "`", "inline": true},
		},
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildScreenshotFailedEmbed returns an embed when a screenshot request failed.
func BuildScreenshotFailedEmbed(commandID, status, reason, traceID string) map[string]any {
	desc := fmt.Sprintf("Screenshot request **%s** was **%s**.", commandID, status)
	if reason != "" {
		desc += "\n" + reason
	}
	return map[string]any{
		"title":       "Screenshot Failed",
		"color":       embedColorRed,
		"description": desc,
		"fields": []map[string]any{
			{"name": "Command ID", "value": "`" + commandID + "`", "inline": true},
			{"name": "Status", "value": status, "inline": true},
		},
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildScreenshotEmptyEmbed returns an embed when a screenshot returned no data.
func BuildScreenshotEmptyEmbed(traceID string) map[string]any {
	return map[string]any{
		"title":       "Screenshot Empty",
		"color":       embedColorYellow,
		"description": "Screenshot returned empty data.",
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildPingEmbed returns an embed showing bot latency and optionally agent
// information when the command was issued from an agent's category channel.
func BuildPingEmbed(botLatency time.Duration, agentOnline bool, snapshot *provider.AgentSnapshot, traceID string) map[string]any {
	fields := []map[string]any{
		{"name": "Bot Latency", "value": fmt.Sprintf("%dms", botLatency.Milliseconds()), "inline": true},
	}

	if snapshot != nil {
		status := "Offline"
		if snapshot.IsOnline {
			status = "Online"
		}
		agentLatency := time.Since(snapshot.LastSeen)
		fields = append(fields,
			map[string]any{"name": "Agent", "value": nonEmpty(snapshot.Hostname), "inline": true},
			map[string]any{"name": "Agent Status", "value": status, "inline": true},
			map[string]any{"name": "Agent Last Seen", "value": fmt.Sprintf("%dms ago", agentLatency.Milliseconds()), "inline": true},
		)
	}

	color := embedColorGreen
	if !agentOnline && snapshot != nil {
		color = embedColorYellow
	}

	return map[string]any{
		"title":       "Pong!",
		"color":       color,
		"description": "Bot is responsive.",
		"fields":      fields,
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildCleanEmbed returns an embed confirming log cleanup.
func BuildCleanEmbed(agentID string, channelsRemoved bool, traceID string) map[string]any {
	desc := fmt.Sprintf("Logs and messages for agent `%s` have been cleaned up.", agentID)
	if channelsRemoved {
		desc += "\nAll agent channels have been removed."
	}

	return map[string]any{
		"title":       "Cleanup Complete",
		"color":       embedColorGreen,
		"description": desc,
		"fields": []map[string]any{
			{"name": "Agent ID", "value": "`" + agentID + "`", "inline": true},
			{"name": "Channels Removed", "value": fmt.Sprintf("%v", channelsRemoved), "inline": true},
		},
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildErrorEmbed returns a generic error embed with a censored trace ID.
func BuildErrorEmbed(message, traceID string) map[string]any {
	return map[string]any{
		"title":       "Error",
		"color":       embedColorRed,
		"description": message,
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildUnknownCommandEmbed returns an embed for an unrecognized command.
func BuildUnknownCommandEmbed(command, traceID string) map[string]any {
	return map[string]any{
		"title":       "Unknown Command",
		"color":       embedColorYellow,
		"description": fmt.Sprintf("Unknown command `/%s`. Try `/help` to see available commands.", command),
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildQueueNotAvailableEmbed returns an embed when the command queue is nil.
func BuildQueueNotAvailableEmbed(traceID string) map[string]any {
	return map[string]any{
		"title":       "Unavailable",
		"color":       embedColorRed,
		"description": "Command queue is not available.",
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}

// BuildStatusProviderUnavailableEmbed returns an embed when the status
// provider is not configured.
func BuildStatusProviderUnavailableEmbed(traceID string) map[string]any {
	return map[string]any{
		"title":       "Unavailable",
		"color":       embedColorRed,
		"description": "Status provider is not available.",
		"footer": map[string]any{
			"text": footerText(traceID),
		},
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
}
