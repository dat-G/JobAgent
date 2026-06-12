package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"presto/internal/agent"
)

const maxRequestBytes = 1 << 20
const defaultAsyncRunTimeout = 10 * time.Minute

type Server struct {
	store           *Store
	runner          *agent.Runner
	asyncRunTimeout time.Duration
}

type Option func(*Server)

func WithRunner(runner *agent.Runner) Option {
	return func(s *Server) {
		s.runner = runner
	}
}

func WithAsyncRunTimeout(timeout time.Duration) Option {
	return func(s *Server) {
		if timeout > 0 {
			s.asyncRunTimeout = timeout
		}
	}
}

func NewServer(store *Store, options ...Option) *Server {
	if store == nil {
		store = NewStore()
	}
	server := &Server{
		store:           store,
		asyncRunTimeout: defaultAsyncRunTimeout,
	}
	for _, option := range options {
		option(server)
	}
	return server
}

func NewHandler() http.Handler {
	return NewServer(NewStore())
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := pathParts(r.URL.Path)

	if len(parts) == 1 && parts[0] == "healthz" {
		s.handleHealthz(w, r)
		return
	}

	if len(parts) == 1 && parts[0] == "sessions" {
		s.handleSessions(w, r)
		return
	}

	if len(parts) == 2 && parts[0] == "sessions" {
		s.handleSession(w, r, parts[1])
		return
	}

	if len(parts) == 3 && parts[0] == "sessions" && parts[2] == "runs" {
		s.handleSessionRuns(w, r, parts[1])
		return
	}

	if len(parts) == 2 && parts[0] == "runs" {
		s.handleRun(w, r, parts[1])
		return
	}

	if len(parts) == 3 && parts[0] == "runs" && parts[2] == "events" {
		s.handleRunEvents(w, r, parts[1])
		return
	}

	writeError(w, http.StatusNotFound, "not found")
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"sessions": s.store.ListSessions()})
	case http.MethodPost:
		var req createSessionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		session := s.createSession(r.Context(), req.Metadata)
		writeJSON(w, http.StatusCreated, session)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	session, ok := s.store.GetSession(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) handleSessionRuns(w http.ResponseWriter, r *http.Request, sessionID string) {
	switch r.Method {
	case http.MethodGet:
		runs, ok := s.store.ListRuns(sessionID)
		if !ok {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
	case http.MethodPost:
		var req createRunRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		var message string
		if s.runner != nil {
			var err error
			message, err = runMessage(req)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
		}
		run, ok := s.store.CreateRun(sessionID, req.Input)
		if !ok {
			writeError(w, http.StatusNotFound, "session not found")
			return
		}
		if s.runner != nil {
			if req.Async {
				ctx, cancel := s.asyncRunContext(r.Context())
				go func() {
					defer cancel()
					s.executeRun(ctx, run.ID, sessionID, message, req.TokenStream)
				}()
				writeJSON(w, http.StatusCreated, run)
				return
			}
			finished := s.executeRun(r.Context(), run.ID, sessionID, message, req.TokenStream)
			writeJSON(w, http.StatusCreated, finished)
			return
		}
		writeJSON(w, http.StatusCreated, run)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request, runID string) {
	switch r.Method {
	case http.MethodGet:
		run, ok := s.store.GetRun(runID)
		if !ok {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeJSON(w, http.StatusOK, run)
	case http.MethodPatch:
		var req updateRunRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		run, ok, err := s.store.UpdateRunStatus(runID, req.Status)
		if errors.Is(err, ErrInvalidRunStatus) {
			writeError(w, http.StatusBadRequest, "invalid run status")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeJSON(w, http.StatusOK, run)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPatch)
	}
}

func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request, runID string) {
	switch r.Method {
	case http.MethodGet:
		s.streamRunEvents(w, r, runID)
	case http.MethodPost:
		var req appendEventRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		event, ok := s.store.AppendEvent(runID, req.Type, req.Data)
		if !ok {
			writeError(w, http.StatusNotFound, "run not found")
			return
		}
		writeJSON(w, http.StatusCreated, event)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPost)
	}
}

type createSessionRequest struct {
	Metadata map[string]string `json:"metadata,omitempty"`
}

type createRunRequest struct {
	Input       json.RawMessage `json:"input,omitempty"`
	Message     string          `json:"message,omitempty"`
	Async       bool            `json:"async,omitempty"`
	TokenStream bool            `json:"token_stream,omitempty"`
}

type appendEventRequest struct {
	Type string          `json:"type,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type updateRunRequest struct {
	Status string `json:"status"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil || r.Body == http.NoBody {
		return true
	}
	defer r.Body.Close()

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		writeError(w, http.StatusBadRequest, "invalid json body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func pathParts(path string) []string {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil
	}
	return strings.Split(path, "/")
}

func (s *Server) createSession(ctx context.Context, metadata map[string]string) Session {
	if s.runner == nil {
		return s.store.CreateSession(metadata)
	}
	session, err := s.runner.CreateSession(ctx)
	if err != nil {
		return s.store.CreateSession(metadata)
	}
	return s.store.CreateSessionWithID(session.ID, metadata)
}

func (s *Server) asyncRunContext(ctx context.Context) (context.Context, context.CancelFunc) {
	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	return context.WithTimeout(base, s.asyncRunTimeout)
}

func (s *Server) executeRun(ctx context.Context, runID string, sessionID string, message string, tokenStream bool) Run {
	events, results := s.runner.RunStream(ctx, agent.RunInput{
		RunID:     runID,
		SessionID: sessionID,
		UserInput: message,
		Stream:    tokenStream,
	})

	lastRunError := ""
	for event := range events {
		if event.Type == agent.EventRunError {
			lastRunError = event.Message
		}
		payload, _ := json.Marshal(event)
		s.store.AppendEvent(runID, string(event.Type), payload)
	}

	result, ok := <-results
	if !ok {
		if strings.TrimSpace(lastRunError) == "" {
			lastRunError = "runner failed without result"
		}
		run, _ := s.store.FinishRun(runID, "", lastRunError)
		return run
	}
	run, ok := s.store.FinishRun(runID, result.Output, "")
	if !ok {
		return Run{ID: runID, SessionID: sessionID, Status: RunStatusFailed, Error: "run disappeared"}
	}
	return run
}

func runMessage(req createRunRequest) (string, error) {
	if strings.TrimSpace(req.Message) != "" {
		return req.Message, nil
	}
	if len(req.Input) == 0 {
		return "", errors.New("message or input is required")
	}

	var asString string
	if err := json.Unmarshal(req.Input, &asString); err == nil && strings.TrimSpace(asString) != "" {
		return asString, nil
	}

	var asObject struct {
		Message string `json:"message"`
		Prompt  string `json:"prompt"`
		Input   string `json:"input"`
	}
	if err := json.Unmarshal(req.Input, &asObject); err != nil {
		return "", errors.New("input must be a string or object with message, prompt, or input")
	}
	switch {
	case strings.TrimSpace(asObject.Message) != "":
		return asObject.Message, nil
	case strings.TrimSpace(asObject.Prompt) != "":
		return asObject.Prompt, nil
	case strings.TrimSpace(asObject.Input) != "":
		return asObject.Input, nil
	default:
		return "", errors.New("input must include message, prompt, or input")
	}
}
