# Transcript Workflow

This workflow focuses on transcript structuring.

Scope:

- Input: transcript PDFs, images, or Markdown extracted by Legato.
- Cleaning: normalize OCR/text-layer noise before formatting.
- Formatting: produce transcript target JSON.
- Validation: preserve course rows, terms, credits, grades, GPA, and summary fields.

This directory is the working area for transcript-specific workflow changes.

Current sample:

- `/Users/sunnychen/Dev/JobAgent/test/chenxi/11_成绩单.pdf`

Findings:

- PDF text layer is empty.
- MarkItDown returns empty text.
- This sample requires OCR.
- PaddleOCR is wired as an optional backend, but current local CPU testing timed out. See `PADDLEOCR_EVAL.md`.
