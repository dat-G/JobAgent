from __future__ import annotations

import json
import re
import time
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from functools import lru_cache
from pathlib import Path
from typing import Any

from .formatter import PrestoFormatter, extract_json_object
from .model_routing import load_model_route

CERT_AWARD_KEYWORDS = (
    "证书",
    "考试",
    "竞赛",
    "比赛",
    "奖",
    "一等奖",
    "二等奖",
    "三等奖",
    "名次",
    "排名",
    "六级",
    "四级",
    "CET",
    "NOIP",
    "建模",
    "奥林匹克",
    "决赛",
    "六强",
    "荣誉",
    "奖学金",
)
EXPERIENCE_GROUPS = ("experience_work_project", "experience_contest", "experience_campus")
EXPERIENCE_WORK_PROJECT_KEYWORDS = (
    "工作经历",
    "科研与项目经历",
    "项目经历",
    "实习",
    "实验室",
    "项目",
    "高新兴",
    "GNSS",
    "DBSCAN",
)
EXPERIENCE_CONTEST_KEYWORDS = (
    "获奖情况",
    "队长",
    "竞赛",
    "比赛",
    "总决赛",
    "建模",
    "奥林匹克",
)
EXPERIENCE_CAMPUS_KEYWORDS = (
    "在校经历",
    "社团组织",
    "副会长",
    "协会",
    "活动",
)
LOW_VALUE_HONOR_SIGNALS = (
    "优秀学生干部",
    "三好学生",
    "三好学生标兵",
    "优秀学生",
    "三八红旗手",
    "奖学金",
    "计算机二级",
    "计算机三级",
    "计算机四级",
    "WPS证书",
    "办公软件",
    "英语四级",
    "英语六级",
    "CET4",
    "CET6",
)
MIN_CERT_AWARD_SLICE_CHARS = 80
MIN_EXPERIENCE_SLICE_CHARS = 160
CONTEXT_LINES = 4
ENGLISH_EXAM_RE = re.compile(r"(全国大学英语(?:四|六)级考试)")
ORPHAN_SCORE_RE = re.compile(r"^(\d{3})\s*分$")
EDUCATION_DATE_RE = re.compile(
    r"(?P<start>20\d{2})[.年/-]?\d{0,2}\s*[-~至]\s*(?P<end>20\d{2})[.年/-]?\d{0,2}"
)
INTERNSHIP_LINE_RE = re.compile(
    r"(?P<date>20\d{2}[.年/-]?\d{0,2}\s*[-~至]\s*20\d{2}[.年/-]?\d{0,2})\s+"
    r"(?P<body>.+?(?:实习生|实习|工程师|标注|开发|测试).*)"
)
REVERSED_INTERNSHIP_LINE_RE = re.compile(
    r"(?P<body>.+?(?:实习生|实习|工程师|标注|开发|测试).*)\s+"
    r"(?P<date>20\d{2}[.年/-]?\d{0,2}\s*[-~至]\s*20\d{2}[.年/-]?\d{0,2})"
)
RANKING_CACHE = (
    Path(__file__).resolve().parents[1]
    / "workflows"
    / "resume"
    / "cache"
    / "ruanke_china_university_ranking_2026_structured.json"
)


@dataclass(frozen=True)
class WorkflowFormatResult:
    data: dict[str, Any]
    formatter: str
    warnings: list[str]
    debug: dict[str, Any]


class ResumeWorkflowFormatter:
    groups = ("profile", "certifications_awards")

    def __init__(
        self,
        *,
        presto_url: str | None = None,
        timeout_seconds: float = 30,
        max_retries: int = 5,
        max_workers: int = 8,
        combine_agents: bool = False,
    ) -> None:
        self.presto = PrestoFormatter(presto_url, timeout_seconds=timeout_seconds)
        self.max_retries = max_retries
        self.max_workers = max_workers
        self.combine_agents = combine_agents
        self.prompts_dir = Path(__file__).resolve().parents[1] / "workflows" / "resume" / "prompts"
        self.common_prompt = self._read_prompt("common.md")
        self.retry_prompt = self._read_prompt("retry_json.md")
        self.route = load_model_route(Path(__file__))

    def format(self, resume_text: str) -> WorkflowFormatResult:
        if self.combine_agents:
            return self._format_combined(resume_text)

        started = time.perf_counter()
        with ThreadPoolExecutor(max_workers=min(self.max_workers, len(self.groups))) as executor:
            futures = {
                group: executor.submit(self._run_group_with_retry, group, resume_text)
                for group in self.groups
            }
            results = {group: future.result() for group, future in futures.items()}

        education_tags_started = time.perf_counter()
        education = enrich_education(results["profile"]["data"].get("education", []), resume_text)
        education_tags_ms = int((time.perf_counter() - education_tags_started) * 1000)
        experience_started = time.perf_counter()
        experience = build_local_experience(
            resume_text,
            results["certifications_awards"]["data"].get("certifications_awards", []),
        )
        experience_ms = int((time.perf_counter() - experience_started) * 1000)
        merge_started = time.perf_counter()
        data = {
            "identity": results["profile"]["data"].get("identity", {}),
            "education": education,
            "certifications_awards": results["certifications_awards"]["data"].get("certifications_awards", []),
            "experience": experience,
        }
        merge_ms = int((time.perf_counter() - merge_started) * 1000)
        total_ms = int((time.perf_counter() - started) * 1000)
        return WorkflowFormatResult(
            data=data,
            formatter="presto_resume_workflow",
            warnings=[],
            debug=self._debug_envelope(
                results,
                merge_ms,
                total_ms,
                local_stages=[
                    {"stage": "education_degree_inference_agent", "elapsed_ms": education_tags_ms},
                    {"stage": "experience_local", "elapsed_ms": experience_ms},
                ],
            ),
        )

    def format_stage(self, resume_text: str, stage: str) -> WorkflowFormatResult:
        started = time.perf_counter()
        if stage == "profile":
            result = self._run_group_with_retry("profile", resume_text)
            education_started = time.perf_counter()
            education = enrich_education(result["data"].get("education", []), resume_text)
            education_ms = int((time.perf_counter() - education_started) * 1000)
            total_ms = int((time.perf_counter() - started) * 1000)
            return WorkflowFormatResult(
                data={
                    "identity": result["data"].get("identity", {}),
                    "education": education,
                },
                formatter="presto_resume_workflow_profile",
                warnings=[],
                debug=self._debug_envelope(
                    {"profile": result},
                    merge_ms=0,
                    total_ms=total_ms,
                    local_stages=[{"stage": "education_degree_inference_agent", "elapsed_ms": education_ms}],
                ),
            )
        if stage == "certifications_awards":
            result = self._run_group_with_retry("certifications_awards", resume_text)
            total_ms = int((time.perf_counter() - started) * 1000)
            return WorkflowFormatResult(
                data={"certifications_awards": result["data"].get("certifications_awards", [])},
                formatter="presto_resume_workflow_certifications_awards",
                warnings=[],
                debug=self._debug_envelope({"certifications_awards": result}, merge_ms=0, total_ms=total_ms),
            )
        if stage == "item_benchmark":
            items_result = self._run_group_with_retry("certifications_awards", resume_text)
            benchmark_started = time.perf_counter()
            benchmark_result = self._run_item_benchmarks(
                resume_text,
                items_result["data"].get("certifications_awards", []),
            )
            benchmark_ms = int((time.perf_counter() - benchmark_started) * 1000)
            total_ms = int((time.perf_counter() - started) * 1000)
            return WorkflowFormatResult(
                data={"item_benchmark": benchmark_result["items"]},
                formatter="presto_resume_workflow_item_benchmark",
                warnings=[],
                debug=self._debug_envelope(
                    {"certifications_awards": items_result, **benchmark_result["agents"]},
                    merge_ms=0,
                    total_ms=total_ms,
                    local_stages=[{"stage": "item_benchmark_merge", "elapsed_ms": benchmark_ms}],
                ),
            )
        if stage == "experience":
            experience_started = time.perf_counter()
            experience = build_local_experience(resume_text, [])
            experience_ms = int((time.perf_counter() - experience_started) * 1000)
            total_ms = int((time.perf_counter() - started) * 1000)
            return WorkflowFormatResult(
                data={"experience": experience},
                formatter="resume_workflow_local_experience",
                warnings=[],
                debug=self._debug_envelope(
                    {},
                    merge_ms=0,
                    total_ms=total_ms,
                    local_stages=[{"stage": "experience_local", "elapsed_ms": experience_ms}],
                ),
            )
        if stage == "experience_hybrid":
            local_started = time.perf_counter()
            local_experience = build_local_experience(resume_text, [])
            local_ms = int((time.perf_counter() - local_started) * 1000)
            result = self._run_experience_refine_with_retry(resume_text, local_experience)
            filter_started = time.perf_counter()
            llm_experience = filter_llm_experience_candidates(result["data"].get("experience", []))
            experience = merge_experience_candidates(local_experience, llm_experience)
            filter_ms = int((time.perf_counter() - filter_started) * 1000)
            total_ms = int((time.perf_counter() - started) * 1000)
            return WorkflowFormatResult(
                data={"experience": experience},
                formatter="presto_resume_workflow_experience_hybrid",
                warnings=[],
                debug=self._debug_envelope(
                    {"experience_refine": result},
                    merge_ms=0,
                    total_ms=total_ms,
                    local_stages=[
                        {"stage": "experience_local_recall", "elapsed_ms": local_ms},
                        {"stage": "experience_candidate_filter", "elapsed_ms": filter_ms},
                    ],
                ),
            )
        raise ValueError(f"unsupported resume workflow stage {stage!r}")

    def _format_combined(self, resume_text: str) -> WorkflowFormatResult:
        started = time.perf_counter()
        result = self._run_group_with_retry("combined", resume_text)
        data = {
            "identity": result["data"].get("identity", {}),
            "education": enrich_education(result["data"].get("education", []), resume_text),
            "certifications_awards": result["data"].get("certifications_awards", []),
            "experience": result["data"].get("experience", []),
        }
        total_ms = int((time.perf_counter() - started) * 1000)
        debug = self._debug_envelope({"combined": result}, merge_ms=0, total_ms=total_ms)
        return WorkflowFormatResult(
            data=data,
            formatter="presto_resume_workflow_combined",
            warnings=[],
            debug=debug,
        )

    def _run_group_with_retry(self, group: str, resume_text: str) -> dict[str, Any]:
        started = time.perf_counter()
        agent_input, input_debug = self._input_for_group(group, resume_text)
        previous_output = ""
        last_error = ""
        for attempt in range(1, self.max_retries + 1):
            prompt = self._group_prompt(group, agent_input)
            if attempt > 1:
                prompt += "\n\n" + self._retry_block(last_error, previous_output)
            call_started = time.perf_counter()
            output = self._call_presto(prompt, group)
            call_ms = int((time.perf_counter() - call_started) * 1000)
            previous_output = output
            try:
                data = extract_json_object(output)
                self._validate_group(group, data)
                return {
                    "data": data,
                    "attempts": attempt,
                    "retry_count": attempt - 1,
                    "elapsed_ms": int((time.perf_counter() - started) * 1000),
                    "last_call_ms": call_ms,
                    "input": input_debug,
                }
            except Exception as exc:
                last_error = str(exc)
        raise RuntimeError(f"{group} did not return valid JSON after {self.max_retries} attempts: {last_error}")

    def _call_presto(self, prompt: str, group: str) -> str:
        session = self.presto._request("POST", "/sessions", {"metadata": {"app": "legato", "workflow": "resume", "group": group}})
        run = self.presto._request("POST", f"/sessions/{session['id']}/runs", {"message": prompt})
        if run.get("error"):
            raise RuntimeError(f"presto run failed: {run['error']}")
        return run.get("output") or ""

    def _run_experience_refine_with_retry(self, resume_text: str, local_experience: list[dict[str, Any]]) -> dict[str, Any]:
        started = time.perf_counter()
        previous_output = ""
        last_error = ""
        input_debug = {
            "mode": "local_candidates_plus_full",
            "input_chars": len(resume_text),
            "original_chars": len(resume_text),
            "local_candidate_count": len(local_experience),
            "fallback_full_text": False,
        }
        for attempt in range(1, self.max_retries + 1):
            prompt = self._experience_refine_prompt(resume_text, local_experience)
            if attempt > 1:
                prompt += "\n\n" + self._retry_block(last_error, previous_output)
            call_started = time.perf_counter()
            output = self._call_presto(prompt, "experience_refine")
            call_ms = int((time.perf_counter() - call_started) * 1000)
            previous_output = output
            try:
                data = extract_json_object(output)
                self._validate_group("experience", data)
                return {
                    "data": data,
                    "attempts": attempt,
                    "retry_count": attempt - 1,
                    "elapsed_ms": int((time.perf_counter() - started) * 1000),
                    "last_call_ms": call_ms,
                    "input": input_debug,
                }
            except Exception as exc:
                last_error = str(exc)
        raise RuntimeError(f"experience_refine did not return valid JSON after {self.max_retries} attempts: {last_error}")

    def _experience_refine_prompt(self, resume_text: str, local_experience: list[dict[str, Any]]) -> str:
        prompt = self._read_prompt("experience_refine.md")
        local_candidates = json.dumps({"experience": local_experience}, ensure_ascii=False, separators=(",", ":"))
        return (
            prompt.replace("{{common}}", self.common_prompt)
            .replace("{{local_candidates}}", local_candidates)
            .replace("{{resume_text}}", resume_text)
        )

    def _run_item_benchmarks(self, resume_text: str, items: list[dict[str, Any]]) -> dict[str, Any]:
        benchmarks: list[dict[str, Any] | None] = [None] * len(items)
        agents: dict[str, dict[str, Any]] = {}
        if not items:
            return {"items": [], "agents": agents}

        with ThreadPoolExecutor(max_workers=min(self.max_workers, max(1, len(items)))) as executor:
            futures = {
                executor.submit(self._run_item_benchmark_with_retry, resume_text, item, index): index
                for index, item in enumerate(items)
            }
            for future, index in futures.items():
                result = future.result()
                benchmarks[index] = result["item"]
                agents[f"item_benchmark_{index}"] = result["agent"]
        return {"items": [item for item in benchmarks if item is not None], "agents": agents}

    def _run_item_benchmark_with_retry(self, resume_text: str, item: dict[str, Any], index: int) -> dict[str, Any]:
        started = time.perf_counter()
        previous_output = ""
        last_error = ""
        prompt = self._item_benchmark_prompt(resume_text, item)
        input_debug = {
            "mode": "single_item_plus_context",
            "input_chars": len(prompt),
            "original_chars": len(resume_text),
            "item_index": index,
            "fallback_full_text": False,
        }
        for attempt in range(1, self.max_retries + 1):
            call_prompt = prompt
            if attempt > 1:
                call_prompt += "\n\n" + self._retry_block(last_error, previous_output)
            call_started = time.perf_counter()
            output = self._call_presto(call_prompt, "item_benchmark")
            call_ms = int((time.perf_counter() - call_started) * 1000)
            previous_output = output
            try:
                data = extract_json_object(output)
                benchmark = normalize_item_benchmark(item, data)
                validate_six_dim_scores(benchmark["scores"], "scores")
                return {
                    "item": benchmark,
                    "agent": {
                        "data": data,
                        "attempts": attempt,
                        "retry_count": attempt - 1,
                        "elapsed_ms": int((time.perf_counter() - started) * 1000),
                        "last_call_ms": call_ms,
                        "input": input_debug,
                    },
                }
            except Exception as exc:
                last_error = str(exc)
        fallback = local_item_benchmark(item)
        return {
            "item": fallback,
            "agent": {
                "data": {"item_benchmark": fallback},
                "attempts": self.max_retries,
                "retry_count": self.max_retries,
                "elapsed_ms": int((time.perf_counter() - started) * 1000),
                "last_call_ms": 0,
                "input": input_debug,
                "fallback": True,
                "last_error": last_error,
            },
        }

    def _item_benchmark_prompt(self, resume_text: str, item: dict[str, Any]) -> str:
        prompt = self._read_prompt("item_benchmark.md")
        context = compact_education_context(resume_text)
        item_text = json.dumps(item, ensure_ascii=False, separators=(",", ":"))
        return prompt.replace("{{education_context}}", context).replace("{{item}}", item_text)

    def _group_prompt(self, group: str, resume_text: str) -> str:
        prompt = self._read_prompt(f"{group}.md")
        return prompt.replace("{{common}}", self.common_prompt).replace("{{resume_text}}", resume_text)

    def _input_for_group(self, group: str, resume_text: str) -> tuple[str, dict[str, Any]]:
        if group in EXPERIENCE_GROUPS:
            sliced = slice_experience_text(resume_text, group)
            use_slice = len(sliced.strip()) >= MIN_EXPERIENCE_SLICE_CHARS
            agent_input = sliced if use_slice else resume_text
            return agent_input, {
                "mode": "keyword_slice" if use_slice else "full",
                "input_chars": len(agent_input),
                "original_chars": len(resume_text),
                "candidate_chars": len(sliced),
                "fallback_full_text": not use_slice,
            }
        if group != "certifications_awards":
            return resume_text, {
                "mode": "full",
                "input_chars": len(resume_text),
                "original_chars": len(resume_text),
                "fallback_full_text": False,
            }
        sliced = slice_certifications_awards_text(resume_text)
        repaired = repair_orphan_certification_scores(resume_text)
        if repaired:
            sliced = "\n".join(part for part in (sliced, repaired) if part)
        use_slice = len(sliced.strip()) >= MIN_CERT_AWARD_SLICE_CHARS
        agent_input = sliced if use_slice else resume_text
        return agent_input, {
            "mode": "keyword_slice" if use_slice else "full",
            "input_chars": len(agent_input),
            "original_chars": len(resume_text),
            "candidate_chars": len(sliced),
            "repaired_chars": len(repaired),
            "fallback_full_text": not use_slice,
        }

    def _retry_block(self, error: str, previous_output: str) -> str:
        return self.retry_prompt.replace("{{error}}", error).replace("{{previous_output}}", previous_output)

    def _read_prompt(self, name: str) -> str:
        return (self.prompts_dir / name).read_text(encoding="utf-8").strip()

    def _validate_group(self, group: str, data: dict[str, Any]) -> None:
        if group == "profile":
            identity = data.get("identity")
            if not isinstance(identity, dict):
                raise ValueError("identity must be an object")
            for key in ("name", "birth_year", "sex"):
                if key not in identity:
                    raise ValueError(f"identity missing {key}")
            if not isinstance(data.get("education"), list):
                raise ValueError("education must be an array")
            return
        if group == "certifications_awards":
            items = data.get("certifications_awards")
            if not isinstance(items, list):
                raise ValueError("certifications_awards must be an array")
            for index, item in enumerate(items):
                if not isinstance(item, dict):
                    raise ValueError(f"certifications_awards[{index}] must be an object")
                for key in ("name", "result"):
                    if key not in item:
                        raise ValueError(f"certifications_awards[{index}] missing {key}")
            return
        if group == "item_benchmark":
            items = data.get("item_benchmark")
            if not isinstance(items, list):
                raise ValueError("item_benchmark must be an array")
            for index, item in enumerate(items):
                if not isinstance(item, dict):
                    raise ValueError(f"item_benchmark[{index}] must be an object")
                for key in ("item", "scores", "impact_factor"):
                    if key not in item:
                        raise ValueError(f"item_benchmark[{index}] missing {key}")
                validate_six_dim_scores(item["scores"], f"item_benchmark[{index}].scores")
                impact = item["impact_factor"]
                if not isinstance(impact, (int, float)) or isinstance(impact, bool) or impact < 0 or impact > 10:
                    raise ValueError(f"item_benchmark[{index}].impact_factor must be between 0 and 10")
            return
        if group in EXPERIENCE_GROUPS or group == "experience":
            items = data.get("experience")
            if not isinstance(items, list):
                raise ValueError("experience must be an array")
            for index, item in enumerate(items):
                if not isinstance(item, dict):
                    raise ValueError(f"experience[{index}] must be an object")
                for key in ("type", "role", "contribution", "level"):
                    if key not in item:
                        raise ValueError(f"experience[{index}] missing {key}")
                level = item["level"]
                if not isinstance(level, (int, float)) or isinstance(level, bool):
                    raise ValueError(f"experience[{index}].level must be a number")
                if level < 0 or level > 10:
                    raise ValueError(f"experience[{index}].level must be between 0 and 10")
            return
        if group == "combined":
            self._validate_group("profile", data)
            self._validate_group("certifications_awards", data)
            self._validate_group("experience", data)
            return
        raise ValueError(f"unknown group {group}")

    def _debug_envelope(
        self,
        results: dict[str, dict[str, Any]],
        merge_ms: int,
        total_ms: int,
        *,
        local_stages: list[dict[str, Any]] | None = None,
    ) -> dict[str, Any]:
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
        chain = [
            {"stage": f"{group}_agent", "elapsed_ms": result["elapsed_ms"], "attempts": result["attempts"]}
            for group, result in results.items()
        ]
        chain.extend(local_stages or [])
        chain.append({"stage": "merge", "elapsed_ms": merge_ms})
        chain.append({"stage": "formatting_total", "elapsed_ms": total_ms})
        return {
            "route": route,
            "agents": agents,
            "chain": chain,
            "max_retries": self.max_retries,
            "max_workers": self.max_workers,
        }


def slice_certifications_awards_text(resume_text: str) -> str:
    return slice_text_by_keywords(resume_text, CERT_AWARD_KEYWORDS)


def slice_experience_text(resume_text: str, group: str = "experience_work_project") -> str:
    keyword_map = {
        "experience_work_project": EXPERIENCE_WORK_PROJECT_KEYWORDS,
        "experience_contest": EXPERIENCE_CONTEST_KEYWORDS,
        "experience_campus": EXPERIENCE_CAMPUS_KEYWORDS,
    }
    keywords = keyword_map.get(group, EXPERIENCE_WORK_PROJECT_KEYWORDS)
    return slice_text_by_keywords(resume_text, keywords)


def slice_text_by_keywords(resume_text: str, keywords: tuple[str, ...]) -> str:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    selected: set[int] = set()
    for index, line in enumerate(lines):
        if any(keyword.lower() in line.lower() for keyword in keywords):
            start = max(0, index - CONTEXT_LINES)
            end = min(len(lines), index + CONTEXT_LINES + 1)
            selected.update(range(start, end))
    if not selected:
        return ""
    return "\n".join(lines[index] for index in sorted(selected))


def repair_orphan_certification_scores(resume_text: str) -> str:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    exams: list[str] = []
    scores: list[str] = []
    for line in lines:
        match = ENGLISH_EXAM_RE.search(line)
        if match and not ORPHAN_SCORE_RE.search(line):
            exams.append(match.group(1))
        score_match = ORPHAN_SCORE_RE.match(line)
        if score_match:
            scores.append(f"{score_match.group(1)}分")

    if not exams or not scores:
        return ""
    pair_count = min(len(exams), len(scores))
    return "\n".join(f"{exams[index]} {scores[index]}" for index in range(pair_count))


def build_local_experience(resume_text: str, certifications_awards: list[dict[str, Any]]) -> list[dict[str, Any]]:
    _ = certifications_awards
    experiences: list[dict[str, Any]] = []

    experiences.extend(extract_internship_experiences(resume_text))
    experiences.extend(extract_project_experiences(resume_text))
    experiences.extend(extract_described_contest_experiences(resume_text))
    experiences.extend(extract_campus_role_experiences(resume_text))
    return dedupe_experiences(experiences)


def extract_internship_experiences(resume_text: str) -> list[dict[str, Any]]:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    experiences: list[dict[str, Any]] = []
    for index, line in enumerate(lines):
        match = INTERNSHIP_LINE_RE.search(line) or REVERSED_INTERNSHIP_LINE_RE.search(line)
        if not match:
            continue
        if not is_internship_body(match.group("body")):
            continue
        organization, role = split_experience_body(match.group("body"))
        contribution_context = "\n".join(lines[max(0, index - 30) : min(len(lines), index + 6)])
        scoring_context = "\n".join(lines[index : min(len(lines), index + 6)])
        contribution = summarize_internship_contribution(organization, role, contribution_context)
        experiences.append(
            {
                "type": "实习",
                "role": compose_experience_role(organization, role),
                "contribution": contribution,
                "level": score_internship_level(organization, scoring_context),
            }
        )
    return experiences


def split_experience_body(body: str) -> tuple[str, str]:
    normalized = body.replace("(", "（").replace(")", "）").strip()
    parts = re.split(r"\s+[-–—]\s+", normalized, maxsplit=1)
    if len(parts) == 2:
        return parts[0].strip(), parts[1].strip()
    tokens = normalized.split()
    if len(tokens) >= 2:
        return tokens[0], "".join(tokens[1:])
    if "高新兴科技集团股份有限公司" in normalized:
        return "高新兴科技集团股份有限公司", "前端开发实习生" if "前端开发实习生" in normalized else ""
    return normalized, ""


def is_internship_body(body: str) -> bool:
    return any(signal in body for signal in ("实习", "工程师", "标注"))


def summarize_internship_contribution(organization: str, role: str, context: str) -> str:
    if "MCP" in context and ("XML" in context or "xml" in context):
        return "MCP标注与XML修正"
    if any(signal in context for signal in ("免杀", "逆向", "漏洞挖掘", "二进制")):
        return "免杀平台开发与逆向漏洞挖掘"
    if "视频云平台" in context or "GoCloud" in context:
        return "视频云平台前端开发"
    if role:
        return role[:35]
    return "实习任务执行"


def score_internship_level(organization: str, context: str) -> int:
    top_orgs = ("字节跳动", "腾讯", "阿里", "百度", "华为", "美团", "京东")
    low_tech_work = any(signal in context for signal in ("标注", "审核", "改写", "判断回答", "XML标签", "XML 包裹"))
    engineering_work = any(signal in context for signal in ("开发", "设计", "实现", "调试", "系统", "平台", "算法", "数据库"))
    core_ownership = any(signal in context for signal in ("独立", "负责", "主导", "核心", "上线", "优化"))
    known_org = any(org in organization for org in top_orgs)

    if low_tech_work and not engineering_work:
        return 4 if known_org else 3
    if known_org and engineering_work and core_ownership:
        return 7
    if known_org and engineering_work:
        return 6
    if engineering_work and core_ownership:
        return 6
    if any(signal in context for signal in ("主要工作", "项目", "负责")):
        return 5
    return 5


def extract_project_experiences(resume_text: str) -> list[dict[str, Any]]:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    experiences: list[dict[str, Any]] = []
    experiences.extend(extract_dated_project_experiences(lines))
    experiences.extend(extract_bullet_project_experiences(lines))
    for index, line in enumerate(lines):
        if not (is_project_anchor(line) or is_research_description_anchor(line)):
            continue
        if line.startswith("#") or line in ("项目经历", "科研经历"):
            continue
        context = "\n".join(lines[index : min(len(lines), index + 8)])
        if not has_contribution_signal(context):
            continue
        role = extract_project_subject(context)
        if not role:
            continue
        experiences.append(
            {
                "type": "科研项目" if "实验室" in context or "科研" in context or "研究" in context else "项目",
                "role": role,
                "contribution": summarize_project_contribution(context),
                "level": score_project_level(context),
            }
        )
        break
    return experiences


def extract_bullet_project_experiences(lines: list[str]) -> list[dict[str, Any]]:
    experiences: list[dict[str, Any]] = []
    for line in lines:
        parsed = parse_bullet_project_line(line)
        if not parsed:
            continue
        role, contribution, level = parsed
        experiences.append(
            {
                "type": "项目",
                "role": role,
                "contribution": contribution,
                "level": level,
            }
        )
    return experiences


def parse_bullet_project_line(line: str) -> tuple[str, str, int] | None:
    cleaned = line.lstrip("·-* ").strip()
    match = re.search(r"对(?P<target>[^，。,；;\n]{2,24})进行(?P<activity>漏洞挖掘|模糊测试)", cleaned)
    if not match:
        return None
    target = match.group("target").strip()
    activity = match.group("activity").strip()
    role = f"{target} / {activity}"[:35]
    if "CVE" in cleaned or "RCE" in cleaned:
        contribution = "漏洞发现与CVE/RCE验证"
        level = 7
    elif "UAF" in cleaned or "DoS" in cleaned or "官方" in cleaned:
        contribution = "模糊测试漏洞发现与上报"
        level = 6
    else:
        contribution = f"{activity}与漏洞验证"
        level = 6
    return role, contribution, level


def extract_dated_project_experiences(lines: list[str]) -> list[dict[str, Any]]:
    experiences: list[dict[str, Any]] = []
    for index, line in enumerate(lines):
        parsed = parse_dated_project_line(line)
        if not parsed:
            continue
        subject, role = parsed
        if is_internship_body(f"{subject}{role}"):
            continue
        context = "\n".join(lines[index : min(len(lines), index + 5)])
        nearby = "\n".join(lines[max(0, index - 2) : min(len(lines), index + 5)])
        if not (
            any(signal in nearby for signal in ("科研经历", "项目经历", "研究项目", "科研项目"))
            or any(signal in role for signal in ("参与者", "负责人", "队长", "成员"))
        ):
            continue
        if not has_contribution_signal(context):
            continue
        experiences.append(
            {
                "type": "科研项目" if any(signal in nearby for signal in ("科研", "研究", "实验")) else "项目",
                "role": compose_experience_role(subject, role),
                "contribution": summarize_project_contribution(context),
                "level": score_project_level(context),
            }
        )
    return experiences


def parse_dated_project_line(line: str) -> tuple[str, str] | None:
    match = re.search(r"(?P<body>.+?)\s+20\d{2}[.年/-]?\d{0,2}\s*[-~至]\s*20\d{2}[.年/-]?\d{0,2}", line)
    if not match:
        return None
    body = match.group("body").strip()
    if any(skip in body for skip in ("大学", "学院", "本科", "硕士", "博士", "学士")):
        return None
    parts = re.split(r"\s+[-–—]\s+", body, maxsplit=1)
    if len(parts) == 2:
        return parts[0].strip(), parts[1].strip()
    return body, ""


def is_project_anchor(line: str) -> bool:
    if any(skip in line for skip in ("荣誉", "奖项", "证书", "技能", "教育背景", "主修课程", "公众号")):
        return False
    if is_descriptive_sentence(line):
        return False
    if "实验室" in line and not ("重点实验室" in line or len(line) <= 28):
        return False
    return any(signal in line for signal in ("实验室", "科研项目", "项目经历", "研究项目", "研究中"))


def is_research_description_anchor(line: str) -> bool:
    return "研究项目" in line and any(signal in line for signal in ("GNSS", "DBSCAN", "点云", "轨迹", "田路分割"))


def extract_described_contest_experiences(resume_text: str) -> list[dict[str, Any]]:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    experiences: list[dict[str, Any]] = []
    for index, line in enumerate(lines):
        if "多次参加" in line or "各种大小" in line:
            continue
        if "参加" not in line or not any(word in line for word in ("比赛", "竞赛", "大赛", "挑战赛")):
            continue
        context = "\n".join(lines[index : min(len(lines), index + 5)])
        if not has_contribution_signal(context):
            continue
        experiences.append(
            {
                "type": "比赛",
                "role": compose_experience_role(extract_event_name(context), extract_role(context)),
                "contribution": summarize_contest_contribution(context),
                "level": score_contest_context_level(context),
            }
        )
    experiences.extend(extract_contest_role_experiences(lines))
    return experiences


def extract_contest_role_experiences(lines: list[str]) -> list[dict[str, Any]]:
    experiences: list[dict[str, Any]] = []
    for index, line in enumerate(lines):
        if not any(signal in line for signal in ("出题人", "负责人", "队长")):
            continue
        if not any(signal in line for signal in ("赛", "比赛", "竞赛", "挑战赛", "CTF")):
            continue
        context = "\n".join(lines[max(0, index - 1) : min(len(lines), index + 3)])
        role = extract_role(context)
        experiences.append(
            {
                "type": "比赛",
                "role": compose_experience_role(extract_event_name(context), role),
                "contribution": summarize_contest_role_contribution(context, role),
                "level": score_contest_context_level(context),
            }
        )
    return experiences


def extract_campus_role_experiences(resume_text: str) -> list[dict[str, Any]]:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    experiences: list[dict[str, Any]] = []
    for index, line in enumerate(lines):
        if not any(role in line for role in ("会长", "部长", "负责人", "主席", "干部", "班长")):
            continue
        window_start = max(0, index - 12)
        context = "\n".join(lines[window_start : min(len(lines), index + 6)])
        if not any(org in context for org in ("协会", "社团", "学生会", "班级", "组织")):
            continue
        if not has_contribution_signal(context):
            continue
        experiences.append(
            {
                "type": "社团" if "协会" in context or "社团" in context else "任职",
                "role": compose_experience_role(extract_org_name(context), extract_role(context)),
                "contribution": summarize_campus_contribution(context),
                "level": 6 if any(signal in context for signal in ("组织", "负责", "带领", "赞助", "活动")) else 5,
            }
        )
    return experiences


def has_contribution_signal(context: str) -> bool:
    return any(
        signal in context
        for signal in (
            "主要工作",
            "负责",
            "独立",
            "开发",
            "修改",
            "添加",
            "判断",
            "提出",
            "制作",
            "分析",
            "组织",
            "带领",
            "协商",
            "实现",
            "完成",
            "验证",
            "测试",
            "发现",
        )
    )


def extract_role(context: str) -> str:
    for role in ("前端开发实习生", "安全研究实习生", "实习生", "出题人", "队长", "副会长", "会长", "部长", "负责人", "主席", "班长", "参与者"):
        if role in context:
            return role
    if "带领团队" in context:
        return "队长"
    return ""


def compose_experience_role(subject: str, role: str) -> str:
    subject = compact_role_part(subject)
    role = compact_role_part(role)
    if subject and role and subject not in role:
        return f"{subject} / {role}"[:35]
    return role or subject


def compact_role_part(value: str) -> str:
    return value.strip(" -–—,，。；;|").replace("（", "(").replace("）", ")")


def summarize_project_contribution(context: str) -> str:
    if "RPKI" in context or "CFG" in context:
        return "RPKI验证器测试与覆盖率实验"
    if "Shellcode" in context or "免杀" in context:
        return "Shellcode免杀与自动化平台开发"
    if "CVE" in context or "漏洞挖掘" in context:
        return "二进制漏洞挖掘与漏洞验证"
    if "数据清洗" in context or "GNSS" in context:
        return "数据清洗与研究方法实现"
    if "MCP" in context:
        return "MCP任务标注与内容修正"
    return compact_context_summary(context, ("项目", "研究", "开发", "实现", "分析"))


def summarize_contest_contribution(context: str) -> str:
    result = extract_result_phrase(context)
    if "产品" in context or "PPT" in context or "方案" in context:
        return f"产品方案设计{result}"[:35]
    return result or compact_context_summary(context, ("制作", "分析", "实现", "带领", "组织"))


def summarize_contest_role_contribution(context: str, role: str) -> str:
    if "出题人" in context:
        direction_match = re.search(r"担任([^，。,；;\n]{1,16})出题人", context)
        if direction_match:
            return f"{direction_match.group(1)}出题"
        return "赛题设计"
    if role:
        return compact_context_summary(context, ("组织", "负责", "带领", "担任"))
    return compact_context_summary(context, ("组织", "负责", "带领", "担任"))


def summarize_campus_contribution(context: str) -> str:
    if "活动" in context:
        return "活动组织与资源协调"
    return compact_context_summary(context, ("组织", "负责", "带领", "协商", "活动"))


def extract_project_subject(context: str) -> str:
    lines = [line.strip(" ,，。；;") for line in context.splitlines() if line.strip()]
    for line in lines:
        if not is_clean_subject_line(line):
            continue
        if "实验室" in line:
            return line[:24]
    for line in lines:
        if not is_clean_subject_line(line):
            continue
        if any(signal in line for signal in ("项目", "研究", "系统", "平台")):
            return line[:24]
    research_subject = extract_research_subject_from_context(context)
    if research_subject:
        return research_subject
    return ""


def extract_research_subject_from_context(context: str) -> str:
    if "GNSS" in context:
        if "田路分割" in context or "时序" in context:
            return "GNSS轨迹田路分割研究项目"
        if "点云" in context or "DBSCAN" in context:
            return "GNSS点云处理研究项目"
    return ""


def is_clean_subject_line(line: str) -> bool:
    if line.startswith("#"):
        return False
    if is_descriptive_sentence(line):
        return False
    if len(line) > 28:
        return False
    return any(signal in line for signal in ("实验室", "项目", "研究", "系统", "平台"))


def is_descriptive_sentence(line: str) -> bool:
    return any(signal in line for signal in ("参与", "指导", "独立", "提出", "开发", "分析", "能力", "认可", "对于"))


def compact_context_summary(context: str, signals: tuple[str, ...]) -> str:
    lines = [line.strip(" ,，。；;") for line in context.splitlines() if line.strip() and not line.strip().startswith("#")]
    for line in lines:
        if any(signal in line for signal in signals):
            return line[:35]
    return lines[0][:35] if lines else ""


def extract_event_name(context: str) -> str:
    sictf_match = re.search(r"(20\d{2}[^，。,；;\n]{0,16}?(?:CTF|新生赛)[^，。,；;\n]{0,12})", context, re.IGNORECASE)
    if sictf_match:
        return clean_event_name(sictf_match.group(1))
    hosted_match = re.search(r"主办的[“\"']?([^，。,；;\n]{2,24}(?:大赛|比赛|竞赛|挑战赛))", context)
    if hosted_match:
        return clean_event_name(hosted_match.group(1))
    match = re.search(r"([“\"']?[^，。,；;\n]{2,24}(?:大赛|比赛|竞赛|挑战赛))", context)
    if match:
        return clean_event_name(match.group(1))
    return ""


def clean_event_name(value: str) -> str:
    value = value.strip("“”\"' ,，。；;")
    for marker in ("中担任", "担任"):
        if marker in value:
            value = value.split(marker)[0].strip("“”\"' ,，。；;")
    for marker in ("”", "\"", "'"):
        if marker in value:
            tail = value.split(marker)[-1].strip("“”\"' ,，。；;")
            if tail:
                value = tail
    return value


def extract_org_name(context: str) -> str:
    match = re.search(r"([^，。,；;\n]{2,16}(?:协会|社团|学生会|班级))", context)
    if match:
        return match.group(1).strip()
    return ""


def extract_result_phrase(context: str) -> str:
    for result in ("全国六强总决赛", "总决赛", "一等奖", "二等奖", "三等奖", "省级立项"):
        if result in context:
            return result
    return ""


def score_project_level(context: str) -> int:
    if "国家级" in context or "重点实验室" in context:
        return 8 if any(signal in context for signal in ("独立", "提出", "领导", "负责")) else 7
    if any(signal in context for signal in ("独立", "负责", "提出", "开发")):
        return 6
    return 5


def score_contest_context_level(context: str) -> int:
    if is_low_value_honor_or_certificate(context) and not has_substantial_contest_contribution(context):
        return 2
    if "全国六强" in context or "总决赛" in context:
        return 9
    if "国家级" in context or "全国" in context:
        if has_substantial_contest_contribution(context):
            return 8 if "一等奖" in context else 7
        return 6
    if "省级" in context or "省" in context:
        return 6 if has_substantial_contest_contribution(context) else 5
    if has_substantial_contest_contribution(context):
        return 6
    return 5


def is_low_value_honor_or_certificate(context: str) -> bool:
    return any(signal in context for signal in LOW_VALUE_HONOR_SIGNALS)


def has_substantial_contest_contribution(context: str) -> bool:
    return any(signal in context for signal in ("建模", "算法", "产品", "方案", "系统", "实现", "开发", "分析", "队长", "带领团队", "组织", "出题"))


def dedupe_experiences(experiences: list[dict[str, Any]]) -> list[dict[str, Any]]:
    seen: set[tuple[str, str, str]] = set()
    deduped: list[dict[str, Any]] = []
    for item in experiences:
        key = (
            str(item.get("type", "")),
            str(item.get("role", "")),
            str(item.get("contribution", "")),
        )
        if key in seen:
            continue
        seen.add(key)
        deduped.append(item)
    return deduped


def filter_llm_experience_candidates(candidates: list[Any]) -> list[dict[str, Any]]:
    experiences: list[dict[str, Any]] = []
    for candidate in candidates:
        if not isinstance(candidate, dict):
            continue
        item = normalize_llm_experience_candidate(candidate)
        if not item:
            continue
        if is_low_quality_experience(item):
            continue
        item["level"] = calibrate_llm_experience_level(item)
        experiences.append(item)
    return dedupe_experiences(experiences)


BENCHMARK_DIMENSIONS = ("逻辑", "语言", "专业", "领导", "抗压", "成长")


def normalize_item_benchmark(item: dict[str, Any], data: dict[str, Any]) -> dict[str, Any]:
    raw_scores = data.get("scores") or data.get("score_vector") or data.get("dimension_scores")
    scores = normalize_six_dim_scores(raw_scores)
    impact = data.get("impact_factor", data.get("level", data.get("impact", 0)))
    if not isinstance(impact, (int, float)) or isinstance(impact, bool):
        impact = local_item_impact(item)
    impact = max(0, min(float(impact), 10))
    impact = calibrate_item_impact(item, impact)
    return {
        "item": {
            "name": str(item.get("name", "")),
            "result": str(item.get("result", "")),
        },
        "dimensions": list(BENCHMARK_DIMENSIONS),
        "scores": scores,
        "impact_factor": impact,
    }


def normalize_six_dim_scores(raw_scores: Any) -> list[float]:
    if isinstance(raw_scores, dict):
        values = [raw_scores.get(name, 0) for name in BENCHMARK_DIMENSIONS]
    elif isinstance(raw_scores, list):
        values = raw_scores[:6]
    else:
        values = []
    values = list(values) + [0] * (6 - len(values))
    scores: list[float] = []
    for value in values[:6]:
        if not isinstance(value, (int, float)) or isinstance(value, bool):
            value = 0
        if value > 1:
            value = value / 10
        scores.append(max(0, min(float(value), 1)))
    return normalize_score_distribution(scores)


def validate_six_dim_scores(scores: Any, label: str) -> None:
    if not isinstance(scores, list) or len(scores) != 6:
        raise ValueError(f"{label} must be an array of length 6")
    for index, score in enumerate(scores):
        if not isinstance(score, (int, float)) or isinstance(score, bool) or score < 0 or score > 1:
            raise ValueError(f"{label}[{index}] must be between 0 and 1")
    if abs(sum(float(score) for score in scores) - 1.0) > 0.01:
        raise ValueError(f"{label} must sum to 1")


def normalize_score_distribution(scores: list[float]) -> list[float]:
    total = sum(scores)
    if total <= 0:
        scores = [1 / 6] * 6
    else:
        scores = [score / total for score in scores]
    rounded = [round(score, 3) for score in scores]
    drift = round(1.0 - sum(rounded), 3)
    if rounded:
        max_index = max(range(len(rounded)), key=lambda index: rounded[index])
        rounded[max_index] = round(rounded[max_index] + drift, 3)
    return rounded


def calibrate_item_impact(item: dict[str, Any], impact: float) -> float:
    text = item_text(item)
    if is_low_value_honor_or_certificate(text):
        return round(min(impact, 2.5), 1)
    if any(signal in text for signal in ("CET", "英语四级", "英语六级", "NISP一级", "计算机二级")):
        return round(min(impact, 3.0), 1)
    if "强网杯" in text or "信息安全竞赛" in text or "网络安全" in text:
        return round(max(impact, 6.0), 1)
    return round(impact, 1)


def local_item_benchmark(item: dict[str, Any]) -> dict[str, Any]:
    text = item_text(item)
    scores = [0.0] * 6
    impact = local_item_impact(item)
    if any(signal in text for signal in ("建模", "数学", "算法", "ACM", "程序设计")):
        scores = [0.85, 0.25, 0.65, 0.25, 0.65, 0.55]
    elif any(signal in text for signal in ("网络安全", "信息安全", "CTF", "强网杯", "漏洞", "NISP")):
        scores = [0.7, 0.25, 0.9, 0.3, 0.75, 0.65]
    elif any(signal in text for signal in ("英语", "CET", "写作", "演讲")):
        scores = [0.2, 0.75, 0.15, 0.1, 0.25, 0.35]
    elif is_low_value_honor_or_certificate(text):
        scores = [0.15, 0.15, 0.1, 0.2, 0.2, 0.2]
    else:
        scores = [0.3, 0.25, 0.25, 0.2, 0.25, 0.3]
    return {
        "item": {
            "name": str(item.get("name", "")),
            "result": str(item.get("result", "")),
        },
        "dimensions": list(BENCHMARK_DIMENSIONS),
        "scores": normalize_score_distribution(scores),
        "impact_factor": calibrate_item_impact(item, impact),
    }


def local_item_impact(item: dict[str, Any]) -> float:
    text = item_text(item)
    if is_low_value_honor_or_certificate(text):
        return 2.0
    return float(score_contest_level(str(item.get("name", "")), str(item.get("result", ""))))


def item_text(item: dict[str, Any]) -> str:
    return f"{item.get('name', '')}{item.get('result', '')}"


def compact_education_context(resume_text: str) -> str:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    selected: list[str] = []
    for line in lines[:16]:
        if is_education_context_line(line):
            selected.append(line)
    if not selected:
        for line in lines:
            if is_education_context_line(line):
                selected.append(line)
            if len(selected) >= 8:
                break
    return "\n".join(selected[:8])[:800]


def is_education_context_line(line: str) -> bool:
    if line.startswith("#") and "教育" not in line:
        return False
    return any(
        signal in line
        for signal in (
            "大学",
            "学院",
            "本科",
            "专科",
            "硕士",
            "博士",
            "学士",
            "研究方向",
            "专业",
            "网络空间安全",
            "计算机",
            "信息安全",
        )
    )


def merge_experience_candidates(local_experience: list[dict[str, Any]], llm_experience: list[dict[str, Any]]) -> list[dict[str, Any]]:
    merged = list(llm_experience)
    for local_item in local_experience:
        if is_covered_by_existing_experience(local_item, merged):
            continue
        merged.append(local_item)
    return dedupe_experiences([item for item in merged if not is_low_quality_experience(item)])


def is_covered_by_existing_experience(candidate: dict[str, Any], existing: list[dict[str, Any]]) -> bool:
    candidate_role = str(candidate.get("role", ""))
    candidate_contribution = str(candidate.get("contribution", ""))
    candidate_text = f"{candidate_role}{candidate_contribution}"
    for item in existing:
        role = str(item.get("role", ""))
        contribution = str(item.get("contribution", ""))
        text = f"{role}{contribution}"
        if candidate_role and role and (candidate_role in role or role in candidate_role):
            return True
        shared_tokens = [token for token in extract_experience_tokens(candidate_text) if token in text]
        if len(shared_tokens) >= 2:
            return True
    return False


def extract_experience_tokens(text: str) -> list[str]:
    tokens = []
    for pattern in (
        r"CVE-\d{4}-\d+",
        r"[A-Za-z][A-Za-z0-9_-]{2,}",
        r"[\u4e00-\u9fff]{2,8}",
    ):
        tokens.extend(re.findall(pattern, text))
    return [token for token in tokens if token not in ("项目", "研究", "开发", "实现", "漏洞", "挖掘", "测试")]


def normalize_llm_experience_candidate(candidate: dict[str, Any]) -> dict[str, Any] | None:
    type_value = compact_role_part(str(candidate.get("type", "")))
    role = compact_role_part(str(candidate.get("role", "")))
    contribution = str(candidate.get("contribution", "")).strip(" -–—,，。；;")
    if not type_value or not contribution:
        return None
    level = candidate.get("level", 0)
    if not isinstance(level, (int, float)) or isinstance(level, bool):
        level = 0
    return {
        "type": normalize_experience_type(type_value),
        "role": role[:35],
        "contribution": contribution[:35],
        "level": max(0, min(int(round(level)), 10)),
    }


def normalize_experience_type(value: str) -> str:
    lowered = value.lower()
    if "实习" in value or "工作" in value or lowered in ("internship", "work"):
        return "实习"
    if "科研" in value or "研究" in value or lowered in ("research", "research-project"):
        return "科研项目"
    if "项目" in value or lowered == "project":
        return "项目"
    if "比赛" in value or "竞赛" in value or "赛" in value or lowered in ("contest", "competition"):
        return "比赛"
    if "社团" in value or lowered == "club":
        return "社团"
    if "任职" in value or "校园" in value or lowered in ("campus-role", "campus"):
        return "任职"
    return value[:12]


def is_low_quality_experience(item: dict[str, Any]) -> bool:
    text = f"{item.get('type', '')}{item.get('role', '')}{item.get('contribution', '')}"
    if any(marker in text for marker in ("##", "教育背景", "自我评价", "技能", "证书", "CET", "NISP")):
        return True
    if is_low_value_honor_or_certificate(text):
        return True
    if "多次参加" in text or "各种大小" in text:
        return True
    contribution = str(item.get("contribution", ""))
    if len(contribution) < 4:
        return True
    if contribution in ("参与", "参加", "负责", "实习", "项目经历", "科研经历"):
        return True
    return False


def calibrate_llm_experience_level(item: dict[str, Any]) -> int:
    text = f"{item.get('type', '')}{item.get('role', '')}{item.get('contribution', '')}"
    level = int(item.get("level", 0))
    if is_low_value_honor_or_certificate(text):
        return min(level, 2)
    if any(signal in text for signal in ("标注", "审核", "XML修正", "判断回答")) and not any(
        signal in text for signal in ("开发", "算法", "系统", "平台")
    ):
        return min(level, 4)
    if any(signal in text for signal in ("出题", "组织")) and any(signal in text for signal in ("CTF", "赛")):
        return max(level, 6)
    return level


def compact_contest_contribution(name: str, result: str) -> str:
    text = f"{name}{result}".replace(" ", "")
    return text[:35]


def score_contest_level(name: str, result: str) -> int:
    text = f"{name}{result}"
    if is_low_value_honor_or_certificate(text):
        return 2
    if "六强" in text or "总决赛" in text:
        return 9
    base = contest_scope_base_score(text)
    if "一等奖" in text:
        base += 2
    elif "二等奖" in text:
        base += 1
    elif "三等奖" in text:
        base += 0
    if has_substantial_contest_contribution(text):
        base += 1
    return max(3, min(base, 8))


def contest_scope_base_score(text: str) -> int:
    if "全国大学生数学建模竞赛" in text:
        return 6
    if "全国" in text or "国家级" in text:
        return 5
    if any(scope in text for scope in ("省", "区域", "地区", "华中", "华北", "华东", "华南", "东北", "西南", "西北")):
        return 5
    if any(signal in text for signal in ("数学建模", "程序设计", "算法", "创新训练", "科研项目")):
        return 4
    if "校级" in text or "院级" in text:
        return 4 if any(signal in text for signal in ("软件", "开发", "系统", "算法", "数据", "工程", "项目")) else 3
    return 3


def enrich_education(education: Any, resume_text: str = "") -> list[dict[str, Any]]:
    if not isinstance(education, list):
        return []
    raw_items = [item for item in education if isinstance(item, dict)]
    single_unknown_degree = len(raw_items) == 1 and not str(raw_items[0].get("degree", "")).strip()
    enriched: list[dict[str, Any]] = []
    for item in raw_items:
        copied = dict(item)
        degree = str(copied.get("degree", "")).strip()
        copied["degree_level"] = normalize_degree_level(degree)
        if not copied["degree_level"] and single_unknown_degree:
            copied["degree_level"] = infer_single_education_degree_level(copied, resume_text)
        copied["school_tags"] = school_tags_for(str(copied.get("school", "")))
        enriched.append(copied)
    return enriched


def enrich_education_school_tags(education: Any) -> list[dict[str, Any]]:
    return enrich_education(education)


def normalize_degree_level(degree: str) -> str:
    if not degree:
        return ""
    if any(word in degree for word in ("博士", "PhD", "PHD")):
        return "博士"
    if any(word in degree for word in ("硕士", "研究生", "Master", "MASTER")):
        return "硕士"
    if any(word in degree for word in ("本科", "学士", "Bachelor", "BACHELOR")):
        return "本科"
    if any(word in degree for word in ("专科", "大专", "高职")):
        return "专科"
    return ""


def infer_single_education_degree_level(education: dict[str, Any], resume_text: str) -> str:
    school = str(education.get("school", ""))
    context = education_context_for_school(school, resume_text)
    explicit = normalize_degree_level(context)
    if explicit:
        return explicit

    years = education_year_span(context or resume_text)
    if years >= 5:
        return "博士" if any(word in context for word in ("博士", "直博")) else "本科"
    if 2 <= years <= 3 and any(word in context for word in ("硕士", "研究生", "MBA", "Master")):
        return "硕士"
    if 2 <= years <= 3 and any(word in school for word in ("职业", "专科", "高等专科学校")):
        return "专科"
    if years >= 3:
        return "本科"
    if any(word in school for word in ("大学", "学院")):
        return "本科"
    return "本科"


def education_context_for_school(school: str, resume_text: str) -> str:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    normalized_school = normalize_school_name(school)
    for index, line in enumerate(lines):
        if normalized_school and normalized_school in normalize_school_name(line):
            start = max(0, index - 2)
            end = min(len(lines), index + 3)
            return "\n".join(lines[start:end])
    return ""


def education_year_span(text: str) -> int:
    spans = []
    for match in EDUCATION_DATE_RE.finditer(text):
        try:
            spans.append(int(match.group("end")) - int(match.group("start")))
        except ValueError:
            continue
    return max(spans) if spans else 0


def school_tags_for(school_name: str) -> dict[str, Any]:
    matched_school, info = match_school_ranking(school_name)
    return {
        "matched_school": matched_school,
        "is_985": bool(info.get("是否是985学校")) if info else False,
        "is_211": bool(info.get("是否是211学校")) if info else False,
        "is_double_first_class": bool(info.get("是否是双一流学校")) if info else False,
        "ruanke_rank": info.get("软科排名") if info else None,
    }


def match_school_ranking(school_name: str) -> tuple[str, dict[str, Any] | None]:
    normalized = normalize_school_name(school_name)
    if not normalized:
        return "", None
    rankings = load_school_rankings()
    normalized_index = {normalize_school_name(name): name for name in rankings}

    exact = normalized_index.get(normalized)
    if exact:
        return exact, rankings[exact]

    candidates: list[tuple[int, int, str]] = []
    for normalized_cache_name, original_name in normalized_index.items():
        if normalized_cache_name in normalized or normalized in normalized_cache_name:
            overlap = min(len(normalized_cache_name), len(normalized))
            candidates.append((overlap, len(normalized_cache_name), original_name))
    if not candidates:
        return "", None
    _, _, matched = max(candidates)
    return matched, rankings[matched]


def normalize_school_name(name: str) -> str:
    text = re.sub(r"[\s·•,，。|/\\()（）【】\[\]{}:：;；\-至0-9]+", "", name)
    for suffix in ("本科", "硕士", "博士", "学院", "大学"):
        if text.endswith(suffix) and suffix in ("本科", "硕士", "博士"):
            text = text[: -len(suffix)]
    return text


@lru_cache(maxsize=1)
def load_school_rankings() -> dict[str, dict[str, Any]]:
    with RANKING_CACHE.open(encoding="utf-8") as handle:
        payload = json.load(handle)
    if not isinstance(payload, dict):
        return {}
    return {str(name): info for name, info in payload.items() if isinstance(info, dict)}
