# PaddleOCR-VL SiliconFlow Evaluation

Test file: `test/chenxi/11_成绩单.pdf`

Rendered image: `/private/tmp/chenxi_transcript_page1_scale1.png`

Provider/model: SiliconFlow `PaddlePaddle/PaddleOCR-VL-1.5`

## Runs

| Variant | Time | Finish | Output file | Notes |
| --- | ---: | --- | --- | --- |
| transcript table prompt | 13.46s | `length` | `chenxi_11_paddleocr_vl_siliconflow_text.md` | Produced many course rows, then hallucinated a long page-number sequence. |
| focused transcript OCR prompt | 10.50s | `length` | `chenxi_11_paddleocr_vl_siliconflow_focused_text.md` | Similar course coverage; still hallucinated page numbers after the footer. |
| course-grade rows with API stop | 9.80s | `stop` | `chenxi_11_paddleocr_vl_course_grade_rows.txt` | Best prompt so far. Stops before footer/page-number hallucination and keeps lower-left rows after the right column says `以下空白`. Parsed into 83 course-grade candidates. |

## Observed Strengths

- Much faster than SiliconFlow `deepseek-ai/DeepSeek-OCR` on this sample.
- Recovered important course names that DeepSeek-OCR missed or renamed, including:
  - `Java程序设计`
  - `计算机网络`
  - `软件工程`
  - `算法设计与分析`
  - `计算方法实验`
  - `计算机组成原理实验`
- Output is line-oriented and easier to parse than DeepSeek-OCR's unstable HTML/table output.
- With API `stop` set to `已获总学分数`, the hallucinated page-number tail can be removed reliably.

## Observed Problems

- Both runs ended with `finish_reason=length`, even with a focused prompt.
- The model hallucinated a long footer sequence after the real transcript footer, such as `第1页共1页`, `第2页共3页`, and so on.
- It misread several course names:
  - `班级` as `计利2101`
  - `数字逻辑电路` as `数字营销电路`
  - `数据库原理及应用实验` as `数据资源使用与实验`
  - `中国近现代史纲要` as `中国近代现代史纲要`
  - elective course names were especially unstable.
- Some fields differ from the PaddleOCR raw baseline, including exam times and course categories on later rows.
- Even when asked for only course-grade pairs, the model still tends to output full row text. This is acceptable if Legato parses the row text deterministically.
- The parsed course-grade candidate list still contains obvious bad values, for example `数据结构与算法课程设计 -> 110`, so grade validation is required.

## Best Current Prompt

Use this prompt shape with API-level stop sequences, not prompt-only stopping:

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

The canonical prompt file is:

- `prompts/transcript_course_grade_ocr.md`

## Course-Grade Cleaning

The cleaning module is `legato.transcript_cleaning`.

Current behavior:

- Parses PaddleOCR-VL compact rows into `course -> grade` candidates.
- Keeps grades that are numeric `0-100` or one of `优秀`, `良好`, `中等`, `合格`, `不合格`, `不及格`, `通过`.
- Drops empty rows, metadata/footer cells, `以下空白`, year-like course placeholders, duplicate exact pairs, and invalid grades.
- Does not attempt semantic course-name correction. OCR-confused but structurally valid course names require a course dictionary or cross-OCR validation.

On `chenxi_11_paddleocr_vl_course_grade_rows.txt`, cleaning produced:

- kept: 81
- rejected: 18
- output: `chenxi_11_paddleocr_vl_course_grade_cleaned.json`

Benchmark command:

```sh
PYTHONDONTWRITEBYTECODE=1 python3 scripts/benchmark_transcript_cleaning.py \
  workflows/transcript/chenxi_11_paddleocr_vl_course_grade_rows.txt \
  --iterations 5000 --workers 1 2 4 8
```

Benchmark result:

| Workers | Per document |
| ---: | ---: |
| 1 | 1.52ms |
| 2 | 1.11ms |
| 4 | 1.35ms |
| 8 | 1.19ms |

Cleaning is not worth parallelizing by itself. It is already about 1-2ms per transcript, far below OCR/network latency. Parallelism should be reserved for independent remote OCR calls or batch document processing.

## Recommendation

`PaddlePaddle/PaddleOCR-VL-1.5` on SiliconFlow is a better remote OCR candidate than `deepseek-ai/DeepSeek-OCR` for this transcript sample, mainly because it recovers more real course names in about 10-14 seconds.

It is still not accurate enough to be the final authoritative extractor. The practical workflow should be:

1. Use local PaddleOCR or PaddleOCR-VL as OCR candidate generators.
2. Trim obvious hallucinated footer/page-number tails after `已获总学分数`.
3. Group rows with a deterministic transcript parser.
4. Validate each row against the schema `course_name, credits, grade, hours, category, exam_time`.
5. Mark low confidence when course names are semantically suspicious or fields fail validation.
