from __future__ import annotations

import unittest
from pathlib import Path

from legato.pdf_text_frontend import is_pdf_path


class PdfTextFrontendTest(unittest.TestCase):
    def test_is_pdf_path_is_case_insensitive(self) -> None:
        self.assertTrue(is_pdf_path(Path("resume.PDF")))
        self.assertFalse(is_pdf_path(Path("resume.md")))


if __name__ == "__main__":
    unittest.main()
