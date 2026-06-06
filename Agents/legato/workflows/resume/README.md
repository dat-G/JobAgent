# Resume Workflow

This workflow focuses on resume structuring.

Current intended flow:

```text
resume file
  -> PDF text-layer fast path or MarkItDown fallback
  -> cleaning
  -> Presto formatter
  -> resume JSON
```

This directory is the place for workflow-specific prompts, schemas, examples, and acceptance notes.

## Current Contract

The first workflow contract is intentionally narrow:

- Extract `identity`.
- Extract `education`.
- Extract `certifications_awards`.
- Extract `experience`.
- Enrich education schools with local ranking tags.
- Add normalized `degree_level` for every education item.
- `identity` contains `name`, `birth_year`, and `sex`.
- `sex` may be `""` when not found.
- Run field extraction concurrently.
- Each field extractor must return a JSON object.
- If a field output is not valid JSON or misses the requested field, retry.
- Maximum retries: `5`.
- Final workflow output is JSON:

```json
{
  "identity": {
    "name": "陈曦",
    "birth_year": "2002",
    "sex": "男"
  },
  "education": [
    {
      "school": "东北农业大学",
      "degree": "本科",
      "degree_level": "本科",
      "major": "计算机科学与技术",
      "department": "电气与信息学院",
      "school_tags": {
        "matched_school": "东北农业大学",
        "is_985": false,
        "is_211": true,
        "is_double_first_class": true,
        "ruanke_rank": 120
      }
    }
  ],
  "certifications_awards": [
    {
      "name": "全国大学英语六级考试",
      "result": "567分"
    },
    {
      "name": "2023年全国大学生数学建模竞赛 黑龙江赛区",
      "result": "一等奖"
    }
  ],
  "experience": [
    {
      "type": "实习",
      "role": "前端开发实习生",
      "contribution": "在高新兴参与视频云平台监控前台系统开发。",
      "level": 7
    }
  ]
}
```

## Prompt Layout

```text
prompts/common.md      shared short rules for JSON and accuracy
prompts/profile.md     identity + education in one request
prompts/certifications_awards.md certificates and awards
prompts/item_benchmark.md six-dimensional item scoring
prompts/combined.md    profile + certifications_awards + experience in one request
prompts/merge.md       final schema merge
prompts/retry_json.md  short retry instruction for invalid JSON
schema.json            first-version output schema
```

First-version concurrency:

```text
profile agent ----------------\
certifications_awards agent --+-- local school tags + local experience -> merge -> validate -> JSON
```

`certifications_awards` does not receive the full resume by default. It first uses a broad keyword recall step and keeps matched lines plus nearby context. If the recalled candidate text is too short, it falls back to the full resume.

Experience is generated locally in the first version to avoid an extra high-latency model request. It uses resume text plus `certifications_awards` to produce work/project, contest, and campus-role entries.
To avoid double counting, `experience` keeps only described work/project/role/contest entries with contribution text. Undescribed competitions and certificates stay only in `certifications_awards`.
`role` is optional, but when available it should prefer `organization/company/event / role`, for example `字节跳动 / MCP标注(实习生)` or `杜邦青年创新大赛 / 队长`. For projects without a clear organization or title, keep `role` as `""` instead of fabricating one.
`contribution` should be a short ability-focused contribution summary. It should avoid repeating the organization/event name and focus on what the student did in that experience.
Ability assessment should rely primarily on `contribution` and `level`.

Education school tags are generated locally from `cache/ruanke_china_university_ranking_2026_structured.json`. Matching uses exact school name first, then a conservative contains match for extracted names such as `东北农业大学 · 本科`.
Each education item gets `degree_level` as one of `专科` / `本科` / `硕士` / `博士` / `""`. If there is only one education item and its degree is missing, the local `education_degree_inference_agent` infers it from study years and school name, defaulting toward `本科`.

Comparison mode:

```text
combined agent -> validate -> JSON
```

Use `--workflow-combine-agents` to compare single-request latency with the concurrent path.

No contact extraction in this workflow version.

## Item Benchmark

`--workflow-stage item_benchmark` first extracts `certifications_awards`, then benchmarks each item concurrently through Presto.

Each output item contains:

- `item`: original `name` and `result`.
- `dimensions`: fixed order `逻辑`, `语言`, `专业`, `领导`, `抗压`, `成长`.
- `scores`: normalized six-dimensional weight vector, each value in `0.0-1.0`, and the six values sum to `1.0`.
- `impact_factor`: `0-10`, similar to `experience.level`, measuring how strongly the item proves ability.

Dimension definitions:

- `逻辑`: math, science, analysis, modeling, and problem-solving ability.
- `语言`: writing, communication, presentation, and humanities expression.
- `专业`: ability related to the student's major or technical field.
- `领导`: leadership, ownership, organization, and team influence.
- `抗压`: pressure, difficulty, persistence, and delivery under constraints.
- `成长`: learning potential, initiative, improvement, and exploration.

For DeepSeek thinking mode, run Presto with:

```sh
PRESTO_THINKING=enabled PRESTO_REASONING_EFFORT=high go run ./cmd/presto
```

The final JSON does not expose chain-of-thought; the model is instructed to use brief internal reasoning and return only scores.

## Experience Level

`experience.level` is a 0-10 numeric score:

Detailed scoring rules live in `SCORING_RUBRIC.md`.

- `9-10`: rare, high-signal experience with national/top-tier context and clear core ownership.
- `7-8`: strong technical/research/contest experience with clear ownership and meaningful output.
- `5-6`: normal internship/project/campus leadership with concrete contribution but limited signal.
- `3-4`: low-technical-content work such as labeling, audit, simple correction, or routine participation.
- `1-2`: weak evidence or mostly title-only experience.
- `0`: invalid or unusable experience.

Pure award lines without contribution descriptions are not scored here; they remain in `certifications_awards`.
Company brand or contest name alone is not enough for a high score. Formal contests should be judged by scope, award, and technical relevance; concrete school-level technical projects can score mid-high but should not reach the top tier. Basic certificates and generic honors such as 三好学生, 优秀学生干部, and routine computer certificates carry low value. Outsourced/labeling-style internships should stay conservative unless the text proves substantial technical ownership.

## Tests

```sh
python3 -m unittest discover -s workflows/resume/tests -p "test*.py"
```
