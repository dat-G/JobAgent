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
