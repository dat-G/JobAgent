{{common}}

Extract identity and education.

Schema:
{"identity":{"name":"","birth_year":"","sex":""},"education":[{"school":"","degree":"","major":"","department":""}],"certifications_awards":[{"name":"","result":"","level":0,"evidence_scope":"校内"}],"experience":[{"type":"","role":"","contribution":"","level":0,"evidence_scope":"校内"}]}

Rules:
- identity.name: person's real name only.
- identity.birth_year: 4-digit year from birthday/age/education if explicit.
- identity.sex: "男", "女", or "" if not found.
- education: one item per education record; keep multiple schools if present.
- education.degree: explicit 专科/本科/硕士/博士 only; use "" if not explicit.
- certifications_awards: certificate or competition name plus score/prize/ranking, level 0-10, evidence_scope 校内/校外.
- experience: activity/work/project/contest with type, role, contribution, level 0-10, evidence_scope 校内/校外.
- evidence_scope: 校内 only for school/college/class/student-union/society/internal evidence; otherwise 校外.
- Missing fields are "".

Resume:
{{resume_text}}
