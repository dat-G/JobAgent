package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"presto/internal/agent"
	"presto/internal/api"
	"presto/internal/provider"
)

type runtimeOptions struct {
	addr         string
	cache        bool
	cacheDir     string
	clearCache   bool
	asyncTimeout time.Duration
}

func main() {
	options := parseOptions()
	if options.clearCache {
		if err := provider.ClearCache(options.cacheDir); err != nil {
			log.Fatalf("cache clear failed: %v", err)
		}
		log.Printf("presto cache cleared: %s", options.cacheDir)
	}

	server := &http.Server{
		Addr:              options.addr,
		Handler:           api.NewServer(api.NewStore(), api.WithRunner(buildRunner(options)), api.WithAsyncRunTimeout(options.asyncTimeout)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("presto listening on %s", options.addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown failed: %v", err)
	}
}

func parseOptions() runtimeOptions {
	var options runtimeOptions
	flag.StringVar(&options.addr, "addr", envDefault("PRESTO_ADDR", "127.0.0.1:8080"), "HTTP listen address")
	flag.BoolVar(&options.cache, "cache", envBool("PRESTO_CACHE", false), "enable persistent provider response cache")
	flag.StringVar(&options.cacheDir, "cache-dir", envDefault("PRESTO_CACHE_DIR", provider.DefaultCacheDir), "persistent cache directory")
	flag.BoolVar(&options.clearCache, "clear-cache", false, "clear persistent provider response cache before starting")
	flag.Parse()
	options.asyncTimeout = asyncRunTimeout()
	return options
}

func buildRunner(options runtimeOptions) *agent.Runner {
	if err := configureModelRoutingFromWorkspace(); err != nil {
		log.Fatalf("model routing config failed: %v", err)
	}
	model := envDefault("PRESTO_MODEL", "presto-mock")
	apiKey := firstEnv("PRESTO_API_KEY", "OPENAI_API_KEY")
	baseURL := firstEnv("PRESTO_BASE_URL", "OPENAI_BASE_URL")

	var llm agent.Provider = provider.Echo{}
	if apiKey != "" {
		if model == "presto-mock" {
			model = "gpt-4.1-mini"
		}
		llm = provider.NewOpenAIChat(baseURL, apiKey, nil)
	}
	if options.cache {
		cached, err := provider.NewCachedProvider(llm, provider.CacheOptions{
			Dir:   options.cacheDir,
			Scope: cacheScope(apiKey, baseURL),
		})
		if err != nil {
			log.Fatalf("cache init failed: %v", err)
		}
		llm = cached
		log.Printf("presto cache enabled: %s", options.cacheDir)
	}

	runner, err := agent.NewRunner(agent.Config{
		Name:         "Presto",
		Model:        model,
		Instructions: "You are a low-latency backend agent. Keep answers concise.",
		MaxSteps:     8,
		LLMRetry:     agent.FastRetry(),
		Tools: []agent.Tool{{
			Spec: agent.ToolSpec{
				Name:        "now",
				Description: "Return the current server time in RFC3339 format.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
			},
			Handler: func(context.Context, json.RawMessage) (any, error) {
				return map[string]string{"time": time.Now().UTC().Format(time.RFC3339)}, nil
			},
		}},
	}, llm, agent.NewMemoryStore())
	if err != nil {
		log.Fatalf("runner init failed: %v", err)
	}
	return runner
}

func cacheScope(apiKey string, baseURL string) string {
	if apiKey == "" {
		return "provider=echo"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return "provider=openai-chat;base_url=" + baseURL +
		";thinking=" + os.Getenv("PRESTO_THINKING") +
		";reasoning_effort=" + os.Getenv("PRESTO_REASONING_EFFORT")
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func envDefault(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	switch value {
	case "":
		return fallback
	case "1", "t", "T", "true", "TRUE", "True", "yes", "YES", "Yes", "on", "ON", "On":
		return true
	case "0", "f", "F", "false", "FALSE", "False", "no", "NO", "No", "off", "OFF", "Off":
		return false
	default:
		log.Fatalf("invalid %s %q: use true/false", key, value)
		return fallback
	}
}

func asyncRunTimeout() time.Duration {
	value := envDefault("PRESTO_ASYNC_RUN_TIMEOUT", "10m")
	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 {
		log.Fatalf("invalid PRESTO_ASYNC_RUN_TIMEOUT %q: use a positive duration like 10m", value)
	}
	return timeout
}
