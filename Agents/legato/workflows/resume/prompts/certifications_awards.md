{{common}}

Extract certificates and awards only.

Schema:
{"certifications_awards":[{"name":"","result":""}]}

Rules:
- name: certificate or competition name.
- result: score, grade, prize, ranking, or "" if not found.
- Include English tests, contests, competitions, scholarships, and honors.
- Extract paragraph-style contest/award mentions too; do not rely only on award lists.
- Do not include skills or project names.

Resume:
{{resume_text}}
