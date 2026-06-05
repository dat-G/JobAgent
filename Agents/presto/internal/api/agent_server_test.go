package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"presto/internal/agent"
)

func TestHTTPAPICanExecuteAgentRun(t *testing.T) {
	runner := newAPITestRunner(t, apiProvider{})

	server := httptest.NewServer(NewServer(NewStore(), WithRunner(runner)))
	defer server.Close()

	session := postJSON[Session](t, server.URL+"/sessions", `{}`)
	run := postJSON[Run](t, server.URL+"/sessions/"+session.ID+"/runs", `{"message":"hello"}`)

	if run.Status != RunStatusCompleted {
		t.Fatalf("run status = %q, want %q", run.Status, RunStatusCompleted)
	}
	if run.Output != "api provider: hello" {
		t.Fatalf("run output = %q", run.Output)
	}
	if run.EventCount == 0 {
		t.Fatal("expected runner events to be recorded")
	}
}

func TestHTTPAPIRejectsInvalidRunBeforeCreation(t *testing.T) {
	runner := newAPITestRunner(t, apiProvider{})

	server := httptest.NewServer(NewServer(NewStore(), WithRunner(runner)))
	defer server.Close()

	session := postJSON[Session](t, server.URL+"/sessions", `{}`)
	resp, err := http.Post(server.URL+"/sessions/"+session.ID+"/runs", "application/json", strings.NewReader(`{"input":{"unused":"value"}}`))
	if err != nil {
		t.Fatal(err)
	}
	assertStatus(t, resp, http.StatusBadRequest)

	listed := getJSON[struct {
		Runs []Run `json:"runs"`
	}](t, server.URL+"/sessions/"+session.ID+"/runs")
	if len(listed.Runs) != 0 {
		t.Fatalf("stored runs after rejected request = %d, want 0", len(listed.Runs))
	}
}

func TestHTTPAPIAsyncRunUsesConfiguredTimeout(t *testing.T) {
	deadlineSeen := make(chan bool, 1)
	runner := newAPITestRunner(t, blockingProvider{deadlineSeen: deadlineSeen})

	server := httptest.NewServer(NewServer(
		NewStore(),
		WithRunner(runner),
		WithAsyncRunTimeout(20*time.Millisecond),
	))
	defer server.Close()

	session := postJSON[Session](t, server.URL+"/sessions", `{}`)
	run := postJSON[Run](t, server.URL+"/sessions/"+session.ID+"/runs", `{"async":true,"message":"hello"}`)
	if run.Status != RunStatusQueued {
		t.Fatalf("initial async status = %q, want %q", run.Status, RunStatusQueued)
	}

	select {
	case ok := <-deadlineSeen:
		if !ok {
			t.Fatal("async run context did not have a deadline")
		}
	case <-time.After(time.Second):
		t.Fatal("provider was not called")
	}

	finished := waitForRunStatus(t, server.URL+"/runs/"+run.ID, RunStatusFailed)
	if finished.Error == "" {
		t.Fatal("expected timeout-driven failure to record an error")
	}
}

func newAPITestRunner(t *testing.T, provider agent.Provider) *agent.Runner {
	t.Helper()

	runner, err := agent.NewRunner(agent.Config{
		Name:     "APITestAgent",
		Model:    "unit-model",
		MaxSteps: 2,
		LLMRetry: agent.NoRetry(),
	}, provider, agent.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func waitForRunStatus(t *testing.T, url string, status string) Run {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run := getJSON[Run](t, url)
		if run.Status == status {
			return run
		}
		time.Sleep(10 * time.Millisecond)
	}
	run := getJSON[Run](t, url)
	t.Fatalf("run status = %q, want %q", run.Status, status)
	return Run{}
}

type apiProvider struct{}

func (apiProvider) Chat(_ context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == agent.RoleUser {
			return agent.ChatResponse{
				Message: agent.Message{Content: "api provider: " + req.Messages[i].Content},
			}, nil
		}
	}
	return agent.ChatResponse{Message: agent.Message{Content: "api provider ready"}}, nil
}

type blockingProvider struct {
	deadlineSeen chan<- bool
}

func (p blockingProvider) Chat(ctx context.Context, _ agent.ChatRequest) (agent.ChatResponse, error) {
	if p.deadlineSeen != nil {
		_, ok := ctx.Deadline()
		p.deadlineSeen <- ok
	}
	<-ctx.Done()
	return agent.ChatResponse{}, ctx.Err()
}
