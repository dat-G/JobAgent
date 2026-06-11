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

    def test_answer_can_return_schema_intent(self) -> None:
        formatter = ChatWorkflowFormatter(
            max_retries=1,
            stage_input={
                "question": "给我路径规划 schema",
                "ui_schema_catalog": {
                    "path_plan": {
                        "roots": ["/path_plan"],
                        "schema": {"type": "object", "fields": {"stages": "array"}},
                    }
                },
            },
        )

        def fake_call(prompt: str, group: str) -> str:
            self.assertIn("UI schema catalog JSON:", prompt)
            self.assertIn("path_plan", prompt)
            return json.dumps(
                {
                    "answer": "结论：路径规划 schema 如下。",
                    "conclusion": "已返回路径规划 schema。",
                    "actions": ["按 schema 指定要改的阶段"],
                    "evidence_refs": ["ui_schema_catalog.path_plan"],
                    "missing_evidence": [],
                    "confidence": 0.9,
                    "ui_intent": {
                        "mode": "show_schema",
                        "target": "path_plan",
                        "patches": [],
                        "schema": {"type": "object", "fields": {"stages": "array"}},
                        "summary": "路径规划 schema 已返回。",
                    },
                },
                ensure_ascii=False,
            )

        formatter._call_presto = fake_call  # type: ignore[method-assign]
        result = formatter.format_stage("", "answer")
        self.assertEqual(result.data["chat"]["ui_intent"]["mode"], "show_schema")
        self.assertEqual(result.data["chat"]["ui_intent"]["target"], "path_plan")

    def test_answer_can_return_update_patch_intent(self) -> None:
        formatter = ChatWorkflowFormatter(max_retries=1, stage_input={"question": "把第一周任务改成补作品集"})

        def fake_call(_prompt: str, _group: str) -> str:
            return json.dumps(
                {
                    "answer": "结论：将更新第一周任务。",
                    "conclusion": "准备更新路径任务。",
                    "actions": ["刷新路径规划查看结果"],
                    "evidence_refs": ["path_plan"],
                    "missing_evidence": [],
                    "confidence": 0.82,
                    "ui_intent": {
                        "mode": "update_result",
                        "target": "path_plan",
                        "patches": [
                            {"op": "replace", "path": "/path_plan/stages/0/weeks/0/task", "value": "补作品集首页和项目说明"}
                        ],
                        "schema": {},
                        "summary": "已准备更新路径规划第一周任务。",
                    },
                },
                ensure_ascii=False,
            )

        formatter._call_presto = fake_call  # type: ignore[method-assign]
        result = formatter.format_stage("", "answer")
        patch = result.data["chat"]["ui_intent"]["patches"][0]
        self.assertEqual(patch["path"], "/path_plan/stages/0/weeks/0/task")
        self.assertEqual(patch["value"], "补作品集首页和项目说明")

    def test_answer_can_return_job_recommendation_intent(self) -> None:
        formatter = ChatWorkflowFormatter(max_retries=1, stage_input={"question": "我不喜欢当前岗位，我想做渗透测试"})

        def fake_call(_prompt: str, _group: str) -> str:
            return json.dumps(
                {
                    "answer": "结论：会按渗透测试方向更新推荐。",
                    "conclusion": "按渗透测试方向更新。",
                    "actions": ["查看新的推荐岗位"],
                    "evidence_refs": ["matching", "top_jobs"],
                    "missing_evidence": [],
                    "confidence": 0.76,
                    "ui_intent": {
                        "mode": "update_result",
                        "target": "job_recommendations",
                        "patches": [
                            {
                                "op": "replace",
                                "path": "/top_jobs",
                                "value": [{"title": "渗透测试工程师", "match": 70, "category": "用户偏好方向"}],
                            }
                        ],
                        "schema": {},
                        "summary": "岗位推荐已准备更新。",
                    },
                },
                ensure_ascii=False,
            )

        formatter._call_presto = fake_call  # type: ignore[method-assign]
        result = formatter.format_stage("", "answer")
        self.assertEqual(result.data["chat"]["ui_intent"]["target"], "job_recommendations")

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

    def test_lightweight_chat_workflow_accepts_ui_intent(self) -> None:
        workflow = ChatWorkflow(
            lambda _context: json.dumps(
                {
                    "answer": "结论：已准备修改。",
                    "conclusion": "准备修改。",
                    "actions": ["查看界面"],
                    "evidence_refs": ["matching"],
                    "missing_evidence": [],
                    "confidence": 0.8,
                    "ui_intent": {
                        "mode": "update_result",
                        "target": "matching",
                        "patches": [{"op": "replace", "path": "/matching_result/match_level", "value": "需补强"}],
                        "schema": {},
                        "summary": "匹配结论已准备更新。",
                    },
                },
                ensure_ascii=False,
            ),
            max_retries=1,
        )
        result = workflow.run_with_meta({"question": "改一下匹配结论"})
        self.assertEqual(result.data["ui_intent"]["target"], "matching")

    def test_lightweight_chat_workflow_accepts_job_recommendation_intent(self) -> None:
        workflow = ChatWorkflow(
            lambda _context: json.dumps(
                {
                    "answer": "结论：会按产品方向更新推荐。",
                    "conclusion": "按产品方向更新。",
                    "actions": ["查看新的岗位队列"],
                    "evidence_refs": ["matching", "top_jobs"],
                    "missing_evidence": [],
                    "confidence": 0.72,
                    "ui_intent": {
                        "mode": "update_result",
                        "target": "job_recommendations",
                        "patches": [
                            {
                                "op": "replace",
                                "path": "/top_jobs",
                                "value": [
                                    {
                                        "title": "AI 产品经理",
                                        "match": 68,
                                        "category": "用户偏好方向",
                                        "fit_summary": "更贴近用户表达的产品方向。",
                                    }
                                ],
                            },
                            {"op": "replace", "path": "/matching_result/target_role", "value": "AI 产品经理"},
                        ],
                        "schema": {},
                        "summary": "岗位推荐已准备更新。",
                    },
                },
                ensure_ascii=False,
            ),
            max_retries=1,
        )
        result = workflow.run_with_meta({"question": "我不喜欢当前岗位，我想做 AI 产品"})
        self.assertEqual(result.data["ui_intent"]["target"], "job_recommendations")

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
