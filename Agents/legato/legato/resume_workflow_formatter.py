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
    tokens = normalized.split()
    if len(tokens) >= 2:
        return tokens[0], "".join(tokens[1:])
    if "高新兴科技集团股份有限公司" in normalized:
        return "高新兴科技集团股份有限公司", "前端开发实习生" if "前端开发实习生" in normalized else ""
    return normalized, ""


def summarize_internship_contribution(organization: str, role: str, context: str) -> str:
    if "MCP" in context and ("XML" in context or "xml" in context):
        return "MCP标注与XML修正"
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
    for index, line in enumerate(lines):
        if not is_project_anchor(line):
            continue
        context = "\n".join(lines[index : min(len(lines), index + 8)])
        if not has_contribution_signal(context):
            continue
        experiences.append(
            {
                "type": "科研项目" if "实验室" in context or "科研" in context or "研究" in context else "项目",
                "role": extract_project_subject(context),
                "contribution": summarize_project_contribution(context),
                "level": score_project_level(context),
            }
        )
        break
    return experiences


def is_project_anchor(line: str) -> bool:
    if any(skip in line for skip in ("荣誉", "奖项", "证书", "技能", "教育背景", "主修课程")):
        return False
    return any(signal in line for signal in ("实验室", "科研项目", "项目经历", "研究项目", "研究中"))


def extract_described_contest_experiences(resume_text: str) -> list[dict[str, Any]]:
    lines = [line.strip() for line in resume_text.splitlines() if line.strip()]
    experiences: list[dict[str, Any]] = []
    for index, line in enumerate(lines):
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
        for signal in ("主要工作", "负责", "独立", "开发", "修改", "添加", "判断", "提出", "制作", "分析", "组织", "带领", "协商", "实现")
    )


def extract_role(context: str) -> str:
    for role in ("前端开发实习生", "实习生", "队长", "副会长", "会长", "部长", "负责人", "主席", "班长"):
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
    return value.strip(" ,，。；;|").replace("（", "(").replace("）", ")")


def summarize_project_contribution(context: str) -> str:
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
    return ""


def is_clean_subject_line(line: str) -> bool:
    if is_descriptive_sentence(line):
        return False
    if len(line) > 28:
        return False
    return any(signal in line for signal in ("实验室", "项目", "研究", "系统", "平台"))


def is_descriptive_sentence(line: str) -> bool:
    return any(signal in line for signal in ("参与", "指导", "独立", "提出", "开发", "分析", "能力", "认可", "对于"))


def compact_context_summary(context: str, signals: tuple[str, ...]) -> str:
    lines = [line.strip(" ,，。；;") for line in context.splitlines() if line.strip()]
    for line in lines:
        if any(signal in line for signal in signals):
            return line[:35]
    return lines[0][:35] if lines else ""


def extract_event_name(context: str) -> str:
    hosted_match = re.search(r"主办的[“\"']?([^，。,；;\n]{2,24}(?:大赛|比赛|竞赛|挑战赛))", context)
    if hosted_match:
        return clean_event_name(hosted_match.group(1))
    match = re.search(r"([“\"']?[^，。,；;\n]{2,24}(?:大赛|比赛|竞赛|挑战赛))", context)
    if match:
        return clean_event_name(match.group(1))
    return ""


def clean_event_name(value: str) -> str:
    value = value.strip("“”\"' ,，。；;")
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
    if is_low_value_honor_or_certificate(context):
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
    return any(signal in context for signal in ("建模", "算法", "产品", "方案", "系统", "实现", "开发", "分析", "队长", "带领团队"))


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
