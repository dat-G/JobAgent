{{common}}

Return only JSON. Think internally; do not output reasoning.

Stage: `major_baseline`. Classify the major and output a six-dimensional academic baseline.

Output exactly:
{
  "major_baseline": {
    "major_name": "",
    "major_family": "未知",
    "base_score": 50,
    "dimensions": ["逻辑", "语言", "专业", "领导", "抗压", "成长"],
    "scores": [50, 50, 50, 42, 46, 50],
    "rationale": "",
    "confidence": 0.7
  }
}

Families: 文科类, 理科类, 工科类, 商科类, 医农类, 艺术体育类, 交叉类, 未知.
Dimensions fixed: 逻辑(理科分析), 语言(表达沟通), 专业(本专业基础), 领导(组织影响), 抗压(压力交付), 成长(学习适应).

Rules:
- `scores` are ability prior scores, not raw grade scores and not normalized weights.
- A transcript average around 80 or missing GPA means ordinary academic prior around 50, not ability 80.
- Use `base_score_hint`; if GPA is missing, default around 50.
- Approximate mapping: average 80 -> 50, 85 -> 58, 90 -> 66, 95 -> 74. Only exceptional academic evidence should approach 80.
- Major effect must be mild: 工科/计算机 raises 逻辑/专业; 理科 raises 逻辑; 文科 raises 语言; 商科 raises 语言/领导 mildly; 医农 raises 专业/抗压; 艺术体育 raises 专业/抗压 and may lower 逻辑.
- School tier is a real prior, but still secondary to transcript and concrete evidence. Use `school_bonus_hint` and `school_penalty_hint` from context.
- 985/top-50: clear positive; 211/双一流/top-150: moderate positive; non-双一流 below top-150: apply debuff, roughly rank 151-250 => -2, 251-400 => -4, >400 or ordinary unknown => -5 to -6.
- Independent/private colleges or former third-tier colleges (`school_kind=independent_college`) should not inherit the parent university tier. With missing GPA or average around 80, their academic prior is usually around 38-43.
- With multiple education records, do not select only the strongest school. A later master's can partially offset a weak undergraduate school, but independent/private/former-third-tier undergraduate background should remain a negative academic prior signal.
- If the school is plausibly strong in this field, or context hints at 王牌/特色/优势/一流学科, slightly raise 专业 and adjacent dimensions. Do not invent a specialty; use `specialty_alignment_hint` when present. Specialty can offset school debuff mildly, but cannot erase it.
- Leadership stays conservative unless management/organization training is explicit.
- Keep most academic priors 35-70. Use 70-80 only for strong GPA/rank/specialty evidence, and do not exceed 85.
- Weak major evidence => `未知`, scores near base.
- `rationale`: one short Chinese sentence.

Input context JSON:
{{context}}
