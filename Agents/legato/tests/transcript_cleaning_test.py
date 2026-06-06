from __future__ import annotations

import unittest

from legato.transcript_cleaning import (
    CourseGradePair,
    clean_course_grade_pairs,
    clean_course_grade_text,
    parse_course_grade_rows,
)


class TranscriptCleaningTest(unittest.TestCase):
    def test_parse_two_course_groups_from_ocr_row(self) -> None:
        text = (
            "计算机类专业导论 | 1.0 | 优秀 | 16 | 必修 | 202112 | "
            "计算方法实验 | 0.5 | 优秀 | 16 | 必修 | 202306 |\n"
        )
        pairs = parse_course_grade_rows(text)
        self.assertEqual(
            pairs,
            [
                CourseGradePair(course="计算机类专业导论", grade="优秀"),
                CourseGradePair(course="计算方法实验", grade="优秀"),
            ],
        )

    def test_clean_discards_non_compliant_entries(self) -> None:
        result = clean_course_grade_pairs(
            [
                CourseGradePair(course="计算机网络", grade="优秀"),
                CourseGradePair(course="计算机类专业导论", grade="优秀"),
                CourseGradePair(course="数据结构与算法课程设计", grade="110"),
                CourseGradePair(course="以下空白", grade="优秀"),
                CourseGradePair(course="20212", grade="86"),
                CourseGradePair(course="课程名", grade="成绩"),
            ]
        )
        self.assertEqual(
            result.pairs,
            [
                CourseGradePair(course="计算机网络", grade="优秀"),
                CourseGradePair(course="计算机类专业导论", grade="优秀"),
            ],
        )
        self.assertEqual(len(result.rejected), 4)
        self.assertEqual(result.rejected[0].reason, "invalid_grade")

    def test_clean_accepts_json_array_output(self) -> None:
        text = '[{"course":" Java程序设计 ","grade":"优秀"},{"course":"异常课程","grade":"A+"}]'
        result = clean_course_grade_text(text)
        self.assertEqual(result.pairs, [CourseGradePair(course="Java程序设计", grade="优秀")])
        self.assertEqual(result.rejected[0].reason, "invalid_grade")


if __name__ == "__main__":
    unittest.main()
