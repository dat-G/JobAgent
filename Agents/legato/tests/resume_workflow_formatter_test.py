from __future__ import annotations

import unittest

from legato.resume_workflow_formatter import (
    ResumeWorkflowFormatter,
    build_local_experience,
    calibrate_llm_experience_level,
    enrich_education,
    enrich_education_school_tags,
    local_major_baseline,
    local_item_benchmark,
    major_baseline_context,
    normalize_benchmark_input_items,
    normalize_evidence_scope,
    normalize_item_benchmark,
    normalize_major_baseline,
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
                        "evidence_scope": "校外",
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

    def test_build_local_experience_names_gnss_research_description(self) -> None:
        experience = build_local_experience(
            "\n".join(
                [
                    "于DBSCAN的GNSS点云分块处理方法,对于研究项目使用的数据集开发了一系列数据清洗模块。",
                    "独立领导了基于视觉与时序模型的农机GNSS轨迹的田路分割的研究项目,对学界前沿的非开源文章独立复现与分析。",
                ]
            ),
            [],
        )
        self.assertEqual(len(experience), 1)
        self.assertEqual(experience[0]["role"], "GNSS轨迹田路分割研究项目")
        self.assertEqual(experience[0]["contribution"], "数据清洗与研究方法实现")

    def test_build_local_experience_names_generic_research_description(self) -> None:
        experience = build_local_experience(
            "\n".join(
                [
                    "基于Transformer的中文命名实体识别模型，完成模型训练、误差分析和实验评估。",
                    "针对医疗文本数据集开发清洗模块，并复现与分析相关论文方法。",
                ]
            ),
            [],
        )
        self.assertEqual(len(experience), 1)
        self.assertEqual(experience[0]["role"], "中文命名实体识别模型研究项目")
        self.assertEqual(experience[0]["contribution"], "模型实现与实验评估")

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

    def test_item_benchmark_keeps_caller_assembled_item_identity(self) -> None:
        items = normalize_benchmark_input_items(
            [
                {
                    "kind": "experience",
                    "key": "experience:0",
                    "type": "科研项目",
                    "role": "GNSS轨迹田路分割研究项目",
                    "contribution": "数据清洗与研究方法实现",
                    "level": 6,
                }
            ]
        )
        benchmark = normalize_item_benchmark(items[0], {"scores": [4, 1, 7, 1, 4, 3], "impact_factor": 5.5})
        self.assertEqual(benchmark["item"]["kind"], "experience")
        self.assertEqual(benchmark["item"]["key"], "experience:0")
        self.assertEqual(benchmark["item"]["level"], 6.0)
        self.assertEqual(benchmark["item"]["evidence_scope"], "校内")
        self.assertIn("GNSS", benchmark["item"]["role"])

    def test_evidence_scope_inference_distinguishes_campus_and_external(self) -> None:
        self.assertEqual(normalize_evidence_scope("", {"name": "校级优秀学生干部", "result": "荣誉称号"}), "校内")
        self.assertEqual(normalize_evidence_scope("", {"name": "全国大学英语六级考试", "result": "567分"}), "校外")

    def test_item_benchmark_caps_low_value_certificate(self) -> None:
        benchmark = normalize_item_benchmark(
            {"name": "全国计算机二级WPS证书", "result": ""},
            {"scores": [0.5, 0.5, 0.5, 0.5, 0.5, 0.5], "impact_factor": 7},
        )
        self.assertLessEqual(benchmark["impact_factor"], 2.5)

    def test_item_benchmark_caps_campus_award_bucket(self) -> None:
        benchmark = normalize_item_benchmark(
            {"kind": "award", "name": "校级软件开发项目", "result": "一等奖", "evidence_scope": "校内"},
            {"scores": [0.2, 0.0, 0.5, 0.0, 0.2, 0.1], "impact_factor": 8},
        )
        self.assertEqual(benchmark["impact_factor"], 4.0)

    def test_untitled_professional_project_has_strict_impact_cap(self) -> None:
        benchmark = normalize_item_benchmark(
            {
                "kind": "experience",
                "key": "experience:0",
                "type": "科研项目",
                "role": "",
                "contribution": "完成网络安全平台开发与漏洞测试",
                "level": 8,
            },
            {"scores": [0.2, 0.0, 0.5, 0.0, 0.2, 0.1], "impact_factor": 8},
        )
        self.assertEqual(benchmark["impact_factor"], 4.0)

    def test_untitled_professional_project_has_strict_level_cap(self) -> None:
        item = {
            "type": "科研项目",
            "role": "",
            "contribution": "完成网络安全平台开发与漏洞测试",
            "level": 8,
        }
        self.assertEqual(calibrate_llm_experience_level(item), 4)

    def test_local_item_benchmark_scores_major_related_security_item(self) -> None:
        benchmark = local_item_benchmark({"name": "2023年ISCC第20届信息安全与对抗技术竞赛", "result": "全国二等奖"})
        self.assertAlmostEqual(sum(benchmark["scores"]), 1.0, places=3)
        self.assertEqual(max(range(6), key=lambda index: benchmark["scores"][index]), 2)
        self.assertGreaterEqual(benchmark["impact_factor"], 6)

    def test_item_benchmark_does_not_fallback_when_presto_fails(self) -> None:
        formatter = ResumeWorkflowFormatter(max_retries=2)

        def fail_call(*_args: object) -> str:
            raise RuntimeError("presto unavailable")

        formatter._call_presto = fail_call  # type: ignore[method-assign]
        with self.assertRaisesRegex(RuntimeError, "item_benchmark\\[0\\] failed"):
            formatter._run_item_benchmark_with_retry("", {"name": "全国大学生数学建模竞赛", "result": "一等奖"}, 0)

    def test_major_baseline_does_not_fallback_when_presto_fails(self) -> None:
        formatter = ResumeWorkflowFormatter(max_retries=2, stage_input={"basic_info": {"major": "计算机科学与技术"}})

        def fail_call(*_args: object) -> str:
            raise RuntimeError("presto unavailable")

        formatter._call_presto = fail_call  # type: ignore[method-assign]
        with self.assertRaisesRegex(RuntimeError, "major_baseline failed"):
            formatter._run_major_baseline_with_retry("东北农业大学 计算机科学与技术")

    def test_item_benchmark_prompt_includes_education_context(self) -> None:
        formatter = ResumeWorkflowFormatter()
        prompt = formatter._item_benchmark_prompt(
            "\n".join(
                [
                    "桂林电子科技大学 - 网络空间安全 - 硕士 2025-09 ~ 至今",
                    "杭州电子科技大学信息工程学院 - 计算机科学与技术 - 学士 2020-09 ~ 2024-06",
                    "2023年ISCC第20届信息安全与对抗技术竞赛 全国二等奖",
                ]
            ),
            {"name": "2023年ISCC第20届信息安全与对抗技术竞赛", "result": "全国二等奖"},
        )
        self.assertIn("Education context:", prompt)
        self.assertIn("桂林电子科技大学", prompt)
        self.assertIn("网络空间安全", prompt)
        self.assertIn("硕士", prompt)
        self.assertNotIn("{{education_context}}", prompt)

    def test_major_baseline_context_uses_profile_stage_input(self) -> None:
        context = major_baseline_context(
            "教育背景\n东北农业大学 计算机科学与技术 本科",
            {
                "basic_info": {"school": "东北农业大学", "major": "计算机科学与技术", "degree": "本科"},
                "education": [
                    {
                        "school": "东北农业大学",
                        "major": "计算机科学与技术",
                        "degree": "本科",
                        "is_211": True,
                        "is_double_first_class": True,
                        "ruanke_rank": 120,
                    }
                ],
                "transcript_use": "GPA: 3.7/4.0",
            },
        )
        self.assertEqual(context["major_family_hint"], "工科类")
        self.assertGreater(context["base_score_hint"], 60)
        self.assertLess(context["base_score_hint"], 70)
        self.assertEqual(context["school_quality_hint"]["school_bonus_hint"], 2)
        self.assertIn("东北农业大学", context["resume_education_context"])

    def test_local_major_baseline_scores_engineering_major(self) -> None:
        baseline = local_major_baseline({"major_name_hint": "计算机科学与技术", "major_family_hint": "工科类", "base_score_hint": 50})
        self.assertEqual(baseline["major_family"], "工科类")
        self.assertEqual(len(baseline["scores"]), 6)
        self.assertGreater(baseline["scores"][0], baseline["scores"][1])
        self.assertGreater(baseline["scores"][2], baseline["base_score"])
        self.assertLess(baseline["scores"][3], baseline["base_score"])
        self.assertEqual(baseline["scores"], [54, 46, 57, 42, 48, 52])

    def test_local_major_baseline_applies_marginal_school_tier_effect(self) -> None:
        baseline = local_major_baseline(
            {
                "major_name_hint": "计算机科学与技术",
                "major_family_hint": "工科类",
                "base_score_hint": 50,
                "school_quality_hint": {
                    "school_tier": "211/双一流/软科#120",
                    "school_bonus_hint": 2,
                    "specialty_alignment_hint": "未见明确特色专业匹配",
                    "specialty_bonus_hint": 0,
                },
            }
        )
        self.assertEqual(baseline["base_score"], 52)
        self.assertEqual(baseline["scores"], [58, 48, 61, 44, 51, 55])
        self.assertIn("边际调整", baseline["rationale"])

    def test_local_major_baseline_debuffs_non_double_first_class_school(self) -> None:
        context = major_baseline_context(
            "",
            {
                "education": [
                    {
                        "school": "桂林电子科技大学",
                        "major": "网络空间安全",
                        "degree": "硕士",
                        "school_tags": {
                            "is_985": False,
                            "is_211": False,
                            "is_double_first_class": False,
                            "ruanke_rank": 183,
                        },
                    }
                ],
            },
        )
        baseline = local_major_baseline(context)
        self.assertEqual(context["school_quality_hint"]["school_penalty_hint"], 2)
        self.assertEqual(context["school_quality_hint"]["school_bonus_hint"], 0)
        self.assertEqual(baseline["base_score"], 48)
        self.assertEqual(baseline["scores"], [53, 44, 57, 40, 46, 51])

    def test_major_baseline_keeps_independent_undergraduate_history(self) -> None:
        context = major_baseline_context(
            "",
            {
                "education": [
                    {
                        "school": "桂林电子科技大学",
                        "major": "网络空间安全",
                        "degree": "硕士",
                        "school_tags": {
                            "is_985": False,
                            "is_211": False,
                            "is_double_first_class": False,
                            "ruanke_rank": 183,
                        },
                    },
                    {
                        "school": "杭州电子科技大学信息工程学院",
                        "major": "计算机科学与技术",
                        "degree": "本科",
                        "school_tags": {
                            "is_985": False,
                            "is_211": False,
                            "is_double_first_class": False,
                            "ruanke_rank": 0,
                            "school_kind": "independent_college",
                            "parent_school": "杭州电子科技大学",
                        },
                    },
                ],
            },
        )
        baseline = local_major_baseline(context)
        self.assertIn("独立学院/原三本学历", context["school_quality_hint"]["school_tier"])
        self.assertEqual(context["school_quality_hint"]["school_penalty_hint"], 5)
        self.assertEqual(baseline["base_score"], 45)
        self.assertEqual(baseline["scores"], [50, 41, 54, 37, 43, 48])

    def test_local_major_baseline_applies_specialty_alignment_mildly(self) -> None:
        context = major_baseline_context(
            "杭州电子科技大学信息工程学院 - 计算机科学与技术 - 学士",
            {
                "education": [
                    {
                        "school": "杭州电子科技大学",
                        "major": "计算机科学与技术",
                        "degree": "本科",
                        "school_tags": {
                            "is_985": False,
                            "is_211": False,
                            "is_double_first_class": False,
                            "ruanke_rank": 91,
                        },
                    }
                ],
            },
        )
        baseline = local_major_baseline(context)
        self.assertEqual(context["school_quality_hint"]["specialty_bonus_hint"], 2)
        self.assertGreaterEqual(baseline["scores"][2], 61)
        self.assertGreater(baseline["scores"][0], 54)

    def test_local_major_baseline_lowers_independent_college_prior(self) -> None:
        context = major_baseline_context(
            "杭州电子科技大学信息工程学院 - 计算机科学与技术 - 学士",
            {
                "education": [
                    {
                        "school": "杭州电子科技大学信息工程学院",
                        "major": "计算机科学与技术",
                        "degree": "本科",
                    }
                ],
            },
        )
        baseline = local_major_baseline(context)
        self.assertEqual(context["school_quality_hint"]["school_tier"], "独立学院/原三本")
        self.assertEqual(context["school_quality_hint"]["school_bonus_hint"], 0)
        self.assertEqual(context["school_quality_hint"]["school_penalty_hint"], 10)
        self.assertEqual(baseline["base_score"], 40)
        self.assertEqual(baseline["scores"], [45, 36, 48, 32, 38, 42])

    def test_normalize_major_baseline_accepts_dimension_object_scores(self) -> None:
        baseline = normalize_major_baseline(
            {
                "major_baseline": {
                    "major_name": "工商管理",
                    "major_family": "商科类",
                    "base_score": 80,
                    "scores": {"逻辑": 82, "语言": 86, "专业": 87, "领导": 83, "抗压": 80, "成长": 82},
                    "confidence": 0.8,
                }
            },
            {"major_name_hint": "工商管理", "major_family_hint": "商科类", "base_score_hint": 80},
        )
        self.assertEqual(baseline["dimensions"], ["逻辑", "语言", "专业", "领导", "抗压", "成长"])
        self.assertEqual(baseline["major_family"], "商科类")
        self.assertEqual(baseline["scores"], [82, 85, 85, 83, 80, 82])
        self.assertEqual(baseline["source"], "presto_major_baseline")

    def test_major_baseline_prompt_includes_context_slot(self) -> None:
        formatter = ResumeWorkflowFormatter()
        context = major_baseline_context(
            "杭州电子科技大学信息工程学院 - 计算机科学与技术 - 学士",
            {
                "basic_info": {"major": "计算机科学与技术"},
                "education": [{"school": "杭州电子科技大学信息工程学院", "major": "计算机科学与技术", "degree": "本科"}],
            },
        )
        prompt = formatter._major_baseline_prompt(context)
        self.assertIn("Stage: `major_baseline`", prompt)
        self.assertIn("计算机科学与技术", prompt)
        self.assertNotIn("{{context}}", prompt)

    def test_experience_refine_prompt_includes_generic_research_guidance(self) -> None:
        formatter = ResumeWorkflowFormatter()
        prompt = formatter._experience_refine_prompt(
            "基于Transformer的中文命名实体识别模型，完成模型训练和实验评估。",
            [{"type": "科研项目", "role": "中文命名实体识别模型研究项目", "contribution": "模型实现与实验评估", "level": 6}],
        )
        self.assertIn("Treat descriptive research text as possible 科研项目", prompt)
        self.assertIn("基于 X 的 Y 方法/模型/算法/系统/框架/模块", prompt)
        self.assertIn("Do not hard-code one domain", prompt)
        self.assertIn("中文命名实体识别模型研究项目", prompt)
        self.assertNotIn("{{local_candidates}}", prompt)
        self.assertNotIn("{{resume_text}}", prompt)

    def test_school_tags_match_ruanke_cache(self) -> None:
        tags = school_tags_for("东北农业大学 · 本科")
        self.assertEqual(tags["matched_school"], "东北农业大学")
        self.assertFalse(tags["is_985"])
        self.assertTrue(tags["is_211"])
        self.assertTrue(tags["is_double_first_class"])
        self.assertEqual(tags["ruanke_rank"], 120)

    def test_school_tags_do_not_inherit_parent_for_independent_college(self) -> None:
        tags = school_tags_for("杭州电子科技大学信息工程学院")
        self.assertEqual(tags["matched_school"], "杭州电子科技大学信息工程学院")
        self.assertFalse(tags["is_985"])
        self.assertFalse(tags["is_211"])
        self.assertFalse(tags["is_double_first_class"])
        self.assertIsNone(tags["ruanke_rank"])
        self.assertEqual(tags["school_kind"], "independent_college")
        self.assertEqual(tags["parent_school"], "杭州电子科技大学")

    def test_school_tags_still_match_parent_for_internal_department(self) -> None:
        tags = school_tags_for("东北农业大学电气与信息学院")
        self.assertEqual(tags["matched_school"], "东北农业大学")
        self.assertTrue(tags["is_211"])
        self.assertTrue(tags["is_double_first_class"])

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
