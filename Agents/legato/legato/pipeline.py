from __future__ import annotations

import time
from dataclasses import asdict, dataclass
from pathlib import Path
from typing import Any

from .cleaning import clean_markdown
from .formatter import LocalRuleFormatter, PrestoFormatter
from .markitdown_frontend import MarkItDownFrontend
from .ocr_paddle import ocr_with_paddle
from .pdf_text_frontend import extract_pdf_text_layer, is_pdf_path
from .resume_workflow_formatter import ResumeWorkflowFormatter
from .schemas import schema_for


@dataclass(frozen=True)
class LegatoResult:
    status: str
    target: str
    source_path: str
    frontend: str
    formatter: str
    elapsed_ms: int
    markdown_chars: int
    data: dict[str, Any]
    warnings: list[str]
    debug: dict[str, Any] | None = None

    def to_dict(self) -> dict[str, Any]:
        data = asdict(self)
        if self.debug is None:
            data.pop("debug")
        return data


def process(
    source: str | Path,
    target: str,
    *,
    use_presto: bool = True,
    presto_url: str | None = None,
    timeout_ms: int = 3000,
    max_markdown_chars: int = 120_000,
    use_pdf_text_layer: bool = True,
    pdf_text_min_chars: int = 80,
    ocr_backend: str = "none",
    ocr_max_pages: int = 1,
    ocr_render_scale: float = 1.0,
    workflow: str | None = None,
    workflow_stage: str | None = None,
    workflow_combine_agents: bool = False,
    debug: bool = False,
) -> LegatoResult:
    schema_for(target)
    started = time.monotonic()
    source_path = Path(source)
    warnings: list[str] = []
    debug_chain: list[dict[str, Any]] = []
    frontend = "markitdown"
    raw_markdown = ""

    if use_pdf_text_layer and is_pdf_path(source_path):
        try:
            stage_started = time.monotonic()
            pdf_text = extract_pdf_text_layer(source_path)
            debug_chain.append(
                {
                    "stage": "text_extraction",
                    "frontend": "pdfium_text",
                    "elapsed_ms": int((time.monotonic() - stage_started) * 1000),
                }
            )
            if pdf_text.non_whitespace_chars >= pdf_text_min_chars:
                raw_markdown = pdf_text.text
                frontend = "pdfium_text"
            else:
                warnings.append(
                    "pdf text layer below threshold "
                    f"({pdf_text.non_whitespace_chars} < {pdf_text_min_chars}); falling back to MarkItDown"
                )
        except Exception as exc:
            warnings.append(f"pdf text layer extraction failed: {exc}; falling back to MarkItDown")

    if not raw_markdown:
        stage_started = time.monotonic()
        document = MarkItDownFrontend().convert(source_path)
        raw_markdown = document.markdown
        debug_chain.append(
            {
                "stage": "text_extraction",
                "frontend": "markitdown",
                "elapsed_ms": int((time.monotonic() - stage_started) * 1000),
            }
        )

    if not raw_markdown.strip() and ocr_backend == "paddle":
        stage_started = time.monotonic()
        ocr_result = ocr_with_paddle(
            source_path,
            max_pages=ocr_max_pages,
            render_scale=ocr_render_scale,
        )
        raw_markdown = ocr_result.text
        frontend = ocr_result.backend
        debug_chain.append(
            {
                "stage": "ocr",
                "frontend": frontend,
                "elapsed_ms": int((time.monotonic() - stage_started) * 1000),
            }
        )
    elif not raw_markdown.strip() and ocr_backend != "none":
        raise ValueError(f"unsupported OCR backend {ocr_backend!r}")

    stage_started = time.monotonic()
    cleaned = clean_markdown(raw_markdown)
    markdown = cleaned.markdown
    debug_chain.append({"stage": "cleaning", "elapsed_ms": int((time.monotonic() - stage_started) * 1000)})
    warnings.extend(cleaned.warnings)
    if len(markdown) > max_markdown_chars:
        markdown = markdown[:max_markdown_chars]
        warnings.append(f"markdown truncated to {max_markdown_chars} characters")

    remaining = max(0.1, timeout_ms / 1000 - (time.monotonic() - started))
    if workflow == "resume":
        if target != "resume":
            raise ValueError("--workflow resume requires --target resume")
        if not use_presto:
            raise ValueError("--workflow resume requires Presto; remove --no-presto")
        formatter = ResumeWorkflowFormatter(
            presto_url=presto_url,
            timeout_seconds=remaining,
            combine_agents=workflow_combine_agents,
        )
        if workflow_stage:
            formatted = formatter.format_stage(markdown, workflow_stage)
        else:
            formatted = formatter.format(markdown)
        formatter_debug = formatted.debug
    else:
        formatter = PrestoFormatter(presto_url, timeout_seconds=remaining) if use_presto else LocalRuleFormatter()
        formatted = formatter.format(markdown, target)
        formatter_debug = getattr(formatted, "debug", None)
    elapsed_ms = int((time.monotonic() - started) * 1000)
    warnings.extend(formatted.warnings)
    status = "ok" if elapsed_ms <= timeout_ms else "partial"
    if elapsed_ms > timeout_ms:
        warnings.append(f"elapsed time exceeded timeout_ms={timeout_ms}")

    result_debug = None
    if debug:
        formatter_chain = (formatter_debug or {}).get("chain", [])
        result_debug = {
            "chain": debug_chain + formatter_chain + [{"stage": "total", "elapsed_ms": elapsed_ms}],
            "formatter": formatter_debug,
        }

    return LegatoResult(
        status=status,
        target=target,
        source_path=str(source_path),
        frontend=frontend,
        formatter=formatted.formatter,
        elapsed_ms=elapsed_ms,
        markdown_chars=len(markdown),
        data=formatted.data,
        warnings=warnings,
        debug=result_debug,
    )
