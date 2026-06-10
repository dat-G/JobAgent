from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from .pipeline import process
from .schemas import SCHEMAS


def write_utf8(stream, text: str) -> None:
    if hasattr(stream, "reconfigure"):
        try:
            stream.reconfigure(encoding="utf-8", errors="replace")
        except Exception:
            pass
    stream.write(text + "\n")


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="legato", description="Structure resumes, transcripts, and chat context.")
    parser.add_argument("source", help="Input PDF, image, or Markdown file.")
    parser.add_argument("--target", choices=sorted(SCHEMAS), required=True)
    parser.add_argument("--workflow", choices=["resume", "chat"], help="Use a workflow-specific formatter.")
    parser.add_argument(
        "--workflow-stage",
        choices=[
            "profile",
            "certifications_awards",
            "experience",
            "experience_hybrid",
            "experience_hybrid_item",
            "item_benchmark",
            "major_baseline",
            "job_matching",
            "source_text",
            "answer",
        ],
        help="Run one workflow stage and return partial structured data.",
    )
    parser.add_argument(
        "--workflow-combine-agents",
        action="store_true",
        help="For resume workflow, combine identity and education into one model request.",
    )
    parser.add_argument(
        "--workflow-stage-input",
        help="Read JSON input for workflow stages that accept caller-assembled items.",
    )
    parser.add_argument("--debug", action="store_true", help="Include timings, retry counts, model, and endpoint.")
    parser.add_argument("--presto-url", default=None, help="Presto base URL. Defaults to LEGATO_PRESTO_URL or localhost.")
    parser.add_argument("--no-presto", action="store_true", help="Use local deterministic formatter for acceptance tests.")
    parser.add_argument("--timeout-ms", type=int, default=3000)
    parser.add_argument("--max-markdown-chars", type=int, default=120_000)
    parser.add_argument(
        "--no-pdf-text-layer",
        action="store_true",
        help="Disable pypdfium2 PDF text-layer fast path and use MarkItDown directly.",
    )
    parser.add_argument(
        "--pdf-text-min-chars",
        type=int,
        default=80,
        help="Minimum non-whitespace chars required to accept the PDF text layer.",
    )
    parser.add_argument(
        "--ocr-backend",
        choices=["none", "paddle"],
        default="none",
        help="Optional OCR backend used only when text-layer and MarkItDown extraction are empty.",
    )
    parser.add_argument("--ocr-max-pages", type=int, default=1)
    parser.add_argument("--ocr-render-scale", type=float, default=1.0)
    parser.add_argument("-o", "--output", help="Write JSON output to this file.")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        workflow_stage_input = None
        if args.workflow_stage_input:
            workflow_stage_input = json.loads(Path(args.workflow_stage_input).read_text(encoding="utf-8"))
        result = process(
            args.source,
            args.target,
            use_presto=not args.no_presto,
            presto_url=args.presto_url,
            timeout_ms=args.timeout_ms,
            max_markdown_chars=args.max_markdown_chars,
            use_pdf_text_layer=not args.no_pdf_text_layer,
            pdf_text_min_chars=args.pdf_text_min_chars,
            ocr_backend=args.ocr_backend,
            ocr_max_pages=args.ocr_max_pages,
            ocr_render_scale=args.ocr_render_scale,
            workflow=args.workflow,
            workflow_stage=args.workflow_stage,
            workflow_stage_input=workflow_stage_input,
            workflow_combine_agents=args.workflow_combine_agents,
            debug=args.debug,
        ).to_dict()
    except Exception as exc:
        payload = {"status": "failed", "error": str(exc)}
        write_utf8(sys.stderr, json.dumps(payload, ensure_ascii=False, indent=2))
        return 1

    output = json.dumps(result, ensure_ascii=False, indent=2)
    if args.output:
        Path(args.output).write_text(output + "\n", encoding="utf-8")
    else:
        write_utf8(sys.stdout, output)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
