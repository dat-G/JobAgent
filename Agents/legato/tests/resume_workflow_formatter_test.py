from __future__ import annotations

import unittest

from legato.resume_workflow_formatter import (
    ResumeWorkflowFormatter,
    build_local_experience,
    enrich_education,
    enrich_education_school_tags,
    local_item_benchmark,
    normalize_item_benchmark,
    repair_orphan_certification_scores,
    school_tags_for,
    score_contest_level,
    slice_certifications_awards_text,
    slice_experience_text,
)


class ResumeWorkflowFormatterTest(unittest.TestCase):
    def test_slice_certifications_awards_keeps_scattered_awards(self) -> None:
        text = "\n".join(
            [
                "陈曦 男",
                "教育背景",
                "东北农业大学 本科",
                "项目经历",
                "智能住院服项目，闯入全国六强总决赛。",
                "工作经历",
                "高新兴科技集团股份有限公司",
                "获奖情况",
                "2023年全国大学生数学建模竞赛 黑龙江赛区 一等奖",
                "资格证书",
                "全国大学英语六级考试 567分",
            ]
        )
        sliced = slice_certifications_awards_text(text)
        self.assertIn("全国六强总决赛", sliced)
        self.assertIn("数学建模竞赛", sliced)
        self.assertIn("六级考试", sliced)

    def test_slice_certifications_awards_returns_empty_when_no_candidate(self) -> None:
        self.assertEqual(slice_certifications_awards_text("陈曦\n东北农业大学\n前端开发"), "")

    def test_slice_experience_keeps_work_project_contest_and_campus_roles(self) -> None:
        text = "\n".join(
            [
                "基本信息",
                "陈曦 男",
                "工作经历",
                "高新兴科技集团股份有限公司 前端开发实习生",
                "科研与项目经历",
                "农业农村部东北智慧农业技术重点实验室 GNSS轨迹研究项目",
                "获奖情况",
                "2023年全国大学生数学建模竞赛 黑龙江赛区 一等奖",
                "在校经历",
                "吉他协会 副会长",
            ]
        )
        work_project = slice_experience_text(text, "experience_work_project")
        contest = slice_experience_text(text, "experience_contest")
        campus = slice_experience_text(text, "experience_campus")
        self.assertIn("前端开发实习生", work_project)
        self.assertIn("GNSS轨迹研究项目", work_project)
        self.assertIn("数学建模竞赛", contest)
        self.assertIn("副会长", campus)

    def test_repair_orphan_certification_scores_pairs_english_exam_scores(self) -> None:
        text = "\n".join(
            [
                "Python 精通",
                "567分",
                "598分",
                "全国大学英语六级考试",
                "全国大学英语四级考试",
            ]
        )
        repaired = repair_orphan_certification_scores(text)
        self.assertIn("全国大学英语六级考试 567分", repaired)
        self.assertIn("全国大学英语四级考试 598分", repaired)

    def test_validate_experience_requires_level_number_in_range(self) -> None:
        formatter = ResumeWorkflowFormatter()
        formatter._validate_group(
            "experience",
            {
                "experience": [
                    {
                        "type": "实习",
                        "role": "前端开发实习生",
                        "contribution": "在高新兴参与视频云平台监控前台系统开发。",
                        "level": 7,
                    }
                ]
            },
        )

        with self.assertRaises(ValueError):
            formatter._validate_group(
                "experience",
                {
                    "experience": [
                        {
                            "type": "比赛",
                            "role": "队长",
                            "contribution": "带队参加比赛。",
                            "level": "8",
                        }
                    ]
                },
            )

    def test_build_local_experience_keeps_only_described_experiences(self) -> None:
        experience = build_local_experience(
            (
                "2023-07 至 2023-08 高新兴科技集团股份有限公司 前端开发实习生\n"
                "队长\n"
                "吉他协会 副会长\n"
                "国家级实验室 GNSS\n"
                "独立带领团队参加由杜邦公司主办的杜邦青年创新大赛，"
                "制作产品设想与展示PPT，闯入全国六强总决赛。"
            ),
            [
                {"name": "全国大学英语六级考试", "result": "567分"},
                {"name": "2023年全国大学生数学建模竞赛", "result": "一等奖"},
                {"name": "杜邦青年创新大赛", "result": "全国六强总决赛"},
            ],
        )
        self.assertTrue(any(item["type"] == "实习" for item in experience))
        self.assertTrue(any(item["type"] == "科研项目" for item in experience))
        self.assertTrue(any(item["type"] == "社团" for item in experience))
        contests = [item for item in experience if item["type"] == "比赛"]
        self.assertEqual(len(contests), 1)
        self.assertEqual(contests[0]["role"], "杜邦青年创新大赛 / 队长")
        self.assertEqual(contests[0]["level"], 9)

    def test_build_local_experience_does_not_duplicate_undescribed_awards(self) -> None:
        experience = build_local_experience(
            "获奖情况\n2023年全国大学生数学建模竞赛 黑龙江赛区 一等奖",
            [{"name": "2023年全国大学生数学建模竞赛", "result": "黑龙江赛区一等奖"}],
        )
        self.assertEqual(experience, [])

    def test_build_local_experience_extracts_generic_internship_section(self) -> None:
        experience = build_local_experience(
            "\n".join(
                [
                    "实习经历",
                    "2025.5-2025.6 字节跳动 MCP 标注(实习生)",
                    "本项目主要是语言大模型 MCP 标注项目,主要工作为判断回答的答案是否正确,",
                    "需要修改和添加 XML 包裹的内容。",
                    "荣誉奖项",
                ]
            ),
            [],
        )
        self.assertEqual(len(experience), 1)
        self.assertEqual(experience[0]["type"], "实习")
        self.assertEqual(experience[0]["role"], "字节跳动 / MCP标注(实习生)")
        self.assertIn("MCP标注", experience[0]["contribution"])
        self.assertEqual(experience[0]["level"], 4)

    def test_build_local_experience_recalls_security_project_bullets(self) -> None:
        experience = build_local_experience(
            "\n".join(
                [
                    "科研经历",
                    "基于拓扑结构构建与语义修复机制的RPKI验证器测试 - 参与者 2025-10 ~ 2026-03",
                    "· 完成了基于CFG生成各种不同结构的RPKI证书仓库实验",
                    "项目经历",
                    "Shellcode免杀研究 2024-02 ~ 2024-05",
                    "自动化免杀平台设计与实现 2024-05 ~ 2024-07",
                    "· 对Tenda FH451路由器进行漏洞挖掘,发现多个栈溢出漏洞,可实现远程RCE,获得CVE-2025-45513",
                    "· 对libcoap进行模糊测试,发现条件竞争情况下的UAF漏洞,可实现远程DoS,已上报",
                ]
            ),
            [],
        )
        roles = {item["role"] for item in experience}
        self.assertIn("基于拓扑结构构建与语义修复机制的RPKI验证器测试 / 参与者", roles)
        self.assertIn("Tenda FH451路由器 / 漏洞挖掘", roles)
        self.assertIn("libcoap / 模糊测试", roles)
        self.assertFalse(any(item["role"] == "" and "Shellcode免杀" in item["contribution"] for item in experience))

    def test_contest_and_low_value_honor_scores_are_calibrated(self) -> None:
        self.assertEqual(score_contest_level("2024年第16届华中杯大学生数学建模挑战赛", "二等奖"), 7)
        self.assertEqual(score_contest_level("2024年第五届华数杯全国大学生数学建模竞赛", "三等奖"), 7)
        self.assertEqual(score_contest_level("校级软件开发项目", "负责核心模块开发"), 5)
        self.assertEqual(score_contest_level("优秀学生干部", ""), 2)
        self.assertEqual(score_contest_level("全国计算机二级WPS证书", ""), 2)

    def test_item_benchmark_normalizes_six_dimensional_scores(self) -> None:
        benchmark = normalize_item_benchmark(
            {"name": "2023年全国大学生信息安全竞赛华东南赛区", "result": "三等奖"},
            {"scores": [7, 2, 9, 3, 8, 6], "impact_factor": 7},
        )
        self.assertEqual(benchmark["dimensions"], ["逻辑", "语言", "专业", "领导", "抗压", "成长"])
        self.assertEqual(len(benchmark["scores"]), 6)
        self.assertAlmostEqual(sum(benchmark["scores"]), 1.0, places=3)
        self.assertEqual(max(range(6), key=lambda index: benchmark["scores"][index]), 2)
        self.assertEqual(benchmark["impact_factor"], 7.0)

    def test_item_benchmark_caps_low_value_certificate(self) -> None:
        benchmark = normalize_item_benchmark(
            {"name": "全国计算机二级WPS证书", "result": ""},
            {"scores": [0.5, 0.5, 0.5, 0.5, 0.5, 0.5], "impact_factor": 7},
        )
        self.assertLessEqual(benchmark["impact_factor"], 2.5)

    def test_local_item_benchmark_scores_major_related_security_item(self) -> None:
        benchmark = local_item_benchmark({"name": "2023年ISCC第20届信息安全与对抗技术竞赛", "result": "全国二等奖"})
        self.assertAlmostEqual(sum(benchmark["scores"]), 1.0, places=3)
        self.assertEqual(max(range(6), key=lambda index: benchmark["scores"][index]), 2)
        self.assertGreaterEqual(benchmark["impact_factor"], 6)

    def test_school_tags_match_ruanke_cache(self) -> None:
        tags = school_tags_for("东北农业大学 · 本科")
        self.assertEqual(tags["matched_school"], "东北农业大学")
        self.assertFalse(tags["is_985"])
        self.assertTrue(tags["is_211"])
        self.assertTrue(tags["is_double_first_class"])
        self.assertEqual(tags["ruanke_rank"], 120)

    def test_enrich_education_adds_school_tags(self) -> None:
        enriched = enrich_education_school_tags(
            [
                {
                    "school": "东北农业大学",
                    "degree": "本科",
                    "major": "计算机科学与技术",
                    "department": "电气与信息学院",
                }
            ]
        )
        self.assertEqual(enriched[0]["school_tags"]["matched_school"], "东北农业大学")
        self.assertEqual(enriched[0]["degree_level"], "本科")

    def test_enrich_education_keeps_multiple_records_and_tags_each_school(self) -> None:
        enriched = enrich_education(
            [
                {"school": "湖南科技学院", "degree": "本科", "major": "信息与计算科学", "department": ""},
                {"school": "清华大学", "degree": "硕士", "major": "计算机科学", "department": ""},
            ],
            "",
        )
        self.assertEqual(len(enriched), 2)
        self.assertEqual(enriched[0]["degree_level"], "本科")
        self.assertEqual(enriched[0]["school_tags"]["matched_school"], "湖南科技学院")
        self.assertEqual(enriched[1]["degree_level"], "硕士")
        self.assertEqual(enriched[1]["school_tags"]["matched_school"], "清华大学")

    def test_single_unknown_degree_infers_undergraduate_by_years_and_school(self) -> None:
        enriched = enrich_education(
            [{"school": "湖南科技学院", "degree": "", "major": "信息与计算科学", "department": ""}],
            "教育背景\n2022-09~ 2026-06 湖南科技学院 信息与计算科学",
        )
        self.assertEqual(enriched[0]["degree_level"], "本科")


if __name__ == "__main__":
    unittest.main()
