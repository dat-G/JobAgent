from __future__ import annotations

import json
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from typing import Callable


FieldRunner = Callable[[str, str], str]


class JsonRetryError(RuntimeError):
    pass


@dataclass(frozen=True)
class FieldResult:
    field: str
    value: str | int
    attempts: int


class ResumeWorkflow:
    fields = ("name", "birth_year")

    def __init__(self, runner: FieldRunner, *, max_retries: int = 5, max_workers: int = 8) -> None:
        if max_retries < 1:
            raise ValueError("max_retries must be >= 1")
        if max_workers < 1:
            raise ValueError("max_workers must be >= 1")
        self.runner = runner
        self.max_retries = max_retries
        self.max_workers = max_workers

    def run(self, resume_text: str) -> dict[str, str | int]:
        workers = min(self.max_workers, len(self.fields))
        with ThreadPoolExecutor(max_workers=workers) as executor:
            futures = {
                field: executor.submit(self._extract_field, field, resume_text)
                for field in self.fields
            }
            results = {field: future.result() for field, future in futures.items()}
        return {field: result.value for field, result in results.items()}

    def run_json(self, resume_text: str) -> str:
        return json.dumps(self.run(resume_text), ensure_ascii=False, separators=(",", ":"))

    def _extract_field(self, field: str, resume_text: str) -> FieldResult:
        last_error: Exception | None = None
        for attempt in range(1, self.max_retries + 1):
            output = self.runner(field, resume_text)
            try:
                payload = json.loads(output)
                if not isinstance(payload, dict):
                    raise ValueError("field output must be a JSON object")
                if field not in payload:
                    raise ValueError(f"field output missing {field!r}")
                return FieldResult(field=field, value=payload[field], attempts=attempt)
            except (json.JSONDecodeError, ValueError, TypeError) as exc:
                last_error = exc
        raise JsonRetryError(
            f"{field} did not return valid JSON after {self.max_retries} attempts"
        ) from last_error

