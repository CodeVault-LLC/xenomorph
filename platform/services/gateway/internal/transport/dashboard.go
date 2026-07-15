package transport

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/activity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/fileworkspace"
)

const dashboardReadHeaderTimeout time.Duration = 10 * time.Second

const (
	clientStreamInterval     time.Duration = 250 * time.Millisecond
	screenFrameTimeout       time.Duration = 15 * time.Second
	dashboardShutdownTimeout time.Duration = 5 * time.Second
	liveScreenFrameRateCap   uint64        = 60
	liveScreenQuality        int           = 70
	maxLiveViewers           int           = 25
)

const defaultScreenContentType = "image/png"

// ClientDirectory is the read-only presence view required by the browser
// dashboard. The activity.Monitor implements this interface.
type ClientDirectory interface {
	ListClients() []activity.ClientSnapshot
}

// AgentLogDirectory is the read-only recent log view required by the browser
// dashboard. The transport AgentLogStore implements this interface.
type AgentLogDirectory interface {
	List(agentID string, limit int) []AgentLogEntry
}

// TerminalDirectory is the dashboard terminal read model required by the
// browser API. It stores gateway-authored command IDs and authenticated agent
// responses in memory.
type TerminalDirectory interface {
	CreateSession(agentID, label, shell, workingDirectory string) TerminalSession
	ListSessions(agentID string) []TerminalSession
	Session(agentID, sessionID string) (TerminalSession, bool)
	DeleteSession(agentID, sessionID string) bool
	AppendQueued(entry TerminalEntry)
	ListEntries(agentID, sessionID string, limit int) []TerminalEntry
}

// DashboardRuntime contains the read and command dependencies needed by the
// browser dashboard API. The dashboard does not own agent authentication or
// event ingestion.
type DashboardRuntime struct {
	Directory       ClientDirectory
	CommandQueue    *command.Queue
	Screens         *ScreenStore
	Sessions        *ScreenSessions
	Logs            AgentLogDirectory
	Terminals       TerminalDirectory
	Files           *fileworkspace.Service
	FileOperatorID  string
	DashboardOrigin string
	Readiness       readinessProvider
}

type healthResponse struct {
	Status string `json:"status"`
}

// RunDashboard starts the administrative browser API listener. This listener
// is separate from the mTLS gateway listener so browser access does not weaken
// the authenticated agent ingestion boundary. Operator authentication remains
// a release-blocking responsibility of this listener.
func RunDashboard(ctx context.Context, addr, certPath string, runtime DashboardRuntime) error {
	mux := http.NewServeMux()
	registerFileRoutes(mux, runtime)
	registerHealthRoutes(mux, runtime.Readiness)
	registerDashboardRoutes(mux, runtime)

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: dashboardReadHeaderTimeout,
		TLSConfig: &tls.Config{
			MinVersion:       tls.VersionTLS13,
			CurvePreferences: []tls.CurveID{tls.CurveP384},
		},
	}

	go func(shutdownContext context.Context) {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(shutdownContext, dashboardShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(ctx, "dashboard shutdown failed", "error", err)
		}
	}(context.WithoutCancel(ctx))

	certificate := filepath.Join(certPath, "server.crt")
	privateKey := filepath.Join(certPath, "server.key")
	if err := server.ListenAndServeTLS(certificate, privateKey); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func registerHealthRoutes(mux *http.ServeMux, readiness readinessProvider) {
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeDashboardJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	})
	mux.HandleFunc("GET /api/health/live", func(w http.ResponseWriter, _ *http.Request) {
		writeDashboardJSON(w, http.StatusOK, healthResponse{Status: "live"})
	})
	mux.HandleFunc("GET /api/health/ready", func(w http.ResponseWriter, _ *http.Request) {
		if readiness == nil || readiness.Ready() != nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "unready"})
			return
		}
		writeDashboardJSON(w, http.StatusOK, healthResponse{Status: "ready"})
	})
}

type terminalSessionRequest struct {
	Label            string `json:"label"`
	Shell            string `json:"shell"`
	WorkingDirectory string `json:"working_directory"`
}

type terminalCommandRequest struct {
	Command          string `json:"command"`
	WorkingDirectory string `json:"working_directory"`
}

func dashboardClients(directory ClientDirectory) []activity.ClientSnapshot {
	clients := []activity.ClientSnapshot{}
	if directory != nil {
		clients = directory.ListClients()
	}

	sort.Slice(clients, func(i, j int) bool {
		if clients[i].IsOnline != clients[j].IsOnline {
			return clients[i].IsOnline
		}
		return clients[i].LastSeen.After(clients[j].LastSeen)
	})

	return clients
}

func dashboardLogs(directory AgentLogDirectory, agentID string) []AgentLogEntry {
	if directory == nil {
		return []AgentLogEntry{}
	}
	return directory.List(agentID, maxLogEntriesPerAgent)
}

func streamDashboardEvents(w http.ResponseWriter, r *http.Request, interval time.Duration, snapshot func() any) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeDashboardJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	writeDashboardSSE(w, "snapshot", snapshot())
	flusher.Flush()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			writeDashboardSSE(w, "snapshot", snapshot())
			flusher.Flush()
		}
	}
}

func screenFramePayload(agentID string, frame ScreenFrame) map[string]any {
	return map[string]any{
		"has_frame":    true,
		"agent_id":     agentID,
		"command_id":   frame.CommandID,
		"captured_at":  frame.CapturedAt,
		"content_type": screenContentType(frame),
		"image_url":    "/api/clients/" + agentID + "/screen/latest.png",
	}
}

func screenContentType(frame ScreenFrame) string {
	if frame.ContentType == "" {
		return defaultScreenContentType
	}
	return frame.ContentType
}

func enqueueScreenStreamCommand(queue *command.Queue, sessions *ScreenSessions, agentID string, start bool) {
	if queue == nil || sessions == nil {
		return
	}

	cmdType := string(CommandTypeStopScreenStream)
	reason := "Live screen media stream stopped after last dashboard viewer disconnected"
	payload := json.RawMessage(nil)
	var authorization MediaGenerationAuthorization
	if start {
		generationID := uuid.New()
		authorization = MediaGenerationAuthorization{
			GenerationID:      generationID,
			FrameRateCap:      liveScreenFrameRateCap,
			MaximumFrameBytes: maxScreenMediaFrameBytes,
		}
		if !sessions.AuthorizeMediaGeneration(agentID, authorization) {
			slog.Error("screen media generation authorization failed")
			return
		}
		cmdType = string(CommandTypeStartScreenStream)
		reason = "Live screen media stream requested from website dashboard"
		payload = json.RawMessage(fmt.Sprintf(
			`{"fps":%d,"quality":%d,"generation_id":%q,"maximum_frame_bytes":%d}`,
			liveScreenFrameRateCap, liveScreenQuality, generationID.String(), maxScreenMediaFrameBytes,
		))
	} else {
		sessions.RevokeMediaGeneration(agentID)
	}

	if err := queue.Enqueue(agentID, &command.Envelope{
		Type:        cmdType,
		Payload:     payload,
		RequestedBy: "website",
		Reason:      reason,
	}); err != nil {
		if start {
			sessions.RevokeMediaGeneration(agentID)
		}
		slog.Error("screen stream command enqueue failed", "error", err)
	}
}

func latestScreen(store *ScreenStore, agentID string) (ScreenFrame, bool) {
	if store == nil {
		return ScreenFrame{}, false
	}
	return store.Latest(agentID)
}

func findClient(directory ClientDirectory, agentID string) (activity.ClientSnapshot, bool) {
	if directory == nil {
		return activity.ClientSnapshot{}, false
	}

	for _, client := range directory.ListClients() {
		if client.AgentID == agentID {
			return client, true
		}
	}
	return activity.ClientSnapshot{}, false
}

func writeDashboardJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeDashboardSSE(w http.ResponseWriter, event string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		data = []byte(`{"error":"encode_failed"}`)
	}
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
}
