package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
)

const defaultAPIBaseURL = "https://discord.com/api/v10"

type Config struct {
	BotToken   string
	GuildID    string
	APIBaseURL string
}

type Provider struct {
	config Config
	client *http.Client

	mu            sync.Mutex
	agentChannels map[string]channelSet
}

type channelSet struct {
	CategoryID string
	LogsID     string
	UploadsID  string
	CommandsID string
}

type discordChannel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	ParentID string `json:"parent_id"`
	Topic    string `json:"topic"`
}

const (
	discordChannelTypeText     = 0
	discordChannelTypeCategory = 4
)

var slugifyPattern = regexp.MustCompile(`[^a-z0-9]+`)

func New(config Config, client *http.Client) (*Provider, error) {
	config.BotToken = strings.TrimSpace(config.BotToken)
	config.GuildID = strings.TrimSpace(config.GuildID)
	config.APIBaseURL = strings.TrimRight(strings.TrimSpace(config.APIBaseURL), "/")

	if config.BotToken == "" {
		return nil, fmt.Errorf("missing bot token")
	}
	if config.GuildID == "" {
		return nil, fmt.Errorf("missing guild id")
	}
	if config.APIBaseURL == "" {
		config.APIBaseURL = defaultAPIBaseURL
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}

	return &Provider{
		config:        config,
		client:        client,
		agentChannels: make(map[string]channelSet),
	}, nil
}

func (p *Provider) Name() string {
	return "discord"
}

func (p *Provider) PreflightCheck(ctx context.Context) error {
	if err := p.discordGet(ctx, "/users/@me", "bot authentication check"); err != nil {
		return err
	}
	if err := p.discordGet(ctx, "/guilds/"+p.config.GuildID, "guild access check"); err != nil {
		return err
	}
	if err := p.discordGet(ctx, "/guilds/"+p.config.GuildID+"/channels", "guild channel listing check"); err != nil {
		return err
	}
	return nil
}

func (p *Provider) Notify(ctx context.Context, event provider.ActivityEvent) error {
	if p.config.GuildID == "" {
		return nil
	}

	set, err := p.ensureAgentChannels(ctx, event.AgentID, event.Hostname)
	if err != nil {
		return fmt.Errorf("ensure channels: %w", err)
	}
	if set.LogsID == "" {
		return fmt.Errorf("logs channel missing for agent %s", event.AgentID)
	}

	embed := p.buildActivityEmbed(event)
	payload, err := json.Marshal(map[string]any{
		"embeds": []map[string]any{embed},
	})
	if err != nil {
		return fmt.Errorf("marshal embed: %w", err)
	}

	url := p.config.APIBaseURL + "/channels/" + set.LogsID + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	p.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("discord status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}
	return nil
}

func (p *Provider) buildActivityEmbed(event provider.ActivityEvent) map[string]any {
	var color int
	var title string
	if event.Status == provider.StatusOnline {
		color = 0x2ECC71
		title = "Agent Connected"
	} else {
		color = 0xE74C3C
		title = "Agent Disconnected"
	}

	ts := event.OccurredAt.UTC().Format("Mon Jan 02 2006 15:04:05 UTC")

	return map[string]any{
		"title":       title,
		"color":       color,
		"timestamp":   event.OccurredAt.UTC().Format(time.RFC3339),
		"description": fmt.Sprintf("**%s** is now **%s**", nonEmpty(event.Hostname), event.Status),
		"fields": []map[string]any{
			{"name": "Agent ID", "value": nonEmpty(event.AgentID), "inline": true},
			{"name": "Hostname", "value": nonEmpty(event.Hostname), "inline": true},
			{"name": "Status", "value": string(event.Status), "inline": true},
			{"name": "Timestamp", "value": ts, "inline": false},
		},
		"footer": map[string]any{
			"text": fmt.Sprintf("Source: %s", event.Source),
		},
	}
}

func (p *Provider) ensureAgentChannels(ctx context.Context, agentID, hostname string) (channelSet, error) {
	if strings.TrimSpace(agentID) == "" {
		return channelSet{}, fmt.Errorf("empty agent id")
	}

	p.mu.Lock()
	if set, ok := p.agentChannels[agentID]; ok && set.CategoryID != "" && set.LogsID != "" && set.UploadsID != "" && set.CommandsID != "" {
		p.mu.Unlock()
		return set, nil
	}
	p.mu.Unlock()

	channels, err := p.listGuildChannels(ctx)
	if err != nil {
		return channelSet{}, err
	}

	set := discoverChannelSet(agentID, channels)
	if set.CategoryID == "" {
		categoryID, err := p.createCategory(ctx, categoryName(hostname, agentID))
		if err != nil {
			return channelSet{}, err
		}
		set.CategoryID = categoryID
	}

	if set.LogsID == "" {
		id, err := p.createTextChannel(ctx, "logs", set.CategoryID, channelTopic(agentID, "logs"))
		if err != nil {
			return channelSet{}, err
		}
		set.LogsID = id
	}
	if set.UploadsID == "" {
		id, err := p.createTextChannel(ctx, "uploads", set.CategoryID, channelTopic(agentID, "uploads"))
		if err != nil {
			return channelSet{}, err
		}
		set.UploadsID = id
	}
	if set.CommandsID == "" {
		id, err := p.createTextChannel(ctx, "commands", set.CategoryID, channelTopic(agentID, "commands"))
		if err != nil {
			return channelSet{}, err
		}
		set.CommandsID = id
	}

	p.mu.Lock()
	p.agentChannels[agentID] = set
	p.mu.Unlock()

	return set, nil
}

func (p *Provider) listGuildChannels(ctx context.Context) ([]discordChannel, error) {
	url := p.config.APIBaseURL + "/guilds/" + p.config.GuildID + "/channels"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build guild channel list request: %w", err)
	}
	p.setAuthHeader(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("guild channel list request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("guild channel list status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}

	var channels []discordChannel
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&channels); err != nil {
		return nil, fmt.Errorf("decode guild channel list: %w", err)
	}

	return channels, nil
}

func discoverChannelSet(agentID string, channels []discordChannel) channelSet {
	topicNeedle := "xenomorph agent_id=" + agentID + " "
	short := shortAgentID(agentID)
	set := channelSet{}
	for _, ch := range channels {
		if ch.Type == discordChannelTypeCategory && strings.HasSuffix(strings.ToLower(ch.Name), "-"+short) {
			set.CategoryID = ch.ID
		}
		if ch.Type != discordChannelTypeText {
			continue
		}
		if !strings.Contains(ch.Topic, topicNeedle) {
			continue
		}
		switch strings.ToLower(ch.Name) {
		case "logs":
			set.LogsID = ch.ID
			if set.CategoryID == "" {
				set.CategoryID = ch.ParentID
			}
		case "uploads":
			set.UploadsID = ch.ID
			if set.CategoryID == "" {
				set.CategoryID = ch.ParentID
			}
		case "commands":
			set.CommandsID = ch.ID
			if set.CategoryID == "" {
				set.CategoryID = ch.ParentID
			}
		}
	}
	return set
}

func (p *Provider) createCategory(ctx context.Context, name string) (string, error) {
	body := map[string]any{
		"name": name,
		"type": discordChannelTypeCategory,
	}
	var created discordChannel
	if err := p.discordPostJSON(ctx, "/guilds/"+p.config.GuildID+"/channels", body, &created, "create category"); err != nil {
		return "", err
	}
	if strings.TrimSpace(created.ID) == "" {
		return "", fmt.Errorf("create category: missing channel id")
	}
	return created.ID, nil
}

func (p *Provider) createTextChannel(ctx context.Context, name, categoryID, topic string) (string, error) {
	body := map[string]any{
		"name":      name,
		"type":      discordChannelTypeText,
		"parent_id": categoryID,
		"topic":     topic,
	}
	var created discordChannel
	if err := p.discordPostJSON(ctx, "/guilds/"+p.config.GuildID+"/channels", body, &created, "create text channel"); err != nil {
		return "", err
	}
	if strings.TrimSpace(created.ID) == "" {
		return "", fmt.Errorf("create text channel: missing channel id")
	}
	return created.ID, nil
}

func (p *Provider) discordPostJSON(ctx context.Context, path string, body any, out any, operation string) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("%s: marshal request: %w", operation, err)
	}
	url := p.config.APIBaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("%s: build request: %w", operation, err)
	}
	p.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: request failed: %w", operation, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s: discord status %d: %s", operation, resp.StatusCode, strings.TrimSpace(string(errBody)))
	}
	if out != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
			return fmt.Errorf("%s: decode response: %w", operation, err)
		}
	}
	return nil
}

func (p *Provider) discordGet(ctx context.Context, path string, operation string) error {
	url := p.config.APIBaseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", operation, err)
	}
	p.setAuthHeader(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: request failed: %w", operation, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("%s: discord status %d: %s", operation, resp.StatusCode, strings.TrimSpace(string(errBody)))
	}
	return nil
}

func (p *Provider) setAuthHeader(req *http.Request) {
	req.Header.Set("Authorization", "Bot "+p.config.BotToken)
}

func channelTopic(agentID, kind string) string {
	return fmt.Sprintf("xenomorph agent_id=%s kind=%s", agentID, kind)
}

func categoryName(hostname, agentID string) string {
	slug := slugify(hostname)
	if slug == "" {
		slug = "agent"
	}
	short := shortAgentID(agentID)
	return trimDiscordName("client-" + slug + "-" + short)
}

func shortAgentID(agentID string) string {
	trimmed := strings.TrimSpace(agentID)
	if len(trimmed) >= 8 {
		return strings.ToLower(trimmed[:8])
	}
	if trimmed == "" {
		return "unknown"
	}
	return strings.ToLower(trimmed)
}

func trimDiscordName(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "client-unknown"
	}
	if len(trimmed) <= 100 {
		return trimmed
	}
	return trimmed[:100]
}

func slugify(value string) string {
	lowered := strings.ToLower(strings.TrimSpace(value))
	if lowered == "" {
		return ""
	}
	slug := slugifyPattern.ReplaceAllString(lowered, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 40 {
		slug = strings.Trim(slug[:40], "-")
	}
	return slug
}

func nonEmpty(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func (p *Provider) SendChannelMessage(ctx context.Context, channelID, content string) error {
	payload, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	url := p.config.APIBaseURL + "/channels/" + channelID + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	p.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("discord status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}
	return nil
}

func (p *Provider) SendChannelFile(ctx context.Context, channelID, fileName string, data []byte, content string) error {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)

	payloadJSON, err := json.Marshal(map[string]string{"content": content})
	if err != nil {
		return fmt.Errorf("marshal payload json: %w", err)
	}
	if err := w.WriteField("payload_json", string(payloadJSON)); err != nil {
		return fmt.Errorf("write payload_json field: %w", err)
	}

	fw, err := w.CreateFormFile("file", fileName)
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := fw.Write(data); err != nil {
		return fmt.Errorf("write file data: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("close multipart writer: %w", err)
	}

	url := p.config.APIBaseURL + "/channels/" + channelID + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &b)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	p.setAuthHeader(req)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("discord status %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
	}
	return nil
}

func (p *Provider) CommandsChannelID(agentID string) (string, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	set, ok := p.agentChannels[agentID]
	if !ok {
		return "", false
	}
	return set.CommandsID, set.CommandsID != ""
}

// AllCommandsChannels returns a snapshot of all known agent-to-commands-channel mappings.
func (p *Provider) AllCommandsChannels() map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()

	result := make(map[string]string, len(p.agentChannels))
	for id, set := range p.agentChannels {
		if set.CommandsID != "" {
			result[id] = set.CommandsID
		}
	}
	return result
}
