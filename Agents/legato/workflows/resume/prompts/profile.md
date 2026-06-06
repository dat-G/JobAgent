{{common}}

Extract identity and education.

Schema:
{"identity":{"name":"","birth_year":"","sex":""},"education":[{"school":"","degree":"","major":"","department":""}]}

Rules:
- identity.name: person's real name only.
- identity.birth_year: 4-digit year from birthday/age/education if explicit.
- identity.sex: "男", "女", or "" if not found.
- education: one item per education record; keep multiple schools if present.
- education.degree: explicit 专科/本科/硕士/博士 only; use "" if not explicit.
- Missing fields are "".

Resume:
{{resume_text}}
