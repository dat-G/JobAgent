#!/usr/bin/env python3
"""Install Legato development dependencies across Windows, macOS, and Linux."""

from __future__ import annotations

import argparse
import os
import platform
import shlex
import subprocess
import sys
from pathlib import Path


ROOT_DIR = Path(__file__).resolve().parents[1]
WHEELHOUSE_DIR = ROOT_DIR / "vendor" / "wheelhouse"
MARKITDOWN_REQUIREMENTS = ROOT_DIR / "requirements.markitdown.lock"
BUILD_REQUIREMENTS = ROOT_DIR / "requirements.build.lock"
PADDLEOCR_REQUIREMENTS = ROOT_DIR / "requirements.paddleocr.optional.txt"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description=(
            "Install Legato dependencies. By default this downloads packages from "
            "the configured Python package index. Use --offline to install from "
            "vendor/wheelhouse instead."
        )
    )
    parser.add_argument(
        "--with-paddleocr",
        action="store_true",
        help="also install the optional PaddleOCR stack",
    )
    parser.add_argument(
        "--paddleocr-only",
        action="store_true",
        help="install only the optional PaddleOCR stack",
    )
    parser.add_argument(
        "--offline",
        action="store_true",
        help="install from vendor/wheelhouse without contacting package indexes",
    )
    parser.add_argument(
        "--no-editable",
        action="store_true",
        help="skip installing the Legato package in editable mode",
    )
    parser.add_argument(
        "--upgrade",
        action="store_true",
        help="pass --upgrade to pip install",
    )
    parser.add_argument(
        "--index-url",
        help="custom package index URL, passed through to pip",
    )
    parser.add_argument(
        "--extra-index-url",
        action="append",
        default=[],
        help="extra package index URL, can be provided more than once",
    )
    parser.add_argument(
        "--trusted-host",
        action="append",
        default=[],
        help="trusted host for custom package indexes, can be provided more than once",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="print pip commands without executing them",
    )
    return parser.parse_args()


def check_runtime(args: argparse.Namespace) -> None:
    if sys.version_info < (3, 11):
        raise SystemExit(
            "Legato requires Python 3.11 or newer. "
            f"Current interpreter: {sys.version.split()[0]}"
        )

    print(
        "Using "
        f"Python {sys.version.split()[0]} on {platform.system()} {platform.machine()}"
    )
    print(f"Project root: {ROOT_DIR}")

    if args.offline:
        if not WHEELHOUSE_DIR.exists():
            raise SystemExit(f"Offline wheelhouse not found: {WHEELHOUSE_DIR}")
        if not any(WHEELHOUSE_DIR.iterdir()):
            raise SystemExit(f"Offline wheelhouse is empty: {WHEELHOUSE_DIR}")
        print(
            "Offline mode enabled. The bundled wheelhouse must match this OS, "
            "CPU architecture, and Python version."
        )


def pip_base_command(args: argparse.Namespace) -> list[str]:
    command = [sys.executable, "-m", "pip", "install"]
    if args.upgrade:
        command.append("--upgrade")

    if args.offline:
        command.extend(["--no-index", "--find-links", str(WHEELHOUSE_DIR)])
    else:
        command.append("--prefer-binary")
        if args.index_url:
            command.extend(["--index-url", args.index_url])
        for extra_index_url in args.extra_index_url:
            command.extend(["--extra-index-url", extra_index_url])
        for trusted_host in args.trusted_host:
            command.extend(["--trusted-host", trusted_host])

    return command


def run(command: list[str], *, dry_run: bool) -> None:
    printable = subprocess.list2cmdline(command) if os.name == "nt" else shlex.join(command)
    print(f"+ {printable}")
    if dry_run:
        return
    subprocess.run(command, cwd=ROOT_DIR, check=True)


def install_requirements(path: Path, args: argparse.Namespace) -> None:
    if not path.exists():
        raise SystemExit(f"Requirements file not found: {path}")
    run([*pip_base_command(args), "-r", str(path)], dry_run=args.dry_run)


def install_editable(args: argparse.Namespace) -> None:
    command = [*pip_base_command(args), "--no-build-isolation", "-e", str(ROOT_DIR)]
    run(command, dry_run=args.dry_run)


def main() -> int:
    args = parse_args()
    check_runtime(args)

    if not args.paddleocr_only:
        install_requirements(MARKITDOWN_REQUIREMENTS, args)
        install_requirements(BUILD_REQUIREMENTS, args)
        if not args.no_editable:
            install_editable(args)

    if args.with_paddleocr or args.paddleocr_only:
        install_requirements(PADDLEOCR_REQUIREMENTS, args)

    print("Legato dependency installation finished.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
