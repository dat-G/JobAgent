from __future__ import annotations

import json
import subprocess
import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


class CLIAcceptanceTest(unittest.TestCase):
    def run_legato(self, fixture: str, target: str) -> dict:
        proc = subprocess.run(
            [
                sys.executable,
                "-m",
                "legato.cli",
                str(ROOT / "fixtures" / fixture),
                "--target",
                target,
                "--no-presto",
            ],
            cwd=ROOT,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            check=True,
        )
        return json.loads(proc.stdout)

    def test_resume_fixture_formats_to_target_json(self) -> None:
        payload = self.run_legato("resume.md", "resume")
        self.assertEqual(payload["status"], "ok")
        self.assertEqual(payload["target"], "resume")
        self.assertEqual(payload["frontend"], "markitdown")
        self.assertEqual(payload["formatter"], "local_rules")
        self.assertEqual(payload["data"]["candidate"]["name"], "Ada Chen")
        self.assertEqual(payload["data"]["contacts"]["email"], "ada@example.com")
        self.assertIn("Python", payload["data"]["skills"])

    def test_transcript_fixture_preserves_course_rows(self) -> None:
        payload = self.run_legato("transcript.md", "transcript")
        self.assertEqual(payload["status"], "ok")
        self.assertEqual(payload["data"]["student"]["name"], "Ada Chen")
        self.assertEqual(payload["data"]["summary"]["gpa"], "3.85")
        self.assertEqual(len(payload["data"]["courses"]), 3)
        self.assertEqual(payload["data"]["courses"][0]["course_code"], "CS101")


if __name__ == "__main__":
    unittest.main()

