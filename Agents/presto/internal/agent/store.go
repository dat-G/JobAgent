package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

var ErrSessionNotFound = errors.New("session not found")

type Session struct {
	ID        string    `json:"id"`
	Messages  []Message `json:"messages"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SessionStore interface {
	CreateSession(context.Context) (Session, error)
	GetSession(context.Context, string) (Session, error)
	SaveSession(context.Context, Session) error
}

type SessionUpdateFunc func(Session) (Session, error)

type SessionUpdater interface {
	UpdateSession(context.Context, string, SessionUpdateFunc) (Session, error)
}

type MemoryStore struct {
	mu       sync.RWMutex
	sessions map[string]Session
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{sessions: make(map[string]Session)}
}

func (s *MemoryStore) CreateSession(context.Context) (Session, error) {
	now := time.Now().UTC()
	session := Session{
		ID:        NewID("ses"),
		Messages:  make([]Message, 0, 8),
		CreatedAt: now,
		UpdatedAt: now,
	}

	s.mu.Lock()
	s.sessions[session.ID] = cloneSession(session)
	s.mu.Unlock()
	return session, nil
}

func (s *MemoryStore) GetSession(_ context.Context, id string) (Session, error) {
	s.mu.RLock()
	session, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return cloneSession(session), nil
}

func (s *MemoryStore) SaveSession(_ context.Context, session Session) error {
	if session.ID == "" {
		return errors.New("session id is required")
	}
	session.UpdatedAt = time.Now().UTC()

	s.mu.Lock()
	s.sessions[session.ID] = cloneSession(session)
	s.mu.Unlock()
	return nil
}

func (s *MemoryStore) UpdateSession(_ context.Context, id string, update SessionUpdateFunc) (Session, error) {
	if id == "" {
		return Session{}, errors.New("session id is required")
	}
	if update == nil {
		return Session{}, errors.New("session update function is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	next, err := update(cloneSession(session))
	if err != nil {
		return Session{}, err
	}
	if next.ID == "" {
		next.ID = id
	}
	if next.ID != id {
		return Session{}, errors.New("session update cannot change session id")
	}
	next.CreatedAt = session.CreatedAt
	next.UpdatedAt = time.Now().UTC()
	s.sessions[id] = cloneSession(next)
	return cloneSession(next), nil
}

func cloneSession(session Session) Session {
	clone := session
	clone.Messages = append([]Message(nil), session.Messages...)
	return clone
}

func NewID(prefix string) string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return prefix + "_" + hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405.000000000")))
	}
	return prefix + "_" + hex.EncodeToString(buf[:])
}
