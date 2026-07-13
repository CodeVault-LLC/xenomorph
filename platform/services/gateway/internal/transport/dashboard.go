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

	"github.com/gorilla/websocket"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/activity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/fileworkspace"
)

const dashboardReadHeaderTimeout time.Duration = 10 * time.Second

const (
	clientStreamInterval time.Duration = 250 * time.Millisecond
	screenFrameTimeout   time.Duration = 15 * time.Second
	liveScreenFPS        int           = 60
	liveScreenQuality    int           = 70
	maxLiveViewers       int           = 25
)

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
}

// RunDashboard starts the read-only browser API listener. This listener is
// separate from the mTLS gateway listener so browser access does not weaken
// the authenticated agent ingestion boundary.
func RunDashboard(ctx context.Context, addr, certPath string, runtime DashboardRuntime) error {
	mux := http.NewServeMux()
	registerFileRoutes(mux, runtime)

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeDashboardJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/clients", func(w http.ResponseWriter, _ *http.Request) {
		writeDashboardJSON(w, http.StatusOK, map[string]any{
			"clients": dashboardClients(runtime.Directory),
		})
	})

	mux.HandleFunc("GET /api/clients/stream", func(w http.ResponseWriter, r *http.Request) {
		streamDashboardEvents(w, r, clientStreamInterval, func() any {
			return map[string]any{"clients": dashboardClients(runtime.Directory)}
		})
	})

	mux.HandleFunc("GET /api/clients/{agentID}/logs", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		if _, ok := findClient(runtime.Directory, agentID); !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
			return
		}

		writeDashboardJSON(w, http.StatusOK, map[string]any{
			"agent_id": agentID,
			"logs":     dashboardLogs(runtime.Logs, agentID),
		})
	})

	mux.HandleFunc("GET /api/clients/{agentID}/terminal/sessions", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		if _, ok := findClient(runtime.Directory, agentID); !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
			return
		}
		if runtime.Terminals == nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal store unavailable"})
			return
		}

		writeDashboardJSON(w, http.StatusOK, map[string]any{
			"agent_id": agentID,
			"sessions": runtime.Terminals.ListSessions(agentID),
		})
	})

	mux.HandleFunc("POST /api/clients/{agentID}/terminal/sessions", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		client, ok := findClient(runtime.Directory, agentID)
		if !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
			return
		}
		if runtime.Terminals == nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal store unavailable"})
			return
		}

		var req terminalSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid terminal session request"})
			return
		}
		shell := req.Shell
		if shell == "" {
			shell = defaultTerminalShell(client.OSVersion)
		}
		session := runtime.Terminals.CreateSession(agentID, req.Label, shell, req.WorkingDirectory)
		writeDashboardJSON(w, http.StatusCreated, map[string]any{
			"agent_id": agentID,
			"session":  session,
		})
	})

	mux.HandleFunc("GET /api/clients/{agentID}/terminal/sessions/{sessionID}/entries", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		sessionID := r.PathValue("sessionID")
		if _, ok := findClient(runtime.Directory, agentID); !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
			return
		}
		if runtime.Terminals == nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal store unavailable"})
			return
		}
		if _, ok := runtime.Terminals.Session(agentID, sessionID); !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown terminal session"})
			return
		}

		writeDashboardJSON(w, http.StatusOK, map[string]any{
			"agent_id":   agentID,
			"session_id": sessionID,
			"entries":    runtime.Terminals.ListEntries(agentID, sessionID, maxTerminalEntriesPerAgent),
		})
	})

	mux.HandleFunc("DELETE /api/clients/{agentID}/terminal/sessions/{sessionID}", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		sessionID := r.PathValue("sessionID")
		if _, ok := findClient(runtime.Directory, agentID); !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
			return
		}
		if runtime.Terminals == nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal store unavailable"})
			return
		}
		if !runtime.Terminals.DeleteSession(agentID, sessionID) {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown terminal session"})
			return
		}

		writeDashboardJSON(w, http.StatusOK, map[string]any{
			"status":     "deleted",
			"agent_id":   agentID,
			"session_id": sessionID,
		})
	})

	mux.HandleFunc("POST /api/clients/{agentID}/terminal/sessions/{sessionID}/commands", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		sessionID := r.PathValue("sessionID")
		client, ok := findClient(runtime.Directory, agentID)
		if !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
			return
		}
		if !client.IsOnline {
			writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "agent is offline"})
			return
		}
		if runtime.CommandQueue == nil || runtime.Terminals == nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal command path unavailable"})
			return
		}
		session, ok := runtime.Terminals.Session(agentID, sessionID)
		if !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown terminal session"})
			return
		}

		var req terminalCommandRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid terminal command request"})
			return
		}
		req.Command = clampText(req.Command, maxLogMessageLen)
		if req.Command == "" {
			writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
			return
		}

		workingDirectory := session.WorkingDirectory
		if req.WorkingDirectory != "" {
			workingDirectory = clampText(req.WorkingDirectory, maxPathLen)
		}
		payload, err := json.Marshal(map[string]string{
			"session_id":        sessionID,
			"command":           req.Command,
			"shell":             session.Shell,
			"working_directory": workingDirectory,
		})
		if err != nil {
			writeDashboardJSON(w, http.StatusInternalServerError, map[string]string{"error": "terminal payload encode failed"})
			return
		}

		cmd := &command.Envelope{
			Type:        string(CommandTypeTerminalRun),
			Payload:     payload,
			RequestedBy: "website",
			Reason:      "Terminal command requested from website dashboard",
		}
		if err := runtime.CommandQueue.Enqueue(agentID, cmd); err != nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal command queue unavailable"})
			return
		}
		entry := TerminalEntry{
			AgentID:          agentID,
			SessionID:        sessionID,
			CommandID:        cmd.CommandID,
			Command:          req.Command,
			Shell:            session.Shell,
			WorkingDirectory: workingDirectory,
			Status:           "queued",
			SubmittedAt:      time.Now().UTC(),
		}
		runtime.Terminals.AppendQueued(entry)

		writeDashboardJSON(w, http.StatusAccepted, map[string]any{
			"status":     "queued",
			"agent_id":   agentID,
			"session_id": sessionID,
			"command_id": cmd.CommandID,
			"entry":      entry,
		})
	})

	mux.HandleFunc("POST /api/clients/{agentID}/screen/request", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		client, ok := findClient(runtime.Directory, agentID)
		if !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
			return
		}
		if !client.IsOnline {
			writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "agent is offline"})
			return
		}
		if runtime.CommandQueue == nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "command queue unavailable"})
			return
		}

		cmd := &command.Envelope{
			Type:        string(CommandTypeRequestScreenshot),
			RequestedBy: "website",
			Reason:      "Live screen frame requested from website dashboard",
		}
		if err := runtime.CommandQueue.Enqueue(agentID, cmd); err != nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "screen command queue unavailable"})
			return
		}
		writeDashboardJSON(w, http.StatusAccepted, map[string]any{
			"status":     "queued",
			"command_id": cmd.CommandID,
			"agent_id":   agentID,
		})
	})

	mux.HandleFunc("GET /api/clients/{agentID}/screen/latest", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		frame, ok := latestScreen(runtime.Screens, agentID)
		if !ok {
			writeDashboardJSON(w, http.StatusOK, map[string]any{
				"has_frame": false,
				"agent_id":  agentID,
			})
			return
		}

		contentType := frame.ContentType
		if contentType == "" {
			contentType = "image/png"
		}

		writeDashboardJSON(w, http.StatusOK, map[string]any{
			"has_frame":    true,
			"agent_id":     agentID,
			"command_id":   frame.CommandID,
			"captured_at":  frame.CapturedAt,
			"content_type": contentType,
			"image_url":    "/api/clients/" + agentID + "/screen/latest.png",
		})
	})

	mux.HandleFunc("GET /api/clients/{agentID}/screen/latest.png", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		frame, ok := latestScreen(runtime.Screens, agentID)
		if !ok {
			http.NotFound(w, r)
			return
		}

		contentType := frame.ContentType
		if contentType == "" {
			contentType = "image/png"
		}

		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(frame.Content)
	})

	mux.HandleFunc("GET /api/clients/{agentID}/screen/live", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		client, ok := findClient(runtime.Directory, agentID)
		if !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
			return
		}
		if !client.IsOnline {
			writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "agent is offline"})
			return
		}
		if runtime.CommandQueue == nil || runtime.Screens == nil || runtime.Sessions == nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "screen stream unavailable"})
			return
		}

		agentViewers, totalViewers := runtime.Sessions.BeginViewer(agentID)
		if totalViewers > maxLiveViewers {
			runtime.Sessions.EndViewer(agentID)
			writeDashboardJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many live screen viewers"})
			return
		}
		defer func() {
			remainingAgentViewers, _ := runtime.Sessions.EndViewer(agentID)
			if remainingAgentViewers == 0 {
				enqueueScreenStreamCommand(runtime.CommandQueue, agentID, false)
			}
		}()

		if agentViewers == 1 {
			enqueueScreenStreamCommand(runtime.CommandQueue, agentID, true)
		}

		upgrader := websocket.Upgrader{CheckOrigin: func(request *http.Request) bool {
			return request.Header.Get("Origin") == runtime.DashboardOrigin
		}}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()

		var after time.Time
		if frame, ok := runtime.Screens.Latest(agentID); ok {
			after = frame.CapturedAt
			if err := conn.WriteMessage(websocket.BinaryMessage, frame.Content); err != nil {
				return
			}
		}

		for r.Context().Err() == nil {
			frame, ok := runtime.Screens.WaitLatestAfter(r.Context(), agentID, after)
			if !ok {
				return
			}
			after = frame.CapturedAt
			if err := conn.WriteMessage(websocket.BinaryMessage, frame.Content); err != nil {
				return
			}
		}
	})

	mux.HandleFunc("GET /api/clients/{agentID}/screen/stream", func(w http.ResponseWriter, r *http.Request) {
		agentID := r.PathValue("agentID")
		client, ok := findClient(runtime.Directory, agentID)
		if !ok {
			writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
			return
		}
		if !client.IsOnline {
			writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "agent is offline"})
			return
		}
		if runtime.CommandQueue == nil || runtime.Screens == nil {
			writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "screen stream unavailable"})
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			writeDashboardJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)

		var after time.Time
		if frame, ok := runtime.Screens.Latest(agentID); ok {
			after = frame.CapturedAt
			writeDashboardSSE(w, "frame", screenFramePayload(agentID, frame))
			flusher.Flush()
		}

		for r.Context().Err() == nil {
			cmd := &command.Envelope{
				Type:        string(CommandTypeRequestScreenshot),
				RequestedBy: "website",
				Reason:      "Live screen stream requested from website dashboard",
			}
			if err := runtime.CommandQueue.Enqueue(agentID, cmd); err != nil {
				writeDashboardSSE(w, "error", map[string]string{"error": "screen command queue unavailable"})
				flusher.Flush()
				return
			}

			waitCtx, cancel := context.WithTimeout(r.Context(), screenFrameTimeout)
			frame, ok := runtime.Screens.WaitLatestAfter(waitCtx, agentID, after)
			cancel()
			if !ok {
				writeDashboardSSE(w, "waiting", map[string]any{
					"agent_id": agentID,
					"status":   "waiting_for_frame",
				})
				flusher.Flush()
				continue
			}

			after = frame.CapturedAt
			writeDashboardSSE(w, "frame", screenFramePayload(agentID, frame))
			flusher.Flush()
		}
	})

	server := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: dashboardReadHeaderTimeout,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS13},
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(ctx, "dashboard shutdown failed", "error", err)
		}
	}()

	certificate := filepath.Join(certPath, "server.crt")
	privateKey := filepath.Join(certPath, "server.key")
	if err := server.ListenAndServeTLS(certificate, privateKey); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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
	contentType := frame.ContentType
	if contentType == "" {
		contentType = "image/png"
	}

	return map[string]any{
		"has_frame":    true,
		"agent_id":     agentID,
		"command_id":   frame.CommandID,
		"captured_at":  frame.CapturedAt,
		"content_type": contentType,
		"image_url":    "/api/clients/" + agentID + "/screen/latest.png",
	}
}

func enqueueScreenStreamCommand(queue *command.Queue, agentID string, start bool) {
	if queue == nil {
		return
	}

	cmdType := string(CommandTypeStopScreenStream)
	reason := "Live screen media stream stopped after last dashboard viewer disconnected"
	payload := json.RawMessage(nil)
	if start {
		cmdType = string(CommandTypeStartScreenStream)
		reason = "Live screen media stream requested from website dashboard"
		payload = json.RawMessage(fmt.Sprintf(`{"fps":%d,"quality":%d}`, liveScreenFPS, liveScreenQuality))
	}

	if err := queue.Enqueue(agentID, &command.Envelope{
		Type:        cmdType,
		Payload:     payload,
		RequestedBy: "website",
		Reason:      reason,
	}); err != nil {
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
