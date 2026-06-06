from __future__ import annotations

import json
import threading
import time
import unittest

from workflows.resume.workflow import JsonRetryError, ResumeWorkflow


class ResumeWorkflowTest(unittest.TestCase):
    def test_extracts_name_and_birth_year_as_json(self) -> None:
        def runner(field: str, resume_text: str) -> str:
            if field == "name":
                return json.dumps({"name": "陈曦"}, ensure_ascii=False)
            if field == "birth_year":
                return json.dumps({"birth_year": 2002}, ensure_ascii=False)
            raise AssertionError(f"unexpected field: {field}")

        workflow = ResumeWorkflow(runner, max_retries=5, max_workers=8)
        output = workflow.run_json("陈曦 男\n生日： 2002-10-24")

        self.assertEqual(json.loads(output), {"name": "陈曦", "birth_year": 2002})

    def test_field_extraction_runs_concurrently(self) -> None:
        lock = threading.Lock()
        active = 0
        max_active = 0

        def runner(field: str, resume_text: str) -> str:
            nonlocal active, max_active
            with lock:
                active += 1
                max_active = max(max_active, active)
            time.sleep(0.05)
            with lock:
                active -= 1
            if field == "name":
                return '{"name":"陈曦"}'
            return '{"birth_year":2002}'

        workflow = ResumeWorkflow(runner, max_retries=5, max_workers=8)
        self.assertEqual(workflow.run(""), {"name": "陈曦", "birth_year": 2002})
        self.assertGreaterEqual(max_active, 2)

    def test_retries_non_json_output_up_to_five_attempts(self) -> None:
        attempts: dict[str, int] = {"name": 0, "birth_year": 0}

        def runner(field: str, resume_text: str) -> str:
            attempts[field] += 1
            if attempts[field] < 5:
                return "not json"
            if field == "name":
                return '{"name":"陈曦"}'
            return '{"birth_year":2002}'

        workflow = ResumeWorkflow(runner, max_retries=5, max_workers=8)
        self.assertEqual(workflow.run(""), {"name": "陈曦", "birth_year": 2002})
        self.assertEqual(attempts, {"name": 5, "birth_year": 5})

    def test_fails_after_five_invalid_json_attempts(self) -> None:
        attempts: dict[str, int] = {"name": 0, "birth_year": 0}

        def runner(field: str, resume_text: str) -> str:
            attempts[field] += 1
            return "not json"

        workflow = ResumeWorkflow(runner, max_retries=5, max_workers=1)
        with self.assertRaises(JsonRetryError):
            workflow.run("陈曦 男\n生日： 2002-10-24")
        self.assertEqual(attempts["name"], 5)


if __name__ == "__main__":
    unittest.main()
