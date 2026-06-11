package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type SequenceNode struct {
	name  string
	nodes []Node
}

func Sequence(nodes ...Node) *SequenceNode {
	return &SequenceNode{name: "sequence", nodes: nodes}
}

func (n *SequenceNode) Name() string {
	if n == nil || n.name == "" {
		return "sequence"
	}
	return n.name
}

func (n *SequenceNode) Run(ctx context.Context, state State, emit Emitter) (State, error) {
	if n == nil {
		return state, ErrNodeRequired
	}
	next := cloneState(state)
	for index, child := range n.nodes {
		if child == nil {
			return next, fmt.Errorf("sequence child %d: %w", index, ErrNodeRequired)
		}
		var err error
		next, err = child.Run(ctx, next, emit)
		if err != nil {
			return next, err
		}
	}
	return next, nil
}

type ParallelOption func(*ParallelNode)

const maxParallelNodeConcurrency = 500

type ParallelNode struct {
	name           string
	nodes          []Node
	maxConcurrency int
}

type branchResult struct {
	index int
	state State
	err   error
}

func Parallel(nodes ...Node) *ParallelNode {
	return &ParallelNode{name: "parallel", nodes: nodes}
}

func WithMaxConcurrency(limit int) ParallelOption {
	return func(node *ParallelNode) {
		node.maxConcurrency = limit
	}
}

func ConfigureParallel(node *ParallelNode, options ...ParallelOption) *ParallelNode {
	for _, option := range options {
		option(node)
	}
	return node
}

func (n *ParallelNode) With(options ...ParallelOption) *ParallelNode {
	return ConfigureParallel(n, options...)
}

func (n *ParallelNode) Name() string {
	if n == nil || n.name == "" {
		return "parallel"
	}
	return n.name
}

func (n *ParallelNode) Run(ctx context.Context, state State, emit Emitter) (State, error) {
	if n == nil {
		return state, ErrNodeRequired
	}
	if len(n.nodes) == 0 {
		return cloneState(state), nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	limit := n.maxConcurrency
	if limit <= 0 || limit > maxParallelNodeConcurrency {
		limit = maxParallelNodeConcurrency
	}
	if limit > len(n.nodes) {
		limit = len(n.nodes)
	}
	sem := make(chan struct{}, limit)
	results := make(chan branchResult, len(n.nodes))
	emit = serializedEmitter(emit)

	var wg sync.WaitGroup
	for index, child := range n.nodes {
		if child == nil {
			return state, fmt.Errorf("parallel child %d: %w", index, ErrNodeRequired)
		}
		wg.Add(1)
		go func(index int, child Node) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				results <- branchResult{index: index, state: cloneState(state), err: ctx.Err()}
				return
			}

			next, err := child.Run(ctx, cloneState(state), emit)
			if err != nil {
				cancel()
			}
			results <- branchResult{index: index, state: next, err: err}
		}(index, child)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	branches := make([]branchResult, len(n.nodes))
	var errs []error
	for result := range results {
		branches[result.index] = result
		if result.err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", n.nodes[result.index].Name(), result.err))
		}
	}
	if len(errs) > 0 {
		return mergeParallel(state, branches), errors.Join(errs...)
	}
	return mergeParallel(state, branches), nil
}

func mergeParallel(base State, branches []branchResult) State {
	next := cloneState(base)
	baseResults := len(base.Results)

	for _, branch := range branches {
		if len(branch.state.Results) <= baseResults {
			continue
		}
		next.Results = append(next.Results, branch.state.Results[baseResults:]...)
	}
	next.Last = formatResults(next.Results[baseResults:])
	if next.Last == "" {
		next.Last = base.Last
	}
	return next
}

func serializedEmitter(emit Emitter) Emitter {
	if emit == nil {
		return nil
	}
	var mu sync.Mutex
	return func(event Event) {
		mu.Lock()
		defer mu.Unlock()
		emit(event)
	}
}
