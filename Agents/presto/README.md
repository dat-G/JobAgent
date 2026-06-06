# Presto

Presto is a lightweight Go backend framework for fast agent workflows.

Current scope:

- multi-turn sessions
- prompt assembly
- OpenAI-compatible Chat Completions provider
- local Echo provider for zero-config smoke tests
- typed function tools
- LLM and tool retry policy
- composable single-agent and multi-agent workflows
- run events and SSE replay
- stdlib-only HTTP server

## Run

```sh
go run ./cmd/presto
```

By default the server listens on `127.0.0.1:8080`. Set `PRESTO_ADDR` explicitly if it must bind another interface.

By default, Presto uses the local Echo provider. To use an OpenAI-compatible API:

```sh
export PRESTO_API_KEY=...
export PRESTO_BASE_URL=https://api.openai.com/v1
export PRESTO_MODEL=...
export PRESTO_ASYNC_RUN_TIMEOUT=10m
go run ./cmd/presto
```

## API

```sh
curl -sS http://127.0.0.1:8080/healthz

curl -sS -X POST http://127.0.0.1:8080/sessions \
  -H 'Content-Type: application/json' \
  -d '{}'

curl -sS -X POST http://127.0.0.1:8080/sessions/{session_id}/runs \
  -H 'Content-Type: application/json' \
  -d '{"message":"hello presto"}'

curl -N http://127.0.0.1:8080/runs/{run_id}/events
```

Use `{"async":true,"message":"..."}` to return immediately and consume progress from SSE. Async runs continue after the request returns and are capped by `PRESTO_ASYNC_RUN_TIMEOUT`, a positive Go duration that defaults to `10m`.

## Workflow

Use `internal/workflow` to compose agent runs:

```go
wf, err := workflow.New(workflow.Sequence(
    workflow.Agent("planner", plannerRunner),
    workflow.Parallel(
        workflow.Agent("researcher", researcherRunner),
        workflow.Agent("reviewer", reviewerRunner),
        workflow.Agent("writer", writerRunner),
    ),
    workflow.Agent(
        "synthesizer",
        synthesizerRunner,
        workflow.WithPrompt(workflow.ResultsPrompt("Synthesize the parallel findings.")),
    ),
))
```

`Parallel` runs branches concurrently with goroutines and fan-in preserves branch declaration order for deterministic downstream prompts. Use a limit when needed:

```go
workflow.Parallel(a, b, c).With(workflow.WithMaxConcurrency(2))
```

For a fixed workflow, declare every step, agent count, prompt, and structured output contract:

```go
contract := workflow.ObjectContract(map[string]workflow.FieldSpec{
    "summary": workflow.Required(workflow.KindString),
    "score":   workflow.Required(workflow.KindNumber),
})
contract.MaxAttempts = 3

wf, err := workflow.Fixed(
    workflow.Step("plan", workflow.AgentSpec{
        Name:   "planner",
        Runner: plannerRunner,
        Prompt: func(state workflow.State) string {
            return "Create a plan for: " + state.Input
        },
        Output: contract,
    }),
    workflow.Step("parallel review",
        workflow.AgentSpec{Name: "researcher", Runner: researcherRunner, Output: contract},
        workflow.AgentSpec{Name: "reviewer", Runner: reviewerRunner, Output: contract},
    ),
    workflow.Step("synthesize", workflow.AgentSpec{
        Name:   "synthesizer",
        Runner: synthesizerRunner,
        Prompt: workflow.ResultsPrompt("Return the final decision."),
        Output: contract,
    }),
)
```

If an agent returns invalid JSON or misses a required field, Presto retries that node with a repair prompt. The compact validated JSON is stored in `Result.Structured`.

## Test

```sh
go test ./...
go test -bench=. -run=^$ ./...
```
