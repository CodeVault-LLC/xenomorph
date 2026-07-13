// Package transport owns the HTTP/mTLS transport layer for the gateway. It
// handles request authentication, agent identity extraction, event ingestion,
// command queue dispatching, and dashboard state delivery.
package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/activity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/fileworkspace"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/identity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/sdk"
	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
)

const (
	maxCommandIDLen      = 128
	maxTypeLen           = 64
	maxStatusLen         = 32
	maxHostnameLen       = 120
	maxReasonLen         = 512
	maxBrowserNameLen    = 80
	maxPathLen           = 260
	maxAppNameLen        = 120
	maxOSVersionLen      = 120
	maxLogLevelLen       = 16
	maxComponentLen      = 120
	maxLogMessageLen     = 2048
	maxTerminalOutputLen = 128 * 1024

	maxBrowsers = 32
	maxApps     = 200

	readHeaderTimeout        = 30 * time.Second
	commandPollTimeout       = 5 * time.Second
	maxScreenMediaFrameBytes = 10 << 20
)

var screenMediaUpgrader = websocket.Upgrader{
	CheckOrigin: func(request *http.Request) bool {
		return strings.TrimSpace(request.Header.Get("Origin")) == ""
	},
}

// Server owns the HTTP transport layer for the gateway. It handles mTLS
// termination, agent identity extraction, request routing, and event
// publishing to the NATS broker.
type Server struct {
	broker          *broker.NATS
	commandQueue    *command.Queue
	statusProvider  agentStatusProvider
	screenStore     *ScreenStore
	screenSessions  *ScreenSessions
	logStore        *AgentLogStore
	terminalStore   *TerminalStore
	fileWorkspace   *fileworkspace.Service
	fileOperatorID  string
	dashboardOrigin string
	engine          *gin.Engine

	seenMu     sync.Mutex
	seenAgents map[string]struct{}
}

// agentStatusProvider is the interface the Server requires for agent presence
// queries. The activity.Monitor implements this interface.
type agentStatusProvider interface {
	Snapshot(agentID string) (activity.AgentSnapshot, bool)
}

// NewServer constructs a Server with the given gateway dependencies.
func NewServer(b *broker.NATS, commandQueue *command.Queue, statusProvider agentStatusProvider) *Server {
	s := &Server{
		broker:         b,
		commandQueue:   commandQueue,
		statusProvider: statusProvider,
		screenStore:    NewScreenStore(),
		screenSessions: NewScreenSessions(),
		logStore:       NewAgentLogStore(maxLogEntriesPerAgent),
		terminalStore:  NewTerminalStore(),
		engine:         gin.Default(),
		seenAgents:     make(map[string]struct{}),
	}
	s.routes()
	return s
}

// routes registers all HTTP middleware and endpoints on the Gin engine.
//
// Middleware execution order:
//  1. traceMiddleware — injects trace_id from X-Trace-ID header or generates one.
//  2. mtlsMiddleware — authenticates the client via mTLS peer certificate.
//
// Endpoints:
//
//	POST /ingest/heartbeat   — accepts agent heartbeat payloads.
//	POST /ingest/entry       — accepts stage-2 onboarding reports.
//	POST /ingest/logs        — accepts client diagnostic log entries.
//	GET  /commands/next      — dequeues the next pending command for an agent.
//	POST /commands/result    — accepts command execution results.
//	GET  /screen/media       — accepts authenticated live screen media frames.
func (s *Server) routes() {
	s.engine.Use(s.traceMiddleware)
	s.engine.Use(s.mtlsMiddleware)

	s.engine.POST("/ingest/heartbeat", s.handleHeartbeat)
	s.engine.POST("/ingest/entry", s.handleEntry)
	s.engine.POST("/ingest/logs", s.handleLogEntry)
	s.engine.GET("/commands/next", s.handleNextCommand)
	s.engine.POST("/commands/result", s.handleCommandResult)
	s.engine.GET("/screen/media", s.handleScreenMedia)
	s.engine.PUT("/files/transfers/:transferID/chunks/:chunkIndex", s.handleAgentTransferChunkPut)
	s.engine.GET("/files/transfers/:transferID/chunks/:chunkIndex", s.handleAgentTransferChunkGet)
	s.engine.POST("/files/transfers/:transferID/finalize", s.handleAgentTransferFinalize)
}

// handleLogEntry processes an authenticated client diagnostic log entry.
//
// The agent identity and client IP are extracted from the mTLS session. Level,
// component, and message are client-authored payload fields and are normalized
// before publication and dashboard storage.
func (s *Server) handleLogEntry(c *gin.Context) {
	agentID := c.GetString("agent_id")
	if agentID == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "missing authenticated agent identity"})
		return
	}

	var req pb.LogEntry
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schema"})
		return
	}

	logEntry := normalizeLogEntry(&req)
	env := &pb.EventEnvelope{
		EventId:   uuid.New().String(),
		TraceId:   c.GetHeader("X-Trace-ID"),
		Timestamp: timestamppb.Now(),
		Security: &pb.SecurityContext{
			AgentId:         agentID,
			SessionId:       uuid.New().String(),
			ClientIp:        c.ClientIP(),
			IsAuthenticated: true,
		},
		Payload: &pb.EventEnvelope_LogEntry{LogEntry: logEntry},
	}

	subject := "sys.in.default." + agentID + ".logs"
	if err := s.broker.Publish(subject, env); err != nil {
		slog.ErrorContext(c.Request.Context(), "client log publish failed",
			"agent_id", agentID,
			"error", err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue event"})
		return
	}

	s.storeLogEnvelope(env)

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "event_id": env.EventId})
}

// traceMiddleware extracts the X-Trace-ID header from the incoming request
// and stores it in both the Gin context and the request context. When the
// header is absent, a new UUID is generated.
//
// The trace ID is set on the response header and automatically injected into
// all log records produced by the slog context handler.
func (s *Server) traceMiddleware(c *gin.Context) {
	traceID := c.GetHeader("X-Trace-ID")
	if traceID == "" {
		traceID = uuid.New().String()
	}
	c.Set("trace_id", traceID)
	c.Header("X-Trace-ID", traceID)
	c.Request = c.Request.WithContext(sdk.WithTraceID(c.Request.Context(), traceID))
	c.Next()
}

// mtlsMiddleware authenticates every request using the mTLS peer certificate.
// The agent identity is derived deterministically from the certificate
// fingerprint and stored in the Gin context for downstream handlers.
//
// The middleware aborts with HTTP 403 when:
//   - The TLS connection has no peer certificates (plain HTTP or no client cert).
//   - The peer certificate cannot be parsed into a valid agent identity.
//
// Downstream handlers access the authenticated identity through Gin context
// keys: "agent_id", "agent_cert_fingerprint_sha256", "agent_subject_cn".
func (s *Server) mtlsMiddleware(c *gin.Context) {
	if c.Request.TLS != nil && len(c.Request.TLS.PeerCertificates) > 0 {
		cert := c.Request.TLS.PeerCertificates[0]
		authenticatedAgent, err := identity.FromPeerCertificate(cert)
		if err != nil {
			slog.ErrorContext(c.Request.Context(), "invalid client certificate identity",
				"error", err,
			)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid client certificate identity"})
			return
		}

		c.Set("agent_id", authenticatedAgent.ID)
		c.Set("agent_cert_fingerprint_sha256", authenticatedAgent.FingerprintSHA256)
		c.Set("agent_subject_cn", authenticatedAgent.SubjectCommonName)
	} else {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "mTLS required"})
		return
	}
	c.Next()
}

// handleHeartbeat processes an authenticated heartbeat submission.
//
// The agent identity is extracted from the mTLS peer certificate (set by
// mtlsMiddleware). No client-supplied identity fields in the request body
// are trusted. The heartbeat is wrapped in a server-authored EventEnvelope
// with the authenticated agent ID and published to NATS on subject
// "sys.in.default.<agentID>.heartbeat".
//
// Expected request body: proto.Heartbeat JSON representation.
// Response: 202 Accepted with event_id and is_new_agent flags.
func (s *Server) handleHeartbeat(c *gin.Context) {
	agentID := c.GetString("agent_id")
	if agentID == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "missing authenticated agent identity"})
		return
	}

	var hb pb.Heartbeat
	if err := c.ShouldBindJSON(&hb); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schema"})
		return
	}

	env := &pb.EventEnvelope{
		EventId:   uuid.New().String(),
		TraceId:   c.GetHeader("X-Trace-ID"),
		Timestamp: timestamppb.Now(),
		Security: &pb.SecurityContext{
			AgentId:         agentID,
			SessionId:       uuid.New().String(),
			ClientIp:        c.ClientIP(),
			IsAuthenticated: true,
		},
		Payload: &pb.EventEnvelope_Heartbeat{Heartbeat: &hb},
	}

	subject := "sys.in.default." + agentID + ".heartbeat"
	if err := s.broker.Publish(subject, env); err != nil {
		slog.ErrorContext(c.Request.Context(), "heartbeat publish failed",
			"agent_id", agentID,
			"error", err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue event"})
		return
	}

	_, existed := s.markSeen(agentID)

	c.JSON(http.StatusAccepted, gin.H{
		"status":       "accepted",
		"event_id":     env.EventId,
		"is_new_agent": !existed,
	})
}

// entryBrowser is the JSON shape for a browser entry in the onboarding report.
type entryBrowser struct {
	Name       string `json:"name"`
	BinaryPath string `json:"binary_path"`
	ProfileDir string `json:"profile_dir"`
}

// entryRequest is the JSON shape for the stage-2 onboarding submission.
type entryRequest struct {
	Hostname              string         `json:"hostname" binding:"required"`
	OSVersion             string         `json:"os_version"`
	IsNewAgent            bool           `json:"is_new_agent"`
	Browsers              []entryBrowser `json:"browsers"`
	InstalledApplications []string       `json:"installed_applications"`
}

// browserInfo is normalized client-authored browser installation metadata.
type browserInfo struct {
	Name       string
	BinaryPath string
	ProfileDir string
}

// normalizedEntryReport is the validated onboarding data used to construct a
// gateway-authored audit event. Its fields remain client-authored telemetry.
type normalizedEntryReport struct {
	AgentID               string
	Hostname              string
	OSVersion             string
	IsNewAgent            bool
	Browsers              []browserInfo
	InstalledApplications []string
	OccurredAt            time.Time
}

// handleEntry processes an authenticated stage-2 onboarding report.
//
// The agent identity is extracted from the mTLS session. The request body
// is validated and normalized through normalizeEntryRequest, which applies
// length limits to every text field. The normalized report is published to NATS.
//
// Expected request body: entryRequest JSON.
// Response: 202 Accepted with event_id.
func (s *Server) handleEntry(c *gin.Context) {
	agentID := c.GetString("agent_id")
	if agentID == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "missing authenticated agent identity"})
		return
	}

	var req entryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schema"})
		return
	}

	report := normalizeEntryRequest(agentID, req)

	env := &pb.EventEnvelope{
		EventId:   uuid.New().String(),
		TraceId:   c.GetHeader("X-Trace-ID"),
		Timestamp: timestamppb.New(report.OccurredAt),
		Security: &pb.SecurityContext{
			AgentId:         agentID,
			SessionId:       uuid.New().String(),
			ClientIp:        c.ClientIP(),
			IsAuthenticated: true,
		},
		Payload: &pb.EventEnvelope_LogEntry{LogEntry: &pb.LogEntry{
			Level:     "INFO",
			Component: "gateway.ingest.entry",
			Message: fmt.Sprintf(
				"stage2 entry accepted hostname=%s browsers=%d apps=%d is_new_agent=%t",
				report.Hostname,
				len(report.Browsers),
				len(report.InstalledApplications),
				report.IsNewAgent,
			),
		}},
	}

	subject := "sys.in.default." + agentID + ".entry"
	if err := s.broker.Publish(subject, env); err != nil {
		slog.ErrorContext(c.Request.Context(), "entry publish failed",
			"agent_id", agentID,
			"error", err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue event"})
		return
	}

	s.storeLogEnvelope(env)

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "event_id": env.EventId})
}

// handleNextCommand dequeues the next pending command for the authenticated
// agent. Returns 204 No Content when the queue is empty.
func (s *Server) handleNextCommand(c *gin.Context) {
	agentID := c.GetString("agent_id")
	if agentID == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "missing authenticated agent identity"})
		return
	}

	if s.commandQueue == nil {
		c.Status(http.StatusNoContent)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), commandPollTimeout)
	defer cancel()

	cmd := s.commandQueue.WaitDequeue(ctx, agentID)
	if cmd == nil {
		c.Status(http.StatusNoContent)
		return
	}

	c.JSON(http.StatusOK, cmd)
}

// commandResultRequest is the JSON shape for a command execution result.
type commandResultRequest struct {
	CommandID                string          `json:"command_id" binding:"required"`
	Type                     string          `json:"type" binding:"required"`
	Status                   string          `json:"status" binding:"required"`
	Reason                   string          `json:"reason"`
	ClientHostname           string          `json:"client_hostname"`
	OutputData               []byte          `json:"output_data,omitempty"`
	TerminalSessionID        string          `json:"terminal_session_id"`
	TerminalShell            string          `json:"terminal_shell"`
	TerminalCommand          string          `json:"terminal_command"`
	TerminalWorkingDirectory string          `json:"terminal_working_directory"`
	TerminalExitCode         int             `json:"terminal_exit_code"`
	Result                   json.RawMessage `json:"result"`
}

// handleCommandResult processes an authenticated command execution result.
//
// The result is published as an audit log entry to NATS.
func (s *Server) handleCommandResult(c *gin.Context) {
	agentID := c.GetString("agent_id")
	if agentID == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "missing authenticated agent identity"})
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxScreenMediaFrameBytes+maxTerminalOutputLen)
	var req commandResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid schema"})
		return
	}

	message := fmt.Sprintf(
		"command_result command_id=%s type=%s status=%s hostname=%s reason=%s output_bytes=%d",
		clampText(req.CommandID, maxCommandIDLen),
		clampText(req.Type, maxTypeLen),
		clampText(req.Status, maxStatusLen),
		clampText(req.ClientHostname, maxHostnameLen),
		clampText(req.Reason, maxReasonLen),
		len(req.OutputData),
	)

	env := &pb.EventEnvelope{
		EventId:   uuid.New().String(),
		TraceId:   c.GetHeader("X-Trace-ID"),
		Timestamp: timestamppb.Now(),
		Security: &pb.SecurityContext{
			AgentId:         agentID,
			SessionId:       uuid.New().String(),
			ClientIp:        c.ClientIP(),
			IsAuthenticated: true,
		},
		Payload: &pb.EventEnvelope_LogEntry{LogEntry: &pb.LogEntry{
			Level:     "INFO",
			Component: "gateway.command.audit",
			Message:   message,
		}},
	}

	subject := "sys.in.default." + agentID + ".command.audit"
	if err := s.broker.Publish(subject, env); err != nil {
		slog.ErrorContext(c.Request.Context(), "command result publish failed",
			"agent_id", agentID,
			"command_id", req.CommandID,
			"error", err,
		)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue event"})
		return
	}

	s.storeLogEnvelope(env)
	if err := s.recordSpecialCommandResult(agentID, req); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "command result does not match an active operation"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "event_id": env.EventId})
}

func (s *Server) recordSpecialCommandResult(agentID string, req commandResultRequest) error {
	commandType := CommandType(req.Type)
	switch {
	case commandType == CommandTypeRequestScreenshot:
		s.recordScreenshotResult(agentID, req)
	case commandType == CommandTypeTerminalRun:
		s.storeTerminalResult(agentID, req)
	case strings.HasPrefix(req.Type, "files.") && s.fileWorkspace != nil:
		return s.fileWorkspace.Complete(agentID, req.CommandID, req.Status, req.Result)
	}
	return nil
}

func (s *Server) recordScreenshotResult(agentID string, req commandResultRequest) {
	if req.Status == "executed" && len(req.OutputData) > 0 {
		s.screenStore.Save(agentID, ScreenFrame{
			AgentID: agentID, CommandID: req.CommandID, CapturedAt: time.Now().UTC(),
			Content: append([]byte(nil), req.OutputData...),
		})
	}
}

// handleScreenMedia receives binary live screen frames from an authenticated
// agent over the media plane. The agent identity is derived from mTLS; frame
// bytes are agent-authored opaque media data and are only stored in memory.
func (s *Server) handleScreenMedia(c *gin.Context) {
	agentID := c.GetString("agent_id")
	if agentID == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "missing authenticated agent identity"})
		return
	}

	conn, err := screenMediaUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		slog.WarnContext(c.Request.Context(), "screen media upgrade failed",
			"agent_id", agentID,
			"error", err,
		)
		return
	}
	defer func() { _ = conn.Close() }()
	conn.SetReadLimit(maxScreenMediaFrameBytes)

	contentType := c.Query("content_type")
	if contentType == "" {
		contentType = "image/png"
	}

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if messageType != websocket.BinaryMessage || len(data) == 0 {
			continue
		}

		s.screenStore.Save(agentID, ScreenFrame{
			AgentID:     agentID,
			CapturedAt:  time.Now().UTC(),
			ContentType: contentType,
			Content:     append([]byte(nil), data...),
		})
	}
}

// DashboardRuntime exposes browser dashboard dependencies without giving the
// dashboard direct access to the mTLS HTTP handlers.
func (s *Server) DashboardRuntime() DashboardRuntime {
	directory, _ := s.statusProvider.(ClientDirectory)
	return DashboardRuntime{
		Directory:       directory,
		CommandQueue:    s.commandQueue,
		Screens:         s.screenStore,
		Sessions:        s.screenSessions,
		Logs:            s.logStore,
		Terminals:       s.terminalStore,
		Files:           s.fileWorkspace,
		FileOperatorID:  s.fileOperatorID,
		DashboardOrigin: s.dashboardOrigin,
	}
}

// ConfigureFileWorkspace attaches the gateway-owned durable file service and
// its server-authored audit source label to the dashboard transport.
func (s *Server) ConfigureFileWorkspace(service *fileworkspace.Service, operatorID string) {
	s.fileWorkspace = service
	s.fileOperatorID = operatorID
}

// ConfigureDashboardOrigin sets the exact browser origin permitted to open
// administrative WebSocket connections.
func (s *Server) ConfigureDashboardOrigin(origin string) {
	s.dashboardOrigin = strings.TrimSpace(origin)
}

func (s *Server) storeTerminalResult(agentID string, req commandResultRequest) {
	if s == nil || s.terminalStore == nil {
		return
	}
	s.terminalStore.Complete(agentID, req.CommandID, TerminalEntry{
		AgentID:          agentID,
		SessionID:        clampText(strings.TrimSpace(req.TerminalSessionID), maxCommandIDLen),
		CommandID:        clampText(strings.TrimSpace(req.CommandID), maxCommandIDLen),
		Command:          clampText(strings.TrimSpace(req.TerminalCommand), maxLogMessageLen),
		Shell:            normalizeTerminalShell(req.TerminalShell),
		WorkingDirectory: clampText(strings.TrimSpace(req.TerminalWorkingDirectory), maxPathLen),
		Status:           clampText(strings.TrimSpace(req.Status), maxStatusLen),
		ExitCode:         req.TerminalExitCode,
		OutputLog:        clampTerminalOutput(req.OutputData, maxTerminalOutputLen),
		Reason:           clampText(strings.TrimSpace(req.Reason), maxReasonLen),
	})
}

func (s *Server) storeLogEnvelope(env *pb.EventEnvelope) {
	if s == nil || s.logStore == nil || env == nil || env.Security == nil {
		return
	}

	entry := env.GetLogEntry()
	if entry == nil {
		return
	}

	observedAt := time.Now().UTC()
	if env.Timestamp != nil {
		observedAt = env.Timestamp.AsTime()
	}

	s.logStore.Append(AgentLogEntry{
		EventID:    env.EventId,
		AgentID:    env.Security.AgentId,
		ClientIP:   env.Security.ClientIp,
		ObservedAt: observedAt,
		Level:      entry.Level,
		Component:  entry.Component,
		Message:    entry.Message,
	})
}

func normalizeLogEntry(entry *pb.LogEntry) *pb.LogEntry {
	if entry == nil {
		return &pb.LogEntry{Level: "INFO", Component: "client", Message: ""}
	}

	level := strings.ToUpper(clampText(strings.TrimSpace(entry.Level), maxLogLevelLen))
	switch level {
	case "DEBUG", "INFO", "WARN", "ERROR":
	default:
		level = "INFO"
	}

	component := clampText(strings.TrimSpace(entry.Component), maxComponentLen)
	if component == "" {
		component = "client"
	}

	return &pb.LogEntry{
		Level:     level,
		Component: component,
		Message:   clampText(strings.TrimSpace(entry.Message), maxLogMessageLen),
	}
}

// markSeen records that an agent has been observed. Returns whether this is
// the first observation (inserted) and whether the agent was previously known
// (existed). Thread-safe.
func (s *Server) markSeen(agentID string) (inserted bool, existed bool) {
	s.seenMu.Lock()
	defer s.seenMu.Unlock()

	_, existed = s.seenAgents[agentID]
	if !existed {
		s.seenAgents[agentID] = struct{}{}
		return true, false
	}

	return false, true
}

// normalizeEntryRequest validates and constrains an entry request into a
// normalizedEntryReport. Every client-supplied text field is clamped to a
// maximum length before entering the event pipeline.
//
// Length limits and why:
//   - Hostname: 120 characters (typical FQDN limit per RFC 1035).
//   - OSVersion: 120 characters (free-form, but bounded to prevent abuse).
//   - Browser name: 80 characters.
//   - BinaryPath: 260 characters (Windows MAX_PATH convention).
//   - ProfileDir: 260 characters (same convention).
//   - Application name: 120 characters.
//   - Maximum browsers: 32 (reasonable upper bound for browser enumeration).
//   - Maximum applications: 200 (prevents unbounded memory growth).
func normalizeEntryRequest(agentID string, req entryRequest) normalizedEntryReport {
	browsers := make([]browserInfo, 0, len(req.Browsers))
	for _, b := range req.Browsers {
		if len(browsers) >= maxBrowsers {
			break
		}

		name := clampText(b.Name, maxBrowserNameLen)
		if name == "" {
			continue
		}

		browsers = append(browsers, browserInfo{
			Name:       name,
			BinaryPath: clampText(b.BinaryPath, maxPathLen),
			ProfileDir: clampText(b.ProfileDir, maxPathLen),
		})
	}

	apps := make([]string, 0, len(req.InstalledApplications))
	for _, app := range req.InstalledApplications {
		if len(apps) >= maxApps {
			break
		}

		item := clampText(app, maxAppNameLen)
		if item == "" {
			continue
		}
		apps = append(apps, item)
	}

	return normalizedEntryReport{
		AgentID:               agentID,
		Hostname:              clampText(req.Hostname, maxHostnameLen),
		OSVersion:             clampText(req.OSVersion, maxOSVersionLen),
		IsNewAgent:            req.IsNewAgent,
		Browsers:              browsers,
		InstalledApplications: apps,
		OccurredAt:            timestamppb.Now().AsTime(),
	}
}

// clampText truncates the input to the specified byte length after trimming
// whitespace. Returns an empty string when the trimmed input is empty.
//
// This is the final length gate for client-supplied strings entering the
// event pipeline. Every handler that stores or forwards user-authored text
// must pass the text through clampText with a limit appropriate for the
// downstream consumer.
func clampText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit]
}

// Run starts the HTTPS listener with strict mTLS enforcement. The server
// loads the CA certificate, server certificate, and server key from certPath.
//
// TLS configuration:
//   - ClientAuth: RequireAndVerifyClientCert (strict mTLS).
//   - MinVersion: TLS 1.3 (no fallback to older versions).
//   - ClientCAs: CA pool built from certPath/ca.crt.
//
// Certificate files expected in certPath:
//   - ca.crt — CA certificate for client verification.
//   - server.crt — server certificate for TLS handshake.
//   - server.key — server private key.
func (s *Server) Run(addr, certPath string) error {
	caCert, err := os.ReadFile(filepath.Clean(filepath.Join(certPath, "ca.crt")))
	if err != nil {
		return fmt.Errorf("read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return fmt.Errorf("parse CA certificate: invalid PEM data")
	}

	tlsConfig := &tls.Config{
		ClientCAs:  caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS13,
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           s.engine,
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	return server.ListenAndServeTLS(filepath.Join(certPath, "server.crt"), filepath.Join(certPath, "server.key"))
}
