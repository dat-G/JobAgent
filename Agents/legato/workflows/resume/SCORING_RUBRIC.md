# Resume Experience Scoring Rubric

This rubric scores `experience.level` for ability assessment. It should be conservative: score the evidence in the resume text, not the brand name alone.

## Source Basis

- NACE career readiness competencies: technology, critical thinking, leadership, teamwork, professionalism, communication, and career/self-development.
- Structured interview practice: evaluate job-related competencies with consistent criteria.
- STAR/CAR evidence framing: situation/context, task, action, result. Resume text rarely contains full STAR, but stronger entries should show action and outcome.

## 10-Point Formula

Use six dimensions. Add points, then apply caps and penalties.

| Dimension | Points | What to look for |
| --- | ---: | --- |
| Relevance to target role | 0-2.0 | Directly related work/technical domain scores higher than generic campus activity. |
| Technical / professional depth | 0-2.0 | Engineering, analysis, research, product, data, algorithm, system, testing, or domain depth. Routine labeling/audit is low. |
| Individual ownership | 0-2.0 | Led, owned, independently built, designed, analyzed, or solved a concrete part. Team-only or title-only evidence is weak. |
| Outcome / impact | 0-1.5 | Award, ranking, launch, recognized result, measurable output, successful delivery, or clear improvement. |
| Scope / selectivity | 0-1.5 | National/top-tier/provincial/regional/known organization/school level. Scope alone should not dominate. |
| Evidence quality | 0-1.0 | Specific actions and results. Vague titles or one-line honors get little evidence credit. |

## Score Bands

| Score | Meaning |
| ---: | --- |
| 9-10 | Exceptional: high selectivity plus clear core ownership and meaningful result. Rare. |
| 7-8 | Strong: substantial technical/professional contribution, clear ownership, relevant scope/result. |
| 5-6 | Solid: concrete relevant work or campus/project leadership, but limited depth, scope, or impact. |
| 3-4 | Low signal: routine work, labeling/audit/simple correction, generic participation, or weakly described contribution. |
| 1-2 | Very weak: generic honor/certificate/title with little ability evidence. |
| 0 | Invalid or unusable. |

## Caps And Penalties

Apply these after the base score.

| Case | Cap / score |
| --- | ---: |
| Basic certificates: CET, computer rank certificates, WPS, office software | 1-2 |
| Generic honors: 三好学生, 优秀学生干部, 标兵, 红旗手 | 1-2 |
| Scholarship only, no described project/work | 2-3 |
| Labeling/audit/correction task, no technical ownership | cap 4 |
| Outsourced-looking platform annotation or simple data labeling | cap 4 unless technical ownership is explicit |
| School-level project with concrete technical work | usually 5-6, cap 6 |
| Formal regional/provincial contest with award | usually 5-7 depending award and technical relevance |
| National contest title but weak contribution evidence | usually 6-7, not automatically 8+ |
| National/top-tier result plus described ownership | 8-9 |
| Pure award line without contribution description | keep in `certifications_awards`; do not create `experience` |

## Field Semantics

- `role`: identity of the experience. Prefer `organization/company/event / role` when both parts are explicit. Keep it empty when only a vague project description is available.
- `contribution`: concise evidence of what the student did. Do not use it to restate company, school, or contest names unless that is the only meaningful evidence.
- `level`: score the contribution evidence first, then adjust by organization/contest/school scope. Brand or title alone should not produce a high score.
- `evidence_scope`: `校内` for school, college, class, student-union, society, internal honor, or campus project evidence; `校外` for internships, certificates, public competitions, external organizers, and company evidence. Scope is used for grouping, not as a direct score cap.

## Six-Dimension Radar

`item_benchmark` returns a six-dimensional distribution and an `impact_factor`. The frontend derives radar scores from item evidence:

1. Evidence strength = `0.4 * level/10 + 0.6 * impact_factor/10`.
2. Dimension contribution = `six_dim_score * evidence_strength * 1.85`, capped at `0.96`.
3. Multiple items combine with diminishing returns: `combined = 1 - product(1 - contribution_i)`.
4. 校内 and 综合 blend an academic prior from the `major_baseline` workflow. A missing GPA or transcript average around 80 maps to an ability prior around 50, not ability 80.
5. Low-confidence evidence buckets have both single-item and total-bucket caps. Untitled professional projects, campus/internal awards, low-impact awards/basic certificates, and generic campus roles can support a profile, but cannot stack into strong evidence without concrete titles, formal external selection, or measured outcomes.
5. Render three overlays: `综合` for all items plus academic baseline, `校内` for campus/internal items plus academic baseline, `校外` for external items.

This keeps low-value certificates from dominating while letting repeated high-quality evidence accumulate.

## Major Baseline Calibration

`major_baseline` is a separate Presto-backed stage. It classifies the major family and returns 0-100 ability prior scores for the same six dimensions.

Major family is one of `文科类`, `理科类`, `工科类`, `商科类`, `医农类`, `艺术体育类`, `交叉类`, or `未知`.

Grade-to-ability mapping:

| Academic signal | Ability prior |
| --- | ---: |
| missing transcript or average around 80 | around 50 |
| average around 85 | around 58 |
| average around 90 | around 66 |
| average around 95 | around 74 |

Do not treat raw grades as ability scores. An average score of 80 is normal academic completion, not 80/100 ability proof.

Use mild offsets from `base_score`:

| Major family | Higher baseline dimensions | Lower/conservative dimensions |
| --- | --- | --- |
| 工科类 | 逻辑, 专业 | 语言, 领导 |
| 理科类 | 逻辑, 专业 | 语言, 领导 |
| 文科类 | 语言, 专业 | 领导 unless trained by role evidence |
| 商科类 | 语言, 专业, mild 领导 | none high without evidence |
| 医农类 | 专业, 抗压 | 领导 |
| 艺术体育类 | 专业, 抗压, 语言 when expression-heavy | 逻辑 |
| 交叉类 | 逻辑/语言/专业 balanced | 领导 conservative |
| 未知 | near `base_score` | 领导 and 抗压 conservative |

Major alone should not create extreme scores. Keep most academic priors between 35 and 70; use 70-80 only for strong academic evidence. Leadership should generally remain conservative unless management/organization training is explicit.

School tier is a visible academic prior, not a ranking contest:

| School evidence | Suggested effect |
| --- | --- |
| 985 or soft-rank <= 50 | clear positive, usually around +3 on affected dimensions |
| 211 / 双一流 / soft-rank <= 150 | moderate positive, usually around +2 on affected dimensions |
| non-双一流 soft-rank 151-250 | light debuff, around -2 before specialty offset |
| non-双一流 soft-rank 251-400 | medium debuff, around -4 before specialty offset |
| ordinary / unknown / lower rank | medium debuff; do not collapse the score, but avoid treating it as neutral |
| independent/private/former third-tier college | separate from parent school; missing GPA or average around 80 usually maps to 38-43 prior |

Apply school tier mainly to `逻辑`, `专业`, `抗压`, and `成长`; do not raise `领导` from school tier alone.
If the school's known field orientation or resume context suggests a strong specialty match, add a small extra lift to `专业` and one adjacent dimension, but do not erase a below-tier debuff.
Do not invent a specialty. Use explicit context such as `王牌专业`, `特色专业`, `优势专业`, `一流学科`, or broad school-domain alignment such as electronic-information schools with computing/electronics majors.
Independent/private/former third-tier colleges do not inherit the parent university's ranking or specialty strength. Their real projects, internships, and competitions can still raise final ability scores normally.

## Contest Calibration

Do not blacklist specific contest names. Score by:

1. Scope: school < regional/provincial < national/top-tier.
2. Contest type: technical/professional contests count more than generic honors.
3. Award: first prize > second prize > third prize, but award alone is not enough.
4. Contribution: leader/core builder/modeler/developer/designer is stronger than unnamed participant.
5. Evidence: if the resume only lists the award, keep it in `certifications_awards`.

Examples:

| Entry | Suggested score |
| --- | ---: |
| Basic computer certificate | 2 |
| 三好学生 / 优秀学生干部 | 2 |
| Routine model annotation internship | 4 |
| School technical project with specific module ownership | 5-6 |
| Regional formal modeling contest, second prize | 6-7 |
| National-title modeling contest, third prize | 6-7 |
| National contest final with described team leadership and product/technical work | 8-9 |

## Implementation Notes

- `role` is optional. Do not overfit extraction to a role if contribution and score are clear.
- Prefer conservative scoring when evidence is missing.
- Treat text evidence as primary. Organization/contest names are modifiers, not the main score.
- If future model-based scoring is added, ask the model to output dimension-level subscores and a one-line rationale, then validate with these caps.

## References

- National Association of Colleges and Employers. Career Readiness Competencies. https://www.naceweb.org/career-readiness/competencies/career-readiness-defined
  - Used for competency dimensions such as technology, critical thinking, leadership, teamwork, professionalism, and communication.
- U.S. Office of Personnel Management. Structured Interviews. https://www.opm.gov/policy-data-oversight/assessment-and-selection/structured-interviews
  - Used for the principle that candidate evidence should be evaluated against consistent, job-related criteria.
- STAR method overview. https://en.wikipedia.org/wiki/Situation%2C_task%2C_action%2C_result
  - Used for the evidence-quality idea that stronger experience descriptions include context/task, action, and result.
