#!/usr/bin/env python3
"""测试两个 OpenAI 兼容聊天补全 API 的速度。"""

from __future__ import annotations

import argparse
import concurrent.futures
import json
import os
import statistics
import sys
import time
import urllib.error
import urllib.request
from dataclasses import dataclass
from typing import Any


DEFAULT_SYSTEM_PROMPT = "你是一个有用的助手"
DEFAULT_USER_PROMPT = "你好，请介绍一下你自己"


@dataclass(frozen=True)
class ApiConfig:
    name: str
    url: str
    api_key_env: str
    model: str


@dataclass
class RunResult:
    api: str
    ok: bool
    status: int | None
    error: str | None
    latency_s: float
    ttft_s: float | None
    output_chars: int
    prompt_tokens: int | None
    completion_tokens: int | None
    total_tokens: int | None


def load_dotenv(path: str) -> None:
    if not os.path.exists(path):
        return
    with open(path, "r", encoding="utf-8") as file:
        for line in file:
            stripped = line.strip()
            if not stripped or stripped.startswith("#") or "=" not in stripped:
                continue
            key, value = stripped.split("=", 1)
            key = key.strip()
            value = value.strip().strip('"').strip("'")
            os.environ.setdefault(key, value)


def normalize_chat_url(endpoint: str) -> str:
    endpoint = endpoint.rstrip("/")
    if endpoint.endswith("/chat/completions"):
        return endpoint
    if endpoint.endswith("/v1"):
        return f"{endpoint}/chat/completions"
    return f"{endpoint}/chat/completions"


def percentile(values: list[float], pct: float) -> float:
    if not values:
        return 0.0
    if len(values) == 1:
        return values[0]
    ordered = sorted(values)
    rank = (len(ordered) - 1) * pct
    lower = int(rank)
    upper = min(lower + 1, len(ordered) - 1)
    weight = rank - lower
    return ordered[lower] * (1 - weight) + ordered[upper] * weight


def build_payload(args: argparse.Namespace, config: ApiConfig) -> dict[str, Any]:
    payload: dict[str, Any] = {
        "model": config.model,
        "messages": [
            {"role": "system", "content": args.system_prompt},
            {"role": "user", "content": args.user_prompt},
        ],
        "temperature": args.temperature,
        "max_tokens": args.max_tokens,
    }
    if args.stream:
        payload["stream"] = True
    return payload


def request_once(config: ApiConfig, args: argparse.Namespace) -> RunResult:
    api_key = os.environ.get(config.api_key_env)
    if not api_key:
        return RunResult(
            api=config.name,
            ok=False,
            status=None,
            error=f"缺少环境变量 {config.api_key_env}",
            latency_s=0.0,
            ttft_s=None,
            output_chars=0,
            prompt_tokens=None,
            completion_tokens=None,
            total_tokens=None,
        )

    payload = build_payload(args, config)
    request = urllib.request.Request(
        config.url,
        data=json.dumps(payload, ensure_ascii=False).encode("utf-8"),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
        },
        method="POST",
    )

    started = time.perf_counter()
    ttft_s: float | None = None
    output_chars = 0
    prompt_tokens = completion_tokens = total_tokens = None

    try:
        with urllib.request.urlopen(request, timeout=args.timeout) as response:
            status = response.status
            if args.stream:
                while True:
                    line = response.readline()
                    if not line:
                        break
                    text = line.decode("utf-8", errors="replace").strip()
                    if not text.startswith("data:"):
                        continue
                    data = text.removeprefix("data:").strip()
                    if data == "[DONE]":
                        break
                    try:
                        chunk = json.loads(data)
                    except json.JSONDecodeError:
                        continue
                    usage = chunk.get("usage") or {}
                    prompt_tokens = usage.get("prompt_tokens", prompt_tokens)
                    completion_tokens = usage.get("completion_tokens", completion_tokens)
                    total_tokens = usage.get("total_tokens", total_tokens)
                    delta = (chunk.get("choices") or [{}])[0].get("delta") or {}
                    content = delta.get("content") or ""
                    if content and ttft_s is None:
                        ttft_s = time.perf_counter() - started
                    output_chars += len(content)
            else:
                body = response.read().decode("utf-8")
                parsed = json.loads(body)
                usage = parsed.get("usage") or {}
                prompt_tokens = usage.get("prompt_tokens")
                completion_tokens = usage.get("completion_tokens")
                total_tokens = usage.get("total_tokens")
                message = (parsed.get("choices") or [{}])[0].get("message") or {}
                output_chars = len(message.get("content") or "")
                ttft_s = None
            latency_s = time.perf_counter() - started
            return RunResult(
                api=config.name,
                ok=True,
                status=status,
                error=None,
                latency_s=latency_s,
                ttft_s=ttft_s,
                output_chars=output_chars,
                prompt_tokens=prompt_tokens,
                completion_tokens=completion_tokens,
                total_tokens=total_tokens,
            )
    except urllib.error.HTTPError as exc:
        latency_s = time.perf_counter() - started
        detail = exc.read().decode("utf-8", errors="replace")[:500]
        return RunResult(config.name, False, exc.code, detail, latency_s, ttft_s, output_chars, None, None, None)
    except Exception as exc:  # noqa: BLE001 - CLI should report any request failure cleanly.
        latency_s = time.perf_counter() - started
        return RunResult(config.name, False, None, repr(exc), latency_s, ttft_s, output_chars, None, None, None)


def summarize(api: str, results: list[RunResult]) -> dict[str, Any]:
    successful = [result for result in results if result.ok]
    failed = [result for result in results if not result.ok]
    latencies = [result.latency_s for result in successful]
    ttfts = [result.ttft_s for result in successful if result.ttft_s is not None]
    chars_per_second = [
        result.output_chars / result.latency_s
        for result in successful
        if result.latency_s > 0 and result.output_chars > 0
    ]
    completion_tps = [
        result.completion_tokens / result.latency_s
        for result in successful
        if result.latency_s > 0 and result.completion_tokens
    ]

    return {
        "api": api,
        "runs": len(results),
        "ok": len(successful),
        "failed": len(failed),
        "latency_avg_s": statistics.mean(latencies) if latencies else None,
        "latency_p50_s": percentile(latencies, 0.50) if latencies else None,
        "latency_p95_s": percentile(latencies, 0.95) if latencies else None,
        "ttft_avg_s": statistics.mean(ttfts) if ttfts else None,
        "ttft_p50_s": percentile(ttfts, 0.50) if ttfts else None,
        "chars_per_sec_avg": statistics.mean(chars_per_second) if chars_per_second else None,
        "completion_tokens_per_sec_avg": statistics.mean(completion_tps) if completion_tps else None,
        "first_error": failed[0].error if failed else None,
    }


def print_table(rows: list[dict[str, Any]]) -> None:
    headers = [
        "接口",
        "成功/总数",
        "失败",
        "平均耗时(s)",
        "P50耗时(s)",
        "P95耗时(s)",
        "首字耗时(s)",
        "字符/秒",
        "Token/秒",
    ]
    print("\t".join(headers))
    for row in rows:
        values = [
            row["api"],
            f"{row['ok']}/{row['runs']}",
            str(row["failed"]),
            format_number(row["latency_avg_s"]),
            format_number(row["latency_p50_s"]),
            format_number(row["latency_p95_s"]),
            format_number(row["ttft_avg_s"]),
            format_number(row["chars_per_sec_avg"], digits=1),
            format_number(row["completion_tokens_per_sec_avg"], digits=1),
        ]
        print("\t".join(values))
        if row["first_error"]:
            print(f"  首个错误: {row['first_error']}")


def format_number(value: float | None, digits: int = 3) -> str:
    if value is None:
        return "-"
    return f"{value:.{digits}f}"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="测试 DeepSeek 和 SiliconFlow 聊天补全 API 的速度。")
    parser.add_argument("--runs", type=int, default=3, help="每个接口请求次数")
    parser.add_argument("--concurrency", type=int, default=1, help="每个接口并发请求数")
    parser.add_argument("--timeout", type=float, default=60.0, help="单次请求超时时间，单位秒")
    parser.add_argument("--max-tokens", type=int, default=512, help="最大生成 token 数")
    parser.add_argument("--temperature", type=float, default=0.2, help="采样温度")
    parser.add_argument("--system-prompt", default=DEFAULT_SYSTEM_PROMPT, help="系统提示词")
    parser.add_argument("--user-prompt", default=DEFAULT_USER_PROMPT, help="用户提示词")
    parser.add_argument("--dotenv", default=".env", help="可选的环境变量文件路径")
    parser.add_argument("--json", action="store_true", help="输出 JSON，而不是表格")
    parser.add_argument("--no-stream", dest="stream", action="store_false", help="关闭流式请求")
    parser.set_defaults(stream=True)
    return parser.parse_args()


def main() -> int:
    args = parse_args()
    load_dotenv(args.dotenv)

    configs = [
        ApiConfig(
            name="deepseek",
            url=normalize_chat_url(os.environ.get("DEEPSEEK_ENDPOINT", "https://api.deepseek.com")),
            api_key_env="DEEPSEEK_API_KEY",
            model=os.environ.get("DEEPSEEK_MODEL", "deepseek-v4-flash"),
        ),
        ApiConfig(
            name="siliconflow",
            url=os.environ.get("SILICONFLOW_ENDPOINT", "https://api.siliconflow.cn/v1/chat/completions"),
            api_key_env="SILICONFLOW_API_KEY",
            model=os.environ.get("SILICONFLOW_MODEL", "deepseek-ai/DeepSeek-V4-Flash"),
        ),
    ]

    all_results: dict[str, list[RunResult]] = {config.name: [] for config in configs}
    for config in configs:
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency) as executor:
            futures = [executor.submit(request_once, config, args) for _ in range(args.runs)]
            for future in concurrent.futures.as_completed(futures):
                all_results[config.name].append(future.result())

    summaries = [summarize(api, results) for api, results in all_results.items()]
    if args.json:
        print(json.dumps({"summary": summaries, "results": [r.__dict__ for rs in all_results.values() for r in rs]}, ensure_ascii=False, indent=2))
    else:
        print_table(summaries)
    return 0 if all(row["failed"] == 0 for row in summaries) else 1


if __name__ == "__main__":
    sys.exit(main())
