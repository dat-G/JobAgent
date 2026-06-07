# Legato

Legato is a fast resume and transcript structuring tool. It turns PDFs, images, and Markdown-like text into target JSON for downstream agent workflows.

Current focus:

- Resume structuring
- Transcript structuring
- Chat answer workflow for Growth Lens assistant context
- Fast PDF text-layer extraction
- MarkItDown fallback
- Presto-backed schema formatting
- Offline packaging of MarkItDown dependencies
- Optional PaddleOCR backend for scanned documents

## Pipeline

```text
input file
  |
  +-- PDF?
  |     |
  |     +-- pypdfium2 text-layer fast path
  |     |     |
  |     |     +-- text enough? -> cleaning -> formatter -> JSON
  |     |
  |     +-- fallback: MarkItDown -> cleaning -> formatter -> JSON
  |
  +-- Markdown?
  |     |
  |     +-- direct read -> cleaning -> formatter -> JSON
  |
  +-- other file/image?
        |
        +-- MarkItDown -> cleaning -> formatter -> JSON

If all text extraction paths return empty text and `--ocr-backend paddle` is set, Legato can call the optional PaddleOCR backend before cleaning.
```

Formatter modes:

- Presto mode: sends a schema prompt to `Agents/presto` through its session/run API.
- Local mode: `--no-presto`, deterministic rules for smoke tests and local acceptance.

## Install Offline

MarkItDown and PDF dependencies are vendored under `vendor/wheelhouse`.

```sh
cd /Users/sunnychen/Dev/JobAgent/Agents/legato
python3 -m venv .venv
. .venv/bin/activate
scripts/install_dev_offline.sh
```

The script installs:

- `markitdown[pdf]==0.1.6`
- PDF dependencies such as `pdfplumber`, `pdfminer-six`, `pypdfium2`, `pillow`
- offline build dependency `setuptools`
- Legato itself in editable mode

The current wheelhouse was built for macOS arm64 and Python 3.13. Regenerate it for Linux, x86_64, or another Python version.

## Optional PaddleOCR

PaddleOCR is not part of the default offline install because it is large and slow to initialize on the current macOS CPU environment.

Install it only in a dedicated environment:

```sh
scripts/install_paddleocr_optional.sh
```

Use it explicitly:

```sh
legato path/to/scanned-transcript.pdf --target transcript --ocr-backend paddle --ocr-max-pages 1 --ocr-render-scale 1.0
```

Current transcript workflow testing showed that `paddleocr==3.6.0` and `paddlepaddle==3.3.1` install on macOS arm64 + Python 3.13, but PP-OCRv5 server/mobile CPU inference did not complete within the 60-120 second test budget. Treat this backend as experimental until tested on a suitable runtime.

## CLI

Resume:

```sh
legato path/to/resume.pdf --target resume
```

Transcript:

```sh
legato path/to/transcript.pdf --target transcript
```

Chat workflow:

```sh
legato chat.md --target chat --workflow chat --workflow-stage answer \
  --workflow-stage-input chat-input.json \
  --presto-url http://127.0.0.1:8080
```

Local acceptance without Presto:

```sh
legato fixtures/resume.md --target resume --no-presto
legato fixtures/transcript.md --target transcript --no-presto
```

Use a running Presto server:

```sh
legato path/to/resume.pdf --target resume --presto-url http://127.0.0.1:8080
```

PDF fast-path controls:

```sh
legato path/to/resume.pdf --target resume --no-pdf-text-layer
legato path/to/resume.pdf --target resume --pdf-text-min-chars 120
```

OCR fallback:

```sh
legato path/to/scanned.pdf --target transcript --ocr-backend paddle
```

Write output to a file:

```sh
legato path/to/resume.pdf --target resume -o out.json
```

## Output

The CLI returns a JSON envelope:

```json
{
  "status": "ok",
  "target": "resume",
  "source_path": "path/to/resume.pdf",
  "frontend": "pdfium_text",
  "formatter": "presto",
  "elapsed_ms": 57,
  "markdown_chars": 1559,
  "data": {},
  "warnings": []
}
```

Important fields:

- `frontend`: `pdfium_text` or `markitdown`
- `formatter`: `presto` or `local_rules`
- `status`: `ok`, `partial`, or `failed`
- `warnings`: cleaning, fallback, timeout, and local-mode notes

## Performance Snapshot

Measured on `test/chenxi/简历.pdf`, 415 KB, macOS arm64, Python 3.13:

| Path | Median wall time | Median internal time |
| --- | ---: | ---: |
| CLI, default PDF fast path | 128 ms | 47 ms |
| CLI, MarkItDown forced | 886 ms | 771 ms |
| Same-process PDF fast path | 58 ms | 58 ms |
| pypdfium2 text only | 23 ms | n/a |
| MarkItDown reused instance | 299 ms | n/a |

For text-layer PDFs, the Python fast path is already comfortably below the 3 second target. Rust is not the next bottleneck for this path.

## Tests

```sh
cd /Users/sunnychen/Dev/JobAgent/Agents/legato
scripts/acceptance.sh
```

Current coverage:

- Resume fixture to target JSON
- Transcript Markdown table to target JSON
- Mockable MarkItDown frontend
- Presto session/run client behavior
- Cleaning module
- PDF path detection

## Project Layout

```text
legato/
  cleaning.py             markdown normalization and noise cleanup
  cli.py                  command-line interface
  formatter.py            Presto client and local formatter
  markitdown_frontend.py  MarkItDown adapter
  pdf_text_frontend.py    pypdfium2 PDF text-layer fast path
  pipeline.py             orchestration
  schemas.py              target schemas
fixtures/                 acceptance inputs and expected JSON
tests/                    unit and acceptance tests
vendor/wheelhouse/        offline Python wheels
```

## Current Limitations

- Local formatter is only for smoke tests; production formatting should use Presto.
- Scanned PDFs and image OCR are not implemented as a dedicated local OCR path yet.
- PaddleOCR is wired as an optional backend, but current macOS CPU testing is too slow for the 3 second target.
- Presto currently exposes a generic session/run API, not a dedicated `/format` endpoint.
- The wheelhouse is platform-specific.
