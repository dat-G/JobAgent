# Chat Workflow

`chat` is the Legato workflow wrapper for the Growth Lens AI assistant.

Frontend chat calls the Go backend `/api/chat`, which routes through this Legato workflow and then Presto. This keeps assistant behavior under the same workflow/debug/retry conventions as `resume`.

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
  ],
  "ui_schema_catalog": {
    "path_plan": {
      "roots": ["/path_plan"],
      "schema": {},
      "current_value": {}
    }
  }
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
    "confidence": 0.7,
    "ui_intent": {
      "mode": "none",
      "target": "none",
      "patches": [],
      "schema": {},
      "summary": ""
    }
  }
}
```

`ui_intent` is optional and is used only for interactive result editing:

- `mode: "show_schema"` returns the requested region schema from `ui_schema_catalog`.
- `mode: "update_result"` returns minimal JSON Patch operations. Patch paths can be full diagnosis paths or target-relative paths; the frontend normalizes them and validates that every path stays under the target region root before mutating its local diagnosis state.
- Factual extraction fields should only be patched when the user explicitly supplies the corrected value.

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
- UI edits are data patches, not DOM edits. The frontend re-renders from the patched diagnosis object.
