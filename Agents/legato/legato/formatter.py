from __future__ import annotations

import json
import os
import re
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any

from .schemas import schema_for


@dataclass(frozen=True)
class FormatResult:
    data: dict[str, Any]
    formatter: str
    warnings: list[str]


class PrestoFormatter:
    def __init__(self, base_url: str | None = None, timeout_seconds: float = 2.5) -> None:
        self.base_url = (base_url or os.getenv("LEGATO_PRESTO_URL") or "http://127.0.0.1:8080").rstrip("/")
        self.timeout_seconds = timeout_seconds

    def format(self, markdown: str, target: str) -> FormatResult:
        prompt = build_formatter_prompt(markdown, target)
        session = self._request("POST", "/sessions", {"metadata": {"app": "legato", "target": target}})
        session_id = session["id"]
        run = self._request("POST", f"/sessions/{session_id}/runs", {"message": prompt})
        output = run.get("output") or ""
        if run.get("error"):
            raise RuntimeError(f"presto run failed: {run['error']}")
        try:
            data, warnings = extract_json_object_with_warnings(output)
            return FormatResult(data=data, formatter="presto", warnings=warnings)
        except ValueError as exc:
            fallback = LocalRuleFormatter().format(markdown, target)
            return FormatResult(
                data=fallback.data,
                formatter="presto_local_fallback",
                warnings=[
                    "presto formatter returned invalid JSON; used local Legato fallback: "
                    + safe_error_message(exc),
                    *fallback.warnings,
                ],
            )

    def _request(self, method: str, path: str, payload: dict[str, Any]) -> dict[str, Any]:
        body = json.dumps(payload, ensure_ascii=False).encode("utf-8")
        req = urllib.request.Request(
            self.base_url + path,
            data=body,
            method=method,
            headers={"Content-Type": "application/json"},
        )
        try:
            with urllib.request.urlopen(req, timeout=self.timeout_seconds) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except urllib.error.URLError as exc:
            raise RuntimeError(f"cannot reach Presto at {self.base_url}: {exc}") from exc


class LocalRuleFormatter:
    def format(self, markdown: str, target: str) -> FormatResult:
        if target == "resume":
            data = parse_resume_markdown(markdown)
        elif target == "transcript":
            data = parse_transcript_markdown(markdown)
        else:
            raise ValueError(f"unsupported target {target!r}")
        return FormatResult(
            data=data,
            formatter="local_rules",
            warnings=["local rule formatter is for acceptance and smoke tests; use Presto for production formatting"],
        )


def build_formatter_prompt(markdown: str, target: str) -> str:
    schema = schema_for(target)
    return (
        "You are Legato's formatter. Convert the recognized Markdown into the target JSON.\n"
        "Return only one valid JSON object. Do not include markdown fences or commentary.\n"
        "Every string must be valid JSON: escape quotes, backslashes, and newlines.\n"
        "Do not omit commas between object fields or array items.\n\n"
        f"Target: {target}\n"
        f"JSON schema:\n{json.dumps(schema, ensure_ascii=False, indent=2)}\n\n"
        "Rules:\n"
        "- Preserve all explicitly stated values.\n"
        "- Use empty strings or empty arrays when a field is not present.\n"
        "- For transcripts, preserve course rows and term grouping.\n\n"
        "Recognized Markdown:\n"
        f"{markdown}"
    )


def extract_json_object(text: str) -> dict[str, Any]:
    data, _ = extract_json_object_with_warnings(text)
    return data


def extract_json_object_with_warnings(text: str) -> tuple[dict[str, Any], list[str]]:
    stripped = text.strip()
    if stripped.startswith("```"):
        stripped = re.sub(r"^```(?:json)?\s*", "", stripped)
        stripped = re.sub(r"\s*```$", "", stripped)
    start = stripped.find("{")
    end = stripped.rfind("}")
    if start < 0 or end < start:
        raise ValueError("formatter output does not contain a JSON object")
    candidate = stripped[start : end + 1]
    try:
        return json.loads(candidate), []
    except json.JSONDecodeError as original:
        repaired = repair_json_text(candidate)
        if repaired != candidate:
            try:
                return json.loads(repaired), ["repaired invalid JSON returned by Presto formatter"]
            except json.JSONDecodeError:
                pass
        raise original


def repair_json_text(text: str) -> str:
    repaired = text.strip()
    repaired = re.sub(r",(\s*[}\]])", r"\1", repaired)
    repaired = re.sub(r'([}\]"])\s*\n\s*("[-A-Za-z0-9_\u4e00-\u9fff]+":)', r"\1,\n\2", repaired)
    repaired = re.sub(r"([}\]])\s*\n\s*([{[])", r"\1,\n\2", repaired)
    repaired = re.sub(r'("\s*:\s*"[^"]*")\s+("[-A-Za-z0-9_\u4e00-\u9fff]+":)', r"\1, \2", repaired)
    return repaired


def safe_error_message(exc: Exception) -> str:
    message = str(exc).replace("\n", " ").strip()
    return message[:240]


def parse_resume_markdown(markdown: str) -> dict[str, Any]:
    lines = normalized_lines(markdown)
    name = extract_resume_name(lines)
    email_match = re.search(r"[\w.+-]+@[\w.-]+\.[A-Za-z]{2,}", markdown)
    phone_match = re.search(r"(?:\+?\d[\d\s().-]{7,}\d)", markdown)
    links = re.findall(r"https?://\S+|(?:github|linkedin)\.com/\S+", markdown, flags=re.IGNORECASE)
    skills = extract_list_after_label(markdown, "skills")
    education = collect_section_items(lines, "education")
    experience = collect_section_items(lines, "experience")
    projects = collect_section_items(lines, "projects")
    certifications = collect_section_items(lines, "certifications")
    return {
        "candidate": {"name": name, "headline": ""},
        "contacts": {
            "email": email_match.group(0) if email_match else "",
            "phone": phone_match.group(0).strip() if phone_match else "",
            "location": "",
            "links": links,
        },
        "education": [{"text": item} for item in education],
        "experience": [{"text": item} for item in experience],
        "projects": [{"text": item} for item in projects],
        "skills": skills,
        "certifications": certifications,
    }


def extract_resume_name(lines: list[str]) -> str:
    if not lines:
        return ""
    first = lines[0].lstrip("#").strip()
    first = re.split(r"\s+(?:男|女)\b", first, maxsplit=1)[0].strip()
    first = re.split(r"\s+(?:自我评价|基本信息|教育背景|工作经历)", first, maxsplit=1)[0].strip()
    return first or first_heading_or_line(lines)


def parse_transcript_markdown(markdown: str) -> dict[str, Any]:
    lines = normalized_lines(markdown)
    student_name = value_after_label(markdown, "student") or value_after_label(markdown, "name")
    student_id = value_after_label(markdown, "student id") or value_after_label(markdown, "id")
    institution = value_after_label(markdown, "institution") or first_heading_or_line(lines)
    gpa = value_after_label(markdown, "gpa")
    total_credits = value_after_label(markdown, "total credits")
    rank = value_after_label(markdown, "rank")
    courses = parse_markdown_table_courses(lines)
    terms = sorted({course.get("term", "") for course in courses if course.get("term")})
    return {
        "student": {"name": student_name, "student_id": student_id},
        "institution": institution,
        "terms": [{"term": term} for term in terms],
        "courses": courses,
        "summary": {"gpa": gpa, "total_credits": total_credits, "rank": rank},
    }


def normalized_lines(markdown: str) -> list[str]:
    return [line.strip() for line in markdown.splitlines() if line.strip()]


def first_heading_or_line(lines: list[str]) -> str:
    for line in lines:
        text = line.lstrip("#").strip()
        if text and not text.startswith("|"):
            return text
    return ""


def extract_list_after_label(markdown: str, label: str) -> list[str]:
    match = re.search(rf"(?im)^\s*{re.escape(label)}\s*:\s*(.+)$", markdown)
    if not match:
        return []
    return [item.strip(" -") for item in re.split(r"[,;|]", match.group(1)) if item.strip(" -")]


def value_after_label(markdown: str, label: str) -> str:
    match = re.search(rf"(?im)^\s*{re.escape(label)}\s*:\s*(.+)$", markdown)
    return match.group(1).strip() if match else ""


def collect_section_items(lines: list[str], section: str) -> list[str]:
    items: list[str] = []
    in_section = False
    for line in lines:
        plain = line.lstrip("#").strip().lower().rstrip(":")
        if plain == section:
            in_section = True
            continue
        if in_section and line.startswith("#"):
            break
        if in_section:
            item = line.lstrip("-* ").strip()
            if item:
                items.append(item)
    return items


def parse_markdown_table_courses(lines: list[str]) -> list[dict[str, str]]:
    table_lines = [line for line in lines if line.startswith("|") and line.endswith("|")]
    if len(table_lines) < 3:
        return []
    headers = split_table_row(table_lines[0])
    courses: list[dict[str, str]] = []
    for row in table_lines[2:]:
        cells = split_table_row(row)
        if len(cells) != len(headers):
            continue
        record = {normalize_header(header): cell for header, cell in zip(headers, cells)}
        if not any(record.values()):
            continue
        courses.append(
            {
                "term": record.get("term", ""),
                "course_code": record.get("course_code", record.get("code", "")),
                "course_name": record.get("course_name", record.get("course", record.get("name", ""))),
                "credits": record.get("credits", record.get("credit", "")),
                "grade": record.get("grade", ""),
                "points": record.get("points", record.get("gpa_points", "")),
            }
        )
    return courses


def split_table_row(row: str) -> list[str]:
    return [cell.strip() for cell in row.strip().strip("|").split("|")]


def normalize_header(header: str) -> str:
    return re.sub(r"[^a-z0-9]+", "_", header.strip().lower()).strip("_")
