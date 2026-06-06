from __future__ import annotations

import json
import unittest

from legato.formatter import PrestoFormatter, build_formatter_prompt, extract_json_object


class FormatterHelpersTest(unittest.TestCase):
    def test_extract_json_object_accepts_fenced_json(self) -> None:
        self.assertEqual(extract_json_object("```json\n{\"ok\": true}\n```"), {"ok": True})

    def test_prompt_contains_target_schema_and_markdown(self) -> None:
        prompt = build_formatter_prompt("# Ada", "resume")
        self.assertIn("Target: resume", prompt)
        self.assertIn("JSON schema:", prompt)
        self.assertIn("# Ada", prompt)


class MockPrestoFormatter(PrestoFormatter):
    def __init__(self) -> None:
        super().__init__(base_url="http://presto.test", timeout_seconds=2)
        self.calls: list[tuple[str, str, dict]] = []

    def _request(self, method: str, path: str, payload: dict) -> dict:
        self.calls.append((method, path, payload))
        if path == "/sessions":
            return {"id": "session-test"}
        if path == "/sessions/session-test/runs":
            if "message" not in payload:
                raise AssertionError(f"missing message payload: {payload}")
            output = {
                "candidate": {"name": "Ada Chen", "headline": ""},
                "contacts": {"email": "ada@example.com", "phone": "", "location": "", "links": []},
                "education": [],
                "experience": [],
                "projects": [],
                "skills": ["Python"],
                "certifications": [],
            }
            return {"output": json.dumps(output)}
        raise AssertionError(f"unexpected path: {path}")


class PrestoFormatterTest(unittest.TestCase):
    def test_presto_formatter_uses_session_run_api(self) -> None:
        formatter = MockPrestoFormatter()
        result = formatter.format("# Ada", "resume")
        self.assertEqual(result.formatter, "presto")
        self.assertEqual(result.data["candidate"]["name"], "Ada Chen")
        self.assertEqual(
            [(method, path) for method, path, _ in formatter.calls],
            [("POST", "/sessions"), ("POST", "/sessions/session-test/runs")],
        )


if __name__ == "__main__":
    unittest.main()
