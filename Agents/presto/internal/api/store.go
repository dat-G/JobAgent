package api

import (
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var ErrInvalidRunStatus = errors.New("invalid run status")

type Store struct {
	mu sync.RWMutex

	seq uint64

	sessions      map[string]*Session
	runs          map[string]*runRecord
	runsBySession map[string][]string
	subscribers   map[string]map[chan Event]struct{}
}

type runRecord struct {
	run    Run
	events []Event
}

func NewStore() *Store {
	return &Store{
		sessions:      make(map[string]*Session),
		runs:          make(map[string]*runRecord),
		runsBySession: make(map[string][]string),
		subscribers:   make(map[string]map[chan Event]struct{}),
	}
}

func (s *Store) CreateSession(metadata map[string]string) Session {
	return s.CreateSessionWithID("", metadata)
}

func (s *Store) CreateSessionWithID(sessionID string, metadata map[string]string) Session {
	now := time.Now().UTC()
	if sessionID == "" {
		sessionID = s.nextID("sess")
	}
	session := &Session{
		ID:        sessionID,
		CreatedAt: now,
		UpdatedAt: now,
		Metadata:  copyStringMap(metadata),
	}

	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()

	return cloneSession(session)
}

func (s *Store) ListSessions() []Session {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sessions := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		sessions = append(sessions, cloneSession(session))
	}
	return sessions
}

func (s *Store) GetSession(sessionID string) (Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return Session{}, false
	}
	return cloneSession(session), true
}

func (s *Store) CreateRun(sessionID string, input json.RawMessage) (Run, bool) {
	now := time.Now().UTC()
	runID := s.nextID("run")
	run := Run{
		ID:         runID,
		SessionID:  sessionID,
		Status:     RunStatusQueued,
		Input:      copyRaw(input),
		CreatedAt:  now,
		UpdatedAt:  now,
		EventCount: 1,
	}
	event := Event{
		ID:        s.nextID("evt"),
		RunID:     runID,
		Type:      "run.created",
		Data:      mustJSON(map[string]string{"status": RunStatusQueued}),
		CreatedAt: now,
	}

	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return Run{}, false
	}
	session.UpdatedAt = now
	s.runs[runID] = &runRecord{
		run:    run,
		events: []Event{event},
	}
	s.runsBySession[sessionID] = append(s.runsBySession[sessionID], runID)
	out := cloneRun(&run)
	s.mu.Unlock()

	return out, true
}

func (s *Store) ListRuns(sessionID string) ([]Run, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, false
	}

	runIDs := s.runsBySession[sessionID]
	runs := make([]Run, 0, len(runIDs))
	for _, runID := range runIDs {
		if rec := s.runs[runID]; rec != nil {
			runs = append(runs, cloneRun(&rec.run))
		}
	}
	return runs, true
}

func (s *Store) GetRun(runID string) (Run, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rec, ok := s.runs[runID]
	if !ok {
		return Run{}, false
	}
	return cloneRun(&rec.run), true
}

func (s *Store) UpdateRunStatus(runID, status string) (Run, bool, error) {
	if !validRunStatus(status) {
		return Run{}, false, ErrInvalidRunStatus
	}

	now := time.Now().UTC()
	event := Event{
		ID:        s.nextID("evt"),
		RunID:     runID,
		Type:      "run.status",
		Data:      mustJSON(map[string]string{"status": status}),
		CreatedAt: now,
	}

	var subscribers []chan Event

	s.mu.Lock()
	rec, ok := s.runs[runID]
	if !ok {
		s.mu.Unlock()
		return Run{}, false, nil
	}

	applyStatus(&rec.run, status, now)
	rec.events = append(rec.events, event)
	rec.run.EventCount = len(rec.events)
	for ch := range s.subscribers[runID] {
		subscribers = append(subscribers, ch)
	}
	out := cloneRun(&rec.run)
	s.mu.Unlock()

	broadcast(subscribers, event)
	return out, true, nil
}

func (s *Store) AppendEvent(runID, eventType string, data json.RawMessage) (Event, bool) {
	if eventType == "" {
		eventType = "message"
	}
	if len(data) == 0 {
		data = json.RawMessage(`{}`)
	}

	now := time.Now().UTC()
	event := Event{
		ID:        s.nextID("evt"),
		RunID:     runID,
		Type:      eventType,
		Data:      copyRaw(data),
		CreatedAt: now,
	}

	var subscribers []chan Event

	s.mu.Lock()
	rec, ok := s.runs[runID]
	if !ok {
		s.mu.Unlock()
		return Event{}, false
	}
	rec.events = append(rec.events, event)
	rec.run.EventCount = len(rec.events)
	rec.run.UpdatedAt = now
	applyEventStatus(&rec.run, eventType, now)
	for ch := range s.subscribers[runID] {
		subscribers = append(subscribers, ch)
	}
	s.mu.Unlock()

	broadcast(subscribers, event)
	return cloneEvent(event), true
}

func (s *Store) FinishRun(runID string, output string, runErr string) (Run, bool) {
	now := time.Now().UTC()
	status := RunStatusCompleted
	if runErr != "" {
		status = RunStatusFailed
	}

	s.mu.Lock()
	rec, ok := s.runs[runID]
	if !ok {
		s.mu.Unlock()
		return Run{}, false
	}
	rec.run.Output = output
	rec.run.Error = runErr
	applyStatus(&rec.run, status, now)
	out := cloneRun(&rec.run)
	s.mu.Unlock()

	return out, true
}

func (s *Store) Subscribe(runID, lastEventID string) ([]Event, <-chan Event, func(), bool) {
	ch := make(chan Event, 64)

	s.mu.Lock()
	rec, ok := s.runs[runID]
	if !ok {
		s.mu.Unlock()
		return nil, nil, nil, false
	}

	replay := eventsAfter(rec.events, lastEventID)
	if s.subscribers[runID] == nil {
		s.subscribers[runID] = make(map[chan Event]struct{})
	}
	s.subscribers[runID][ch] = struct{}{}
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		if runSubscribers := s.subscribers[runID]; runSubscribers != nil {
			delete(runSubscribers, ch)
			if len(runSubscribers) == 0 {
				delete(s.subscribers, runID)
			}
		}
		s.mu.Unlock()
	}

	return replay, ch, unsubscribe, true
}

func (s *Store) nextID(prefix string) string {
	n := atomic.AddUint64(&s.seq, 1)
	return prefix + "_" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36) + "_" + strconv.FormatUint(n, 36)
}

func applyEventStatus(run *Run, eventType string, now time.Time) {
	switch eventType {
	case "run.started":
		applyStatus(run, RunStatusRunning, now)
	case "run.completed", "run.done":
		applyStatus(run, RunStatusCompleted, now)
	case "run.failed":
		applyStatus(run, RunStatusFailed, now)
	case "run.cancelled":
		applyStatus(run, RunStatusCancelled, now)
	case "run.error":
		applyStatus(run, RunStatusFailed, now)
	}
}

func applyStatus(run *Run, status string, now time.Time) {
	run.Status = status
	run.UpdatedAt = now
	if status == RunStatusRunning && run.StartedAt == nil {
		run.StartedAt = ptrTime(now)
	}
	if terminalStatus(status) && run.FinishedAt == nil {
		run.FinishedAt = ptrTime(now)
	}
}

func validRunStatus(status string) bool {
	switch status {
	case RunStatusQueued, RunStatusRunning, RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return true
	default:
		return false
	}
}

func terminalStatus(status string) bool {
	switch status {
	case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return true
	default:
		return false
	}
}

func eventsAfter(events []Event, lastEventID string) []Event {
	start := 0
	if lastEventID != "" {
		for i, event := range events {
			if event.ID == lastEventID {
				start = i + 1
				break
			}
		}
	}

	out := make([]Event, 0, len(events)-start)
	for _, event := range events[start:] {
		out = append(out, cloneEvent(event))
	}
	return out
}

func broadcast(subscribers []chan Event, event Event) {
	for _, ch := range subscribers {
		select {
		case ch <- cloneEvent(event):
		default:
		}
	}
}

func cloneSession(session *Session) Session {
	if session == nil {
		return Session{}
	}
	return Session{
		ID:        session.ID,
		CreatedAt: session.CreatedAt,
		UpdatedAt: session.UpdatedAt,
		Metadata:  copyStringMap(session.Metadata),
	}
}

func cloneRun(run *Run) Run {
	if run == nil {
		return Run{}
	}
	return Run{
		ID:         run.ID,
		SessionID:  run.SessionID,
		Status:     run.Status,
		Input:      copyRaw(run.Input),
		Output:     run.Output,
		Error:      run.Error,
		CreatedAt:  run.CreatedAt,
		UpdatedAt:  run.UpdatedAt,
		StartedAt:  copyTime(run.StartedAt),
		FinishedAt: copyTime(run.FinishedAt),
		EventCount: run.EventCount,
	}
}

func cloneEvent(event Event) Event {
	return Event{
		ID:        event.ID,
		RunID:     event.RunID,
		Type:      event.Type,
		Data:      copyRaw(event.Data),
		CreatedAt: event.CreatedAt,
	}
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func copyRaw(in json.RawMessage) json.RawMessage {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

func copyTime(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	return ptrTime(*in)
}

func ptrTime(t time.Time) *time.Time {
	out := t
	return &out
}

func mustJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return data
}
