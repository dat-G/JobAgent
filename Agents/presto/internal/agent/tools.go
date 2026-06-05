package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
)

var ErrToolNotFound = errors.New("tool not found")

type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

func NewToolRegistry(tools ...Tool) (*ToolRegistry, error) {
	registry := &ToolRegistry{tools: make(map[string]Tool, len(tools))}
	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (r *ToolRegistry) Register(tool Tool) error {
	if tool.Spec.Name == "" {
		return errors.New("tool name is required")
	}
	if tool.Handler == nil {
		return fmt.Errorf("tool %q handler is nil", tool.Spec.Name)
	}
	if len(tool.Spec.Parameters) == 0 {
		tool.Spec.Parameters = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":true}`)
	}
	if tool.Retry.MaxAttempts == 0 {
		tool.Retry = NoRetry()
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Spec.Name] = tool
	return nil
}

func (r *ToolRegistry) Specs() []ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()

	specs := make([]ToolSpec, 0, len(r.tools))
	for _, tool := range r.tools {
		specs = append(specs, tool.Spec)
	}
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})
	return specs
}

func (r *ToolRegistry) Execute(ctx context.Context, call ToolCall) (any, error) {
	r.mu.RLock()
	tool, ok := r.tools[call.Name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrToolNotFound, call.Name)
	}

	return withRetry(ctx, tool.Retry, func(ctx context.Context) (any, error) {
		return tool.Handler(ctx, call.Arguments)
	})
}

func JSONContent(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":"marshal tool result: %s"}`, err.Error())
	}
	return string(data)
}
