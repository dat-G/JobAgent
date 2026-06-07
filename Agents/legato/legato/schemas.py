from __future__ import annotations

RESUME_SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "required": ["candidate", "contacts", "education", "experience", "projects", "skills"],
    "properties": {
        "candidate": {
            "type": "object",
            "additionalProperties": False,
            "required": ["name"],
            "properties": {
                "name": {"type": "string"},
                "headline": {"type": "string"},
            },
        },
        "contacts": {
            "type": "object",
            "additionalProperties": False,
            "properties": {
                "email": {"type": "string"},
                "phone": {"type": "string"},
                "location": {"type": "string"},
                "links": {"type": "array", "items": {"type": "string"}},
            },
        },
        "education": {"type": "array", "items": {"type": "object"}},
        "experience": {"type": "array", "items": {"type": "object"}},
        "projects": {"type": "array", "items": {"type": "object"}},
        "skills": {"type": "array", "items": {"type": "string"}},
        "certifications": {"type": "array", "items": {"type": "string"}},
    },
}

TRANSCRIPT_SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "required": ["student", "institution", "terms", "courses", "summary"],
    "properties": {
        "student": {
            "type": "object",
            "additionalProperties": False,
            "properties": {
                "name": {"type": "string"},
                "student_id": {"type": "string"},
            },
        },
        "institution": {"type": "string"},
        "terms": {"type": "array", "items": {"type": "object"}},
        "courses": {
            "type": "array",
            "items": {
                "type": "object",
                "additionalProperties": False,
                "properties": {
                    "term": {"type": "string"},
                    "course_code": {"type": "string"},
                    "course_name": {"type": "string"},
                    "credits": {"type": "string"},
                    "grade": {"type": "string"},
                    "points": {"type": "string"},
                },
            },
        },
        "summary": {
            "type": "object",
            "additionalProperties": False,
            "properties": {
                "gpa": {"type": "string"},
                "total_credits": {"type": "string"},
                "rank": {"type": "string"},
            },
        },
    },
}

CHAT_SCHEMA = {
    "type": "object",
    "additionalProperties": False,
    "required": ["chat"],
    "properties": {
        "chat": {
            "type": "object",
            "additionalProperties": False,
            "required": [
                "answer",
                "conclusion",
                "actions",
                "evidence_refs",
                "missing_evidence",
                "confidence",
            ],
            "properties": {
                "answer": {"type": "string"},
                "conclusion": {"type": "string"},
                "actions": {"type": "array", "items": {"type": "string"}},
                "evidence_refs": {"type": "array", "items": {"type": "string"}},
                "missing_evidence": {"type": "array", "items": {"type": "string"}},
                "confidence": {"type": "number", "minimum": 0, "maximum": 1},
            },
        },
    },
}

SCHEMAS = {
    "chat": CHAT_SCHEMA,
    "resume": RESUME_SCHEMA,
    "transcript": TRANSCRIPT_SCHEMA,
}


def schema_for(target: str) -> dict:
    try:
        return SCHEMAS[target]
    except KeyError as exc:
        supported = ", ".join(sorted(SCHEMAS))
        raise ValueError(f"unsupported target {target!r}; expected one of: {supported}") from exc
