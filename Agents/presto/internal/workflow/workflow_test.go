package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"presto/internal/agent"
)

func TestWorkflowRunsSingleParallelSingle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	planner := testRunner(t, testProviderFunc(func(_ context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
		return agent.ChatResponse{Message: agent.Message{Content: "plan: " + lastUser(req)}}, nil
	}))

	gate := newStartGate(2)
	defer gate.release()
	researcher := testRunner(t, gatedProvider{
		name: "researcher",
		gate: gate,
		out:  "research: Go backend is fastest enough",
	})
	reviewer := testRunner(t, gatedProvider{
		name: "reviewer",
		gate: gate,
		out:  "review: keep orchestration explicit",
	})

	var synthPrompt string
	var synthMu sync.Mutex
	synthesizer := testRunner(t, testProviderFunc(func(_ context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
		synthMu.Lock()
		synthPrompt = lastUser(req)
		synthMu.Unlock()
		return agent.ChatResponse{Message: agent.Message{Content: "final synthesis"}}, nil
	}))

	wf, err := New(Sequence(
		Agent("planner", planner),
		Parallel(
			Agent("researcher", researcher),
			Agent("reviewer", reviewer),
		),
		Agent("synthesizer", synthesizer, WithPrompt(ResultsPrompt("Synthesize the parallel findings."))),
	))
	if err != nil {
		t.Fatal(err)
	}

	events := make([]Event, 0, 8)
	var eventsMu sync.Mutex
	state, err := wf.Run(ctx, "design a fast agent backend", func(event Event) {
		eventsMu.Lock()
		events = append(events, event)
		eventsMu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}

	if state.Last != "final synthesis" {
		t.Fatalf("last output = %q, want final synthesis", state.Last)
	}
	if len(state.Results) != 4 {
		t.Fatalf("results = %d, want 4: %#v", len(state.Results), state.Results)
	}
	assertResultOrder(t, state.Results, "planner", "researcher", "reviewer", "synthesizer")

	synthMu.Lock()
	prompt := synthPrompt
	synthMu.Unlock()
	for _, want := range []string{
		"Original input:",
		"design a fast agent backend",
		"planner: plan: design a fast agent backend",
		"researcher: research: Go backend is fastest enough",
		"reviewer: review: keep orchestration explicit",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("synth prompt missing %q:\n%s", want, prompt)
		}
	}

	if !gate.startedAll() {
		t.Fatal("parallel branches did not both start before release")
	}
	if len(events) == 0 {
		t.Fatal("expected workflow events")
	}
}

func TestParallelCancelsSiblingOnError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cancelled := make(chan struct{})
	gate := newStartGate(2)
	defer gate.release()
	failing := testRunner(t, testProviderFunc(func(context.Context, agent.ChatRequest) (agent.ChatResponse, error) {
		gate.markStarted("failing")
		<-gate.releaseCh
		return agent.ChatResponse{}, errors.New("branch failed")
	}))
	waiting := testRunner(t, testProviderFunc(func(ctx context.Context, _ agent.ChatRequest) (agent.ChatResponse, error) {
		gate.markStarted("waiting")
		<-ctx.Done()
		close(cancelled)
		return agent.ChatResponse{}, ctx.Err()
	}))

	wf, err := New(Parallel(
		Agent("failing", failing),
		Agent("waiting", waiting),
	))
	if err != nil {
		t.Fatal(err)
	}

	_, err = wf.Run(ctx, "trigger cancellation", nil)
	if err == nil {
		t.Fatal("expected workflow error")
	}
	if !strings.Contains(err.Error(), "branch failed") {
		t.Fatalf("error should include failing branch cause, got %v", err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("sibling branch was not cancelled")
	}
}

func TestParallelMaxConcurrency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var mu sync.Mutex
	active := 0
	maxActive := 0
	makeRunner := func(name string) *agent.Runner {
		return testRunner(t, testProviderFunc(func(ctx context.Context, _ agent.ChatRequest) (agent.ChatResponse, error) {
			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()

			time.Sleep(20 * time.Millisecond)

			mu.Lock()
			active--
			mu.Unlock()
			return agent.ChatResponse{Message: agent.Message{Content: name + " done"}}, nil
		}))
	}

	wf, err := New(Parallel(
		Agent("a", makeRunner("a")),
		Agent("b", makeRunner("b")),
		Agent("c", makeRunner("c")),
	).With(WithMaxConcurrency(2)))
	if err != nil {
		t.Fatal(err)
	}

	state, err := wf.Run(ctx, "limit fanout", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Results) != 3 {
		t.Fatalf("results = %d, want 3", len(state.Results))
	}
	if maxActive > 2 {
		t.Fatalf("max active branches = %d, want <= 2", maxActive)
	}
}

func TestFixedWorkflowStructuredOutputRetries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	contract := ObjectContract(map[string]FieldSpec{
		"summary": Required(KindString),
		"score":   Required(KindNumber),
	})
	contract.MaxAttempts = 3

	var attempts int
	planner := testRunner(t, testProviderFunc(func(_ context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
		attempts++
		if attempts == 1 {
			return agent.ChatResponse{Message: agent.Message{Content: "not json"}}, nil
		}
		if !strings.Contains(lastUser(req), "Return only valid JSON") {
			t.Fatalf("retry prompt missing strict JSON instruction:\n%s", lastUser(req))
		}
		return agent.ChatResponse{Message: agent.Message{Content: `{"summary":"planned","score":0.9}`}}, nil
	}))

	researcher := testRunner(t, testProviderFunc(func(_ context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
		return agent.ChatResponse{Message: agent.Message{Content: `{"summary":"researched","score":0.8}`}}, nil
	}))
	reviewer := testRunner(t, testProviderFunc(func(_ context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
		return agent.ChatResponse{Message: agent.Message{Content: `{"summary":"reviewed","score":0.7}`}}, nil
	}))
	synthesizer := testRunner(t, testProviderFunc(func(_ context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
		if !strings.Contains(lastUser(req), "researched") || !strings.Contains(lastUser(req), "reviewed") {
			t.Fatalf("synthesizer prompt missing parallel structured outputs:\n%s", lastUser(req))
		}
		return agent.ChatResponse{Message: agent.Message{Content: `{"summary":"done","score":1}`}}, nil
	}))

	wf, err := Fixed(
		Step("plan", AgentSpec{
			Name:     "planner",
			Runner:   planner,
			Prompt:   func(state State) string { return "Plan: " + state.Input },
			Output:   contract,
			Attempts: 3,
		}),
		Step("parallel review",
			AgentSpec{Name: "researcher", Runner: researcher, Output: contract},
			AgentSpec{Name: "reviewer", Runner: reviewer, Output: contract},
		),
		Step("synthesize", AgentSpec{
			Name:   "synthesizer",
			Runner: synthesizer,
			Prompt: ResultsPrompt("Create final JSON."),
			Output: contract,
		}),
	)
	if err != nil {
		t.Fatal(err)
	}

	state, err := wf.Run(ctx, "fixed workflow request", nil)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("planner attempts = %d, want 2", attempts)
	}
	if len(state.Results) != 4 {
		t.Fatalf("results = %d, want 4", len(state.Results))
	}
	assertResultOrder(t, state.Results, "planner", "researcher", "reviewer", "synthesizer")
	for _, result := range state.Results {
		if len(result.Structured) == 0 {
			t.Fatalf("%s missing structured output", result.Node)
		}
		var parsed map[string]any
		if err := json.Unmarshal(result.Structured, &parsed); err != nil {
			t.Fatalf("%s structured output invalid JSON: %v", result.Node, err)
		}
		if _, ok := parsed["summary"].(string); !ok {
			t.Fatalf("%s missing summary string: %#v", result.Node, parsed)
		}
	}
	if state.Last != `{"summary":"done","score":1}` {
		t.Fatalf("last = %q", state.Last)
	}
}

func TestStrictStructuredOutputRequiresWholeOutputJSON(t *testing.T) {
	contract := ObjectContract(map[string]FieldSpec{
		"summary": Required(KindString),
	})

	for _, output := range []string{
		`prefix {"summary":"ok"}`,
		"```json\n{\"summary\":\"ok\"}\n```",
		`{"summary":"ok"} suffix`,
	} {
		if _, err := contract.Validate(output); err == nil {
			t.Fatalf("strict contract accepted non-JSON output %q", output)
		}
	}

	structured, err := contract.Validate("\n  {\"summary\":\"ok\"}  \n")
	if err != nil {
		t.Fatal(err)
	}
	if string(structured) != `{"summary":"ok"}` {
		t.Fatalf("structured = %s, want compact JSON", structured)
	}
}

func TestResultsPromptAndFormatResultsPreferStructured(t *testing.T) {
	results := []Result{
		{
			Node:       "researcher",
			Output:     "raw researcher prose",
			Structured: json.RawMessage(`{"summary":"structured researcher"}`),
		},
		{
			Node:   "reviewer",
			Output: "plain reviewer prose",
		},
	}
	state := State{
		Input:   "compare options",
		Results: results,
	}

	prompt := ResultsPrompt("Synthesize.")(state)
	for _, text := range []string{
		`researcher: {"summary":"structured researcher"}`,
		"reviewer: plain reviewer prose",
	} {
		if !strings.Contains(prompt, text) {
			t.Fatalf("prompt missing %q:\n%s", text, prompt)
		}
	}
	if strings.Contains(prompt, "raw researcher prose") {
		t.Fatalf("prompt used raw output instead of structured output:\n%s", prompt)
	}

	formatted := formatResults(results)
	if !strings.Contains(formatted, `researcher: {"summary":"structured researcher"}`) {
		t.Fatalf("formatted results did not use structured output:\n%s", formatted)
	}
	if strings.Contains(formatted, "raw researcher prose") {
		t.Fatalf("formatted results used raw output instead of structured output:\n%s", formatted)
	}
}

func TestParallelSerializesEmitterCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	const branches = 8
	gate := newStartGate(branches)
	defer gate.release()
	nodes := make([]Node, 0, branches)
	for i := 0; i < branches; i++ {
		nodes = append(nodes, emittingTestNode{name: string(rune('a' + i)), gate: gate})
	}
	wf, err := New(Parallel(nodes...))
	if err != nil {
		t.Fatal(err)
	}

	firstEntered := make(chan struct{}, 1)
	releaseEmitter := make(chan struct{})
	var active int32
	var concurrent int32
	emit := func(Event) {
		if atomic.AddInt32(&active, 1) > 1 {
			atomic.StoreInt32(&concurrent, 1)
		}
		select {
		case firstEntered <- struct{}{}:
		default:
		}
		<-releaseEmitter
		atomic.AddInt32(&active, -1)
	}

	type runResult struct {
		state State
		err   error
	}
	done := make(chan runResult, 1)
	go func() {
		state, err := wf.Run(ctx, "serialize emits", emit)
		done <- runResult{state: state, err: err}
	}()

	select {
	case <-firstEntered:
	case <-time.After(time.Second):
		close(releaseEmitter)
		t.Fatal("emitter was not called")
	}
	time.Sleep(25 * time.Millisecond)
	if atomic.LoadInt32(&concurrent) != 0 {
		close(releaseEmitter)
		t.Fatal("parallel emitter calls overlapped")
	}
	close(releaseEmitter)

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if len(result.state.Results) != branches {
			t.Fatalf("results = %d, want %d", len(result.state.Results), branches)
		}
	case <-time.After(time.Second):
		t.Fatal("workflow did not finish")
	}
}

func TestStructuredOutputFailsAfterAttempts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	runner := testRunner(t, testProviderFunc(func(context.Context, agent.ChatRequest) (agent.ChatResponse, error) {
		return agent.ChatResponse{Message: agent.Message{Content: `{"summary":42}`}}, nil
	}))
	contract := ObjectContract(map[string]FieldSpec{
		"summary": Required(KindString),
	})
	contract.MaxAttempts = 2

	wf, err := New(Agent("bad-json", runner, WithOutputContract(contract)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = wf.Run(ctx, "must produce json", nil)
	if err == nil {
		t.Fatal("expected structured output validation error")
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Fatalf("error should mention failing field, got %v", err)
	}
}

type testProviderFunc func(context.Context, agent.ChatRequest) (agent.ChatResponse, error)

func (fn testProviderFunc) Chat(ctx context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
	return fn(ctx, req)
}

type gatedProvider struct {
	name string
	gate *startGate
	out  string
}

func (p gatedProvider) Chat(ctx context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
	p.gate.markStarted(p.name)
	select {
	case <-p.gate.releaseCh:
		return agent.ChatResponse{Message: agent.Message{Content: p.out + " from " + lastUser(req)}}, nil
	case <-ctx.Done():
		return agent.ChatResponse{}, ctx.Err()
	}
}

type emittingTestNode struct {
	name string
	gate *startGate
}

func (n emittingTestNode) Name() string {
	return n.name
}

func (n emittingTestNode) Run(ctx context.Context, state State, emit Emitter) (State, error) {
	n.gate.markStarted(n.name)
	select {
	case <-n.gate.releaseCh:
	case <-ctx.Done():
		return state, ctx.Err()
	}
	emitEvent(emit, Event{Type: EventNodeStarted, Node: n.name})
	next := cloneState(state)
	next.Last = n.name + " done"
	next.Results = append(next.Results, Result{Node: n.name, Output: next.Last})
	return next, nil
}

type startGate struct {
	want      int
	started   chan string
	releaseCh chan struct{}
	once      sync.Once
	mu        sync.Mutex
	seen      map[string]struct{}
}

func newStartGate(want int) *startGate {
	gate := &startGate{
		want:      want,
		started:   make(chan string, want),
		releaseCh: make(chan struct{}),
		seen:      make(map[string]struct{}, want),
	}
	go func() {
		for name := range gate.started {
			gate.mu.Lock()
			gate.seen[name] = struct{}{}
			ready := len(gate.seen) >= gate.want
			gate.mu.Unlock()
			if ready {
				gate.release()
				return
			}
		}
	}()
	return gate
}

func (g *startGate) markStarted(name string) {
	select {
	case g.started <- name:
	default:
	}
}

func (g *startGate) startedAll() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.seen) >= g.want
}

func (g *startGate) release() {
	g.once.Do(func() {
		close(g.releaseCh)
	})
}

func testRunner(t *testing.T, provider agent.Provider) *agent.Runner {
	t.Helper()
	runner, err := agent.NewRunner(agent.Config{
		Model:    "workflow-test",
		MaxSteps: 2,
		LLMRetry: agent.NoRetry(),
	}, provider, agent.NewMemoryStore())
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func lastUser(req agent.ChatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == agent.RoleUser {
			return req.Messages[i].Content
		}
	}
	return ""
}

func assertResultOrder(t *testing.T, results []Result, names ...string) {
	t.Helper()
	if len(results) != len(names) {
		t.Fatalf("results = %d, want %d", len(results), len(names))
	}
	for i, name := range names {
		if results[i].Node != name {
			t.Fatalf("result %d node = %q, want %q", i, results[i].Node, name)
		}
	}
}
