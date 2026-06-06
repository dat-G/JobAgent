from __future__ import annotations

import unittest
from pathlib import Path
from unittest.mock import patch

from legato.ocr_paddle import OcrLine, OcrResult, materialize_ocr_inputs, run_paddle_ocr_on_image


class PaddleOcrOptionalTest(unittest.TestCase):
    def test_materialize_non_pdf_input_returns_original_path(self) -> None:
        path = Path("image.png")
        self.assertEqual(materialize_ocr_inputs(path, max_pages=1, render_scale=1.0), [path])

    def test_pipeline_uses_paddle_backend_only_when_text_is_empty(self) -> None:
        from legato.pipeline import process
        from legato.markitdown_frontend import MarkdownDocument

        class EmptyFrontend:
            def convert(self, source):
                return MarkdownDocument(markdown="", title=None, source_path=str(source))

        with (
            patch("legato.pipeline.extract_pdf_text_layer", side_effect=RuntimeError("no text")),
            patch("legato.pipeline.MarkItDownFrontend", EmptyFrontend),
            patch("legato.pipeline.ocr_with_paddle", return_value=OcrResult(text="Student: Ada\nGPA: 4.0", lines=[OcrLine("Student: Ada")])),
        ):
            result = process(
                "sample.pdf",
                "transcript",
                use_presto=False,
                ocr_backend="paddle",
            )

        self.assertEqual(result.frontend, "paddleocr")
        self.assertEqual(result.data["student"]["name"], "Ada")

    def test_paddle_result_object_json_method_is_parsed(self) -> None:
        class ResultObject:
            def json(self):
                return {"rec_texts": ["Course A", "Course B"], "rec_scores": [0.98, 0.87]}

        class PaddleStub:
            def predict(self, image_path):
                self.image_path = image_path
                return [ResultObject()]

        lines = run_paddle_ocr_on_image(PaddleStub(), Path("page.png"))

        self.assertEqual([line.text for line in lines], ["Course A", "Course B"])
        self.assertEqual(lines[0].score, 0.98)


if __name__ == "__main__":
    unittest.main()
