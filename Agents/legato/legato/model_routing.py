from __future__ import annotations

import json
import os
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class ModelRoute:
    route: str
    provider: str
    base_url: str
    model: str


def load_model_route(start: Path | None = None) -> ModelRoute | None:
    config_path = find_upward("model-routing.json", start or Path.cwd())
    if config_path is None:
        return None
    env_path = config_path.parent / ".env"
    env_values = read_dotenv(env_path) if env_path.exists() else {}
    config = json.loads(config_path.read_text(encoding="utf-8"))
    route_name = os.getenv("PRESTO_ROUTE") or config.get("default_route", "")
    route = config.get("routes", {}).get(route_name)
    if not route:
        return None
    return ModelRoute(
        route=route_name,
        provider=route.get("provider", ""),
        base_url=value_from_env_or_default(route.get("base_url_env", ""), route.get("base_url", ""), env_values),
        model=value_from_env_or_default(route.get("model_env", ""), route.get("model", ""), env_values),
    )


def value_from_env_or_default(env_name: str, default: str, env_values: dict[str, str]) -> str:
    if env_name:
        return os.getenv(env_name) or env_values.get(env_name, "") or default
    return default


def read_dotenv(path: Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for raw_line in path.read_text(encoding="utf-8").splitlines():
        line = raw_line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip().strip("\"'")
    return values


def find_upward(name: str, start: Path) -> Path | None:
    current = start.resolve()
    if current.is_file():
        current = current.parent
    while True:
        candidate = current / name
        if candidate.exists():
            return candidate
        if current.parent == current:
            return None
        current = current.parent

