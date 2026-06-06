# Transcript Course-Grade OCR Prompt

Use with `PaddlePaddle/PaddleOCR-VL-1.5` on SiliconFlow or another OCR-VL endpoint.

Prompt:

```text
OCR this Chinese transcript course table.
Goal: recover all course-grade pairs without omission.
Output compact rows. For each visual table row, output the visible row text only.
Do not output student info, totals, notes, or page numbers.
Do not stop at 以下空白 because the left column may continue.
Stop only when reaching 已获总学分数.
Do not infer or normalize course names. If unclear, use [?].
```

Recommended API settings:

```json
{
  "temperature": 0,
  "max_tokens": 4096,
  "stop": ["已获总学分数", "备注:", "备注：", "第1页"]
}
```

