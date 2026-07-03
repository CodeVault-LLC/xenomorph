package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/gorilla/websocket"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/activity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
)

const dashboardReadHeaderTimeout = 10 * time.Second
const (
	clientStreamInterval = 250 * time.Millisecond
	screenFrameTimeout   = 15 * time.Second
	liveScreenFPS        = 60
	liveScreenQuality    = 70
	maxLiveViewers       = 25
)

var dashboardScreenUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

// ClientDirectory is the read-only presence view required by the browser
// dashboard. The activity.Monitor implements this interface.
type ClientDirectory interface {
	ListClients() []activity.ClientSnapshot
}

// DashboardRuntime contains the read and command dependencies needed by the
// browser dashboard API. The dashboard does not own agent authentication or
// event ingestion.
type DashboardRuntime struct {
	Directory    ClientDirectory
	CommandQueue *command.Queue
	Screens      *ScreenStore
	Sessions     *ScreenSessions
}

// RunDashboard starts the read-only browser API listener. This listener is
// separate from the mTLS gateway listener so browser access does not weaken
// the authenticated agent ingestion boundary.
func RunDashboard(ctx context.Context, addr string, runtime DashboardRuntime) error {
	mux := http.NewServeMux()

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
			Type:        "support.request_screenshot",
			RequestedBy: "website",
			Reason:      "Live screen frame requested from website dashboard",
		}
		runtime.CommandQueue.Enqueue(agentID, cmd)
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

		conn, err := dashboardScreenUpgrader.Upgrade(w, r, nil)
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
				Type:        "support.request_screenshot",
				RequestedBy: "website",
				Reason:      "Live screen stream requested from website dashboard",
			}
			runtime.CommandQueue.Enqueue(agentID, cmd)

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
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.ErrorContext(ctx, "dashboard shutdown failed", "error", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
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

	cmdType := "support.stop_screen_stream"
	reason := "Live screen media stream stopped after last dashboard viewer disconnected"
	payload := json.RawMessage(nil)
	if start {
		cmdType = "support.start_screen_stream"
		reason = "Live screen media stream requested from website dashboard"
		payload = json.RawMessage(fmt.Sprintf(`{"fps":%d,"quality":%d}`, liveScreenFPS, liveScreenQuality))
	}

	queue.Enqueue(agentID, &command.Envelope{
		Type:        cmdType,
		Payload:     payload,
		RequestedBy: "website",
		Reason:      reason,
	})
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
