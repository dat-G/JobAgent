# PaddleOCR Evaluation

Sample:

```text
/Users/sunnychen/Dev/JobAgent/test/chenxi/11_成绩单.pdf
```

## Baseline Extraction

`pypdfium2` text layer:

```text
page_count=1
raw_chars=0
non_whitespace_chars=0
```

MarkItDown:

```text
markdown_chars=0
real=2.54s
```

Conclusion: this PDF is image/scanned content for Legato purposes.

## PaddleOCR Install

Test environment:

```text
macOS arm64
Python 3.13.3
```

Installed successfully in a temporary environment:

```text
paddleocr==3.6.0
paddlepaddle==3.3.1
```

The runtime writes model cache under `~/.paddlex` by default. For tests, set:

```sh
HOME=/private/tmp/legato-paddle-home
XDG_CACHE_HOME=/private/tmp/legato-paddle-cache
PADDLE_PDX_DISABLE_MODEL_SOURCE_CHECK=True
```

## OCR Runtime Tests

Rendered page images:

```text
/private/tmp/chenxi_transcript_page1.png        1190x1682
/private/tmp/chenxi_transcript_page1_scale1.png 595x841
```

PP-OCRv5 server models:

```text
det=PP-OCRv5_server_det
rec=PP-OCRv5_server_rec
```

Result:

- Model download succeeded.
- Inference did not produce usable output before the command exceeded practical timing.
- One run exceeded 3 minutes and failed during result serialization after prediction objects contained ndarray values.

PP-OCRv5 mobile models:

```text
det=PP-OCRv5_mobile_det
rec=PP-OCRv5_mobile_rec
```

Result:

- Model download succeeded.
- Cached low-resolution run did not finish within 60 seconds.

## Decision

PaddleOCR is useful enough to keep as an optional stack, but not suitable as Legato's default OCR backend on this local CPU runtime.

Current status:

- Optional adapter added: `legato.ocr_paddle`.
- Optional install path added: `python scripts/install_dev.py --with-paddleocr`.
- CLI flag added: `--ocr-backend paddle`.
- Default OCR remains disabled.

Recommended next tests:

1. Re-run PaddleOCR on Linux x86_64.
2. Test GPU or Paddle high-performance inference.
3. Compare RapidOCR on the same transcript sample.
4. Benchmark OCR table recovery quality, not just raw text.
