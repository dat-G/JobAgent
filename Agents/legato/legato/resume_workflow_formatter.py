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
COMPANY_ENTITY_RE = re.compile(
    r"^[\u4e00-\u9fa5A-Za-z0-9（）()·&]{2,40}(?:股份有限公司|有限责任公司|有限公司|集团股份有限公司|集团|公司)$"
)
DEPARTMENT_ENTITY_RE = re.compile(r"(研究院|事业部|平台部|研发部|部门|中心|实验室|团队)$")
DATE_RANGE_OR_PRESENT_RE = re.compile(r"20\d{2}[.年/-]?\d{0,2}\s*[-~至]\s*(?:20\d{2}[.年/-]?\d{0,2}|今)")
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
INDEPENDENT_COLLEGE_CACHE = (
    Path(__file__).resolve().parents[1]
    / "workflows"
    / "resume"
    / "cache"
    / "independent_colleges.json"
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
        stage_input: dict[str, Any] | None = None,
        combine_agents: bool = False,
    ) -> None:
        self.presto = PrestoFormatter(presto_url, timeout_seconds=timeout_seconds)
        self.max_retries = max_retries
        self.max_workers = max_workers
        self.stage_input = stage_input or {}
        self.combine_agents = combine_agents
        self.prompts_dir = Path(__file__).resolve().parents[1] / "workflows" / "resume" / "prompts"
        self.common_prompt = self._read_prompt("common.md")
        self.retry_prompt = self._read_prompt("retry_json.md")
        self.route = load_model_route(Path(__file__))

    def format(self, resume_text: str) -> WorkflowFormatResult:
        if self.combine_agents:
            return self._format_combined(resume_text)

        started = time.perf_counter()
        with ThreadPoolExecutor(max_workers=min(self.max_workers, len(self.groups) + 1)) as executor:
            futures = {
                group: executor.submit(self._run_group_with_retry, group, resume_text)
                for group in self.groups
            }
            experience_future = executor.submit(self._run_experience_hybrid, resume_text)
            results = {group: future.result() for group, future in futures.items()}
            experience_result = experience_future.result()

        education_tags_started = time.perf_counter()
        education = enrich_education(results["profile"]["data"].get("education", []), resume_text)
        education_tags_ms = int((time.perf_counter() - education_tags_started) * 1000)
        merge_started = time.perf_counter()
        data = {
            "identity": results["profile"]["data"].get("identity", {}),
            "education": education,
            "certifications_awards": results["certifications_awards"]["data"].get("certifications_awards", []),
            "experience": experience_result["experience"],
        }
        merge_ms = int((time.perf_counter() - merge_started) * 1000)
        total_ms = int((time.perf_counter() - started) * 1000)
        return WorkflowFormatResult(
            data=data,
            formatter="presto_resume_workflow",
            warnings=[],
            debug=self._debug_envelope(
                {**results, "experience_refine": experience_result["agent"]},
                merge_ms,
                total_ms,
                local_stages=[
                    {"stage": "education_degree_inference_agent", "elapsed_ms": education_tags_ms},
                    *experience_result["local_stages"],
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
        if stage == "major_baseline":
            result = self._run_major_baseline_with_retry(resume_text)
            total_ms = int((time.perf_counter() - started) * 1000)
            return WorkflowFormatResult(
                data={"major_baseline": result["data"].get("major_baseline", {})},
                formatter="presto_resume_workflow_major_baseline",
                warnings=[],
                debug=self._debug_envelope(
                    {"major_baseline": result},
                    merge_ms=0,
                    total_ms=total_ms,
                ),
            )
        if stage == "item_benchmark":
            external_items = normalize_benchmark_input_items(self.stage_input.get("items"))
            stage_max_workers = safe_int(self.stage_input.get("max_workers"), self.max_workers)
            agents: dict[str, dict[str, Any]] = {}
            if external_items:
                items = external_items
            else:
                items_result = self._run_group_with_retry("certifications_awards", resume_text)
                items = benchmark_items_from_certifications(items_result["data"].get("certifications_awards", []))
                agents["certifications_awards"] = items_result
            benchmark_started = time.perf_counter()
            benchmark_result = self._run_item_benchmarks(
                resume_text,
                items,
                max_workers=stage_max_workers,
            )
            benchmark_ms = int((time.perf_counter() - benchmark_started) * 1000)
            total_ms = int((time.perf_counter() - started) * 1000)
            return WorkflowFormatResult(
                data={"item_benchmark": benchmark_result["items"]},
                formatter="presto_resume_workflow_item_benchmark",
                warnings=[],
                debug=self._debug_envelope(
                    {**agents, **benchmark_result["agents"]},
                    merge_ms=0,
                    total_ms=total_ms,
                    local_stages=[{"stage": "item_benchmark_merge", "elapsed_ms": benchmark_ms}],
                ),
            )
        if stage == "job_matching":
            result = self._run_job_matching_with_retry(resume_text)
            total_ms = int((time.perf_counter() - started) * 1000)
            return WorkflowFormatResult(
                data={"job_matching": result["data"].get("job_matching", {})},
                formatter="presto_resume_workflow_job_matching",
                warnings=[],
                debug=self._debug_envelope(
                    {"job_matching": result},
                    merge_ms=0,
                    total_ms=total_ms,
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
            experience_result = self._run_experience_hybrid(resume_text)
            total_ms = int((time.perf_counter() - started) * 1000)
            return WorkflowFormatResult(
                data={"experience": experience_result["experience"]},
                formatter="presto_resume_workflow_experience_hybrid",
                warnings=[],
                debug=self._debug_envelope(
                    {"experience_refine": experience_result["agent"]},
                    merge_ms=0,
                    total_ms=total_ms,
                    local_stages=experience_result["local_stages"],
                ),
            )
        if stage == "experience_hybrid_item":
            item_index = safe_int(self.stage_input.get("index"), 0)
            local_item = normalize_stage_experience_item(self.stage_input.get("item"))
            local_ms = 0
            if not local_item:
                local_started = time.perf_counter()
                local_experience = build_local_experience(resume_text, [])
                local_ms = int((time.perf_counter() - local_started) * 1000)
                if 0 <= item_index < len(local_experience):
                    local_item = normalize_stage_experience_item(local_experience[item_index])
            if not local_item:
                raise ValueError("experience_hybrid_item requires a valid stage_input item")

            result = self._run_experience_refine_with_retry(resume_text, [local_item])
            filter_started = time.perf_counter()
            llm_experience = filter_llm_experience_candidates(result["data"].get("experience", []))
            experience = [select_hybrid_experience_item(local_item, llm_experience)]
            filter_ms = int((time.perf_counter() - filter_started) * 1000)
            total_ms = int((time.perf_counter() - started) * 1000)
            return WorkflowFormatResult(
                data={"experience_index": item_index, "experience": experience},
                formatter="presto_resume_workflow_experience_hybrid_item",
                warnings=[],
                debug=self._debug_envelope(
                    {f"experience_refine_{item_index}": result},
                    merge_ms=0,
                    total_ms=total_ms,
                    local_stages=[
                        {"stage": "experience_local_item", "elapsed_ms": local_ms},
                        {"stage": "experience_item_candidate_filter", "elapsed_ms": filter_ms},
                    ],
                ),
            )
        raise ValueError(f"unsupported resume workflow stage {stage!r}")

    def _run_experience_hybrid(self, resume_text: str) -> dict[str, Any]:
        local_started = time.perf_counter()
        local_experience = build_local_experience(resume_text, [])
        local_ms = int((time.perf_counter() - local_started) * 1000)
        result = self._run_experience_refine_with_retry(resume_text, local_experience)
        filter_started = time.perf_counter()
        llm_experience = filter_llm_experience_candidates(result["data"].get("experience", []))
        experience = merge_experience_candidates(local_experience, llm_experience)
        filter_ms = int((time.perf_counter() - filter_started) * 1000)
        return {
            "experience": experience,
            "agent": result,
            "local_stages": [
                {"stage": "experience_local_recall", "elapsed_ms": local_ms},
                {"stage": "experience_candidate_filter", "elapsed_ms": filter_ms},
            ],
        }

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

    def _run_item_benchmarks(
        self,
        resume_text: str,
        items: list[dict[str, Any]],
        *,
        max_workers: int | None = None,
    ) -> dict[str, Any]:
        benchmarks: list[dict[str, Any] | None] = [None] * len(items)
        agents: dict[str, dict[str, Any]] = {}
        if not items:
            return {"items": [], "agents": agents}

        worker_count = max(1, max_workers or self.max_workers)
        with ThreadPoolExecutor(max_workers=min(worker_count, max(1, len(items)))) as executor:
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
            try:
                output = self._call_presto(call_prompt, "item_benchmark")
                call_ms = int((time.perf_counter() - call_started) * 1000)
                previous_output = output
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
        raise RuntimeError(f"item_benchmark[{index}] failed after {self.max_retries} attempts: {last_error}")

    def _item_benchmark_prompt(self, resume_text: str, item: dict[str, Any]) -> str:
        prompt = self._read_prompt("item_benchmark.md")
        context = compact_education_context(resume_text)
        item_text = json.dumps(item, ensure_ascii=False, separators=(",", ":"))
        return prompt.replace("{{education_context}}", context).replace("{{item}}", item_text)

    def _run_major_baseline_with_retry(self, resume_text: str) -> dict[str, Any]:
        started = time.perf_counter()
        context = major_baseline_context(resume_text, self.stage_input)
        prompt = self._major_baseline_prompt(context)
        previous_output = ""
        last_error = ""
        input_debug = {
            "mode": "profile_education_transcript_context",
            "input_chars": len(prompt),
            "original_chars": len(resume_text),
            "fallback_full_text": False,
        }
        for attempt in range(1, self.max_retries + 1):
            call_prompt = prompt
            if attempt > 1:
                call_prompt += "\n\n" + self._retry_block(last_error, previous_output)
            call_started = time.perf_counter()
            try:
                output = self._call_presto(call_prompt, "major_baseline")
                call_ms = int((time.perf_counter() - call_started) * 1000)
                previous_output = output
                data = extract_json_object(output)
                baseline = normalize_major_baseline(data, context)
                validate_major_baseline(baseline)
                return {
                    "data": {"major_baseline": baseline},
                    "attempts": attempt,
                    "retry_count": attempt - 1,
                    "elapsed_ms": int((time.perf_counter() - started) * 1000),
                    "last_call_ms": call_ms,
                    "input": input_debug,
                }
            except Exception as exc:
                last_error = str(exc)
        raise RuntimeError(f"major_baseline failed after {self.max_retries} attempts: {last_error}")

    def _major_baseline_prompt(self, context: dict[str, Any]) -> str:
        prompt = self._read_prompt("major_baseline.md")
        context_text = json.dumps(context, ensure_ascii=False, separators=(",", ":"))
        return prompt.replace("{{common}}", self.common_prompt).replace("{{context}}", context_text)

    def _run_job_matching_with_retry(self, resume_text: str) -> dict[str, Any]:
        started = time.perf_counter()
        context = job_matching_context(resume_text, self.stage_input)
        prompt = self._job_matching_prompt(context)
        previous_output = ""
        last_error = ""
        input_debug = {
            "mode": "structured_profile_evidence_benchmark_context",
            "input_chars": len(prompt),
            "original_chars": len(resume_text),
            "fallback_full_text": False,
        }
        for attempt in range(1, self.max_retries + 1):
            call_prompt = prompt
            if attempt > 1:
                call_prompt += "\n\n" + self._retry_block(last_error, previous_output)
            call_started = time.perf_counter()
            try:
                output = self._call_presto(call_prompt, "job_matching")
                call_ms = int((time.perf_counter() - call_started) * 1000)
                previous_output = output
                data = extract_json_object(output)
                matching = normalize_job_matching(data, context)
                validate_job_matching(matching)
                return {
                    "data": {"job_matching": matching},
                    "attempts": attempt,
                    "retry_count": attempt - 1,
                    "elapsed_ms": int((time.perf_counter() - started) * 1000),
                    "last_call_ms": call_ms,
                    "input": input_debug,
                }
            except Exception as exc:
                last_error = str(exc)
        raise RuntimeError(f"job_matching failed after {self.max_retries} attempts: {last_error}")

    def _job_matching_prompt(self, context: dict[str, Any]) -> str:
        prompt = self._read_prompt("job_matching.md")
        context_text = json.dumps(context, ensure_ascii=False, separators=(",", ":"))
        return prompt.replace("{{common}}", self.common_prompt).replace("{{context}}", context_text)

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
                for key in ("name", "result", "level", "evidence_scope"):
                    if key not in item:
                        raise ValueError(f"certifications_awards[{index}] missing {key}")
                if item["evidence_scope"] not in EVIDENCE_SCOPES:
                    raise ValueError(f"certifications_awards[{index}].evidence_scope must be 校内 or 校外")
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
        if group == "major_baseline":
            validate_major_baseline(normalize_major_baseline(data, major_baseline_context("", self.stage_input)))
            return
        if group in EXPERIENCE_GROUPS or group == "experience":
            items = data.get("experience")
            if not isinstance(items, list):
                raise ValueError("experience must be an array")
            for index, item in enumerate(items):
                if not isinstance(item, dict):
                    raise ValueError(f"experience[{index}] must be an object")
                for key in ("type", "role", "contribution", "level", "evidence_scope"):
                    if key not in item:
                        raise ValueError(f"experience[{index}] missing {key}")
                level = item["level"]
                if not isinstance(level, (int, float)) or isinstance(level, bool):
                    raise ValueError(f"experience[{index}].level must be a number")
                if level < 0 or level > 10:
                    raise ValueError(f"experience[{index}].level must be between 0 and 10")
                if item["evidence_scope"] not in EVIDENCE_SCOPES:
                    raise ValueError(f"experience[{index}].evidence_scope must be 校内 or 校外")
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
    experiences.extend(extract_company_role_experiences(resume_text))
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
        body = match.group("body")
        organization, role = split_experience_body(body)
        nearby_organization = find_nearby_company_entity(lines, index)
        if nearby_organization and is_department_like_organization(organization):
            organization = nearby_organization
            role = extract_job_role_from_internship_body(body) or role
        contribution_context = "\n".join(lines[max(0, index - 30) : min(len(lines), index + 6)])
        scoring_context = "\n".join(lines[index : min(len(lines), index + 6)])
        contribution = summarize_internship_contribution(organization, role, contribution_context)
        experiences.append(
            {
                "type": "实习",
                "role": compose_experience_role(organization, role),
                "contribution": contribution,
                "level": score_internship_level(organization, scoring_context),
                "evidence_scope": "校外",
            }
        )
    return experiences


def extract_company_role_experiences(resume_text: str) -> list[dict[str, Any]]:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    experiences: list[dict[str, Any]] = []
    for index, line in enumerate(lines[:-1]):
        organization = compact_role_part(line)
        if not COMPANY_ENTITY_RE.match(organization):
            continue
        role_line = lines[index + 1]
        if is_internship_body(role_line):
            continue
        if not DATE_RANGE_OR_PRESENT_RE.search(role_line):
            continue
        role = compact_role_part(DATE_RANGE_OR_PRESENT_RE.sub("", role_line))
        if not role or any(skip in role for skip in ("大学", "学院", "本科", "硕士", "博士", "指导老师")):
            continue
        context = "\n".join(lines[index : index + 2])
        experiences.append(
            {
                "type": "任职",
                "role": compose_experience_role(organization, role),
                "contribution": summarize_company_role_contribution(role, context),
                "level": score_company_role_level(role, context),
                "evidence_scope": "校外",
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


def find_nearby_company_entity(lines: list[str], index: int) -> str:
    for line in reversed(lines[max(0, index - 4) : index]):
        candidate = compact_role_part(strip_date_prefix(line))
        if COMPANY_ENTITY_RE.match(candidate):
            return candidate
    return ""


def strip_date_prefix(line: str) -> str:
    return re.sub(r"^20\d{2}[.年/-]?\d{0,2}\s*[-~至]\s*20\d{2}[.年/-]?\d{0,2}\s+", "", line).strip()


def is_department_like_organization(organization: str) -> bool:
    compacted = compact_role_part(organization)
    return bool(compacted and DEPARTMENT_ENTITY_RE.search(compacted))


def extract_job_role_from_internship_body(body: str) -> str:
    normalized = body.replace("｜", "|").strip()
    if "|" in normalized:
        return compact_role_part(normalized.rsplit("|", maxsplit=1)[-1])
    return ""


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


def summarize_company_role_contribution(role: str, context: str) -> str:
    if any(signal in context for signal in ("实验", "检测", "制备", "免疫", "疫苗", "动物", "生物")):
        return f"{role}岗位实践"
    return f"{role}岗位任职"


def score_company_role_level(role: str, context: str) -> int:
    detailed = has_contribution_signal(context)
    domain_relevant = any(signal in context for signal in ("实验", "检测", "制备", "免疫", "疫苗", "动物", "生物"))
    if detailed and domain_relevant:
        return 5
    if domain_relevant:
        return 4
    return 3


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
        project_anchor = is_project_anchor(line)
        research_anchor = is_research_description_anchor(line)
        if not (project_anchor or research_anchor):
            continue
        if line.startswith("#") or line in ("项目经历", "科研经历"):
            continue
        context_start = max(0, index - 4) if research_anchor else index
        context = "\n".join(lines[context_start : min(len(lines), index + 8)])
        if not has_contribution_signal(context):
            continue
        role = extract_project_subject(context)
        if not role:
            continue
        affiliation = find_project_affiliation(lines, index, context)
        if affiliation and affiliation not in role:
            role = compose_experience_role(affiliation, role)
        experiences.append(
            {
                "type": "科研项目" if "实验室" in context or "科研" in context or "研究" in context else "项目",
                "role": role,
                "contribution": summarize_project_contribution(context),
                "level": score_project_level(context),
                "evidence_scope": normalize_evidence_scope("", {"type": "项目", "role": role, "contribution": context}),
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
                "evidence_scope": normalize_evidence_scope("", {"type": "项目", "role": role, "contribution": contribution}),
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
                "evidence_scope": normalize_evidence_scope("", {"type": "项目", "role": subject, "contribution": nearby}),
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
    if any(skip in line for skip in ("荣誉", "奖项", "证书", "技能", "教育背景", "主修课程", "公众号")):
        return False
    if len(line) < 12:
        return False
    if "研究项目" in line and has_contribution_signal(line):
        return True
    return bool(
        re.search(r"基于[^，。,；;\n]{2,32}的[^，。,；;\n]{2,32}(方法|模型|算法|系统|框架|模块)", line)
        or re.search(r"针对[^，。,；;\n]{2,32}(数据集|问题|场景|任务)[^，。,；;\n]{0,32}(开发|实现|复现|分析|评估|构建)", line)
        or (
            any(marker in line for marker in ("研究项目", "科研项目", "研究中"))
            and any(signal in line for signal in ("数据清洗", "模型训练", "算法实现", "实验评估", "论文复现", "复现与分析"))
        )
    )


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
                "evidence_scope": normalize_evidence_scope("", {"type": "比赛", "role": extract_event_name(context), "contribution": context}),
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
                "evidence_scope": normalize_evidence_scope("", {"type": "比赛", "role": extract_event_name(context), "contribution": context}),
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
                "evidence_scope": "校内",
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
    if "实验评估" in context or "模型训练" in context:
        return "模型实现与实验评估"
    if "论文复现" in context or "复现与分析" in context or "非开源文章" in context:
        return "论文复现与研究方法分析"
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
    research_subject = extract_research_subject_from_context(context)
    if research_subject:
        return research_subject
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
    return ""


def find_project_affiliation(lines: list[str], index: int, context: str) -> str:
    if not any(signal in context for signal in ("实验室", "科研", "研究", "GNSS", "教授", "指导")):
        return ""
    context_affiliation = extract_lab_affiliation(context)
    if context_affiliation:
        return context_affiliation
    best = ""
    best_distance = 10**9
    for candidate_index, line in enumerate(lines):
        affiliation = extract_lab_affiliation(line)
        if not affiliation:
            continue
        distance = abs(candidate_index - index)
        if distance <= 25 and distance < best_distance:
            best = affiliation
            best_distance = distance
    return best


def extract_lab_affiliation(text: str) -> str:
    for line in [item.strip(" ,，。；;") for item in text.splitlines() if item.strip()]:
        if "实验室" not in line:
            continue
        if is_descriptive_sentence(line) and "重点实验室" not in line:
            continue
        match = re.search(r"([\u4e00-\u9fa5A-Za-z0-9（）()·&]{2,40}?实验室)", line)
        if match:
            return match.group(1)[:28]
    return ""


def extract_research_subject_from_context(context: str) -> str:
    lines = [line.strip(" ,，。；;") for line in context.splitlines() if line.strip()]
    for line in lines:
        explicit = research_subject_from_explicit_project_line(line)
        if explicit:
            return explicit
    for line in lines:
        method_subject = research_subject_from_method_line(line)
        if method_subject:
            return method_subject
    for line in lines:
        target_subject = research_subject_from_target_line(line)
        if target_subject:
            return target_subject
    return ""


def research_subject_from_explicit_project_line(line: str) -> str:
    matches = re.findall(r"([^，。,；;\n]{4,36})的研究项目", line)
    if not matches:
        return ""
    subject = clean_research_subject(matches[-1])
    return f"{subject}研究项目" if subject else ""


def research_subject_from_method_line(line: str) -> str:
    match = re.search(r"基于[^，。,；;\n]{2,32}的(?P<subject>[^，。,；;\n]{2,32}?)(方法|模型|算法|系统|框架|模块)", line)
    if not match:
        return ""
    subject = clean_research_subject(match.group("subject"))
    suffix = match.group(2)
    if not subject:
        return ""
    return f"{subject}{suffix}研究项目"[:35]


def research_subject_from_target_line(line: str) -> str:
    match = re.search(r"针对(?P<subject>[^，。,；;\n]{2,32})(数据集|问题|场景|任务)", line)
    if not match:
        return ""
    subject = clean_research_subject(match.group("subject") + match.group(2))
    return f"{subject}研究项目"[:35] if subject else ""


def clean_research_subject(subject: str) -> str:
    subject = compact_role_part(subject)
    for marker in ("模型的", "方法的", "系统的", "算法的", "框架的", "模块的"):
        if marker in subject:
            subject = subject.split(marker)[-1]
    subject = re.sub(r"^(基于|关于|对于|针对|模型的|型的|方法的|项目的|使用的)", "", subject)
    subject = re.sub(r"^(农机|相关|该|本)", "", subject)
    subject = subject.strip("的 -–—,，。；;")
    subject = subject.replace("的", "")
    if len(subject) > 24:
        subject = subject[-24:]
    return subject


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


def safe_int(value: Any, default: int = 0) -> int:
    try:
        return int(value)
    except (TypeError, ValueError):
        return default


def normalize_stage_experience_item(value: Any) -> dict[str, Any] | None:
    if not isinstance(value, dict):
        return None
    experience_type = str(value.get("type") or value.get("experience_type") or "").strip()
    role = str(value.get("role") or "").strip()
    contribution = str(value.get("contribution") or "").strip()
    if not any((experience_type, role, contribution)):
        return None
    level = safe_int(value.get("level"), 5)
    normalized = {
        "type": experience_type,
        "role": role,
        "contribution": contribution,
        "level": max(0, min(level, 10)),
    }
    normalized["evidence_scope"] = normalize_evidence_scope(value.get("evidence_scope"), normalized)
    return normalized


def select_hybrid_experience_item(local_item: dict[str, Any], llm_experience: list[dict[str, Any]]) -> dict[str, Any]:
    if not llm_experience:
        return local_item
    for item in llm_experience:
        if is_covered_by_existing_experience(local_item, [item]) or is_covered_by_existing_experience(item, [local_item]):
            return item
    return local_item


BENCHMARK_DIMENSIONS = ("逻辑", "语言", "专业", "领导", "抗压", "成长")
EVIDENCE_SCOPES = ("校内", "校外")
MAJOR_FAMILIES = ("文科类", "理科类", "工科类", "商科类", "医农类", "艺术体育类", "交叉类", "未知")


def major_baseline_context(resume_text: str, stage_input: dict[str, Any] | None = None) -> dict[str, Any]:
    stage_input = stage_input or {}
    basic_info = stage_input.get("basic_info") if isinstance(stage_input.get("basic_info"), dict) else {}
    raw_education = stage_input.get("education") if isinstance(stage_input.get("education"), list) else []
    education: list[dict[str, Any]] = []
    for item in raw_education:
        if not isinstance(item, dict):
            continue
        tags = item.get("school_tags") if isinstance(item.get("school_tags"), dict) else {}
        education.append(
            {
                "school": str(item.get("school") or ""),
                "degree": str(item.get("degree") or item.get("degree_level") or ""),
                "department": str(item.get("department") or ""),
                "major": str(item.get("major") or ""),
                "is_985": bool(item.get("is_985") or item.get("is985") or tags.get("is_985")),
                "is_211": bool(item.get("is_211") or item.get("is211") or tags.get("is_211")),
                "is_double_first_class": bool(item.get("is_double_first_class") or item.get("isDoubleFirstClass") or tags.get("is_double_first_class")),
                "ruanke_rank": item.get("ruanke_rank") or item.get("ruankeRank") or tags.get("ruanke_rank") or 0,
                "school_kind": str(item.get("school_kind") or tags.get("school_kind") or ""),
                "parent_school": str(item.get("parent_school") or tags.get("parent_school") or ""),
            }
        )
    transcript_use = str(
        stage_input.get("transcript_use")
        or basic_info.get("transcript_use")
        or basic_info.get("transcriptUse")
        or ""
    )
    major_name = first_non_empty(
        [
            basic_info.get("major"),
            *[item.get("major") for item in education],
        ]
    )
    department = first_non_empty([item.get("department") for item in education])
    degree = first_non_empty([basic_info.get("degree"), *[item.get("degree") for item in education]])
    major_text = "".join([major_name, department, degree, compact_education_context(resume_text)])
    base_score_hint = academic_base_score_from_transcript(transcript_use)
    school_hint = school_quality_hint(education, major_name, major_text)
    return {
        "basic_info": {
            "school": str(basic_info.get("school") or ""),
            "major": str(basic_info.get("major") or ""),
            "degree": str(basic_info.get("degree") or ""),
        },
        "education": education,
        "transcript_use": transcript_use,
        "resume_education_context": compact_education_context(resume_text),
        "major_name_hint": major_name,
        "major_family_hint": infer_major_family(major_text),
        "school_quality_hint": school_hint,
        "base_score_hint": base_score_hint,
        "dimensions": list(BENCHMARK_DIMENSIONS),
    }


def normalize_major_baseline(data: dict[str, Any], context: dict[str, Any] | None = None) -> dict[str, Any]:
    context = context or {}
    raw = data.get("major_baseline") if isinstance(data, dict) else {}
    if not isinstance(raw, dict):
        raw = data if isinstance(data, dict) else {}
    fallback = local_major_baseline(context)
    major_name = str(raw.get("major_name") or context.get("major_name_hint") or fallback["major_name"])
    family = str(raw.get("major_family") or context.get("major_family_hint") or fallback["major_family"])
    if family not in MAJOR_FAMILIES:
        family = infer_major_family(major_name)
    if family not in MAJOR_FAMILIES:
        family = fallback["major_family"]
    base_score = numeric_int(raw.get("base_score"), fallback["base_score"])
    base_score = clamp_int(base_score, 30, 85)
    scores = normalize_major_baseline_scores(raw.get("scores") or raw.get("baseline_scores") or raw.get("dimension_scores"), fallback["scores"])
    confidence = numeric_float(raw.get("confidence"), fallback["confidence"])
    return {
        "major_name": major_name,
        "major_family": family,
        "base_score": base_score,
        "dimensions": list(BENCHMARK_DIMENSIONS),
        "scores": scores,
        "rationale": str(raw.get("rationale") or fallback["rationale"])[:120],
        "confidence": round(max(0.0, min(float(confidence), 1.0)), 2),
        "source": str(raw.get("source") or "presto_major_baseline"),
    }


def validate_major_baseline(baseline: dict[str, Any]) -> None:
    if not isinstance(baseline, dict):
        raise ValueError("major_baseline must be an object")
    if baseline.get("major_family") not in MAJOR_FAMILIES:
        raise ValueError("major_baseline.major_family is invalid")
    scores = baseline.get("scores")
    if not isinstance(scores, list) or len(scores) != 6:
        raise ValueError("major_baseline.scores must be an array of length 6")
    for index, score in enumerate(scores):
        if not isinstance(score, int) or isinstance(score, bool) or score < 0 or score > 100:
            raise ValueError(f"major_baseline.scores[{index}] must be an integer between 0 and 100")


def job_matching_context(resume_text: str, stage_input: dict[str, Any] | None = None) -> dict[str, Any]:
    stage_input = stage_input or {}
    basic_info = stage_input.get("basic_info") if isinstance(stage_input.get("basic_info"), dict) else {}
    education = normalize_matching_education(stage_input.get("education"))
    awards = normalize_matching_awards(stage_input.get("awards"))
    experiences = normalize_matching_experiences(stage_input.get("experiences"))
    major_baseline = stage_input.get("major_baseline") if isinstance(stage_input.get("major_baseline"), dict) else {}
    student_radar = normalize_radar_dimensions(stage_input.get("student_radar"), allow_empty=True)
    if not student_radar:
        student_radar = normalize_radar_dimensions(major_baseline.get("scores"), allow_empty=True)
    context: dict[str, Any] = {
        "basic_info": {
            "name": str(basic_info.get("name") or ""),
            "sex": str(basic_info.get("sex") or ""),
            "birth_year": str(basic_info.get("birth_year") or ""),
            "school": str(basic_info.get("school") or ""),
            "major": str(basic_info.get("major") or ""),
            "degree": str(basic_info.get("degree") or ""),
            "graduation_year": str(basic_info.get("graduation_year") or ""),
            "transcript_use": str(basic_info.get("transcript_use") or ""),
        },
        "education": education,
        "major_baseline": {
            "major_name": str(major_baseline.get("major_name") or ""),
            "major_family": str(major_baseline.get("major_family") or ""),
            "base_score": numeric_int(major_baseline.get("base_score"), 0),
            "dimensions": list(BENCHMARK_DIMENSIONS),
            "scores": normalize_radar_score_values(major_baseline.get("scores"), min_value=0, max_value=100),
            "rationale": str(major_baseline.get("rationale") or ""),
            "confidence": numeric_float(major_baseline.get("confidence"), 0),
        },
        "student_radar_hint": student_radar,
        "awards": awards[:16],
        "experiences": experiences[:16],
        "resume_excerpt": compact_resume_excerpt(resume_text),
        "dimensions": list(BENCHMARK_DIMENSIONS),
        "recommendation_principles": [
            "综合六维能力作为第一排序依据。",
            "经历和项目证据作为第二排序依据，优先看相关性、impact、ownership 和结果证据。",
            "学历作为岗位门槛和风险约束，不作为唯一排序依据。",
            "若存在高 impact 且岗位强相关项目，可适当突破学历门槛，但必须写明风险和补证据动作。",
            "必须同时考虑本专业相关岗位和非本专业但能力可迁移岗位。",
            "不得依赖用户主动输入目标岗位，必须由 Agent Team 自动推荐。",
        ],
    }
    return context


def normalize_job_matching(data: dict[str, Any], context: dict[str, Any] | None = None) -> dict[str, Any]:
    context = context or {}
    raw = data.get("job_matching") if isinstance(data, dict) else {}
    if not isinstance(raw, dict):
        raw = data if isinstance(data, dict) else {}
    top_jobs = normalize_matching_jobs(raw.get("top_jobs") or raw.get("jobs") or raw.get("recommended_jobs"))
    selected_job = normalize_matching_job(raw.get("selected_job"), default_rank=1)
    if not selected_job and top_jobs:
        selected_job = top_jobs[0]
    if selected_job and not any(job.get("rank") == selected_job.get("rank") and job.get("title") == selected_job.get("title") for job in top_jobs):
        top_jobs = [selected_job, *top_jobs]
    top_jobs = normalize_matching_job_ranks(top_jobs[:5])

    selected_title = selected_job.get("title") if selected_job else ""
    selected_radar = selected_job.get("requirement_radar") if selected_job else None
    selected_summary = selected_job.get("fit_summary") if selected_job else ""
    target_role = str(raw.get("target_role") or selected_title or "").strip()
    overall_match = clamp_int(numeric_int(raw.get("overall_match"), numeric_int(selected_job.get("match") if selected_job else 0, 0)), 0, 100)
    match_level = str(raw.get("match_level") or match_level_label(overall_match))
    student_radar = normalize_radar_dimensions(raw.get("student_radar") or context.get("student_radar_hint"), allow_empty=False)
    target_radar = normalize_radar_dimensions(raw.get("target_radar") or selected_radar, allow_empty=False)
    if selected_job and not selected_job.get("requirement_radar"):
        selected_job["requirement_radar"] = target_radar
    report_sections = normalize_report_rows(raw.get("report_sections"), student_radar, target_radar)
    return {
        "target_role": target_role,
        "overall_match": overall_match,
        "match_level": match_level[:24],
        "source": str(raw.get("source") or "Legato Job Matching workflow"),
        "method_summary": str(raw.get("method_summary") or "")[:180],
        "fit_summary": str(raw.get("fit_summary") or selected_summary or "")[:220],
        "selected_job": selected_job,
        "student_radar": student_radar,
        "target_radar": target_radar,
        "report_sections": report_sections,
        "gap_details": normalize_gap_details(raw.get("gap_details")),
        "recommendations": normalize_text_list(raw.get("recommendations"), limit=5, max_chars=90),
        "recommended_reasons": normalize_text_list(raw.get("recommended_reasons") or raw.get("reasons"), limit=5, max_chars=90),
        "agent_notes": normalize_text_list(raw.get("agent_notes"), limit=5, max_chars=80),
        "top_jobs": top_jobs,
    }


def validate_job_matching(matching: dict[str, Any]) -> None:
    if not isinstance(matching, dict):
        raise ValueError("job_matching must be an object")
    if not matching.get("target_role"):
        raise ValueError("job_matching.target_role is required")
    if not isinstance(matching.get("overall_match"), int):
        raise ValueError("job_matching.overall_match must be an integer")
    for key in ("student_radar", "target_radar"):
        radar = matching.get(key)
        if not isinstance(radar, list) or len(radar) != 6:
            raise ValueError(f"job_matching.{key} must contain six dimensions")
    jobs = matching.get("top_jobs")
    if not isinstance(jobs, list) or len(jobs) == 0:
        raise ValueError("job_matching.top_jobs must contain at least one job")
    for index, job in enumerate(jobs):
        if not isinstance(job, dict):
            raise ValueError(f"job_matching.top_jobs[{index}] must be an object")
        if not job.get("title"):
            raise ValueError(f"job_matching.top_jobs[{index}].title is required")
        radar = job.get("requirement_radar")
        if not isinstance(radar, list) or len(radar) != 6:
            raise ValueError(f"job_matching.top_jobs[{index}].requirement_radar must contain six dimensions")


def normalize_matching_education(raw: Any) -> list[dict[str, Any]]:
    if not isinstance(raw, list):
        return []
    out: list[dict[str, Any]] = []
    for item in raw:
        if not isinstance(item, dict):
            continue
        out.append(
            {
                "school": str(item.get("school") or ""),
                "degree": str(item.get("degree") or item.get("degree_level") or ""),
                "department": str(item.get("department") or ""),
                "major": str(item.get("major") or ""),
                "is_985": bool(item.get("is_985") or item.get("is985")),
                "is_211": bool(item.get("is_211") or item.get("is211")),
                "is_double_first_class": bool(item.get("is_double_first_class")),
                "ruanke_rank": numeric_int(item.get("ruanke_rank") or item.get("ruankeRank"), 0),
                "school_kind": str(item.get("school_kind") or ""),
                "parent_school": str(item.get("parent_school") or ""),
            }
        )
    return out


def normalize_matching_awards(raw: Any) -> list[dict[str, Any]]:
    if not isinstance(raw, list):
        return []
    out: list[dict[str, Any]] = []
    for index, item in enumerate(raw):
        if not isinstance(item, dict):
            continue
        name = str(item.get("name") or "")
        result = str(item.get("result") or "")
        if not name and not result:
            continue
        out.append(
            {
                "key": f"award:{index}",
                "name": name,
                "result": result,
                "evidence_scope": normalize_evidence_scope(item.get("evidence_scope"), item),
                "level": numeric_float(item.get("level"), 0),
                "impact_factor": numeric_float(item.get("impact_factor"), 0),
                "benchmark_scores": normalize_score_distribution_for_context(item.get("benchmark_scores")),
                "reason": str(item.get("reason") or ""),
            }
        )
    return out


def normalize_matching_experiences(raw: Any) -> list[dict[str, Any]]:
    if not isinstance(raw, list):
        return []
    out: list[dict[str, Any]] = []
    for index, item in enumerate(raw):
        if not isinstance(item, dict):
            continue
        role = str(item.get("role") or "")
        contribution = str(item.get("contribution") or "")
        if not role and not contribution:
            continue
        out.append(
            {
                "key": f"experience:{index}",
                "type": str(item.get("type") or ""),
                "role": role,
                "contribution": contribution,
                "evidence_scope": normalize_evidence_scope(item.get("evidence_scope"), item),
                "level": numeric_float(item.get("level"), 0),
                "impact_factor": numeric_float(item.get("impact_factor"), 0),
                "benchmark_scores": normalize_score_distribution_for_context(item.get("benchmark_scores")),
                "reason": str(item.get("reason") or ""),
            }
        )
    return out


def normalize_matching_jobs(raw: Any) -> list[dict[str, Any]]:
    if not isinstance(raw, list):
        return []
    jobs: list[dict[str, Any]] = []
    for index, item in enumerate(raw):
        job = normalize_matching_job(item, default_rank=index + 1)
        if job:
            jobs.append(job)
    return normalize_matching_job_ranks(jobs[:5])


def normalize_matching_job(raw: Any, default_rank: int = 1) -> dict[str, Any]:
    if not isinstance(raw, dict):
        return {}
    title = str(raw.get("title") or raw.get("role") or "").strip()
    if not title:
        return {}
    requirement_radar = normalize_radar_dimensions(raw.get("requirement_radar") or raw.get("target_radar"), allow_empty=False)
    match = clamp_int(numeric_int(raw.get("match"), numeric_int(raw.get("overall_match"), 0)), 0, 100)
    return {
        "rank": clamp_int(numeric_int(raw.get("rank"), default_rank), 1, 99),
        "title": title[:40],
        "category": str(raw.get("category") or "")[:24],
        "match": match,
        "ability_match": clamp_int(numeric_int(raw.get("ability_match"), match), 0, 100),
        "experience_match": clamp_int(numeric_int(raw.get("experience_match"), 0), 0, 100),
        "education_gate": str(raw.get("education_gate") or "")[:30],
        "fit_summary": str(raw.get("fit_summary") or "")[:180],
        "risk": str(raw.get("risk") or "")[:160],
        "requirement_radar": requirement_radar,
        "reasons": normalize_text_list(raw.get("reasons"), limit=4, max_chars=70),
        "next_proof": str(raw.get("next_proof") or "")[:100],
    }


def normalize_matching_job_ranks(jobs: list[dict[str, Any]]) -> list[dict[str, Any]]:
    ranked = sorted(jobs, key=lambda item: numeric_int(item.get("rank"), 99))
    for index, job in enumerate(ranked):
        job["rank"] = index + 1
    return ranked


def normalize_radar_dimensions(raw: Any, *, allow_empty: bool) -> list[dict[str, Any]]:
    values = normalize_radar_score_values(raw, min_value=0, max_value=100)
    if not values and allow_empty:
        return []
    if not values:
        values = [50, 50, 50, 50, 50, 50]
    return [{"name": name, "score": score, "max_score": 100} for name, score in zip(BENCHMARK_DIMENSIONS, values)]


def normalize_radar_score_values(raw: Any, *, min_value: int, max_value: int) -> list[int]:
    if isinstance(raw, dict):
        values = [raw.get(name) for name in BENCHMARK_DIMENSIONS]
    elif isinstance(raw, list):
        if raw and all(isinstance(item, dict) for item in raw):
            lookup = {str(item.get("name") or item.get("dimension") or ""): item.get("score") for item in raw}
            values = [lookup.get(name) for name in BENCHMARK_DIMENSIONS]
        else:
            values = raw[:6]
    else:
        values = []
    if len(values) < 6:
        return []
    scores: list[int] = []
    for value in values[:6]:
        score = numeric_float(value, 0)
        if 0 <= score <= 1:
            score *= 100
        elif 1 < score <= 10:
            score *= 10
        scores.append(clamp_int(round(score), min_value, max_value))
    return scores


def normalize_score_distribution_for_context(raw: Any) -> list[float]:
    scores = normalize_six_dim_scores(raw)
    return [round(score, 3) for score in scores]


def normalize_report_rows(raw: Any, student_radar: list[dict[str, Any]], target_radar: list[dict[str, Any]]) -> list[dict[str, Any]]:
    rows: list[dict[str, Any]] = []
    if isinstance(raw, list):
        for item in raw:
            if not isinstance(item, dict):
                continue
            name = str(item.get("name") or item.get("capability") or "").strip()
            if not name:
                continue
            student = clamp_int(numeric_int(item.get("student"), 0), 0, 100)
            role_need = clamp_int(numeric_int(item.get("role_need") or item.get("target"), 0), 0, 100)
            rows.append(
                {
                    "name": name[:16],
                    "student": student,
                    "role_need": role_need,
                    "difference": clamp_int(numeric_int(item.get("difference"), student - role_need), -100, 100),
                }
            )
    if rows:
        return rows[:6]
    target_by_name = {item["name"]: item["score"] for item in target_radar}
    for item in student_radar:
        target = clamp_int(numeric_int(target_by_name.get(item["name"]), 0), 0, 100)
        student = clamp_int(numeric_int(item.get("score"), 0), 0, 100)
        rows.append({"name": item["name"], "student": student, "role_need": target, "difference": student - target})
    return rows[:6]


def normalize_gap_details(raw: Any) -> list[dict[str, Any]]:
    if not isinstance(raw, list):
        return []
    details: list[dict[str, Any]] = []
    for item in raw:
        if not isinstance(item, dict):
            continue
        capability = str(item.get("capability") or item.get("name") or "").strip()
        if not capability:
            continue
        details.append(
            {
                "capability": capability[:24],
                "current": str(item.get("current") or "")[:90],
                "expected": str(item.get("expected") or "")[:90],
                "action": str(item.get("action") or "")[:100],
                "severity": str(item.get("severity") or "")[:12],
            }
        )
    return details[:6]


def normalize_text_list(raw: Any, *, limit: int, max_chars: int) -> list[str]:
    if not isinstance(raw, list):
        return []
    out: list[str] = []
    for item in raw:
        text = str(item or "").strip()
        if text:
            out.append(text[:max_chars])
    return out[:limit]


def match_level_label(score: int) -> str:
    if score >= 85:
        return "强匹配"
    if score >= 75:
        return "高潜力匹配"
    if score >= 65:
        return "可迁移匹配"
    return "需补证据"


def compact_resume_excerpt(resume_text: str) -> str:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    selected = [line for line in lines if any(keyword in line for keyword in ("教育", "经历", "项目", "实习", "比赛", "竞赛", "证书", "技能", "奖"))]
    if not selected:
        selected = lines[:80]
    return "\n".join(selected[:120])[:6000]


def local_major_baseline(context: dict[str, Any] | None = None) -> dict[str, Any]:
    context = context or {}
    major_name = str(context.get("major_name_hint") or "")
    major_family = str(context.get("major_family_hint") or infer_major_family(major_name))
    if major_family not in MAJOR_FAMILIES:
        major_family = "未知"
    base_score = clamp_int(numeric_int(context.get("base_score_hint"), 50), 30, 85)
    school_hint = context.get("school_quality_hint") if isinstance(context.get("school_quality_hint"), dict) else {}
    school_bonus = clamp_int(numeric_int(school_hint.get("school_bonus_hint"), 0), 0, 3)
    school_penalty = clamp_int(numeric_int(school_hint.get("school_penalty_hint"), 0), 0, 10)
    specialty_bonus = clamp_int(numeric_int(school_hint.get("specialty_bonus_hint"), 0), 0, 2)
    adjusted_base = clamp_int(base_score + school_bonus - school_penalty, 30, 85)
    score_base = clamp_int(base_score + school_bonus - school_penalty, 30, 85)
    effective_school_bonus = 0 if school_penalty > 0 else school_bonus
    scores = apply_school_major_adjustments(
        major_family_scores(score_base, major_family, bool(major_name)),
        major_family,
        effective_school_bonus,
        specialty_bonus,
    )
    return {
        "major_name": major_name,
        "major_family": major_family,
        "base_score": adjusted_base,
        "dimensions": list(BENCHMARK_DIMENSIONS),
        "scores": scores,
        "rationale": local_major_baseline_rationale(major_family, school_hint),
        "confidence": local_major_baseline_confidence(major_family, school_hint),
        "source": "local_major_baseline",
    }


def normalize_major_baseline_scores(raw_scores: Any, fallback_scores: list[int]) -> list[int]:
    if isinstance(raw_scores, dict):
        values = [raw_scores.get(name) for name in BENCHMARK_DIMENSIONS]
    elif isinstance(raw_scores, list):
        values = raw_scores[:6]
    else:
        values = []
    if len(values) < 6:
        values = values + fallback_scores[len(values):]
    normalized: list[int] = []
    for index, value in enumerate(values[:6]):
        score = numeric_float(value, fallback_scores[index])
        if 0 <= score <= 1:
            score *= 100
        elif 1 < score <= 10:
            score *= 10
        normalized.append(clamp_int(round(score), 25, 85))
    return normalized


def major_family_scores(base_score: int, family: str, has_major: bool) -> list[int]:
    offsets = {
        "工科类": [4, -4, 7, -8, -2, 2],
        "理科类": [6, -4, 5, -8, -2, 2],
        "文科类": [0, 7, 5, -6, -3, 2],
        "商科类": [2, 5, 5, -2, -2, 2],
        "医农类": [2, -3, 7, -8, 2, 1],
        "艺术体育类": [-3, 5, 6, -6, 3, 2],
        "交叉类": [3, 3, 6, -6, -1, 2],
        "未知": [0, 0, 2 if has_major else 0, -8, -4, 0],
    }
    return [clamp_int(base_score + offset, 25, 85) for offset in offsets.get(family, offsets["未知"])]


def apply_school_major_adjustments(
    scores: list[int],
    major_family: str,
    school_bonus: int,
    specialty_bonus: int,
) -> list[int]:
    adjusted = list(scores[:6]) + [50] * max(0, 6 - len(scores))
    school_offsets = [
        min(school_bonus, 3),
        0,
        min(school_bonus, 3),
        1 if school_bonus >= 3 else 0,
        1 if school_bonus >= 2 else 0,
        1 if school_bonus >= 2 else 0,
    ]
    specialty_offsets = [0, 0, min(specialty_bonus, 2), 0, 0, 1 if specialty_bonus >= 2 else 0]
    if specialty_bonus > 0:
        if major_family in ("工科类", "理科类", "医农类"):
            specialty_offsets[0] += 1
        elif major_family in ("文科类", "商科类", "艺术体育类"):
            specialty_offsets[1] += 1
        elif major_family == "交叉类":
            specialty_offsets[0] += 1
            specialty_offsets[1] += 1
    return [clamp_int(score + school_offsets[index] + specialty_offsets[index], 25, 85) for index, score in enumerate(adjusted[:6])]


def school_quality_hint(education: list[dict[str, Any]], major_name: str, major_text: str) -> dict[str, Any]:
    if not education:
        return {
            "school_tier": "未知",
            "school_bonus_hint": 0,
            "school_penalty_hint": 0,
            "specialty_alignment_hint": "未知",
            "specialty_bonus_hint": 0,
        }
    scored = sorted(
        [school_quality_for_item(item, major_name, major_text) for item in education],
        key=lambda item: (item["school_bonus_hint"] - item["school_penalty_hint"], item["specialty_bonus_hint"]),
        reverse=True,
    )
    return combined_school_quality_hint(scored)


def combined_school_quality_hint(scored: list[dict[str, Any]]) -> dict[str, Any]:
    if not scored:
        return {
            "school_tier": "未知",
            "school_bonus_hint": 0,
            "school_penalty_hint": 0,
            "specialty_alignment_hint": "未知",
            "specialty_bonus_hint": 0,
        }
    primary = dict(scored[0])
    primary_tier = str(primary.get("school_tier") or "未知")
    has_independent_history = any(
        str(item.get("school_tier") or "") == "独立学院/原三本"
        for item in scored
        if item is not scored[0]
    )
    if has_independent_history and primary_tier != "独立学院/原三本":
        existing_penalty = numeric_int(primary.get("school_penalty_hint"), 0)
        existing_bonus = numeric_int(primary.get("school_bonus_hint"), 0)
        history_penalty = 2 if existing_bonus > 0 else 3
        primary["school_penalty_hint"] = clamp_int(existing_penalty + history_penalty, 0, 10)
        primary["school_tier"] = f"{primary_tier}；含独立学院/原三本学历"
        if str(primary.get("specialty_alignment_hint") or "") == "未知":
            primary["specialty_alignment_hint"] = "独立学院背景不继承母校层次"
    return primary


def school_quality_for_item(item: dict[str, Any], major_name: str, major_text: str) -> dict[str, Any]:
    rank = numeric_int(item.get("ruanke_rank"), 0)
    is_985 = bool(item.get("is_985"))
    is_211 = bool(item.get("is_211"))
    is_double_first_class = bool(item.get("is_double_first_class"))
    school = str(item.get("school") or "")
    school_kind = str(item.get("school_kind") or "")
    independent = school_kind == "independent_college" or independent_college_for(school) is not None
    bonus = 0
    penalty = 0
    if independent:
        penalty = 10
    if rank > 0:
        if is_985 or is_211 or is_double_first_class:
            if rank <= 50:
                bonus = 3
            elif rank <= 150:
                bonus = 2
            else:
                bonus = 1
        else:
            penalty = max(penalty, school_penalty_for_rank(rank))
            if rank <= 100:
                bonus = max(bonus, 1)
    elif not (is_985 or is_211 or is_double_first_class):
        penalty = max(penalty, 2)
    if is_985:
        bonus = max(bonus, 3)
    elif is_211 or is_double_first_class:
        bonus = max(bonus, 2)
    specialty_bonus, specialty_label = specialty_alignment_bonus(school, major_name or str(item.get("major") or "") or major_text)
    if independent:
        bonus = 0
        specialty_bonus = min(specialty_bonus, 1)
        if specialty_label == "未见明确特色专业匹配":
            specialty_label = "独立学院不继承母校特色"
    return {
        "school": school,
        "school_tier": "独立学院/原三本" if independent else school_tier_label(is_985, is_211, is_double_first_class, rank),
        "school_bonus_hint": clamp_int(bonus, 0, 3),
        "school_penalty_hint": penalty,
        "specialty_alignment_hint": specialty_label,
        "specialty_bonus_hint": specialty_bonus,
    }


def school_penalty_for_rank(rank: int) -> int:
    if rank <= 0:
        return 2
    if rank <= 100:
        return 0
    if rank <= 150:
        return 1
    if rank <= 250:
        return 2
    if rank <= 400:
        return 4
    return 6


def school_tier_label(is_985: bool, is_211: bool, is_double_first_class: bool, rank: int) -> str:
    tags: list[str] = []
    if is_985:
        tags.append("985")
    if is_211:
        tags.append("211")
    if is_double_first_class:
        tags.append("双一流")
    if rank > 0:
        tags.append(f"软科#{rank}")
    return "/".join(tags) if tags else "普通或未知层次"


def specialty_alignment_bonus(school: str, major: str) -> tuple[int, str]:
    text = f"{school}{major}"
    rules = [
        (("电子", "邮电", "通信", "信息", "网络", "软件", "科技", "理工", "工业"), ("计算机", "软件", "网络", "信息", "电子", "通信", "自动化", "人工智能", "数据"), "工科/信息类特色较匹配"),
        (("航空", "航天", "交通", "电力", "矿业", "石油", "建筑", "地质", "海洋"), ("工程", "机械", "交通", "能源", "电气", "土木", "地质", "海洋", "自动化"), "行业工科特色较匹配"),
        (("农业", "林业", "农林"), ("农学", "园艺", "植物", "动物", "兽医", "食品", "林学", "生物"), "农林生命类特色较匹配"),
        (("医科", "医学", "中医药", "药科"), ("医学", "临床", "护理", "药学", "口腔", "公共卫生", "中药"), "医药类特色较匹配"),
        (("财经", "商业", "工商"), ("经济", "金融", "会计", "财务", "管理", "市场营销", "审计"), "财经商科特色较匹配"),
        (("政法",), ("法学", "法律", "政治", "社会学"), "政法类特色较匹配"),
        (("师范",), ("教育", "心理", "汉语言", "英语", "数学", "物理", "化学", "生物"), "师范培养特色较匹配"),
        (("外国语", "外语"), ("英语", "日语", "翻译", "语言", "国际"), "语言类特色较匹配"),
        (("美术", "音乐", "戏剧", "传媒", "体育"), ("艺术", "设计", "音乐", "表演", "播音", "传媒", "体育"), "艺术体育传媒特色较匹配"),
    ]
    for school_signals, major_signals, label in rules:
        if any(signal in school for signal in school_signals) and any(signal in major for signal in major_signals):
            return 2, label
    if any(signal in text for signal in ("国家重点学科", "一流学科", "王牌专业", "特色专业", "优势专业")):
        return 2, "材料中出现优势专业线索"
    return 0, "未见明确特色专业匹配"


def local_major_baseline_rationale(major_family: str, school_hint: dict[str, Any]) -> str:
    tier = str(school_hint.get("school_tier") or "未知")
    alignment = str(school_hint.get("specialty_alignment_hint") or "未知")
    if tier == "未知" and alignment == "未知":
        return f"按{major_family}专业培养要求和成绩线索给出保守基础分。"
    return f"按{major_family}专业、{tier}学校层次和{alignment}给出边际调整。"


def local_major_baseline_confidence(major_family: str, school_hint: dict[str, Any]) -> float:
    confidence = 0.55 if major_family == "未知" else 0.68
    if numeric_int(school_hint.get("school_bonus_hint"), 0) > 0:
        confidence += 0.06
    if numeric_int(school_hint.get("specialty_bonus_hint"), 0) > 0:
        confidence += 0.04
    return round(min(confidence, 0.82), 2)


def infer_major_family(text: Any) -> str:
    normalized = str(text or "")
    if not normalized.strip():
        return "未知"
    rules = [
        ("艺术体育类", ("艺术", "设计", "视觉", "音乐", "美术", "舞蹈", "体育", "播音", "表演")),
        ("医农类", ("医学", "临床", "护理", "药学", "口腔", "公共卫生", "农学", "园艺", "植物", "动物", "兽医", "食品科学", "林学")),
        ("工科类", ("计算机", "软件", "网络", "信息安全", "网络空间安全", "人工智能", "数据科学", "电子", "电气", "自动化", "通信", "机械", "土木", "材料", "工程", "车辆", "能源")),
        ("理科类", ("数学", "物理", "化学", "生物科学", "统计学", "地理信息", "应用统计")),
        ("商科类", ("经济", "金融", "会计", "财务", "工商管理", "市场营销", "人力资源", "国际贸易", "物流", "电子商务", "审计")),
        ("文科类", ("汉语言", "中文", "新闻", "传播", "法学", "法律", "历史", "哲学", "外语", "英语", "日语", "教育学", "社会学", "心理学")),
    ]
    matched = [family for family, signals in rules if any(signal in normalized for signal in signals)]
    if not matched:
        return "未知"
    if len(set(matched)) > 1:
        return "交叉类"
    return matched[0]


def academic_base_score_from_transcript(transcript_use: str) -> int:
    text = str(transcript_use or "")
    gpa_match = re.search(r"(?:GPA|绩点)[:：]?\s*([0-9]+(?:\.[0-9]+)?)", text, re.IGNORECASE)
    if not gpa_match:
        score_match = re.search(r"(?:均分|平均分|平均成绩)[:：]?\s*([0-9]+(?:\.[0-9]+)?)", text)
        if not score_match:
            return 50
        average = numeric_float(score_match.group(1), 80)
        return academic_average_to_prior(average)
    raw = numeric_float(gpa_match.group(1), 0)
    if raw <= 0:
        return 50
    if raw <= 4.3:
        estimated_average = 80 + (raw - 3.0) * 15
        return academic_average_to_prior(estimated_average)
    if raw <= 5:
        estimated_average = 80 + (raw - 3.5) * 10
        return academic_average_to_prior(estimated_average)
    return academic_average_to_prior(raw)


def academic_average_to_prior(average: float) -> int:
    if not isinstance(average, (int, float)) or average <= 0:
        return 50
    return clamp_int(round(50 + (float(average) - 80) * 1.6), 35, 78)


def first_non_empty(values: list[Any]) -> str:
    for value in values:
        text = str(value or "").strip()
        if text:
            return text
    return ""


def numeric_float(value: Any, default: float) -> float:
    if isinstance(value, (int, float)) and not isinstance(value, bool):
        return float(value)
    try:
        return float(str(value).strip())
    except (TypeError, ValueError):
        return float(default)


def numeric_int(value: Any, default: int) -> int:
    return int(round(numeric_float(value, default)))


def clamp_int(value: int, lower: int, upper: int) -> int:
    return max(lower, min(int(value), upper))


def normalize_benchmark_input_items(raw_items: Any) -> list[dict[str, Any]]:
    if not isinstance(raw_items, list):
        return []
    items: list[dict[str, Any]] = []
    for index, raw_item in enumerate(raw_items):
        if not isinstance(raw_item, dict):
            continue
        kind = str(raw_item.get("kind") or raw_item.get("type") or "award")
        if kind not in ("award", "experience"):
            kind = "award"
        item = {
            "kind": kind,
            "key": str(raw_item.get("key") or f"{kind}:{index}"),
            "name": str(raw_item.get("name") or raw_item.get("role") or raw_item.get("contribution") or ""),
            "result": str(raw_item.get("result") or ""),
        }
        if kind == "experience":
            item.update(
                {
                    "type": str(raw_item.get("experience_type") or raw_item.get("type") or ""),
                    "role": str(raw_item.get("role") or ""),
                    "contribution": str(raw_item.get("contribution") or ""),
                }
            )
        level = raw_item.get("level")
        if isinstance(level, (int, float)) and not isinstance(level, bool):
            item["level"] = max(0, min(float(level), 10))
        item["evidence_scope"] = normalize_evidence_scope(raw_item.get("evidence_scope"), item)
        if item["name"] or item["result"] or item.get("contribution"):
            items.append(item)
    return items


def benchmark_items_from_certifications(raw_items: Any) -> list[dict[str, Any]]:
    items: list[dict[str, Any]] = []
    if not isinstance(raw_items, list):
        return items
    for index, raw_item in enumerate(raw_items):
        if not isinstance(raw_item, dict):
            continue
        name = str(raw_item.get("name") or "")
        result = str(raw_item.get("result") or "")
        if not name and not result:
            continue
        item: dict[str, Any] = {
            "kind": "award",
            "key": f"award:{index}",
            "name": name,
            "result": result,
            "evidence_scope": normalize_evidence_scope(raw_item.get("evidence_scope"), raw_item),
        }
        level = raw_item.get("level")
        if isinstance(level, (int, float)) and not isinstance(level, bool):
            item["level"] = max(0, min(float(level), 10))
        items.append(item)
    return items


def normalize_item_benchmark(item: dict[str, Any], data: dict[str, Any]) -> dict[str, Any]:
    raw_scores = data.get("scores") or data.get("score_vector") or data.get("dimension_scores")
    scores = normalize_six_dim_scores(raw_scores)
    impact = data.get("impact_factor", data.get("level", data.get("impact", 0)))
    if not isinstance(impact, (int, float)) or isinstance(impact, bool):
        impact = local_item_impact(item)
    impact = max(0, min(float(impact), 10))
    impact = calibrate_item_impact(item, impact)
    normalized_item: dict[str, Any] = {
        "kind": str(item.get("kind", "award")),
        "key": str(item.get("key", "")),
        "name": str(item.get("name", "")),
        "result": str(item.get("result", "")),
    }
    for key in ("type", "role", "contribution", "level"):
        if key in item:
            normalized_item[key] = item[key]
    normalized_item["evidence_scope"] = normalize_evidence_scope(item.get("evidence_scope"), normalized_item)
    return {
        "item": normalized_item,
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
    untitled_project_cap = untitled_professional_project_cap(item)
    if untitled_project_cap is not None:
        return round(min(impact, untitled_project_cap), 1)
    if is_campus_award_or_honor(item):
        return round(min(impact, 4.0), 1)
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
            "kind": str(item.get("kind", "award")),
            "key": str(item.get("key", "")),
            "name": str(item.get("name", "")),
            "result": str(item.get("result", "")),
            "evidence_scope": normalize_evidence_scope(item.get("evidence_scope"), item),
            **{key: item[key] for key in ("type", "role", "contribution", "level") if key in item},
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


def normalize_evidence_scope(value: Any, item: dict[str, Any] | None = None) -> str:
    text_value = str(value or "").strip()
    if text_value in EVIDENCE_SCOPES:
        return text_value
    text = f"{text_value}{item_text(item or {})}"
    campus_signals = (
        "校内",
        "校级",
        "院级",
        "学院",
        "学校",
        "校学生会",
        "院学生会",
        "学生会",
        "社团",
        "协会",
        "班级",
        "班长",
        "团支书",
        "优秀学生",
        "优秀学生干部",
        "三好学生",
        "奖学金",
        "实验室",
        "大学生创新创业训练计划",
        "大创",
    )
    external_signals = (
        "校外",
        "全国",
        "国家级",
        "省级",
        "省",
        "市级",
        "市",
        "区域",
        "赛区",
        "国际",
        "企业",
        "公司",
        "集团",
        "实习",
        "英语四级",
        "英语六级",
        "CET",
        "计算机等级",
        "蓝桥",
        "ACM",
        "ICPC",
        "CTF",
        "挑战杯",
        "互联网+",
    )
    if any(signal in text for signal in external_signals):
        return "校外"
    if any(signal in text for signal in campus_signals):
        return "校内"
    if str((item or {}).get("kind", "")) == "experience" and any(signal in text for signal in ("项目", "科研", "研究")):
        return "校内"
    return "校外"


def item_text(item: dict[str, Any]) -> str:
    return (
        f"{item.get('name', '')}{item.get('result', '')}"
        f"{item.get('type', '')}{item.get('role', '')}{item.get('contribution', '')}"
    )


def untitled_professional_project_cap(item: dict[str, Any]) -> float | None:
    if str(item.get("kind", "")) != "experience":
        return None
    text = item_text(item)
    if any(signal in text for signal in ("实习", "比赛", "竞赛", "任职", "社团", "学生会")):
        return None
    if not any(signal in text for signal in ("项目", "科研", "研究", "课题", "实验", "系统", "平台", "模型", "算法", "开发", "漏洞", "测试", "数据")):
        return None
    if has_concrete_project_title(str(item.get("role", "")), item):
        return None
    contribution = str(item.get("contribution", ""))
    has_detail = len(contribution) >= 8 and any(
        signal in text
        for signal in ("开发", "实现", "实验", "模型", "算法", "系统", "平台", "漏洞", "CVE", "RCE", "覆盖率", "数据")
    )
    return 4.0 if has_detail else 3.0


def has_concrete_project_title(title: str, item: dict[str, Any]) -> bool:
    title = re.sub(r"\s+", "", str(title or ""))
    if len(title) < 4:
        return False
    contribution = re.sub(r"\s+", "", str(item.get("contribution", "")))
    type_value = re.sub(r"\s+", "", str(item.get("type", "")))
    if title in {contribution, type_value}:
        return False
    generic_titles = {
        "项目",
        "科研项目",
        "研究项目",
        "项目经历",
        "科研经历",
        "参与者",
        "参与人",
        "成员",
        "队员",
        "负责人",
        "核心成员",
        "角色未解析",
        "未解析",
    }
    return title not in generic_titles


def is_campus_award_or_honor(item: dict[str, Any]) -> bool:
    if str(item.get("kind", "award")) == "experience":
        return False
    text = item_text(item)
    scope = normalize_evidence_scope(item.get("evidence_scope"), item)
    return scope == "校内" or any(signal in text for signal in ("校级", "院级", "校内", "学院", "学校"))


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
        "evidence_scope": normalize_evidence_scope(candidate.get("evidence_scope"), candidate),
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
    untitled_project_cap = untitled_professional_project_cap({"kind": "experience", **item})
    if untitled_project_cap is not None:
        return min(level, int(untitled_project_cap))
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
    independent = independent_college_for(school_name)
    if independent:
        return {
            "matched_school": independent["name"],
            "is_985": False,
            "is_211": False,
            "is_double_first_class": False,
            "ruanke_rank": None,
            "school_kind": independent.get("kind", "independent_college"),
            "parent_school": independent.get("parent_school", ""),
        }
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


def independent_college_for(school_name: str) -> dict[str, str] | None:
    normalized = normalize_school_name(school_name)
    if not normalized:
        return None
    index = load_independent_colleges()
    exact = index.get(normalized)
    if exact:
        return exact
    return None


def normalize_school_name(name: str) -> str:
    text = re.sub(r"[\s·•,，。|/\\()（）【】\[\]{}:：;；\-至0-9]+", "", name)
    for suffix in ("本科", "硕士", "博士", "学院", "大学"):
        if text.endswith(suffix) and suffix in ("本科", "硕士", "博士"):
            text = text[: -len(suffix)]
    return text


@lru_cache(maxsize=1)
def load_independent_colleges() -> dict[str, dict[str, str]]:
    if not INDEPENDENT_COLLEGE_CACHE.exists():
        return {}
    with INDEPENDENT_COLLEGE_CACHE.open(encoding="utf-8") as handle:
        payload = json.load(handle)
    if isinstance(payload, list):
        return {
            normalized: {
                "name": name,
                "kind": "independent_college",
                "parent_school": infer_parent_school_for_independent_college(name),
            }
            for raw_name in payload
            if isinstance(raw_name, str)
            for name in [raw_name.strip()]
            for normalized in [normalize_school_name(name)]
            if normalized
        }
    if not isinstance(payload, dict):
        return {}
    out: dict[str, dict[str, str]] = {}
    for name, info in payload.items():
        if not isinstance(info, dict):
            continue
        normalized = normalize_school_name(str(name))
        if not normalized:
            continue
        copied = {str(key): str(value) for key, value in info.items()}
        copied["name"] = str(name)
        out[normalized] = copied
    return out


def infer_parent_school_for_independent_college(name: str) -> str:
    normalized = normalize_school_name(name)
    candidates: list[tuple[int, str]] = []
    for ranked_name in load_school_rankings():
        normalized_ranked = normalize_school_name(ranked_name)
        if normalized_ranked and normalized.startswith(normalized_ranked) and normalized != normalized_ranked:
            candidates.append((len(normalized_ranked), ranked_name))
    if not candidates:
        return ""
    _, parent = max(candidates)
    return parent


@lru_cache(maxsize=1)
def load_school_rankings() -> dict[str, dict[str, Any]]:
    with RANKING_CACHE.open(encoding="utf-8") as handle:
        payload = json.load(handle)
    if not isinstance(payload, dict):
        return {}
    return {str(name): info for name, info in payload.items() if isinstance(info, dict)}
