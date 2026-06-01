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
	type embedPayload struct {
		Embeds []map[string]any `json:"embeds"`
	}

	var gotAuth string
	var gotPayload embedPayload
	var gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/guilds/g-1/channels" && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[
				{"id":"cat-1","name":"client-host-a-agent-5","type":4,"parent_id":null,"topic":""},
				{"id":"logs-1","name":"logs","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=logs"},
				{"id":"up-1","name":"uploads","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=uploads"},
				{"id":"cmd-1","name":"commands","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=commands"}
			]`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/channels/") && strings.HasSuffix(r.URL.Path, "/messages") {
			gotPath = r.URL.Path
			gotAuth = r.Header.Get("Authorization")
			if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	p, err := New(Config{BotToken: "abc", GuildID: "g-1", APIBaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	err = p.Notify(context.Background(), provider.ActivityEvent{
		AgentID:    "agent-5",
		Hostname:   "host-a",
		OccurredAt: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
		Status:     provider.StatusOnline,
		Source:     "heartbeat",
	})
	if err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	if gotAuth != "Bot abc" {
		t.Fatalf("unexpected auth header: %q", gotAuth)
	}
	if !strings.Contains(gotPath, "/messages") {
		t.Fatalf("expected messages endpoint, got: %s", gotPath)
	}
	if len(gotPayload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(gotPayload.Embeds))
	}

	embed := gotPayload.Embeds[0]
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
	type embedPayload struct {
		Embeds []map[string]any `json:"embeds"`
	}

	var gotPayload embedPayload

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/guilds/g-1/channels" && r.Method == http.MethodGet {
			_, _ = w.Write([]byte(`[
				{"id":"cat-1","name":"client-host-a-agent-5","type":4,"parent_id":null,"topic":""},
				{"id":"logs-1","name":"logs","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=logs"},
				{"id":"up-1","name":"uploads","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=uploads"},
				{"id":"cmd-1","name":"commands","type":0,"parent_id":"cat-1","topic":"xenomorph agent_id=agent-5 kind=commands"}
			]`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/channels/") && strings.HasSuffix(r.URL.Path, "/messages") {
			if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	p, err := New(Config{BotToken: "abc", GuildID: "g-1", APIBaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	err = p.Notify(context.Background(), provider.ActivityEvent{
		AgentID:    "agent-5",
		Hostname:   "host-a",
		OccurredAt: time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC),
		Status:     provider.StatusOffline,
		Source:     "heartbeat-timeout",
	})
	if err != nil {
		t.Fatalf("notify failed: %v", err)
	}

	if len(gotPayload.Embeds) != 1 {
		t.Fatalf("expected 1 embed, got %d", len(gotPayload.Embeds))
	}

	embed := gotPayload.Embeds[0]
	if embed["title"] != "Agent Disconnected" {
		t.Fatalf("expected title 'Agent Disconnected', got: %v", embed["title"])
	}
	if embed["color"] != float64(0xE74C3C) {
		t.Fatalf("expected red color, got: %v", embed["color"])
	}
}

func TestProviderNotifyReturnsErrorOnFailureStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/guilds/g-1/channels" && r.Method == http.MethodGet {
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

	p, err := New(Config{BotToken: "abc", GuildID: "g-1", APIBaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	err = p.Notify(context.Background(), provider.ActivityEvent{
		AgentID:    "agent-5",
		Hostname:   "host-a",
		OccurredAt: time.Now(),
		Status:     provider.StatusOnline,
	})
	if err == nil {
		t.Fatal("expected notify error for non-2xx response")
	}
}

func TestProviderNotifyCreatesAndReusesGuildChannels(t *testing.T) {
	type channelCreateRequest struct {
		Name     string `json:"name"`
		Type     int    `json:"type"`
		ParentID string `json:"parent_id"`
		Topic    string `json:"topic"`
	}

	var (
		mu            sync.Mutex
		nextID        = 100
		channels      = make([]map[string]any, 0)
		createCalls   int
		msgPostChanID string
	)

	newID := func() string {
		nextID++
		return fmt.Sprintf("%d", nextID)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/guilds/g-1/channels" && r.Method == http.MethodGet {
			mu.Lock()
			defer mu.Unlock()
			_ = json.NewEncoder(w).Encode(channels)
			return
		}
		if r.URL.Path == "/guilds/g-1/channels" && r.Method == http.MethodPost {
			var req channelCreateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode create request: %v", err)
			}

			mu.Lock()
			createCalls++
			id := newID()
			created := map[string]any{
				"id":        id,
				"name":      req.Name,
				"type":      req.Type,
				"parent_id": req.ParentID,
				"topic":     req.Topic,
			}
			channels = append(channels, created)
			mu.Unlock()

			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/channels/") && strings.HasSuffix(r.URL.Path, "/messages") {
			parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
			if len(parts) != 3 {
				t.Fatalf("unexpected message path: %s", r.URL.Path)
			}
			mu.Lock()
			msgPostChanID = parts[1]
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"msg-1"}`))
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	p, err := New(Config{BotToken: "abc", GuildID: "g-1", APIBaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

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
	firstChanID := msgPostChanID

	if err := p.Notify(context.Background(), event); err != nil {
		t.Fatalf("notify second call failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if createCalls != 4 {
		t.Fatalf("expected 4 create calls (category + 3 channels), got %d", createCalls)
	}
	if firstChanID == "" {
		t.Fatal("expected non-empty channel id for message post")
	}
	if msgPostChanID != firstChanID {
		t.Fatalf("expected channel reuse, got %q and %q", firstChanID, msgPostChanID)
	}

	hasLogs := false
	hasUploads := false
	hasCommands := false
	for _, ch := range channels {
		name, _ := ch["name"].(string)
		if name == "logs" {
			hasLogs = true
		}
		if name == "uploads" {
			hasUploads = true
		}
		if name == "commands" {
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
		case "/guilds/g-1/channels":
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	p, err := New(Config{BotToken: "abc", GuildID: "g-1", APIBaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

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

	p, err := New(Config{BotToken: "abc", GuildID: "g-1", APIBaseURL: srv.URL}, srv.Client())
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	err = p.PreflightCheck(context.Background())
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
