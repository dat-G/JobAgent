# Legato Architecture

## Goals

Legato optimizes for fast, practical structuring of resumes and transcripts.

The project keeps recognition and formatting separate:

- Recognition turns a file into text/Markdown.
- Cleaning removes recognition noise.
- Formatting maps cleaned text into target JSON.

## Runtime Flow

```text
legato.cli
  |
  +-- legato.pipeline.process
        |
        +-- PDF fast path?
        |     |
        |     +-- legato.pdf_text_frontend.extract_pdf_text_layer
        |
        +-- fallback
        |     |
        |     +-- legato.markitdown_frontend.MarkItDownFrontend
        |
        +-- legato.cleaning.clean_markdown
        |
        +-- formatter
              |
              +-- legato.formatter.PrestoFormatter
              +-- legato.formatter.LocalRuleFormatter

Optional OCR fallback:

```text
empty frontend output + --ocr-backend paddle
  |
  +-- legato.ocr_paddle.ocr_with_paddle
  +-- legato.cleaning.clean_markdown
  +-- formatter
```
```

## Module Responsibilities

`legato.cli`

- Parses user arguments.
- Selects Presto or local formatter mode.
- Exposes PDF fast-path controls.
- Writes JSON output.

`legato.pipeline`

- Owns orchestration and time budget accounting.
- Selects `pdfium_text` or `markitdown` frontend.
- Applies cleaning before formatting.
- Builds the final response envelope.

`legato.pdf_text_frontend`

- Uses `pypdfium2` to extract PDF text layer.
- Does not attempt OCR.
- Reports character counts so the pipeline can decide whether to accept the text layer.

`legato.markitdown_frontend`

- Reads Markdown files directly.
- Uses MarkItDown for PDFs, images, and other supported formats.
- Serves as fallback and compatibility path.

`legato.cleaning`

- Normalizes Unicode.
- Removes control characters.
- Normalizes common Chinese resume labels.
- Promotes common section lines to headings.
- Does not perform semantic extraction.

`legato.formatter`

- Builds formatter prompts.
- Calls Presto session/run API.
- Parses JSON returned by Presto.
- Provides local deterministic formatting for tests.

`legato.ocr_paddle`

- Optional PaddleOCR backend.
- Renders PDF pages to images with `pypdfium2`.
- Calls `PaddleOCR.predict`.
- Is not imported unless `--ocr-backend paddle` is requested.

`legato.schemas`

- Stores target schema definitions for `resume` and `transcript`.

## Frontend Decision

PDF inputs use this logic:

```text
if file suffix is .pdf:
    try pypdfium2 text extraction
    if non_whitespace_chars >= pdf_text_min_chars:
        frontend = pdfium_text
    else:
        frontend = markitdown
else:
    frontend = markitdown
```

Default threshold:

```text
pdf_text_min_chars = 80
```

## Formatter Decision

```text
if --no-presto:
    LocalRuleFormatter
else:
    PrestoFormatter
```

Local formatting exists to keep acceptance tests offline and deterministic. Production should use Presto or a future dedicated formatter API.

## OCR Decision

```text
if recognized text is empty and --ocr-backend paddle:
    render PDF page(s) or use image input
    call PaddleOCR
else:
    skip OCR
```

OCR is explicit because current PaddleOCR CPU testing is not fast enough for the default SLA.

## Error Handling

Pipeline behavior:

- PDF text-layer failure adds a warning and falls back to MarkItDown.
- Markdown longer than `max_markdown_chars` is truncated and marked with a warning.
- Formatter errors currently fail the run.
- Runs exceeding `timeout_ms` return `status=partial` with a warning.

## Performance Notes

On a 415 KB Chinese resume PDF:

- `pypdfium2` text layer: about 23 ms median.
- full same-process pipeline with PDF fast path: about 58 ms median.
- CLI default fast path: about 128 ms median.
- CLI forced MarkItDown: about 886 ms median.
- PaddleOCR PP-OCRv5 mobile/server on the scanned transcript sample did not complete within 60-120 seconds on the current macOS CPU environment.

The main performance lever is frontend selection. Cleaning and local formatting are negligible.

## Packaging

The project vendors Python wheels under:

```text
vendor/wheelhouse
```

Cross-platform install script:

```sh
python scripts/install_dev.py
```

By default the script downloads packages from the configured Python package index. Use `--offline` to install from `vendor/wheelhouse` when the wheelhouse matches the deployment target.

## Future API Shape

Recommended Presto endpoint:

```text
POST /format
```

Input:

```json
{
  "target": "resume",
  "input": "...cleaned markdown...",
  "schema": {}
}
```

Output:

```json
{
  "structured": {},
  "output": "...",
  "error": ""
}
```

This would remove string parsing from Legato and move structured validation into Presto's HTTP boundary.
