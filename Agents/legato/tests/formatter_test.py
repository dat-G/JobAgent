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

    def test_extract_json_object_repairs_missing_field_comma(self) -> None:
        payload = '{\n"candidate": {"name": "Ada"}\n"contacts": {"email": "ada@example.com"}\n}'
        self.assertEqual(extract_json_object(payload)["candidate"]["name"], "Ada")


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


class InvalidJSONPrestoFormatter(PrestoFormatter):
    def __init__(self) -> None:
        super().__init__(base_url="http://presto.test", timeout_seconds=2)

    def _request(self, method: str, path: str, payload: dict) -> dict:
        if path == "/sessions":
            return {"id": "session-test"}
        if path == "/sessions/session-test/runs":
            return {"output": "not json"}
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

    def test_presto_formatter_falls_back_to_local_rules_on_invalid_json(self) -> None:
        formatter = InvalidJSONPrestoFormatter()
        result = formatter.format("# Ada Chen\nEmail: ada@example.com\nSkills: Python", "resume")
        self.assertEqual(result.formatter, "presto_local_fallback")
        self.assertEqual(result.data["candidate"]["name"], "Ada Chen")
        self.assertTrue(any("invalid JSON" in warning for warning in result.warnings))


if __name__ == "__main__":
    unittest.main()
