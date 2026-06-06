package main

import (
	"context"
	"encoding/json"
	"errors"
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

func main() {
	addr := os.Getenv("PRESTO_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           api.NewServer(api.NewStore(), api.WithRunner(buildRunner()), api.WithAsyncRunTimeout(asyncRunTimeout())),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("presto listening on %s", addr)
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

func buildRunner() *agent.Runner {
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

func asyncRunTimeout() time.Duration {
	value := envDefault("PRESTO_ASYNC_RUN_TIMEOUT", "10m")
	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 {
		log.Fatalf("invalid PRESTO_ASYNC_RUN_TIMEOUT %q: use a positive duration like 10m", value)
	}
	return timeout
}
