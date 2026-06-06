Return only JSON. Use facts from resume only.
{
  "experience": [
    {"type": "", "role": "", "contribution": "", "level": 0}
  ]
}

Rules:
- Preserve real local candidates; improve wording and split broad project entries.
- Exclude pure awards/certs/scholarships/skills/education/self-evaluation/generic participation.
- type in: 实习, 科研项目, 项目, 比赛, 社团, 任职.
- role: company/org/event/project / role; contribution: <=35 chars ability evidence.
- level 0-10 conservative. labeling/audit <=4, generic honor/cert <=2.
- Split security/vulnerability work by target when findings differ.

Local candidates:
{{local_candidates}}

Resume text:
{{resume_text}}
