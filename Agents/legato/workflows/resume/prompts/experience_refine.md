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
- Treat descriptive research text as possible 科研项目 even when it is not under an explicit "项目经历" heading. Recognize patterns such as "基于 X 的 Y 方法/模型/算法/系统/框架/模块", "针对 X 数据集/问题/场景/任务开发/实现/复现/分析/评估", "论文复现", "模型训练", "算法实现", "实验评估", and "数据清洗".
- For descriptive research, infer a concise project role from the research object and method, for example "中文命名实体识别模型研究项目" or "轨迹田路分割研究项目". Do not hard-code one domain; apply the same rule to NLP, CV, security, databases, robotics, agriculture, and other fields.
- Keep only research descriptions with concrete contribution evidence. Do not convert course names, skill lists, education lines, generic interests, or title-only research directions into experience items.

Local candidates:
{{local_candidates}}

Resume text:
{{resume_text}}
