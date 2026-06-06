# Legato 技术选型

## 当前定位

Legato 是面向简历和成绩单的极速结构化工具。它不追求通用文档理解，而是优先保证常见 PDF 简历、PDF 成绩单和 Markdown 输入能够在稳定时间预算内转为目标 JSON。

核心要求：

- 默认目标：3 秒内完成。
- PDF 优先读取文字层。
- 文字层不足时再 fallback 到 MarkItDown。
- 图片或扫描件未来走 OCR fallback。
- MarkItDown 必须离线保存在项目内，保证可封装。
- 目标格式化由 Presto 承担，本地规则只用于验收和 smoke test。

## 已采用方案

当前实现采用 Python，并把 PDF 文字层 fast path 放在 MarkItDown 之前。

```text
PDF
  -> pypdfium2 text layer
  -> cleaning
  -> Presto/local formatter
  -> JSON

PDF text layer too small / extraction failed
  -> MarkItDown
  -> cleaning
  -> Presto/local formatter
  -> JSON

Markdown
  -> direct read
  -> cleaning
  -> Presto/local formatter
  -> JSON
```

关键库：

- PDF 文字层：`pypdfium2`
- 多格式 fallback：`markitdown[pdf]==0.1.6`
- PDF fallback 依赖：`pdfplumber`、`pdfminer-six`、`pillow`
- 格式化后端：`Agents/presto`
- 本地验收 formatter：`legato.formatter.LocalRuleFormatter`
- 可选 OCR：`paddleocr==3.6.0` + `paddlepaddle==3.3.1`

## 为什么不是 MarkItDown 直接主链路

MarkItDown 仍然有价值，但不适合作为 PDF 的第一路径：

- 对原生文字 PDF，`pypdfium2` 文字层提取更快。
- 在 `test/chenxi/简历.pdf` 上，MarkItDown 产生了 NUL 字符和兼容汉字问题。
- 同一文件上，MarkItDown 误损坏邮箱；`pypdfium2` 直接提取到完整邮箱。
- MarkItDown 输出 Markdown，适合兼容路径和 LLM 上下文，不适合极限低延迟主路径。

MarkItDown 的定位：

- 多格式 fallback。
- 非 PDF 或 PDF 文字层不足时的识别前端。
- 后续给 Presto/LLM 的 Markdown 上下文。

## PDF 文字层 Fast Path

默认开启：

- 模块：`legato.pdf_text_frontend`
- 引擎：`pypdfium2`
- 阈值：`pdf_text_min_chars = 80`

CLI：

```sh
legato file.pdf --target resume
legato file.pdf --target resume --no-pdf-text-layer
legato file.pdf --target resume --pdf-text-min-chars 120
```

接受条件：

- PDF 扩展名为 `.pdf`。
- `pypdfium2` 成功提取文字层。
- 非空白字符数大于等于阈值。

fallback 条件：

- 不是 PDF。
- PDF 文字层提取失败。
- 文字层字符数过低。

## 清洗模块

模块：`legato.cleaning`

职责：

- 清理识别噪声。
- 归一化字符。
- 不做最终语义判断。

当前处理：

- Unicode NFKC 归一化。
- 删除 NUL 和控制字符。
- 压缩空白。
- 中文联系方式归一：`手机` -> `Phone:`，`邮箱` -> `Email:`。
- 常见中文简历章节提升为 Markdown heading。
- 修复常见邮箱损坏形态，例如 `mcfun@.com` -> `mcfun@example.com`。

边界：

- 不在清洗层推断教育、经历、项目、技能。
- 不调用 LLM。
- 不修改 Markdown table 行列结构。
- 必须保持毫秒级开销。

## Presto 格式化

当前 Legato 通过 Presto 现有 HTTP API 调用：

```text
POST /sessions
POST /sessions/{session_id}/runs
```

Legato 将 cleaned markdown、target schema 和 extraction rules 组装成 prompt，Presto 返回 JSON 字符串，Legato 解析 JSON object。

当前限制：

- Presto 没有专用 `/format` API。
- Presto HTTP response 的 `output` 是 string，不是 typed structured JSON。
- 结构化校验能力在 Presto 内部 workflow 包中，还没有暴露到 HTTP。

建议后续给 Presto 增加：

```text
POST /format
```

返回：

```json
{
  "output": "...",
  "structured": {},
  "error": ""
}
```

这样可以把 `workflow.OutputContract` 暴露为稳定 HTTP 契约。

## PaddleOCR 可选栈

PaddleOCR 已作为可选 OCR backend 接入，但不进入默认路径。

安装：

```sh
scripts/install_paddleocr_optional.sh
```

使用：

```sh
legato scanned.pdf --target transcript --ocr-backend paddle
```

实现模块：

- `legato.ocr_paddle`

默认模型：

- `PP-OCRv5_mobile_det`
- `PP-OCRv5_mobile_rec`

当前实测：

- `paddleocr==3.6.0` 可安装。
- `paddlepaddle==3.3.1` 可安装。
- server 模型在 `test/chenxi/11_成绩单.pdf` 渲染页上超过 3 分钟未完成有效输出。
- mobile 模型在 595x841 低分辨率图片上，缓存模型后仍未在 60 秒内完成。

结论：

- 当前 macOS arm64 + Python 3.13 + CPU 环境下，不适合作为 Legato 的默认 OCR。
- 可作为实验性 backend，供 GPU/Linux 或更合适的 Paddle runtime 后续验证。
- transcript workflow 仍需要一个能稳定落在 SLA 内的 OCR 方案；候选包括 RapidOCR、PaddleOCR 高性能部署、云 OCR 或专门的表格 OCR 服务。

## 输出结构

Legato 当前输出 JSON envelope：

```json
{
  "status": "ok",
  "target": "resume",
  "source_path": "file.pdf",
  "frontend": "pdfium_text",
  "formatter": "presto",
  "elapsed_ms": 57,
  "markdown_chars": 1559,
  "data": {},
  "warnings": []
}
```

`data` 的内容由 target 决定：

- `resume`
- `transcript`

## 性能结论

实测样本：`test/chenxi/简历.pdf`，415 KB。

| 路径 | 中位 wall time | 中位 internal time |
| --- | ---: | ---: |
| CLI 默认 PDF fast path | 128 ms | 47 ms |
| CLI 强制 MarkItDown | 886 ms | 771 ms |
| 常驻进程 PDF fast path | 58 ms | 58 ms |
| `pypdfium2` 文字层提取 | 23 ms | n/a |
| MarkItDown 复用实例 | 299 ms | n/a |

结论：

- 对原生文字 PDF，当前 Python fast path 已经远低于 3 秒目标。
- Rust 改写不是当前最高收益项。
- 更高收益来自：PDF text-layer fast path、常驻服务、Presto 专用 formatter API。

## Rust / Go 判断

暂不建议现在改写 Rust。

适合继续 Python 的原因：

- 当前 PDF 文字层 path 已经百毫秒级。
- 清洗和本地 formatter 是毫秒级以下。
- 最大性能差异来自是否使用 MarkItDown，而不是 Python 本身。

未来考虑 Rust 的条件：

- 必须交付单二进制 CLI。
- 需要极低冷启动。
- 高并发服务需要更低内存和更稳定尾延迟。
- OCR 推理链路也准备迁到 ONNX Runtime / TensorRT / OpenVINO 常驻服务。

Go 的定位：

- 适合作 API 编排服务。
- 不建议用纯 Go 免费 PDF 库替代 PDFium/MuPDF 级 text-layer path。

## 离线封装

MarkItDown 和 PDF 依赖已经保存在：

```text
vendor/wheelhouse
```

锁定文件：

```text
requirements.markitdown.lock
requirements.build.lock
```

离线开发安装：

```sh
scripts/install_dev_offline.sh
```

注意：

- 当前 wheelhouse 是 macOS arm64 + Python 3.13。
- Linux、x86_64 或其他 Python 版本需要重新生成 wheelhouse。
- OCR、Presto 服务和 LLM provider 的封装仍需单独定义。

## 后续增强

优先级建议：

1. 给 Presto 增加专用 `/format` API。
2. 为中文简历补更强的规则字段抽取，或完全交由 Presto。
3. 为成绩单增加表格恢复和 GPA 校验。
4. 增加扫描件/OCR fallback。
5. 在 Linux/GPU 或 Paddle 高性能推理环境复测 PaddleOCR。
6. 建立更多真实样本 benchmark。
7. 如果需要单二进制或更低冷启动，再评估 Rust core。
