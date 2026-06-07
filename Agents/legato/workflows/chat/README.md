# Chat Workflow

`chat` is the Legato workflow wrapper for the Growth Lens AI assistant.

Current frontend chat calls Presto directly through `/api/presto/sessions/{id}/runs`. This workflow organizes that behavior under Legato so the assistant can later be routed through the same workflow/debug/retry conventions as `resume`.

## Stages

- `answer`: answer one user question using caller-provided diagnosis context and optional conversation history.

## Input

Use `--workflow-stage-input` for structured runtime context:

```json
{
  "question": "优先补哪项能力？",
  "diagnosis": {
    "ability_profile": {},
    "matching_result": {},
    "path_plan": {}
  },
  "history": [
    { "role": "user", "content": "首位岗位为什么匹配？" },
    { "role": "assistant", "content": "..." }
  ]
}
```

The source file is treated as optional source context. For frontend use, pass the real diagnosis in `diagnosis` and a small `.md` placeholder as source.

## Output

```json
{
  "chat": {
    "answer": "",
    "conclusion": "",
    "actions": [],
    "evidence_refs": [],
    "missing_evidence": [],
    "confidence": 0.7
  }
}
```

## CLI

```sh
legato chat.md --target chat --workflow chat --workflow-stage answer \
  --workflow-stage-input /path/to/chat-input.json \
  --presto-url http://127.0.0.1:8080 \
  --debug
```

## Design Notes

- The workflow does not run resume parsing. It consumes the already-built diagnosis context.
- It uses Presto for generation and JSON retry, matching the `resume` workflow style.
- It should not fabricate profile facts. Missing context is returned through `missing_evidence`.
