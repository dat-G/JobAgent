from __future__ import annotations

import unittest

from legato.cleaning import clean_markdown
from legato.formatter import parse_resume_markdown


class CleaningTest(unittest.TestCase):
    def test_clean_markdown_normalizes_chinese_resume_artifacts(self) -> None:
        raw = "陈曦 男 ⾃我评价\n⼿机 18702033459\n邮箱 mcfun\x00\x00\x00@\x00\x00\x00.com\n⼯作经历\n"
        cleaned = clean_markdown(raw)
        self.assertIn("陈曦 男 自我评价", cleaned.markdown)
        self.assertIn("Phone: 18702033459", cleaned.markdown)
        self.assertIn("Email: mcfun@example.com", cleaned.markdown)
        self.assertIn("## 工作经历", cleaned.markdown)
        self.assertGreaterEqual(cleaned.stats["nul_chars"], 1)

    def test_local_resume_parser_uses_cleaned_name_and_phone(self) -> None:
        raw = "陈曦 男 ⾃我评价\n⼿机 18702033459\n邮箱 mcfun\x00@\x00.com\n"
        cleaned = clean_markdown(raw)
        parsed = parse_resume_markdown(cleaned.markdown)
        self.assertEqual(parsed["candidate"]["name"], "陈曦")
        self.assertEqual(parsed["contacts"]["phone"], "18702033459")
        self.assertEqual(parsed["contacts"]["email"], "mcfun@example.com")

    def test_clean_markdown_repairs_common_chinese_technical_word_wraps(self) -> None:
        raw = "基于视觉与时序模\r\n型的农机GNSS轨迹研究\n开发数据清洗方\n法与自动化系\n统\n"
        cleaned = clean_markdown(raw)
        self.assertIn("时序模型的农机GNSS轨迹研究", cleaned.markdown)
        self.assertIn("数据清洗方法与自动化系统", cleaned.markdown)
        self.assertNotIn("时序模\n型", cleaned.markdown)


if __name__ == "__main__":
    unittest.main()
