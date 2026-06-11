from __future__ import annotations

import json
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

from .formatter import PrestoFormatter, extract_json_object
from .model_routing import load_model_route


CHAT_LIST_FIELDS = ("actions", "evidence_refs", "missing_evidence")
UI_INTENT_MODES = {"none", "show_schema", "update_result"}
UI_INTENT_TARGETS = {
    "none",
    "basic",
    "education",
    "awards",
    "experiences",
    "profile_radar",
    "matching",
    "path_plan",
    "top_jobs",
    "job_recommendations",
}
UI_PATCH_OPS = {"add", "replace", "remove"}


@dataclass(frozen=True)
class WorkflowFormatResult:
    data: dict[str, Any]
    formatter: str
    warnings: list[str]
    debug: dict[str, Any]


class ChatWorkflowFormatter:
    """Presto-backed answer stage for the Growth Lens assistant."""

    def __init__(
        self,
        *,
        presto_url: str | None = None,
        timeout_seconds: float = 30,
        max_retries: int = 3,
        stage_input: dict[str, Any] | None = None,
    ) -> None:
        if max_retries < 1:
            raise ValueError("max_retries must be >= 1")
        self.presto = PrestoFormatter(presto_url, timeout_seconds=timeout_seconds)
        self.max_retries = max_retries
        self.stage_input = stage_input or {}
        self.prompts_dir = Path(__file__).resolve().parents[1] / "workflows" / "chat" / "prompts"
        self.common_prompt = self._read_prompt("common.md")
        self.retry_prompt = self._read_prompt("retry_json.md")
        self.route = load_model_route(Path(__file__))

    def format(self, source_context: str) -> WorkflowFormatResult:
        return self.format_stage(source_context, "answer")

    def format_stage(self, source_context: str, stage: str) -> WorkflowFormatResult:
        if stage != "answer":
            raise ValueError(f"unsupported chat workflow stage {stage!r}")
        started = time.perf_counter()
        result = self._run_answer_with_retry(source_context)
        total_ms = int((time.perf_counter() - started) * 1000)
        return WorkflowFormatResult(
            data={"chat": result["data"]},
            formatter="presto_chat_workflow_answer",
            warnings=[],
            debug=self._debug_envelope({"answer": result}, total_ms),
        )

    def _run_answer_with_retry(self, source_context: str) -> dict[str, Any]:
        started = time.perf_counter()
        input_context = self._input_context(source_context)
        previous_output = ""
        last_error = ""
        for attempt in range(1, self.max_retries + 1):
            prompt = self._answer_prompt(input_context)
            if attempt > 1:
                prompt += "\n\n" + self._retry_block(last_error, previous_output)
            call_started = time.perf_counter()
            try:
                output = self._call_presto(prompt, "answer")
                call_ms = int((time.perf_counter() - call_started) * 1000)
                previous_output = output
                data = extract_json_object(output)
                self._validate_answer(data)
                return {
                    "data": normalize_chat_answer(data),
                    "attempts": attempt,
                    "retry_count": attempt - 1,
                    "elapsed_ms": int((time.perf_counter() - started) * 1000),
                    "last_call_ms": call_ms,
                    "input": input_context["debug"],
                }
            except Exception as exc:
                last_error = str(exc)
        raise RuntimeError(f"chat answer failed after {self.max_retries} attempts: {last_error}")

    def _input_context(self, source_context: str) -> dict[str, Any]:
        question = first_string(
            self.stage_input,
            "question",
            "prompt",
            "message",
            "user_prompt",
            "input",
        )
        if not question:
            question = source_context.strip()

        diagnosis = object_value(self.stage_input.get("diagnosis"))
        context = self.stage_input.get("context")
        if not diagnosis and isinstance(context, dict):
            diagnosis = context

        history = normalize_history(self.stage_input.get("history"))
        ui_schema_catalog = object_value(self.stage_input.get("ui_schema_catalog"))
        extra_context = "" if question == source_context.strip() else source_context.strip()
        return {
            "question": question[:4000],
            "diagnosis_context": diagnosis,
            "conversation_history": history[-12:],
            "ui_schema_catalog": ui_schema_catalog,
            "source_context": extra_context[:20000],
            "debug": {
                "question_chars": len(question),
                "diagnosis_keys": sorted(diagnosis.keys()),
                "history_count": len(history),
                "ui_schema_targets": sorted(ui_schema_catalog.keys()),
                "source_context_chars": len(extra_context),
            },
        }

    def _answer_prompt(self, input_context: dict[str, Any]) -> str:
        prompt = self._read_prompt("answer.md")
        return (
            prompt.replace("{{common}}", self.common_prompt)
            .replace("{{question}}", input_context["question"])
            .replace("{{diagnosis_context}}", compact_json(input_context["diagnosis_context"]))
            .replace("{{conversation_history}}", compact_json(input_context["conversation_history"]))
            .replace("{{ui_schema_catalog}}", compact_json(input_context["ui_schema_catalog"]))
            .replace("{{source_context}}", input_context["source_context"])
        )

    def _retry_block(self, error: str, previous_output: str) -> str:
        return self.retry_prompt.replace("{{error}}", error).replace("{{previous_output}}", previous_output)

    def _call_presto(self, prompt: str, group: str) -> str:
        session = self.presto._request(
            "POST",
            "/sessions",
            {"metadata": {"app": "legato", "workflow": "chat", "group": group}},
        )
        run = self.presto._request("POST", f"/sessions/{session['id']}/runs", {"message": prompt})
        if run.get("error"):
            raise RuntimeError(f"presto run failed: {run['error']}")
        return run.get("output") or ""

    def _read_prompt(self, name: str) -> str:
        return (self.prompts_dir / name).read_text(encoding="utf-8").strip()

    def _validate_answer(self, data: dict[str, Any]) -> None:
        if not isinstance(data, dict):
            raise ValueError("chat answer must be a JSON object")
        for key in ("answer", "conclusion"):
            if not isinstance(data.get(key), str):
                raise ValueError(f"chat answer missing string {key!r}")
        for key in CHAT_LIST_FIELDS:
            if not isinstance(data.get(key), list):
                raise ValueError(f"chat answer missing array {key!r}")
            if not all(isinstance(item, str) for item in data[key]):
                raise ValueError(f"chat answer {key!r} must contain strings")
        confidence = data.get("confidence")
        if not isinstance(confidence, (int, float)) or isinstance(confidence, bool):
            raise ValueError("chat answer missing numeric confidence")
        if confidence < 0 or confidence > 1:
            raise ValueError("chat answer confidence must be between 0 and 1")
        if "ui_intent" in data:
            validate_ui_intent(data["ui_intent"])

    def _debug_envelope(self, results: dict[str, dict[str, Any]], total_ms: int) -> dict[str, Any]:
        route = {
            "presto_url": self.presto.base_url,
            "route": self.route.route if self.route else "",
            "provider": self.route.provider if self.route else "",
            "endpoint": self.route.base_url if self.route else "",
            "model": self.route.model if self.route else "",
        }
        agents = {
            group: {
                "attempts": result["attempts"],
                "retry_count": result["retry_count"],
                "elapsed_ms": result["elapsed_ms"],
                "last_call_ms": result["last_call_ms"],
                "model": route["model"],
                "endpoint": route["endpoint"],
                "input": result.get("input", {}),
            }
            for group, result in results.items()
        }
        return {
            "route": route,
            "agents": agents,
            "chain": [
                {"stage": f"{group}_agent", "elapsed_ms": result["elapsed_ms"], "attempts": result["attempts"]}
                for group, result in results.items()
            ]
            + [{"stage": "formatting_total", "elapsed_ms": total_ms}],
            "max_retries": self.max_retries,
        }


def normalize_chat_answer(data: dict[str, Any]) -> dict[str, Any]:
    normalized = {
        "answer": str(data.get("answer", "")).strip(),
        "conclusion": str(data.get("conclusion", "")).strip(),
        "actions": clean_string_list(data.get("actions")),
        "evidence_refs": clean_string_list(data.get("evidence_refs")),
        "missing_evidence": clean_string_list(data.get("missing_evidence")),
        "confidence": float(data.get("confidence", 0)),
    }
    if "ui_intent" in data:
        normalized["ui_intent"] = normalize_ui_intent(data["ui_intent"])
    return normalized


def validate_ui_intent(value: Any) -> None:
    if not isinstance(value, dict):
        raise ValueError("chat ui_intent must be a JSON object")
    mode = value.get("mode")
    target = value.get("target")
    if mode not in UI_INTENT_MODES:
        raise ValueError("chat ui_intent mode is invalid")
    if target not in UI_INTENT_TARGETS:
        raise ValueError("chat ui_intent target is invalid")
    if not isinstance(value.get("patches"), list):
        raise ValueError("chat ui_intent patches must be an array")
    if not isinstance(value.get("schema"), dict):
        raise ValueError("chat ui_intent schema must be an object")
    if not isinstance(value.get("summary"), str):
        raise ValueError("chat ui_intent summary must be a string")
    for patch in value["patches"]:
        if not isinstance(patch, dict):
            raise ValueError("chat ui_intent patch must be an object")
        if patch.get("op") not in UI_PATCH_OPS:
            raise ValueError("chat ui_intent patch op is invalid")
        path = patch.get("path")
        if not isinstance(path, str) or not path.startswith("/"):
            raise ValueError("chat ui_intent patch path is invalid")
        if patch["op"] in {"add", "replace"} and "value" not in patch:
            raise ValueError("chat ui_intent patch value is required")


def normalize_ui_intent(value: Any) -> dict[str, Any]:
    validate_ui_intent(value)
    patches: list[dict[str, Any]] = []
    for patch in value["patches"]:
        normalized_patch = {
            "op": str(patch.get("op", "")).strip(),
            "path": str(patch.get("path", "")).strip(),
        }
        if "value" in patch and normalized_patch["op"] != "remove":
            normalized_patch["value"] = patch["value"]
        patches.append(normalized_patch)
    return {
        "mode": str(value.get("mode", "none")).strip(),
        "target": str(value.get("target", "none")).strip(),
        "patches": patches[:40],
        "schema": value.get("schema") if isinstance(value.get("schema"), dict) else {},
        "summary": str(value.get("summary", "")).strip()[:500],
    }


def normalize_history(value: Any) -> list[dict[str, str]]:
    if not isinstance(value, list):
        return []
    history: list[dict[str, str]] = []
    for item in value:
        if not isinstance(item, dict):
            continue
        role = str(item.get("role") or "").strip()
        content = str(item.get("content") or "").strip()
        if role not in {"user", "assistant", "system"} or not content:
            continue
        history.append({"role": role, "content": content[:2000]})
    return history


def first_string(source: dict[str, Any], *keys: str) -> str:
    for key in keys:
        value = source.get(key)
        if isinstance(value, str) and value.strip():
            return value.strip()
    return ""


def object_value(value: Any) -> dict[str, Any]:
    return value if isinstance(value, dict) else {}


def clean_string_list(value: Any) -> list[str]:
    if not isinstance(value, list):
        return []
    return [str(item).strip() for item in value if str(item).strip()]


def compact_json(value: Any) -> str:
    return json.dumps(value, ensure_ascii=False, separators=(",", ":"))
