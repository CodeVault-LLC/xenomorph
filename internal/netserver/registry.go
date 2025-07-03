package netserver

import (
	"sync"
)

type Registry struct {
	sync.RWMutex
	sessions map[string]*Session
}

func NewRegistry() *Registry {
	return &Registry{
		sessions: make(map[string]*Session),
	}
}

func (r *Registry) Register(s *Session) {
	r.Lock()
	defer r.Unlock()
	r.sessions[s.ID] = s
}

func (r *Registry) Unregister(id string) {
	r.Lock()
	defer r.Unlock()
	delete(r.sessions, id)
}

func (r *Registry) Update(s *Session) {
	r.Lock()
	defer r.Unlock()
	if _, exists := r.sessions[s.ID]; exists {
		r.sessions[s.ID] = s
	}
}

func (r *Registry) Get(id string) (*Session, bool) {
	r.RLock()
	defer r.RUnlock()
	s, ok := r.sessions[id]
	return s, ok
}

func (r *Registry) List() []*Session {
	r.RLock()
	defer r.RUnlock()
	sessions := make([]*Session, 0, len(r.sessions))
	for _, s := range r.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

func (r *Registry) Count() int {
	r.RLock()
	defer r.RUnlock()
	return len(r.sessions)
}