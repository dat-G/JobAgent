# DeepSeek-OCR Prompt Evaluation

Test file: `test/chenxi/11_成绩单.pdf`

Rendered image: `/private/tmp/chenxi_transcript_page1_scale1.png`

Provider/model: SiliconFlow `deepseek-ai/DeepSeek-OCR`

## Prompt References

The official DeepSeek-OCR examples use short task prompts:

- Document to Markdown: `<image>\n<|grounding|>Convert the document to markdown.`
- Plain OCR without layout: `<image>\nFree OCR.`
- General image OCR: `<image>\n<|grounding|>OCR this image.`

Sources:

- https://github.com/deepseek-ai/DeepSeek-OCR
- https://www.deepseek-ocr.ai/

## Results

| Variant | Prompt shape | Time | Output | Transcript quality |
| --- | --- | ---: | --- | --- |
| `custom_table` | Chinese/English transcript-oriented table prompt | 56.49s | HTML table | Best structure, but not reliable enough. It misses or renames important courses and changes some course-score pairings. |
| `official_free_ocr` | `<image>\nFree OCR.` | 119.60s | Student header plus repeated empty table cells | Unusable for transcript extraction. It hit max output length and did not recover course rows. |
| `official_markdown` | `<image>\n<\|grounding\|>Convert the document to markdown.` | 0.94s | `question.` | Unusable on this API/sample. |
| `conservative_lines` | Free OCR plus strict line-by-line/no-inference constraints | 137.83s | `<\|det\|>` plus repeated detector tokens | Unusable. More constraints increased latency and triggered non-text output. |

## Quality Notes

The `custom_table` output is the only DeepSeek-OCR variant that produced a table-like result, but it still cannot be trusted as the sole OCR source for this transcript:

- It recovered 64 valid-looking course rows, while the PaddleOCR raw OCR baseline exposed about 82 course-like entries.
- It preserved some easy rows, such as `计算机类专业导论 | 1.0 | 优秀 | 16 | 必修 | 202112`.
- It missed or renamed key CS courses, including `Java程序设计`, `计算机网络`, `软件工程`, `算法设计与分析`, `计算方法实验`, `计算机组成原理实验`, and `计算机网络实验`.
- Some rows had plausible numeric fields attached to the wrong course name. For example, `计算机基础 | 0.5 | 优秀 | 16 | 必修 | 202306` appears to correspond to `计算方法实验`, and `数据库系统 | 0.5 | 中等 | 16 | 必修 | 202306` appears to correspond to `计算机组成原理实验`.
- It changed values on exact course-name matches, including `军事理论`, `操作系统`, and `嵌入式系统原理与设计`.

## Prompting Guidance

For this workflow, prompting should stay conservative:

1. Use DeepSeek-OCR only as an optional layout/structure helper, not as the authoritative recognizer.
2. Do not ask DeepSeek-OCR to "understand", "repair", "complete", or "standardize" course names during OCR. That increases hallucination risk.
3. Prefer a two-stage path:
   - OCR engine produces raw line/cell candidates with coordinates.
   - Legato transcript parser aligns cells into rows and validates fields.
4. If DeepSeek-OCR is used, run it after PaddleOCR and treat its table layout as a hint. Course names and numeric fields should be cross-checked against OCR text/boxes.
5. Reject or mark low confidence when the model output has detector/control tokens, repeated empty cells, max-length finish, or course rows missing common required fields.

## Recommendation

Do not spend more time fine-tuning prompt wording for this transcript workflow unless the provider exposes DeepSeek-OCR native parameters such as `base_size`, `image_size`, `crop_mode`, or local inference. On the current SiliconFlow chat-completions wrapper, prompt changes did not reliably improve course-score alignment and often made latency worse.

The next practical improvement is parser-side validation: use PaddleOCR as the primary OCR source, then implement a transcript row aligner that groups text boxes by y-coordinate and validates the schema `course_name, credits, grade, hours, attr, exam_time`.
