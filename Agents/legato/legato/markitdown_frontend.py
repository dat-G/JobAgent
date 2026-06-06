from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class MarkdownDocument:
    markdown: str
    title: str | None
    source_path: str


class MarkItDownFrontend:
    def convert(self, source: str | Path) -> MarkdownDocument:
        path = Path(source)
        if not path.exists():
            raise FileNotFoundError(f"source file not found: {path}")

        if path.suffix.lower() in {".md", ".markdown"}:
            return MarkdownDocument(
                markdown=path.read_text(encoding="utf-8"),
                title=path.stem,
                source_path=str(path),
            )

        try:
            from markitdown import MarkItDown
        except ImportError as exc:
            raise RuntimeError(
                "MarkItDown is not installed. Run "
                "`scripts/install_markitdown_offline.sh` from Agents/legato."
            ) from exc

        result = MarkItDown().convert(path)
        markdown = getattr(result, "text_content", None) or getattr(result, "markdown", "")
        title = getattr(result, "title", None)
        return MarkdownDocument(markdown=str(markdown), title=title, source_path=str(path))
