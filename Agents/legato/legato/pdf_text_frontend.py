from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class PdfTextDocument:
    text: str
    page_count: int
    char_count: int
    non_whitespace_chars: int

    @property
    def has_text_layer(self) -> bool:
        return self.non_whitespace_chars > 0


def is_pdf_path(source: str | Path) -> bool:
    return Path(source).suffix.lower() == ".pdf"


def extract_pdf_text_layer(source: str | Path) -> PdfTextDocument:
    try:
        import pypdfium2 as pdfium
    except ImportError as exc:
        raise RuntimeError(
            "pypdfium2 is not installed. Run `python scripts/install_dev.py` "
            "or fall back to MarkItDown."
        ) from exc

    path = Path(source)
    pdf = pdfium.PdfDocument(str(path))
    parts: list[str] = []
    try:
        page_count = len(pdf)
        for page_index in range(page_count):
            page = pdf[page_index]
            try:
                textpage = page.get_textpage()
                try:
                    parts.append(textpage.get_text_range())
                finally:
                    textpage.close()
            finally:
                page.close()
    finally:
        pdf.close()

    text = "\n".join(parts)
    return PdfTextDocument(
        text=text,
        page_count=page_count,
        char_count=len(text),
        non_whitespace_chars=sum(1 for char in text if not char.isspace()),
    )
