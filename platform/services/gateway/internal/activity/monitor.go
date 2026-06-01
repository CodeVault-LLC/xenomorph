package activity

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/codevault-llc/xenomorph/platform/services/gateway/internal/provider"
	pb "github.com/codevault-llc/xenomorph/platform/shared/proto/gen/go/platform/v1"
)

// Monitor derives online/offline transitions from authenticated heartbeat traffic.
type Monitor struct {
	notify       *provider.Fanout
	offlineAfter time.Duration
	now          func() time.Time

	mu     sync.Mutex
	online map[string]presence
}

type presence struct {
	lastSeen time.Time
	hostname string
}

func NewMonitor(offlineAfter time.Duration, notify *provider.Fanout) *Monitor {
	return &Monitor{
		notify:       notify,
		offlineAfter: offlineAfter,
		now:          time.Now,
		online:       make(map[string]presence),
	}
}

// ProcessHeartbeat updates liveness state based on a gateway-authenticated envelope.
func (m *Monitor) ProcessHeartbeat(ctx context.Context, envelope *pb.EventEnvelope) error {
	if envelope == nil {
		return fmt.Errorf("missing envelope")
	}
	if envelope.Security == nil {
		return fmt.Errorf("missing security context")
	}
	agentID := envelope.Security.AgentId
	if agentID == "" {
		return fmt.Errorf("missing agent id")
	}

	hb := envelope.GetHeartbeat()
	hostname := ""
	if hb != nil {
		hostname = hb.Hostname
	}

	eventTime := m.now()
	if envelope.Timestamp != nil {
		eventTime = envelope.Timestamp.AsTime()
	}

	var shouldNotifyOnline bool

	m.mu.Lock()
	state, exists := m.online[agentID]
	if !exists || eventTime.Sub(state.lastSeen) > m.offlineAfter {
		shouldNotifyOnline = true
	}
	m.online[agentID] = presence{lastSeen: eventTime, hostname: hostname}
	m.mu.Unlock()

	if shouldNotifyOnline {
		return m.notify.Notify(ctx, provider.ActivityEvent{
			AgentID:    agentID,
			Hostname:   hostname,
			OccurredAt: eventTime,
			Status:     provider.StatusOnline,
			Source:     "heartbeat",
		})
	}

	return nil
}

// Sweep marks stale agents as offline.
func (m *Monitor) Sweep(ctx context.Context) error {
	now := m.now()

	type staleAgent struct {
		agentID  string
		hostname string
	}

	stale := make([]staleAgent, 0)

	m.mu.Lock()
	for agentID, state := range m.online {
		if now.Sub(state.lastSeen) > m.offlineAfter {
			stale = append(stale, staleAgent{agentID: agentID, hostname: state.hostname})
			delete(m.online, agentID)
		}
	}
	m.mu.Unlock()

	var errs []error
	for _, agent := range stale {
		err := m.notify.Notify(ctx, provider.ActivityEvent{
			AgentID:    agent.agentID,
			Hostname:   agent.hostname,
			OccurredAt: now,
			Status:     provider.StatusOffline,
			Source:     "heartbeat-timeout",
		})
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// Snapshot returns a point-in-time view of an agent's presence state.
func (m *Monitor) Snapshot(agentID string) (provider.AgentSnapshot, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.online[agentID]
	if !ok {
		return provider.AgentSnapshot{}, false
	}

	return provider.AgentSnapshot{
		AgentID:  agentID,
		Hostname: p.hostname,
		LastSeen: p.lastSeen,
		IsOnline: true,
	}, true
}
