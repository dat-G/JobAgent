package provider

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"presto/internal/agent"
)

func TestCachedProviderUsesPersistentShardedCache(t *testing.T) {
	dir := t.TempDir()
	req := cacheTestRequest("hello")
	firstProvider := &countingProvider{}
	cached, err := NewCachedProvider(firstProvider, CacheOptions{Dir: dir, Scope: "test"})
	if err != nil {
		t.Fatal(err)
	}

	first, err := cached.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cached.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if first.Message.Content != second.Message.Content {
		t.Fatalf("second response = %q, want cached %q", second.Message.Content, first.Message.Content)
	}
	if calls := firstProvider.Count(); calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}

	secondProvider := &countingProvider{}
	restarted, err := NewCachedProvider(secondProvider, CacheOptions{Dir: dir, Scope: "test"})
	if err != nil {
		t.Fatal(err)
	}
	persisted, err := restarted.Chat(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Message.Content != first.Message.Content {
		t.Fatalf("persisted response = %q, want %q", persisted.Message.Content, first.Message.Content)
	}
	if calls := secondProvider.Count(); calls != 0 {
		t.Fatalf("restarted provider calls = %d, want 0", calls)
	}

	matches, err := filepath.Glob(filepath.Join(dir, cacheNS, "*", "*", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("cache entries = %d, want 1", len(matches))
	}
}

func TestCachedProviderIgnoresInternalMessageTimestamps(t *testing.T) {
	dir := t.TempDir()
	provider := &countingProvider{}
	cached, err := NewCachedProvider(provider, CacheOptions{Dir: dir, Scope: "test"})
	if err != nil {
		t.Fatal(err)
	}

	first := cacheTestRequest("same input")
	first.Messages[0].CreatedAt = time.Date(2026, 6, 1, 1, 2, 3, 0, time.UTC)
	second := cacheTestRequest("same input")
	second.Messages[0].CreatedAt = time.Date(2026, 6, 2, 1, 2, 3, 0, time.UTC)

	if _, err := cached.Chat(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if _, err := cached.Chat(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	if calls := provider.Count(); calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}

func TestCachedProviderSeparatesDifferentRequestsAndScopes(t *testing.T) {
	dir := t.TempDir()
	provider := &countingProvider{}
	cached, err := NewCachedProvider(provider, CacheOptions{Dir: dir, Scope: "scope-a"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := cached.Chat(context.Background(), cacheTestRequest("one")); err != nil {
		t.Fatal(err)
	}
	if _, err := cached.Chat(context.Background(), cacheTestRequest("two")); err != nil {
		t.Fatal(err)
	}
	if calls := provider.Count(); calls != 2 {
		t.Fatalf("provider calls = %d, want 2", calls)
	}

	otherScopeProvider := &countingProvider{}
	otherScope, err := NewCachedProvider(otherScopeProvider, CacheOptions{Dir: dir, Scope: "scope-b"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := otherScope.Chat(context.Background(), cacheTestRequest("one")); err != nil {
		t.Fatal(err)
	}
	if calls := otherScopeProvider.Count(); calls != 1 {
		t.Fatalf("other-scope provider calls = %d, want 1", calls)
	}
}

func TestClearCacheRemovesPersistentEntries(t *testing.T) {
	dir := t.TempDir()
	req := cacheTestRequest("clear me")
	provider := &countingProvider{}
	cached, err := NewCachedProvider(provider, CacheOptions{Dir: dir, Scope: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cached.Chat(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if err := ClearCache(dir); err != nil {
		t.Fatal(err)
	}

	afterClearProvider := &countingProvider{}
	afterClear, err := NewCachedProvider(afterClearProvider, CacheOptions{Dir: dir, Scope: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := afterClear.Chat(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if calls := afterClearProvider.Count(); calls != 1 {
		t.Fatalf("provider calls after clear = %d, want 1", calls)
	}
}

func TestCachedProviderCoalescesConcurrentIdenticalMisses(t *testing.T) {
	dir := t.TempDir()
	provider := &blockingProvider{
		called:  make(chan struct{}, 1),
		release: make(chan struct{}),
	}
	cached, err := NewCachedProvider(provider, CacheOptions{Dir: dir, Scope: "test"})
	if err != nil {
		t.Fatal(err)
	}

	const goroutines = 16
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cached.Chat(context.Background(), cacheTestRequest("burst"))
			errs <- err
		}()
	}

	<-provider.called
	close(provider.release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if calls := provider.Count(); calls != 1 {
		t.Fatalf("provider calls = %d, want 1", calls)
	}
}

type countingProvider struct {
	mu    sync.Mutex
	calls int
}

func (p *countingProvider) Chat(_ context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.calls++
	return agent.ChatResponse{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: fmt.Sprintf("call-%d:%s", p.calls, lastCacheTestUser(req)),
		},
		Usage: agent.Usage{TotalTokens: p.calls},
	}, nil
}

func (p *countingProvider) Count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

type blockingProvider struct {
	countingProvider
	called  chan struct{}
	release chan struct{}
}

func (p *blockingProvider) Chat(ctx context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()

	select {
	case p.called <- struct{}{}:
	default:
	}
	select {
	case <-p.release:
	case <-ctx.Done():
		return agent.ChatResponse{}, ctx.Err()
	}
	return agent.ChatResponse{
		Message: agent.Message{
			Role:    agent.RoleAssistant,
			Content: fmt.Sprintf("call-%d:%s", call, lastCacheTestUser(req)),
		},
		Usage: agent.Usage{TotalTokens: call},
	}, nil
}

func cacheTestRequest(input string) agent.ChatRequest {
	return agent.ChatRequest{
		Model: "unit-model",
		Messages: []agent.Message{
			{Role: agent.RoleUser, Content: input},
		},
		Tools: []agent.ToolSpec{{
			Name:        "lookup",
			Description: "lookup data",
		}},
		Temperature: 0,
		MaxTokens:   128,
	}
}

func lastCacheTestUser(req agent.ChatRequest) string {
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == agent.RoleUser {
			return req.Messages[i].Content
		}
	}
	return ""
}
