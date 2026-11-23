package browser

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/foxzi/autobrowser-client/go/internal/apperr"
	"github.com/foxzi/autobrowser-client/go/internal/config"
)

// Manager is SessionManager: the sessions of the HTTP API.
type Manager struct {
	settings *config.Settings
	mu       sync.Mutex
	sessions map[string]*Session
	order    []string // session ids in creation order
	stop     chan struct{}
	reaper   sync.WaitGroup
}

// NewManager starts the idle session reaper when timeouts.idleSessionMs > 0.
func NewManager(settings *config.Settings) *Manager {
	m := &Manager{settings: settings, sessions: map[string]*Session{}, stop: make(chan struct{})}
	if idle := time.Duration(settings.Timeouts.IdleSessionMs) * time.Millisecond; idle > 0 {
		m.reaper.Add(1)
		go m.reapIdle(idle)
	}
	return m
}

func (m *Manager) reapIdle(idle time.Duration) {
	defer m.reaper.Done()
	tick := min(idle/2, 30*time.Second)
	for {
		select {
		case <-m.stop:
			return
		case <-time.After(tick):
		}
		for _, s := range m.all() {
			if s.State() != StateRunning {
				continue
			}
			if d := s.idleFor(); d > idle {
				slog.Info(fmt.Sprintf("Reaping idle session %s (idle %.0fs)", s.id, d.Seconds()))
				s.Close(false)
				m.remove(s.id)
			}
		}
	}
}

func (m *Manager) all() []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Session, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.sessions[id])
	}
	return out
}

func (m *Manager) remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	for i, v := range m.order {
		if v == id {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
}

// Create is create_session. A session that fails to start stays listed as failed.
func (m *Manager) Create(opts Options) (*Session, error) {
	m.mu.Lock()
	active := 0
	for _, s := range m.sessions {
		if s.State() == StateRunning {
			active++
		}
	}
	if max := m.settings.API.MaxSessions; active >= max {
		m.mu.Unlock()
		return nil, apperr.New(apperr.BrowserStartFailed, fmt.Sprintf("Maximum active sessions reached: %d", max))
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	s := newSession("sess_"+hex.EncodeToString(b), m.settings)
	m.sessions[s.id] = s
	m.order = append(m.order, s.id)
	m.mu.Unlock()
	if err := s.start(opts); err != nil {
		return nil, err
	}
	return s, nil
}

// Get is get_session; it marks the session active.
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.Lock()
	s := m.sessions[id]
	m.mu.Unlock()
	if s == nil {
		return nil, apperr.New(apperr.SessionNotFound, "Session not found: "+id)
	}
	s.Touch()
	return s, nil
}

// Stop is stop_session.
func (m *Manager) Stop(id string) (*Session, error) {
	s, err := m.Get(id)
	if err != nil {
		return nil, err
	}
	s.Close(false)
	m.remove(id)
	return s, nil
}

// List is list_sessions: the info of every session.
func (m *Manager) List() []any {
	out := []any{}
	for _, s := range m.all() {
		out = append(out, s.Info())
	}
	return out
}

// StopAll stops the reaper and every session.
func (m *Manager) StopAll() {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
	m.reaper.Wait()
	for _, s := range m.all() {
		s.Close(false)
	}
}
