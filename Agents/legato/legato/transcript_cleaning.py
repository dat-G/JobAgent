from __future__ import annotations

import json
import re
import unicodedata
from dataclasses import asdict, dataclass
from typing import Iterable, Sequence


VALID_GRADE_LABELS = {"优秀", "良好", "中等", "合格", "不合格", "不及格", "通过"}
COURSE_SKIP_VALUES = {
    "",
    "课程名",
    "课程名称",
    "以下空白",
    "已获总学分数",
    "备注",
}
HEADER_MARKERS = {
    "东北农业大学成绩单",
    "姓名",
    "学号",
    "学院",
    "专业",
    "出生日期",
    "入学日期",
}
GRADE_RE = re.compile(r"^(?:100|[1-9]?\d)(?:\.0+)?$")
YEARISH_RE = re.compile(r"^20\d{2,4}$")


@dataclass(frozen=True)
class CourseGradePair:
    course: str
    grade: str


@dataclass(frozen=True)
class RejectedCourseGradePair:
    course: str
    grade: str
    reason: str


@dataclass(frozen=True)
class CourseGradeCleaningResult:
    pairs: list[CourseGradePair]
    rejected: list[RejectedCourseGradePair]

    def to_dict(self) -> dict[str, object]:
        return {
            "pairs": [asdict(pair) for pair in self.pairs],
            "rejected": [asdict(pair) for pair in self.rejected],
            "stats": {
                "kept": len(self.pairs),
                "rejected": len(self.rejected),
                "total": len(self.pairs) + len(self.rejected),
            },
        }


def parse_course_grade_rows(text: str) -> list[CourseGradePair]:
    """Parse OCR table rows into raw course-grade candidates.

    PaddleOCR-VL often ignores requests for strict JSON but emits compact table
    rows. This parser accepts those rows and extracts the grade column from each
    six-cell course group: course, credit, grade, hours, category, exam_time.
    """

    pairs: list[CourseGradePair] = []
    for line in text.splitlines():
        normalized = normalize_cell(line)
        if not normalized or "|" not in normalized:
            continue
        if _is_header_line(normalized):
            continue
        cells = [normalize_cell(cell) for cell in normalized.strip().strip("|").split("|")]
        if len(cells) < 3:
            continue
        for offset in (0, 6):
            if len(cells) <= offset + 2:
                continue
            course = cells[offset]
            grade = cells[offset + 2]
            pairs.append(CourseGradePair(course=course, grade=grade))
    return pairs


def parse_course_grade_json(text: str) -> list[CourseGradePair]:
    data = json.loads(text)
    if not isinstance(data, list):
        raise ValueError("course-grade JSON must be a list")
    pairs: list[CourseGradePair] = []
    for item in data:
        if not isinstance(item, dict):
            continue
        pairs.append(
            CourseGradePair(
                course=normalize_cell(str(item.get("course", ""))),
                grade=normalize_cell(str(item.get("grade", ""))),
            )
        )
    return pairs


def clean_course_grade_pairs(pairs: Iterable[CourseGradePair]) -> CourseGradeCleaningResult:
    kept: list[CourseGradePair] = []
    rejected: list[RejectedCourseGradePair] = []
    seen: set[tuple[str, str]] = set()
    for pair in pairs:
        course = normalize_course_name(pair.course)
        grade = normalize_grade(pair.grade)
        reason = rejection_reason(course, grade)
        if reason:
            rejected.append(RejectedCourseGradePair(course=course, grade=grade, reason=reason))
            continue
        key = (course, grade)
        if key in seen:
            rejected.append(RejectedCourseGradePair(course=course, grade=grade, reason="duplicate_pair"))
            continue
        seen.add(key)
        kept.append(CourseGradePair(course=course, grade=grade))
    return CourseGradeCleaningResult(pairs=kept, rejected=rejected)


def clean_course_grade_text(text: str) -> CourseGradeCleaningResult:
    stripped = text.lstrip()
    if stripped.startswith("["):
        return clean_course_grade_pairs(parse_course_grade_json(text))
    return clean_course_grade_pairs(parse_course_grade_rows(text))


def normalize_cell(value: str) -> str:
    value = unicodedata.normalize("NFKC", value)
    value = value.replace("\u3000", " ")
    value = re.sub(r"\s+", " ", value)
    return value.strip()


def normalize_course_name(value: str) -> str:
    value = normalize_cell(value)
    value = value.replace("（", "(").replace("）", ")")
    return value


def normalize_grade(value: str) -> str:
    value = normalize_cell(value)
    value = value.replace(":", "").replace("：", "")
    if GRADE_RE.match(value):
        if value.endswith(".0"):
            value = value[:-2]
    return value


def rejection_reason(course: str, grade: str) -> str | None:
    if course in COURSE_SKIP_VALUES:
        return "empty_or_non_course"
    if grade == "":
        return "empty_grade"
    if YEARISH_RE.match(course):
        return "course_is_year"
    if len(course) < 2:
        return "course_too_short"
    if len(course) > 80:
        return "course_too_long"
    if not re.search(r"[\u4e00-\u9fffA-Za-z0-9]", course):
        return "course_has_no_text"
    if not is_valid_grade(grade):
        return "invalid_grade"
    return None


def is_valid_grade(value: str) -> bool:
    if value in VALID_GRADE_LABELS:
        return True
    return bool(GRADE_RE.match(value))


def clean_many_course_grade_texts(texts: Sequence[str]) -> list[CourseGradeCleaningResult]:
    return [clean_course_grade_text(text) for text in texts]


def _is_header_line(line: str) -> bool:
    if line.startswith("|---"):
        return True
    if "课程名" in line and "成绩" in line:
        return True
    return any(line.startswith(marker) or f"| {marker} |" in line for marker in HEADER_MARKERS)
