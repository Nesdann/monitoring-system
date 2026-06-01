package main

import "sync"

type AgentState struct {
	KnownProcesses map[string]bool // nombre → visto antes
	RecentCPUs     []float64       // últimas N mediciones
}

type StateStore struct {
	mu     sync.Mutex
	agents map[string]*AgentState
}

func NewStateStore() *StateStore {
	return &StateStore{
		agents: make(map[string]*AgentState),
	}
}

func (s *StateStore) Get(hostname string) *AgentState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.agents[hostname]; !ok {
		s.agents[hostname] = &AgentState{
			KnownProcesses: make(map[string]bool),
			RecentCPUs:     []float64{},
		}
	}
	return s.agents[hostname]
}
