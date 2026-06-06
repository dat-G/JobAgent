import json
import os
import shlex
import subprocess
import sys
import unittest
from pathlib import Path
from unittest.mock import patch


LEGATO_ROOT = Path(__file__).resolve().parents[1]
FIXTURES = LEGATO_ROOT / "fixtures"
if str(LEGATO_ROOT) not in sys.path:
    sys.path.insert(0, str(LEGATO_ROOT))


def load_fixture(name):
    with (FIXTURES / name).open("r", encoding="utf-8") as handle:
        return handle.read()


def load_json_fixture(name):
    with (FIXTURES / name).open("r", encoding="utf-8") as handle:
        return json.load(handle)


def assert_keys(testcase, value, required, path):
    testcase.assertIsInstance(value, dict, f"{path} must be an object")
    missing = sorted(set(required) - set(value))
    testcase.assertFalse(missing, f"{path} missing keys: {missing}")


def assert_string(testcase, value, path):
    testcase.assertIsInstance(value, str, f"{path} must be a string")
    testcase.assertTrue(value.strip(), f"{path} must not be empty")


def assert_string_list(testcase, value, path):
    testcase.assertIsInstance(value, list, f"{path} must be a list")
    for index, item in enumerate(value):
        assert_string(testcase, item, f"{path}[{index}]")


def assert_subset(testcase, expected, actual, path="$"):
    if isinstance(expected, dict):
        testcase.assertIsInstance(actual, dict, f"{path} must be an object")
        for key, expected_value in expected.items():
            testcase.assertIn(key, actual, f"{path} missing key {key!r}")
            assert_subset(testcase, expected_value, actual[key], f"{path}.{key}")
        return
    if isinstance(expected, list):
        testcase.assertIsInstance(actual, list, f"{path} must be a list")
        testcase.assertEqual(len(expected), len(actual), f"{path} list length mismatch")
        for index, expected_value in enumerate(expected):
            assert_subset(testcase, expected_value, actual[index], f"{path}[{index}]")
        return
    testcase.assertEqual(expected, actual, f"{path} mismatch")


def extract_json(stdout):
    text = stdout.strip()
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        start = text.find("{")
        end = text.rfind("}")
        if start >= 0 and end > start:
            return json.loads(text[start : end + 1])
        raise


def acceptance_command():
    override = os.environ.get("LEGATO_ACCEPTANCE_COMMAND")
    if override:
        return shlex.split(override)
    return [sys.executable, "-m", "legato.cli"]


def run_legato(args, env_overrides=None):
    env = os.environ.copy()
    existing_pythonpath = env.get("PYTHONPATH")
    env["PYTHONPATH"] = (
        str(LEGATO_ROOT)
        if not existing_pythonpath
        else str(LEGATO_ROOT) + os.pathsep + existing_pythonpath
    )
    if env_overrides:
        env.update({key: str(value) for key, value in env_overrides.items()})

    proc = subprocess.run(
        acceptance_command() + args,
        cwd=LEGATO_ROOT,
        env=env,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        timeout=15,
        check=False,
    )
    if proc.returncode != 0:
        raise AssertionError(
            "Legato CLI acceptance command failed.\n"
            f"command: {shlex.join(acceptance_command() + args)}\n"
            f"cwd: {LEGATO_ROOT}\n"
            f"exit_code: {proc.returncode}\n"
            f"stdout:\n{proc.stdout}\n"
            f"stderr:\n{proc.stderr}"
        )
    return extract_json(proc.stdout)


class FixtureSchemaTests(unittest.TestCase):
    def test_resume_expected_json_matches_resume_schema(self):
        resume = load_json_fixture("resume.expected.json")
        assert_keys(
            self,
            resume,
            {
                "candidate",
                "contacts",
                "education",
                "experience",
                "projects",
                "skills",
                "certifications",
            },
            "$",
        )
        assert_keys(self, resume["candidate"], {"name", "headline"}, "$.candidate")
        assert_string(self, resume["candidate"]["name"], "$.candidate.name")
        self.assertIsInstance(resume["candidate"]["headline"], str)
        assert_keys(self, resume["contacts"], {"email", "phone", "location", "links"}, "$.contacts")
        for key in ("email", "phone", "location"):
            self.assertIsInstance(resume["contacts"][key], str, f"$.contacts.{key} must be a string")
        assert_string_list(self, resume["contacts"]["links"], "$.contacts.links")

        for section in ("education", "experience", "projects"):
            self.assertGreaterEqual(len(resume[section]), 1, f"$.{section} must not be empty")
            for index, item in enumerate(resume[section]):
                assert_keys(self, item, {"text"}, f"$.{section}[{index}]")
                assert_string(self, item["text"], f"$.{section}[{index}].text")

        assert_string_list(self, resume["skills"], "$.skills")
        assert_string_list(self, resume["certifications"], "$.certifications")

    def test_transcript_expected_json_matches_transcript_schema(self):
        transcript = load_json_fixture("transcript.expected.json")
        assert_keys(
            self,
            transcript,
            {"institution", "student", "terms", "courses", "summary"},
            "$",
        )
        assert_string(self, transcript["institution"], "$.institution")
        assert_keys(self, transcript["student"], {"name", "student_id"}, "$.student")
        assert_string(self, transcript["student"]["name"], "$.student.name")
        assert_string(self, transcript["student"]["student_id"], "$.student.student_id")
        assert_keys(self, transcript["summary"], {"gpa", "total_credits", "rank"}, "$.summary")
        for key in ("gpa", "total_credits", "rank"):
            self.assertIsInstance(transcript["summary"][key], str, f"$.summary.{key} must be a string")

        self.assertGreaterEqual(len(transcript["terms"]), 1)
        for term_index, term in enumerate(transcript["terms"]):
            assert_keys(self, term, {"term"}, f"$.terms[{term_index}]")
            assert_string(self, term["term"], f"$.terms[{term_index}].term")

        self.assertGreaterEqual(len(transcript["courses"]), 1)
        for course_index, course in enumerate(transcript["courses"]):
            assert_keys(
                self,
                course,
                {"term", "course_code", "course_name", "credits", "grade", "points"},
                f"$.courses[{course_index}]",
            )
            for key in ("term", "course_code", "course_name", "credits", "grade", "points"):
                assert_string(self, course[key], f"$.courses[{course_index}].{key}")


class LegatoCliAcceptanceTests(unittest.TestCase):
    def test_resume_markdown_to_resume_json_schema_with_no_presto(self):
        expected = load_json_fixture("resume.expected.json")
        actual = run_legato([str(FIXTURES / "resume.md"), "--target", "resume", "--no-presto"])
        self.assertEqual(actual["status"], "ok")
        self.assertEqual(actual["target"], "resume")
        self.assertEqual(actual["frontend"], "markitdown")
        self.assertEqual(actual["formatter"], "local_rules")
        assert_subset(self, expected, actual["data"])

    def test_transcript_markdown_table_to_transcript_json_schema_with_no_presto(self):
        expected = load_json_fixture("transcript.expected.json")
        actual = run_legato([str(FIXTURES / "transcript.md"), "--target", "transcript", "--no-presto"])
        self.assertEqual(actual["status"], "ok")
        self.assertEqual(actual["target"], "transcript")
        self.assertEqual(actual["frontend"], "markitdown")
        self.assertEqual(actual["formatter"], "local_rules")
        assert_subset(self, expected, actual["data"])

    def test_markitdown_frontend_can_be_mocked_for_non_markdown_inputs(self):
        from legato.markitdown_frontend import MarkdownDocument
        from legato.pipeline import process

        expected = load_json_fixture("resume.expected.json")

        class FakeFrontend:
            def convert(self, source):
                return MarkdownDocument(
                    markdown=load_fixture("resume.md"),
                    title="mocked-resume",
                    source_path=str(source),
                )

        with patch("legato.pipeline.MarkItDownFrontend", FakeFrontend):
            result = process(
                FIXTURES / "mock_source.bin",
                "resume",
                use_presto=False,
                timeout_ms=3000,
            ).to_dict()

        self.assertEqual(result["status"], "ok")
        self.assertEqual(result["formatter"], "local_rules")
        assert_subset(self, expected, result["data"])

    def test_cli_supports_no_presto_for_local_acceptance_without_external_formatter(self):
        actual = run_legato(
            [
                str(FIXTURES / "resume.md"),
                "--target",
                "resume",
                "--no-presto",
                "--timeout-ms",
                "3000",
            ]
        )
        self.assertEqual(actual["formatter"], "local_rules")
        self.assertIn("local rule formatter", " ".join(actual["warnings"]))


if __name__ == "__main__":
    unittest.main()
