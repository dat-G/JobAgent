{{common}}

Extract certificates and awards only.

Schema:
{"certifications_awards":[{"name":"","result":"","level":0,"evidence_scope":"校内"}]}

Rules:
- name: certificate or competition name.
- result: score, grade, prize, ranking, or "" if not found.
- level: 0-10 fast evidence strength, conservative, no deep reasoning.
- evidence_scope: "校内" only for school/college/class/student-union/society/internal honors or activities; otherwise "校外".
- Include English tests, contests, competitions, scholarships, and honors.
- Extract paragraph-style contest/award mentions too; do not rely only on award lists.
- Do not include skills or project names.

Resume:
{{resume_text}}
