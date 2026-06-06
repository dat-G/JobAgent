from __future__ import annotations

import argparse
import json
import sys
import time
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from legato.transcript_cleaning import clean_course_grade_text


def benchmark(text: str, iterations: int, workers: int) -> dict[str, object]:
    started = time.perf_counter()
    if workers <= 1:
        results = [clean_course_grade_text(text) for _ in range(iterations)]
    else:
        with ThreadPoolExecutor(max_workers=workers) as pool:
            results = list(pool.map(clean_course_grade_text, [text] * iterations))
    elapsed_ms = (time.perf_counter() - started) * 1000
    first = results[0]
    return {
        "iterations": iterations,
        "workers": workers,
        "elapsed_ms": round(elapsed_ms, 3),
        "per_doc_ms": round(elapsed_ms / iterations, 4),
        "kept": len(first.pairs),
        "rejected": len(first.rejected),
    }


def main() -> int:
    parser = argparse.ArgumentParser(description="Benchmark transcript course-grade cleaning.")
    parser.add_argument("source", type=Path)
    parser.add_argument("--iterations", type=int, default=1000)
    parser.add_argument("--workers", type=int, nargs="+", default=[1, 2, 4, 8])
    args = parser.parse_args()

    text = args.source.read_text(encoding="utf-8")
    payload = [benchmark(text, args.iterations, workers) for workers in args.workers]
    print(json.dumps(payload, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
