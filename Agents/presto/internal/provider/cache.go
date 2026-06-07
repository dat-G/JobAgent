package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"presto/internal/agent"
)

const (
	DefaultCacheDir = ".presto-cache"

	cacheVersion = 1
	cacheNS      = "v1"
)

var errCacheMiss = errors.New("cache miss")

type CacheOptions struct {
	Dir   string
	Scope string
}

type CachedProvider struct {
	next  agent.Provider
	store *FileCache

	mu       sync.Mutex
	inflight map[string]*cacheCall
}

type cacheCall struct {
	done     chan struct{}
	response agent.ChatResponse
	err      error
}

func NewCachedProvider(next agent.Provider, options CacheOptions) (*CachedProvider, error) {
	if next == nil {
		return nil, errors.New("provider is required")
	}
	if options.Dir == "" {
		options.Dir = DefaultCacheDir
	}
	store, err := NewFileCache(options.Dir, options.Scope)
	if err != nil {
		return nil, err
	}
	return &CachedProvider{
		next:     next,
		store:    store,
		inflight: make(map[string]*cacheCall),
	}, nil
}

func (p *CachedProvider) Chat(ctx context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
	key, err := p.store.Key(req)
	if err != nil {
		return agent.ChatResponse{}, err
	}
	if response, err := p.store.Get(key); err == nil {
		return response, nil
	} else if !errors.Is(err, errCacheMiss) {
		return agent.ChatResponse{}, err
	}

	call, leader := p.join(key)
	if !leader {
		select {
		case <-call.done:
			return call.response, call.err
		case <-ctx.Done():
			return agent.ChatResponse{}, ctx.Err()
		}
	}

	defer func() {
		p.finish(key, call)
	}()

	if response, err := p.store.Get(key); err == nil {
		call.response = response
		return call.response, nil
	} else if !errors.Is(err, errCacheMiss) {
		call.err = err
		return agent.ChatResponse{}, call.err
	}

	response, err := p.next.Chat(ctx, req)
	if err != nil {
		call.err = err
		return agent.ChatResponse{}, err
	}
	if err := p.store.Set(key, req, response); err != nil {
		call.err = err
		return agent.ChatResponse{}, err
	}
	call.response = response
	return response, nil
}

func (p *CachedProvider) ChatStream(ctx context.Context, req agent.ChatRequest, onDelta agent.ChatStreamHandler) (agent.ChatResponse, error) {
	key, err := p.store.Key(req)
	if err != nil {
		return agent.ChatResponse{}, err
	}
	if response, err := p.store.Get(key); err == nil {
		emitCachedDelta(response, onDelta)
		return response, nil
	} else if !errors.Is(err, errCacheMiss) {
		return agent.ChatResponse{}, err
	}

	call, leader := p.join(key)
	if !leader {
		select {
		case <-call.done:
			if call.err == nil {
				emitCachedDelta(call.response, onDelta)
			}
			return call.response, call.err
		case <-ctx.Done():
			return agent.ChatResponse{}, ctx.Err()
		}
	}

	defer func() {
		p.finish(key, call)
	}()

	if response, err := p.store.Get(key); err == nil {
		call.response = response
		emitCachedDelta(response, onDelta)
		return call.response, nil
	} else if !errors.Is(err, errCacheMiss) {
		call.err = err
		return agent.ChatResponse{}, call.err
	}

	var response agent.ChatResponse
	if streaming, ok := p.next.(agent.StreamingProvider); ok {
		response, err = streaming.ChatStream(ctx, req, onDelta)
	} else {
		response, err = p.next.Chat(ctx, req)
		emitCachedDelta(response, onDelta)
	}
	if err != nil {
		call.err = err
		return agent.ChatResponse{}, err
	}
	if err := p.store.Set(key, req, response); err != nil {
		call.err = err
		return agent.ChatResponse{}, err
	}
	call.response = response
	return response, nil
}

func emitCachedDelta(response agent.ChatResponse, onDelta agent.ChatStreamHandler) {
	if onDelta == nil || response.Message.Content == "" {
		return
	}
	onDelta(agent.ChatStreamDelta{Content: response.Message.Content, Raw: response.Raw})
}

func (p *CachedProvider) join(key string) (*cacheCall, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if call, ok := p.inflight[key]; ok {
		return call, false
	}
	call := &cacheCall{done: make(chan struct{})}
	p.inflight[key] = call
	return call, true
}

func (p *CachedProvider) finish(key string, call *cacheCall) {
	p.mu.Lock()
	delete(p.inflight, key)
	close(call.done)
	p.mu.Unlock()
}

type FileCache struct {
	root  string
	scope string
}

func NewFileCache(dir string, scope string) (*FileCache, error) {
	if dir == "" {
		dir = DefaultCacheDir
	}
	root := filepath.Join(dir, cacheNS)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create cache dir: %w", err)
	}
	return &FileCache{root: root, scope: scope}, nil
}

func ClearCache(dir string) error {
	if dir == "" {
		dir = DefaultCacheDir
	}
	if err := os.RemoveAll(filepath.Join(dir, cacheNS)); err != nil {
		return fmt.Errorf("clear cache: %w", err)
	}
	return nil
}

func (c *FileCache) Key(req agent.ChatRequest) (string, error) {
	payload := cacheKeyPayload{
		Version: cacheVersion,
		Scope:   c.scope,
		Request: normalizeCacheRequest(req),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal cache key: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (c *FileCache) Get(key string) (agent.ChatResponse, error) {
	path, err := c.path(key)
	if err != nil {
		return agent.ChatResponse{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return agent.ChatResponse{}, errCacheMiss
	}
	if err != nil {
		return agent.ChatResponse{}, fmt.Errorf("read cache entry: %w", err)
	}

	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		_ = os.Remove(path)
		return agent.ChatResponse{}, errCacheMiss
	}
	if entry.Version != cacheVersion || entry.Key != key {
		_ = os.Remove(path)
		return agent.ChatResponse{}, errCacheMiss
	}
	return entry.Response, nil
}

func (c *FileCache) Set(key string, req agent.ChatRequest, response agent.ChatResponse) error {
	path, err := c.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create cache shard: %w", err)
	}

	entry := cacheEntry{
		Version:   cacheVersion,
		Key:       key,
		Request:   normalizeCacheRequest(req),
		Response:  response,
		CreatedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal cache entry: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create cache temp file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write cache temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close cache temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("commit cache entry: %w", err)
	}
	return nil
}

func (c *FileCache) path(key string) (string, error) {
	if len(key) != sha256.Size*2 {
		return "", fmt.Errorf("invalid cache key length: %d", len(key))
	}
	return filepath.Join(c.root, key[:2], key[2:4], key+".json"), nil
}

type cacheKeyPayload struct {
	Version int               `json:"version"`
	Scope   string            `json:"scope,omitempty"`
	Request agent.ChatRequest `json:"request"`
}

type cacheEntry struct {
	Version   int                `json:"version"`
	Key       string             `json:"key"`
	Request   agent.ChatRequest  `json:"request"`
	Response  agent.ChatResponse `json:"response"`
	CreatedAt time.Time          `json:"created_at"`
}

func normalizeCacheRequest(req agent.ChatRequest) agent.ChatRequest {
	req.Messages = append([]agent.Message(nil), req.Messages...)
	for i := range req.Messages {
		req.Messages[i].CreatedAt = time.Time{}
		req.Messages[i].ToolCalls = append([]agent.ToolCall(nil), req.Messages[i].ToolCalls...)
	}
	req.Tools = append([]agent.ToolSpec(nil), req.Tools...)
	return req
}
