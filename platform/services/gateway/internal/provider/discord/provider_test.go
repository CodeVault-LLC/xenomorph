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
