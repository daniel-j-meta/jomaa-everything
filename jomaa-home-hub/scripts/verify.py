#!/usr/bin/env python3
"""Run every check the hub must pass. Exits non-zero if any fail.

    verify.py           everything: format, modules, vet, lint, builds, tests, vulnerabilities
    verify.py --fast    format, vet, lint and build only (used by the pre-commit hook)

Linters are pinned in tools/go.mod and built with the local Go, so every
machine runs the same versions.
"""

from __future__ import annotations

import argparse
import os
import subprocess
import sys
import time
from pathlib import Path

HUB_DIR = Path(__file__).resolve().parent.parent
BIN = HUB_DIR / "bin"
TOOLS = {
    "golangci-lint": "github.com/golangci/golangci-lint/v2/cmd/golangci-lint",
    "govulncheck": "golang.org/x/vuln/cmd/govulncheck",
}
# The hub ships as a static binary for x86 machines and for a Pi.
TARGETS = [("linux", "amd64"), ("linux", "arm64"), ("darwin", "arm64")]


def run(cmd: list[str], env: dict[str, str] | None = None) -> subprocess.CompletedProcess:
    return subprocess.run(cmd, cwd=HUB_DIR, env={**os.environ, **(env or {})}, capture_output=True, text=True)


def check_gofmt() -> str | None:
    out = run(["gofmt", "-l", "cmd", "internal"])
    if out.returncode != 0:
        return out.stderr
    return f"not gofmt-formatted:\n{out.stdout}" if out.stdout.strip() else None


def check_cmd(*cmd: str, env: dict[str, str] | None = None):
    def check() -> str | None:
        out = run(list(cmd), env)
        return None if out.returncode == 0 else (out.stdout + out.stderr).strip()

    return check


def build_tools() -> str | None:
    BIN.mkdir(exist_ok=True)
    out = run(["go", "build", "-C", "tools", "-o", str(BIN) + "/", *TOOLS.values()])
    return None if out.returncode == 0 else out.stderr


def check_builds() -> str | None:
    for goos, goarch in TARGETS:
        out = run(
            ["go", "build", "-trimpath", "-o", os.devnull, "./cmd/hub"],
            {"CGO_ENABLED": "0", "GOOS": goos, "GOARCH": goarch},
        )
        if out.returncode != 0:
            return f"{goos}/{goarch}:\n{out.stderr}"
    return None


def checks(fast: bool) -> list[tuple[str, object]]:
    steps = [
        ("gofmt", check_gofmt),
        ("go vet", check_cmd("go", "vet", "./...")),
        ("build linters", build_tools),
        ("golangci-lint", check_cmd(str(BIN / "golangci-lint"), "run", "./...")),
        ("build " + ", ".join(f"{o}/{a}" for o, a in TARGETS), check_builds),
    ]
    if fast:
        return steps
    return [
        ("go.mod is tidy", check_cmd("go", "mod", "tidy", "-diff")),
        ("modules match go.sum", check_cmd("go", "mod", "verify")),
        *steps,
        ("go test -race", check_cmd("go", "test", "-race", "-count=1", "-shuffle=on", "./...")),
        ("govulncheck", check_cmd(str(BIN / "govulncheck"), "./...")),
        ("python tests", check_cmd(sys.executable, "-m", "unittest", "discover", "-s", "scripts", "-q")),
    ]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--fast", action="store_true", help="skip tests and the vulnerability scan")
    args = parser.parse_args(argv)

    failed = []
    for name, check in checks(args.fast):
        start = time.monotonic()
        problem = check()
        took = time.monotonic() - start
        if problem:
            failed.append(name)
            print(f"FAIL  {name} ({took:.1f}s)\n{problem}\n", flush=True)
        else:
            print(f"ok    {name} ({took:.1f}s)", flush=True)
    if failed:
        print(f"\n{len(failed)} check(s) failed: {', '.join(failed)}")
        return 1
    print("\nall checks passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
