package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"presto/internal/agent"
)

var ErrNodeRequired = errors.New("workflow node is required")

type State struct {
	Input   string         `json:"input"`
	Last    string         `json:"last,omitempty"`
	Results []Result       `json:"results,omitempty"`
	Values  map[string]any `json:"values,omitempty"`
}

type Result struct {
	Node       string          `json:"node"`
	RunID      string          `json:"run_id,omitempty"`
	SessionID  string          `json:"session_id,omitempty"`
	Output     string          `json:"output,omitempty"`
	Structured json.RawMessage `json:"structured,omitempty"`
	Duration   time.Duration   `json:"duration,omitempty"`
}

type EventType string

const (
	EventWorkflowStarted EventType = "workflow.started"
	EventWorkflowDone    EventType = "workflow.done"
	EventNodeStarted     EventType = "node.started"
	EventNodeDone        EventType = "node.done"
	EventNodeError       EventType = "node.error"
)

type Event struct {
	Type    EventType      `json:"type"`
	Node    string         `json:"node,omitempty"`
	Message string         `json:"message,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
	Time    time.Time      `json:"time"`
}

type Emitter func(Event)

type Node interface {
	Name() string
	Run(context.Context, State, Emitter) (State, error)
}

type Workflow struct {
	root Node
}

func New(root Node) (*Workflow, error) {
	if root == nil {
		return nil, ErrNodeRequired
	}
	return &Workflow{root: root}, nil
}

func (w *Workflow) Run(ctx context.Context, input string, emit Emitter) (State, error) {
	state := State{Input: input, Last: input}
	emitEvent(emit, Event{Type: EventWorkflowStarted})
	next, err := w.root.Run(ctx, state, emit)
	if err != nil {
		emitEvent(emit, Event{Type: EventNodeError, Node: w.root.Name(), Message: err.Error()})
		return next, err
	}
	emitEvent(emit, Event{Type: EventWorkflowDone, Data: map[string]any{"results": len(next.Results)}})
	return next, nil
}

type PromptFunc func(State) string

type AgentOption func(*AgentNode)

type AgentNode struct {
	name      string
	runner    *agent.Runner
	prompt    PromptFunc
	sessionID string
	contract  OutputContract
	attempts  int
}

func Agent(name string, runner *agent.Runner, options ...AgentOption) *AgentNode {
	node := &AgentNode{
		name:   name,
		runner: runner,
	}
	for _, option := range options {
		option(node)
	}
	return node
}

func WithPrompt(prompt PromptFunc) AgentOption {
	return func(node *AgentNode) {
		node.prompt = prompt
	}
}

func WithSession(sessionID string) AgentOption {
	return func(node *AgentNode) {
		node.sessionID = sessionID
	}
}

func WithOutputContract(contract OutputContract) AgentOption {
	return func(node *AgentNode) {
		node.contract = contract
	}
}

func WithAttempts(attempts int) AgentOption {
	return func(node *AgentNode) {
		node.attempts = attempts
	}
}

func (n *AgentNode) Name() string {
	if n == nil || n.name == "" {
		return "agent"
	}
	return n.name
}

func (n *AgentNode) Run(ctx context.Context, state State, emit Emitter) (State, error) {
	if n == nil || n.runner == nil {
		return state, fmt.Errorf("%s: runner is required", n.Name())
	}

	emitEvent(emit, Event{Type: EventNodeStarted, Node: n.Name()})

	prompt := state.Last
	if n.prompt != nil {
		prompt = n.prompt(cloneState(state))
	}

	attempts := n.outputAttempts()
	var validationErr error
	var result agent.RunResult
	var structured json.RawMessage
	started := time.Now()

	for attempt := 1; attempt <= attempts; attempt++ {
		runPrompt := prompt
		if attempt > 1 {
			runPrompt = retryPrompt(prompt, result.Output, validationErr)
		}
		var err error
		result, err = n.runner.Run(ctx, agent.RunInput{
			SessionID: n.sessionID,
			UserInput: runPrompt,
		})
		if err != nil {
			emitEvent(emit, Event{Type: EventNodeError, Node: n.Name(), Message: err.Error()})
			return state, err
		}
		if !n.contract.Enabled() {
			validationErr = nil
			break
		}
		structured, validationErr = n.contract.Validate(result.Output)
		if validationErr == nil {
			break
		}
		emitEvent(emit, Event{
			Type:    EventNodeError,
			Node:    n.Name(),
			Message: validationErr.Error(),
			Data: map[string]any{
				"attempt": attempt,
				"retry":   attempt < attempts,
			},
		})
	}
	if validationErr != nil {
		err := fmt.Errorf("%s structured output invalid after %d attempts: %w", n.Name(), attempts, validationErr)
		emitEvent(emit, Event{Type: EventNodeError, Node: n.Name(), Message: err.Error()})
		return state, err
	}

	out := Result{
		Node:       n.Name(),
		RunID:      result.RunID,
		SessionID:  result.SessionID,
		Output:     result.Output,
		Structured: structured,
		Duration:   time.Since(started),
	}
	next := cloneState(state)
	next.Last = result.Output
	next.Results = append(next.Results, out)

	emitEvent(emit, Event{
		Type: EventNodeDone,
		Node: n.Name(),
		Data: map[string]any{
			"run_id":     result.RunID,
			"session_id": result.SessionID,
			"duration":   out.Duration.String(),
			"attempts":   attempts,
		},
	})
	return next, nil
}

func (n *AgentNode) outputAttempts() int {
	if n.attempts > 0 {
		return n.attempts
	}
	if n.contract.MaxAttempts > 0 {
		return n.contract.MaxAttempts
	}
	if n.contract.Enabled() {
		return 2
	}
	return 1
}

func LastOutputOrInput(state State) string {
	if strings.TrimSpace(state.Last) != "" {
		return state.Last
	}
	return state.Input
}

func ResultsPrompt(instruction string) PromptFunc {
	return func(state State) string {
		var builder strings.Builder
		if strings.TrimSpace(instruction) != "" {
			builder.WriteString(strings.TrimSpace(instruction))
			builder.WriteString("\n\n")
		}
		builder.WriteString("Original input:\n")
		builder.WriteString(state.Input)
		builder.WriteString("\n\nPrior results:\n")
		for _, result := range state.Results {
			builder.WriteString("- ")
			builder.WriteString(result.Node)
			builder.WriteString(": ")
			builder.WriteString(resultText(result))
			builder.WriteString("\n")
		}
		return strings.TrimSpace(builder.String())
	}
}

func retryPrompt(original string, previousOutput string, validationErr error) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(original))
	builder.WriteString("\n\nYour previous output did not match the required structured JSON contract.\n")
	builder.WriteString("Validation error: ")
	if validationErr != nil {
		builder.WriteString(validationErr.Error())
	} else {
		builder.WriteString("unknown validation error")
	}
	builder.WriteString("\nPrevious output:\n")
	builder.WriteString(previousOutput)
	builder.WriteString("\n\nReturn only valid JSON. Do not include markdown or commentary.")
	return builder.String()
}

func formatResults(results []Result) string {
	var builder strings.Builder
	for _, result := range results {
		builder.WriteString(result.Node)
		builder.WriteString(": ")
		builder.WriteString(resultText(result))
		builder.WriteString("\n")
	}
	return strings.TrimSpace(builder.String())
}

func resultText(result Result) string {
	if len(result.Structured) > 0 {
		return string(result.Structured)
	}
	return result.Output
}

func cloneState(state State) State {
	out := state
	out.Results = append([]Result(nil), state.Results...)
	if state.Values != nil {
		out.Values = make(map[string]any, len(state.Values))
		for key, value := range state.Values {
			out.Values[key] = value
		}
	}
	return out
}

func emitEvent(emit Emitter, event Event) {
	if emit == nil {
		return
	}
	event.Time = time.Now().UTC()
	emit(event)
}
