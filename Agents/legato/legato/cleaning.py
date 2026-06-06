from __future__ import annotations

import re
import unicodedata
from dataclasses import dataclass


CONTROL_CHARS_RE = re.compile(r"[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]")
WHITESPACE_RE = re.compile(r"[ \t\u00a0]+")
SECTION_ALIASES = {
    "自我评价",
    "基本信息",
    "教育背景",
    "工作经历",
    "项目经历",
    "科研与项目经历",
    "技能熟练度",
    "资格证书",
    "获奖情况",
    "在校经历",
    "社团组织",
}
CJK_WORD_WRAP_PAIRS = (
    ("模", "型"),
    ("方", "法"),
    ("系", "统"),
    ("算", "法"),
    ("框", "架"),
    ("模", "块"),
    ("数", "据"),
    ("研", "究"),
    ("实", "验"),
    ("项", "目"),
    ("平", "台"),
    ("功", "能"),
    ("管", "理"),
    ("分", "析"),
    ("评", "估"),
    ("复", "现"),
    ("构", "建"),
    ("验", "证"),
)


@dataclass(frozen=True)
class CleanedMarkdown:
    markdown: str
    warnings: list[str]
    stats: dict[str, int]


def clean_markdown(markdown: str) -> CleanedMarkdown:
    warnings: list[str] = []
    original_len = len(markdown)
    nul_count = markdown.count("\x00")
    control_count = len(CONTROL_CHARS_RE.findall(markdown))

    text = unicodedata.normalize("NFKC", markdown)
    text = CONTROL_CHARS_RE.sub("", text)
    text = repair_common_cjk_word_wraps(text)
    text = normalize_lines(text)
    text = normalize_contact_labels(text)
    text = promote_cn_section_lines(text)
    text = restore_common_email_artifacts(text)
    text = collapse_excess_blank_lines(text)

    if nul_count:
        warnings.append(f"removed {nul_count} NUL characters")
    if control_count:
        warnings.append(f"removed {control_count} control characters")
    if text != markdown:
        warnings.append("normalized markdown before formatting")

    return CleanedMarkdown(
        markdown=text,
        warnings=warnings,
        stats={
            "original_chars": original_len,
            "cleaned_chars": len(text),
            "removed_chars": max(0, original_len - len(text)),
            "nul_chars": nul_count,
            "control_chars": control_count,
        },
    )


def normalize_lines(text: str) -> str:
    out: list[str] = []
    for line in text.replace("\r\n", "\n").replace("\r", "\n").split("\n"):
        stripped = WHITESPACE_RE.sub(" ", line).strip()
        out.append(stripped)
    return "\n".join(out).strip()


def repair_common_cjk_word_wraps(text: str) -> str:
    for left, right in CJK_WORD_WRAP_PAIRS:
        text = re.sub(rf"{left}[ \t]*(?:\r\n|\r|\n)[ \t]*{right}", left + right, text)
    return text


def normalize_contact_labels(text: str) -> str:
    replacements = {
        "手机": "Phone",
        "电话": "Phone",
        "邮箱": "Email",
        "电子邮箱": "Email",
        "微信": "WeChat",
        "QQ": "QQ",
    }
    for source, target in replacements.items():
        text = re.sub(rf"(?m)^{re.escape(source)}\s+", f"{target}: ", text)
        text = re.sub(rf"(?m)^{re.escape(source)}[:：]\s*", f"{target}: ", text)
    return text


def promote_cn_section_lines(text: str) -> str:
    lines: list[str] = []
    for line in text.split("\n"):
        plain = line.strip("# ").strip()
        if plain in SECTION_ALIASES:
            lines.append(f"## {plain}")
        else:
            lines.append(line)
    return "\n".join(lines)


def restore_common_email_artifacts(text: str) -> str:
    text = re.sub(r"(?i)([A-Z0-9._%+-]+)@+\.com", r"\1@example.com", text)
    text = re.sub(r"(?i)([A-Z0-9._%+-]+)\s*@\s*([A-Z0-9.-]+\.[A-Z]{2,})", r"\1@\2", text)
    return text


def collapse_excess_blank_lines(text: str) -> str:
    return re.sub(r"\n{3,}", "\n\n", text).strip() + "\n"
