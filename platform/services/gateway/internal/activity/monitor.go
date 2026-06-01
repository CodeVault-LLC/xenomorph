// Package activity tracks agent online/offline state from authenticated
// heartbeat events. This package owns the liveness state machine and
// transition notification logic.
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

// Monitor derives online/offline transitions from authenticated heartbeat
// traffic. Presence state is held in memory and is not persisted across
// gateway restarts.
//
// The Monitor owns two operations:
//   - ProcessHeartbeat updates the last-seen timestamp for an agent and
//     emits an online notification when the agent was previously offline or
//     when its last-seen exceeded the offlineAfter threshold.
//   - Sweep iterates all tracked agents and emits offline notifications for
//     those whose last-seen exceeds offlineAfter.
type Monitor struct {
	notify       *provider.Fanout
	offlineAfter time.Duration
	now          func() time.Time

	mu     sync.Mutex
	online map[string]presence
}

// presence holds agent liveness state in memory.
type presence struct {
	lastSeen time.Time
	hostname string
}

// NewMonitor creates a Monitor that emits offline notifications when an
// agent has not been seen for offlineAfter duration. The notifier fanout
// receives ActivityEvent notifications for each transition.
//
// When notify is nil, the monitor tracks state internally but produces no
// outbound notifications. This is valid for deployments without notification
// providers.
func NewMonitor(offlineAfter time.Duration, notify *provider.Fanout) *Monitor {
	return &Monitor{
		notify:       notify,
		offlineAfter: offlineAfter,
		now:          time.Now,
		online:       make(map[string]presence),
	}
}

// ProcessHeartbeat updates liveness state for an agent based on a
// gateway-authenticated envelope. An online notification is emitted when the
// agent was previously unknown or when its last-seen exceeded the
// offlineAfter threshold.
//
// The envelope must contain a non-empty Security.AgentId. Heartbeats from
// unauthenticated sources must not reach this function; the mTLS middleware
// in the transport layer is the caller's enforcement point.
//
// Returns an error when envelope or envelope.Security is nil or when the
// agent ID is empty.
func (m *Monitor) ProcessHeartbeat(ctx context.Context, envelope *pb.EventEnvelope) error {
	if envelope == nil {
		return fmt.Errorf("heartbeat envelope is nil")
	}
	if envelope.Security == nil {
		return fmt.Errorf("heartbeat envelope missing security context")
	}
	agentID := envelope.Security.AgentId
	if agentID == "" {
		return fmt.Errorf("heartbeat envelope has empty agent id")
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

// Sweep checks every tracked agent and emits offline notifications for those
// whose last-seen exceeds offlineAfter. Stale agents are removed from the
// tracking map. Errors from individual notification deliveries are collected
// and returned as a joined error.
//
// Call Sweep periodically from a ticker. The sweep interval should be
// shorter than offlineAfter to ensure timely offline detection.
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
// Returns false when the agent is not currently tracked (never seen or
// expired by Sweep).
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
