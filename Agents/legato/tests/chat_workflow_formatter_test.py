from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from legato.chat_workflow_formatter import ChatWorkflowFormatter
from legato.pipeline import process
from workflows.chat.workflow import ChatWorkflow, JsonRetryError


class ChatWorkflowFormatterTest(unittest.TestCase):
    def test_answer_retries_until_valid_json(self) -> None:
        formatter = ChatWorkflowFormatter(
            max_retries=2,
            stage_input={
                "question": "优先补哪项能力？",
                "diagnosis": {"ability_profile": {"basic_info": {"name": "陈曦"}}},
            },
        )
        calls: list[str] = []

        def fake_call(prompt: str, group: str) -> str:
            calls.append(prompt)
            self.assertEqual(group, "answer")
            if len(calls) == 1:
                return "not json"
            return json.dumps(
                {
                    "answer": "结论：先补专业项目证据。行动：完善项目描述。",
                    "conclusion": "先补专业项目证据。",
                    "actions": ["完善项目描述", "补充量化结果"],
                    "evidence_refs": ["ability_profile.basic_info"],
                    "missing_evidence": [],
                    "confidence": 0.8,
                },
                ensure_ascii=False,
            )

        formatter._call_presto = fake_call  # type: ignore[method-assign]
        result = formatter.format_stage("", "answer")
        self.assertEqual(result.data["chat"]["conclusion"], "先补专业项目证据。")
        self.assertEqual(result.debug["agents"]["answer"]["retry_count"], 1)
        self.assertIn("previous output was invalid", calls[1])

    def test_answer_requires_stage_input_or_source_question(self) -> None:
        formatter = ChatWorkflowFormatter(max_retries=1)

        def fake_call(prompt: str, group: str) -> str:
            self.assertIn("User question:\n成绩单必须传吗", prompt)
            return json.dumps(
                {
                    "answer": "结论：不是必须传，但会影响准确性。",
                    "conclusion": "成绩单不是必须传。",
                    "actions": ["先上传简历生成诊断", "有成绩单再补传"],
                    "evidence_refs": [],
                    "missing_evidence": ["成绩单"],
                    "confidence": 0.6,
                },
                ensure_ascii=False,
            )

        formatter._call_presto = fake_call  # type: ignore[method-assign]
        result = formatter.format("成绩单必须传吗")
        self.assertEqual(result.data["chat"]["missing_evidence"], ["成绩单"])

    def test_answer_retries_presto_call_errors(self) -> None:
        formatter = ChatWorkflowFormatter(max_retries=2, stage_input={"question": "岗位怎么选？"})
        calls: list[str] = []

        def fake_call(prompt: str, group: str) -> str:
            calls.append(prompt)
            self.assertEqual(group, "answer")
            if len(calls) == 1:
                raise RuntimeError("temporary presto failure")
            self.assertIn("temporary presto failure", prompt)
            return json.dumps(
                {
                    "answer": "结论：先选证据最强的岗位。",
                    "conclusion": "先选证据最强的岗位。",
                    "actions": ["对齐岗位要求", "补充项目证据"],
                    "evidence_refs": ["matching"],
                    "missing_evidence": [],
                    "confidence": 0.72,
                },
                ensure_ascii=False,
            )

        formatter._call_presto = fake_call  # type: ignore[method-assign]
        result = formatter.format_stage("", "answer")
        self.assertEqual(result.data["chat"]["conclusion"], "先选证据最强的岗位。")
        self.assertEqual(result.debug["agents"]["answer"]["retry_count"], 1)

    def test_lightweight_chat_workflow_retries_json(self) -> None:
        outputs = iter(
            [
                "bad",
                json.dumps(
                    {
                        "answer": "结论：可以追问。",
                        "conclusion": "可以追问。",
                        "actions": ["生成诊断后提问", "补充材料"],
                        "evidence_refs": [],
                        "missing_evidence": [],
                        "confidence": 0.7,
                    },
                    ensure_ascii=False,
                ),
            ]
        )
        workflow = ChatWorkflow(lambda _context: next(outputs), max_retries=2)
        result = workflow.run_with_meta({"question": "能问什么？"})
        self.assertEqual(result.attempts, 2)

    def test_lightweight_chat_workflow_fails_after_retries(self) -> None:
        workflow = ChatWorkflow(lambda _context: "bad", max_retries=2)
        with self.assertRaises(JsonRetryError):
            workflow.run({"question": "能问什么？"})

    def test_pipeline_routes_chat_workflow(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            source = Path(tmp) / "chat.md"
            source.write_text("优先补哪项能力？\n", encoding="utf-8")
            original_call = ChatWorkflowFormatter._call_presto
            case = self

            def fake_call(_formatter: ChatWorkflowFormatter, prompt: str, group: str) -> str:
                case.assertEqual(group, "answer")
                case.assertIn("优先补哪项能力", prompt)
                return json.dumps(
                    {
                        "answer": "结论：先补专业证据。",
                        "conclusion": "先补专业证据。",
                        "actions": ["补项目描述", "补成果指标"],
                        "evidence_refs": ["diagnosis"],
                        "missing_evidence": [],
                        "confidence": 0.75,
                    },
                    ensure_ascii=False,
                )

            ChatWorkflowFormatter._call_presto = fake_call  # type: ignore[method-assign]
            try:
                result = process(
                    source,
                    "chat",
                    workflow="chat",
                    timeout_ms=3000,
                    use_presto=True,
                )
            finally:
                ChatWorkflowFormatter._call_presto = original_call  # type: ignore[method-assign]
        self.assertEqual(result.data["chat"]["conclusion"], "先补专业证据。")
        self.assertEqual(result.formatter, "presto_chat_workflow_answer")


if __name__ == "__main__":
    unittest.main()
