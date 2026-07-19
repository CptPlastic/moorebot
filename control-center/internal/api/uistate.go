package api

import "sync"

type Mission struct {
	Title     string `json:"title"`
	Objective string `json:"objective"`
	Callsign  string `json:"callsign"`
	Notes     string `json:"notes"`
}

type UIState struct {
	Reticle bool    `json:"reticle"`
	Hud     bool    `json:"hud"`
	Mission Mission `json:"mission"`
}

type uiStore struct {
	mu sync.RWMutex
	v  UIState
}

func NewUIStore() *uiStore {
	return &uiStore{v: UIState{Reticle: true, Hud: true}}
}

func (s *uiStore) Get() UIState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.v
}

func (s *uiStore) Patch(p UIState, hasReticle, hasHud, hasMission bool) UIState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if hasReticle {
		s.v.Reticle = p.Reticle
	}
	if hasHud {
		s.v.Hud = p.Hud
	}
	if hasMission {
		s.v.Mission = p.Mission
	}
	return s.v
}
