Return only JSON. Use brief internal reasoning, but do not output the reasoning.

Benchmark this resume item as evidence of student ability.

Output:
{
  "scores": [0, 0, 0, 0, 0, 0],
  "impact_factor": 0
}

Dimensions, order fixed:
1. 逻辑: math/science/analysis/modeling/problem solving.
2. 语言: writing/communication/presentation/humanities expression.
3. 专业: ability related to the student's major or technical field.
4. 领导: leadership, ownership, organization, team influence.
5. 抗压: competition pressure, difficulty, persistence, delivery under constraints.
6. 成长: learning potential, initiative, improvement, exploration.

Rules:
- `scores` length must be 6; each score is 0.0-1.0 and the six scores must sum to 1.0.
- `impact_factor` is 0-10 like experience level.
- Score evidence, not title alone.
- Generic honors/certificates usually have low impact.
- Formal technical competitions with awards can be higher.
- Language certificates raise 语言, but usually low 专业 and low impact unless directly relevant.

Student context:
{{student_context}}

Item:
{{item}}
