from __future__ import annotations

import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any


@dataclass(frozen=True)
class OcrLine:
    text: str
    score: float | None = None


@dataclass(frozen=True)
class OcrResult:
    text: str
    lines: list[OcrLine]
    backend: str = "paddleocr"


def ocr_with_paddle(
    source: str | Path,
    *,
    max_pages: int = 1,
    render_scale: float = 1.0,
    detection_model: str = "PP-OCRv5_mobile_det",
    recognition_model: str = "PP-OCRv5_mobile_rec",
) -> OcrResult:
    paths = materialize_ocr_inputs(source, max_pages=max_pages, render_scale=render_scale)
    try:
        ocr = create_paddle_ocr(detection_model=detection_model, recognition_model=recognition_model)
        lines: list[OcrLine] = []
        for path in paths:
            lines.extend(run_paddle_ocr_on_image(ocr, path))
        text = "\n".join(line.text for line in lines)
        return OcrResult(text=text, lines=lines)
    finally:
        for path in paths:
            if path.parent.name.startswith("legato-paddleocr-"):
                try:
                    path.unlink()
                    path.parent.rmdir()
                except OSError:
                    pass


def create_paddle_ocr(*, detection_model: str, recognition_model: str) -> Any:
    try:
        from paddleocr import PaddleOCR
    except ImportError as exc:
        raise RuntimeError(
            "PaddleOCR is not installed. Install the optional stack with "
            "`python scripts/install_dev.py --with-paddleocr`."
        ) from exc

    return PaddleOCR(
        text_detection_model_name=detection_model,
        text_recognition_model_name=recognition_model,
        use_doc_orientation_classify=False,
        use_doc_unwarping=False,
        use_textline_orientation=False,
    )


def materialize_ocr_inputs(source: str | Path, *, max_pages: int, render_scale: float) -> list[Path]:
    path = Path(source)
    if path.suffix.lower() == ".pdf":
        return render_pdf_pages(path, max_pages=max_pages, render_scale=render_scale)
    return [path]


def render_pdf_pages(source: Path, *, max_pages: int, render_scale: float) -> list[Path]:
    try:
        import pypdfium2 as pdfium
    except ImportError as exc:
        raise RuntimeError("pypdfium2 is required to render PDF pages for PaddleOCR") from exc

    pdf = pdfium.PdfDocument(str(source))
    out_paths: list[Path] = []
    try:
        page_count = min(len(pdf), max_pages)
        for page_index in range(page_count):
            page = pdf[page_index]
            try:
                bitmap = page.render(scale=render_scale)
                image = bitmap.to_pil()
                temp_dir = Path(tempfile.mkdtemp(prefix="legato-paddleocr-"))
                out_path = temp_dir / f"page_{page_index + 1}.png"
                image.save(out_path)
                out_paths.append(out_path)
            finally:
                page.close()
    finally:
        pdf.close()
    return out_paths


def run_paddle_ocr_on_image(ocr: Any, image_path: Path) -> list[OcrLine]:
    result = ocr.predict(str(image_path))
    lines: list[OcrLine] = []
    for item in result:
        data = item if isinstance(item, dict) else getattr(item, "json", None)
        if callable(data):
            data = data()
        if not isinstance(data, dict):
            continue
        rec_texts = data.get("rec_texts")
        rec_scores = data.get("rec_scores")
        if rec_texts is None:
            rec_texts = []
        if rec_scores is None:
            rec_scores = []
        for idx, text in enumerate(rec_texts):
            score = float(rec_scores[idx]) if idx < len(rec_scores) else None
            lines.append(OcrLine(text=str(text), score=score))
    return lines
