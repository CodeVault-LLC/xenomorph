package agentquic

import (
	"fmt"
	"sync"
)

type sessionRegistry struct {
	mu             sync.Mutex
	slots          map[string]*agentSlot
	maximumEntries int
	metrics        *Metrics
}

type agentSlot struct {
	mu         sync.RWMutex
	current    *session
	generation uint64
	references int
}

func newSessionRegistry(maximumEntries int, metrics *Metrics) *sessionRegistry {
	return &sessionRegistry{
		slots:          make(map[string]*agentSlot),
		maximumEntries: maximumEntries,
		metrics:        metrics,
	}
}

func (registry *sessionRegistry) install(candidate *session) (*session, error) {
	if registry == nil || candidate == nil || candidate.agentID == "" {
		return nil, fmt.Errorf("install QUIC session: candidate and agent ID are required")
	}

	slot, err := registry.referenceSlot(candidate.agentID)
	if err != nil {
		return nil, err
	}

	slot.mu.Lock()

	previous := slot.current
	if previous != nil {
		previous.fenced.Store(true)
	}

	slot.generation++
	candidate.generation = slot.generation
	slot.current = candidate
	slot.mu.Unlock()
	registry.releaseSlotReference(slot)

	if previous != nil {
		registry.metrics.replacedSessions.Add(1)
	}

	return previous, nil
}

func (registry *sessionRegistry) beginCommit(candidate *session) (func(), bool) {
	if registry == nil || candidate == nil {
		return func() {}, false
	}

	registry.mu.Lock()
	slot := registry.slots[candidate.agentID]
	registry.mu.Unlock()

	if slot == nil {
		return func() {}, false
	}

	slot.mu.RLock()
	if slot.current != candidate || slot.generation != candidate.generation || candidate.fenced.Load() {
		slot.mu.RUnlock()
		return func() {}, false
	}
	return slot.mu.RUnlock, true
}

func (registry *sessionRegistry) remove(candidate *session) {
	if registry == nil || candidate == nil {
		return
	}

	registry.mu.Lock()
	slot := registry.slots[candidate.agentID]
	registry.mu.Unlock()

	if slot == nil {
		return
	}

	slot.mu.Lock()
	if slot.current == candidate && slot.generation == candidate.generation {
		slot.current = nil
	}
	slot.mu.Unlock()
}

func (registry *sessionRegistry) referenceSlot(agentID string) (*agentSlot, error) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	if slot := registry.slots[agentID]; slot != nil {
		slot.references++
		return slot, nil
	}

	if len(registry.slots) >= registry.maximumEntries {
		registry.evictInactiveSlot()
	}

	if len(registry.slots) >= registry.maximumEntries {
		return nil, fmt.Errorf("install QUIC session: registry capacity reached")
	}

	slot := &agentSlot{references: 1}
	registry.slots[agentID] = slot

	return slot, nil
}

func (registry *sessionRegistry) releaseSlotReference(slot *agentSlot) {
	registry.mu.Lock()
	slot.references--
	registry.mu.Unlock()
}

func (registry *sessionRegistry) evictInactiveSlot() {
	for agentID, slot := range registry.slots {
		if slot.references != 0 {
			continue
		}

		slot.mu.RLock()
		inactive := slot.current == nil
		slot.mu.RUnlock()

		if inactive {
			delete(registry.slots, agentID)
			return
		}
	}
}
