from __future__ import annotations

import json
from dataclasses import dataclass
from typing import Any, Callable


ChatRunner = Callable[[dict[str, Any]], str]


class JsonRetryError(RuntimeError):
    pass


@dataclass(frozen=True)
class ChatResult:
    data: dict[str, Any]
    attempts: int


class ChatWorkflow:
    """Small retryable JSON workflow used by tests and local adapters."""

    def __init__(self, runner: ChatRunner, *, max_retries: int = 3) -> None:
        if max_retries < 1:
            raise ValueError("max_retries must be >= 1")
        self.runner = runner
        self.max_retries = max_retries

    def run(self, context: dict[str, Any]) -> dict[str, Any]:
        return self.run_with_meta(context).data

    def run_with_meta(self, context: dict[str, Any]) -> ChatResult:
        last_error: Exception | None = None
        for attempt in range(1, self.max_retries + 1):
            output = self.runner(context)
            try:
                payload = json.loads(output)
                if not isinstance(payload, dict):
                    raise ValueError("chat output must be a JSON object")
                validate_chat_payload(payload)
                return ChatResult(data=payload, attempts=attempt)
            except (json.JSONDecodeError, ValueError, TypeError) as exc:
                last_error = exc
        raise JsonRetryError(f"chat did not return valid JSON after {self.max_retries} attempts") from last_error


def validate_chat_payload(payload: dict[str, Any]) -> None:
    for key in ("answer", "conclusion"):
        if not isinstance(payload.get(key), str):
            raise ValueError(f"missing string {key}")
    for key in ("actions", "evidence_refs", "missing_evidence"):
        value = payload.get(key)
        if not isinstance(value, list) or not all(isinstance(item, str) for item in value):
            raise ValueError(f"missing string array {key}")
    confidence = payload.get("confidence")
    if not isinstance(confidence, (int, float)) or isinstance(confidence, bool):
        raise ValueError("missing numeric confidence")
    if confidence < 0 or confidence > 1:
        raise ValueError("confidence must be between 0 and 1")
