package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
)

const guildChannelsPath = "/guilds/g-1/channels"

// embedCapture is an http.Handler that records the first message POST payload.
type embedCapture struct {
	t       *testing.T
	Auth    string
	Payload embedPayload
	Path    string
}

type embedPayload struct {
	Embeds []map[string]any `json:"embeds"`
}

func (h *embedCapture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == guildChannelsPath && r.Method == http.MethodGet {
		_, _ = w.Write([]byte(`[
			{"id":"cat-1","name":"client-host-a-agent-5","type":4,"parent_id":null,"topic":""},
			{"id":"logs-1","name":"logs","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=logs"},
			{"id":"up-1","name":"uploads","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=uploads"},
			{"id":"cmd-1","name":"commands","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=commands"}
		]`))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/channels/") && strings.HasSuffix(r.URL.Path, "/messages") {
		h.Path = r.URL.Path
		h.Auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&h.Payload); err != nil {
			h.t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		return
	}
	h.t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
}

func testDiscordProvider(t *testing.T, srv *httptest.Server) *Provider {
	t.Helper()
	p, err := New(Config{BotToken: "abc", GuildID: "g-1", APIBaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	return p
}

func TestNewRequiresBotToken(t *testing.T) {
	_, err := New(Config{GuildID: "g-1"}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing bot token") {
		t.Fatalf("expected missing bot token error, got: %v", err)
	}
}

func TestNewRequiresGuildID(t *testing.T) {
	_, err := New(Config{BotToken: "abc"}, nil)
	if err == nil || !strings.Contains(err.Error(), "missing guild id") {
		t.Fatalf("expected missing guild id error, got: %v", err)
	}
}

func TestProviderNotifyPostsEmbedForOnline(t *testing.T) {
	capture := &embedCapture{t: t}
	srv := httptest.NewServer(capture)
	defer srv.Close()

	p := testDiscordProvider(t, srv)
	err := p.Notify(context.Background(), provider.ActivityEvent{
		AgentID:    "agent-5",
		Hostname:   "host-a",
		OccurredAt: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
		Status:     provider.StatusOnline,
		Source:     "heartbeat",
	})
	if err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	if capture.Auth != "Bot abc" {
		t.Fatalf("unexpected auth header: %q", capture.Auth)
	}
	if !strings.Contains(capture.Path, "/messages") {
		t.Fatalf("expected messages endpoint, got: %s", capture.Path)
	}
	if len(capture.Payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(capture.Payload.Embeds))
	}

	embed := capture.Payload.Embeds[0]
	if embed["title"] != "Agent Connected" {
		t.Fatalf("expected title 'Agent Connected', got: %v", embed["title"])
	}
	if embed["color"] != float64(0x2ECC71) {
		t.Fatalf("expected green color, got: %v", embed["color"])
	}
	desc, _ := embed["description"].(string)
	if !strings.Contains(desc, "**host-a**") || !strings.Contains(desc, "**online**") {
		t.Fatalf("unexpected description: %s", desc)
	}
}

func TestProviderNotifyPostsEmbedForOffline(t *testing.T) {
	capture := &embedCapture{t: t}
	srv := httptest.NewServer(capture)
	defer srv.Close()

	p := testDiscordProvider(t, srv)
	err := p.Notify(context.Background(), provider.ActivityEvent{
		AgentID:    "agent-5",
		Hostname:   "host-a",
		OccurredAt: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
		Status:     provider.StatusOffline,
		Source:     "heartbeat-timeout",
	})
	if err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	if len(capture.Payload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(capture.Payload.Embeds))
	}

	embed := capture.Payload.Embeds[0]
	if embed["title"] != "Agent Disconnected" {
		t.Fatalf("expected title 'Agent Disconnected', got: %v", embed["title"])
	}
	if embed["color"] != float64(0xE74C3C) {
		t.Fatalf("expected red color, got: %v", embed["color"])
	}
}

func TestProviderNotifyReturnsErrorOnFailureStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == guildChannelsPath && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[
				{"id":"cat-1","name":"client-host-a-agent-5","type":4,"parent_id":null,"topic":""},
				{"id":"logs-1","name":"logs","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=logs"},
				{"id":"up-1","name":"uploads","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=uploads"},
				{"id":"cmd-1","name":"commands","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=commands"}
			]`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	p := testDiscordProvider(t, srv)
	err := p.Notify(context.Background(), provider.ActivityEvent{
		AgentID:    "agent-5",
		Hostname:   "host-a",
		OccurredAt: time.Now(),
		Status:     provider.StatusOnline,
	})
	if err == nil {
		t.Fatal("expected notify error for non-2xx response")
	}
}

// channelCreateHandler manages state for the channel create/reuse test.
type channelCreateHandler struct {
	mu            sync.Mutex
	nextID        int
	channels      []map[string]any
	createCalls   int
	msgPostChanID string
}

func (h *channelCreateHandler) newID() string {
	h.nextID++
	return fmt.Sprintf("%d", h.nextID)
}

func (h *channelCreateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == guildChannelsPath && r.Method == http.MethodGet:
		h.mu.Lock()
		defer h.mu.Unlock()
		_ = json.NewEncoder(w).Encode(h.channels)

	case r.URL.Path == guildChannelsPath && r.Method == http.MethodPost:
		var req struct {
			Name     string `json:"name"`
			Type     int    `json:"type"`
			ParentID string `json:"parent_id"`
			Topic    string `json:"topic"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		h.mu.Lock()
		h.createCalls++
		id := h.newID()
		created := map[string]any{
			"id":        id,
			"name":      req.Name,
			"type":      req.Type,
			"parent_id": req.ParentID,
			"topic":     req.Topic,
		}
		h.channels = append(h.channels, created)
		h.mu.Unlock()

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(created)

	case strings.HasPrefix(r.URL.Path, "/channels/") && strings.HasSuffix(r.URL.Path, "/messages"):
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 3 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		h.mu.Lock()
		h.msgPostChanID = parts[1]
		h.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg-1"}`))

	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func verifyCreatedChannels(t *testing.T, channels []map[string]any) {
	t.Helper()
	hasLogs := false
	hasUploads := false
	hasCommands := false
	for _, ch := range channels {
		name, _ := ch["name"].(string)
		switch name {
		case "logs":
			hasLogs = true
		case "uploads":
			hasUploads = true
		case "commands":
			hasCommands = true
		}
	}
	if !hasLogs {
		t.Fatal("expected logs channel to be created")
	}
	if !hasUploads {
		t.Fatal("expected uploads channel to be created")
	}
	if !hasCommands {
		t.Fatal("expected commands channel to be created")
	}
}

func TestProviderNotifyCreatesAndReusesGuildChannels(t *testing.T) {
	handler := &channelCreateHandler{nextID: 100}
	srv := httptest.NewServer(handler)
	defer srv.Close()

	p := testDiscordProvider(t, srv)

	event := provider.ActivityEvent{
		AgentID:    "702d64b8-82f0-443f-a3a2-088d396582be",
		Hostname:   "lukas",
		OccurredAt: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
		Status:     provider.StatusOnline,
		Source:     "heartbeat",
	}

	if err := p.Notify(context.Background(), event); err != nil {
		t.Fatalf("notify first call failed: %v", err)
	}
	firstChanID := handler.msgPostChanID

	if err := p.Notify(context.Background(), event); err != nil {
		t.Fatalf("notify second call failed: %v", err)
	}

	handler.mu.Lock()
	defer handler.mu.Unlock()

	if handler.createCalls != 4 {
		t.Fatalf("expected 4 create calls (category + 3 channels), got %d", handler.createCalls)
	}
	if firstChanID == "" {
		t.Fatal("expected non-empty channel id for message post")
	}
	if handler.msgPostChanID != firstChanID {
		t.Fatalf("expected channel reuse, got %q and %q", firstChanID, handler.msgPostChanID)
	}

	verifyCreatedChannels(t, handler.channels)
}

func TestProviderPreflightCheckSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bot abc" {
			t.Fatalf("unexpected auth header: %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/users/@me":
			w.WriteHeader(http.StatusOK)
		case "/guilds/g-1":
			w.WriteHeader(http.StatusOK)
		case guildChannelsPath:
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p := testDiscordProvider(t, srv)
	if err := p.PreflightCheck(context.Background()); err != nil {
		t.Fatalf("preflight should pass: %v", err)
	}
}

func TestProviderPreflightCheckMissingAccess(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if r.URL.Path == "/users/@me" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/guilds/g-1" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Missing Access","code":50001}`))
			return
		}
		t.Fatalf("unexpected path: %s", r.URL.Path)
	}))
	defer srv.Close()

	p := testDiscordProvider(t, srv)
	err := p.PreflightCheck(context.Background())
	if err == nil {
		t.Fatal("expected preflight error")
	}
	if !strings.Contains(err.Error(), "guild access check") {
		t.Fatalf("expected guild access error, got: %v", err)
	}
}

func TestProviderReportEntryNotImplemented(t *testing.T) {
	p, err := New(Config{BotToken: "abc", GuildID: "g-1"}, nil)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, ok := any(p).(provider.EntryReporter); ok {
		t.Fatal("discord provider should not implement EntryReporter")
	}
}

func TestProviderRespondInteraction(t *testing.T) {
	var capturedPath string
	var capturedPayload map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&capturedPayload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"type": 4}`))
	}))
	defer srv.Close()

	p := testDiscordProvider(t, srv)
	embed := BuildHelpEmbed("test-trace-123")
	err := p.RespondInteraction(context.Background(), "interaction-1", "token-abc", embed)
	if err != nil {
		t.Fatalf("respond interaction failed: %v", err)
	}

	expectedPath := "/interactions/interaction-1/token-abc/callback"
	if capturedPath != expectedPath {
		t.Fatalf("expected path %q, got %q", expectedPath, capturedPath)
	}

ResponseType, ok := capturedPayload["type"].(float64)
	if !ok || ResponseType != 4 {
		t.Fatalf("expected type 4, got %v", capturedPayload["type"])
	}

	data, ok := capturedPayload["data"].(map[string]any)
	if !ok {
		t.Fatal("expected data field in payload")
	}
	embeds, ok := data["embeds"].([]any)
	if !ok || len(embeds) != 1 {
		t.Fatalf("expected 1 embed, got %v", data["embeds"])
	}
}

func TestProviderDeleteChannel(t *testing.T) {
	var capturedPath string
	var capturedMethod string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p := testDiscordProvider(t, srv)
	err := p.DeleteChannel(context.Background(), "channel-123")
	if err != nil {
		t.Fatalf("delete channel failed: %v", err)
	}

	expectedPath := "/channels/channel-123"
	if capturedPath != expectedPath {
		t.Fatalf("expected path %q, got %q", expectedPath, capturedPath)
	}
	if capturedMethod != http.MethodDelete {
		t.Fatalf("expected DELETE method, got %s", capturedMethod)
	}
}

func TestProviderGetBotUser(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/@me" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"bot-123","username":"TestBot"}`))
	}))
	defer srv.Close()

	p := testDiscordProvider(t, srv)
	id, username, err := p.GetBotUser(context.Background())
	if err != nil {
		t.Fatalf("get bot user failed: %v", err)
	}
	if id != "bot-123" {
		t.Fatalf("expected bot id 'bot-123', got %q", id)
	}
	if username != "TestBot" {
		t.Fatalf("expected username 'TestBot', got %q", username)
	}
}

func TestProviderAllChannelSets(t *testing.T) {
	p, err := New(Config{BotToken: "abc", GuildID: "g-1"}, nil)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	p.mu.Lock()
	p.agentChannels["agent-1"] = channelSet{
		CategoryID: "cat-1",
		LogsID:     "logs-1",
		UploadsID:  "up-1",
		CommandsID: "cmd-1",
	}
	p.agentChannels["agent-2"] = channelSet{
		CategoryID: "cat-2",
		LogsID:     "logs-2",
		UploadsID:  "up-2",
		CommandsID: "cmd-2",
	}
	p.mu.Unlock()

	sets := p.AllChannelSets()
	if len(sets) != 2 {
		t.Fatalf("expected 2 channel sets, got %d", len(sets))
	}

	set1, ok := sets["agent-1"]
	if !ok {
		t.Fatal("expected agent-1 in channel sets")
	}
	if set1.CategoryID != "cat-1" || set1.LogsID != "logs-1" {
		t.Fatalf("unexpected agent-1 channel set: %+v", set1)
	}
}

func TestProviderRegisterGuildCommands(t *testing.T) {
	var createCalled bool
	var updateCalled bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/users/@me":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"app-123","username":"Bot"}`))
		case r.URL.Path == "/applications/app-123/guilds/g-1/commands" && r.Method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":"existing-1","name":"help"}]`))
		case r.URL.Path == "/applications/app-123/guilds/g-1/commands" && r.Method == http.MethodPost:
			createCalled = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-1","name":"ping"}`))
		case r.URL.Path == "/applications/app-123/guilds/g-1/commands/existing-1" && r.Method == http.MethodPatch:
			updateCalled = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"existing-1","name":"help"}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p := testDiscordProvider(t, srv)
	commands := []guildCommand{
		{Name: "help", Description: "Show help"},
		{Name: "ping", Description: "Check latency"},
	}

	err := p.RegisterGuildCommands(context.Background(), commands)
	if err != nil {
		t.Fatalf("register commands failed: %v", err)
	}

	if !updateCalled {
		t.Fatal("expected help command to be updated")
	}
	if !createCalled {
		t.Fatal("expected ping command to be created")
	}
}

func TestBuildHelpEmbed(t *testing.T) {
	embed := BuildHelpEmbed("trace-abc-123")

	if embed["title"] != "Available Commands" {
		t.Fatalf("expected title 'Available Commands', got %v", embed["title"])
	}
	if embed["color"] != embedColorBlue {
		t.Fatalf("expected blue color, got %v", embed["color"])
	}

	fields, ok := embed["fields"].([]map[string]any)
	if !ok || len(fields) != 5 {
		t.Fatalf("expected 5 fields, got %v", embed["fields"])
	}

	footer, ok := embed["footer"].(map[string]any)
	if !ok {
		t.Fatal("expected footer")
	}
	footerText, ok := footer["text"].(string)
	if !ok || !strings.Contains(footerText, "trace-ab") {
		t.Fatalf("expected censored trace in footer, got %v", footer["text"])
	}
}

func TestBuildStatusEmbed(t *testing.T) {
	snapshot := provider.AgentSnapshot{
		AgentID:  "agent-1",
		Hostname: "host-a",
		LastSeen: time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		IsOnline: true,
	}

	embed := BuildStatusEmbed(snapshot, "trace-123")

	if embed["title"] != "Agent Status" {
		t.Fatalf("expected title 'Agent Status', got %v", embed["title"])
	}
	if embed["color"] != embedColorGreen {
		t.Fatalf("expected green color for online, got %v", embed["color"])
	}

	desc, ok := embed["description"].(string)
	if !ok || !strings.Contains(desc, "host-a") || !strings.Contains(desc, "online") {
		t.Fatalf("unexpected description: %v", embed["description"])
	}
}

func TestBuildPingEmbed(t *testing.T) {
	embed := BuildPingEmbed(50*time.Millisecond, true, nil, "trace-123")

	if embed["title"] != "Pong!" {
		t.Fatalf("expected title 'Pong!', got %v", embed["title"])
	}

	fields, ok := embed["fields"].([]map[string]any)
	if !ok || len(fields) != 1 {
		t.Fatalf("expected 1 field (bot latency only), got %v", embed["fields"])
	}
}

func TestCensorTrace(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"abc123def456", "abc123de****"},
		{"short", "short****"},
		{"", "n/a"},
		{"  ", "n/a"},
		{"12345678", "12345678****"},
	}

	for _, tt := range tests {
		result := censorTrace(tt.input)
		if result != tt.expected {
			t.Errorf("censorTrace(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
