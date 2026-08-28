#!/usr/bin/env python3
"""Repeatable repository administration helpers used by Makefile targets."""

from __future__ import annotations

import re
import shutil
import subprocess
import sys
import hashlib
import json
from pathlib import Path


ROOT = Path(__file__).resolve().parent.parent
IMAGEGEN_SKILL = Path("/home/kyz/.codex/skills/.system/imagegen/SKILL.md")
GENERATED_ICON = Path(
    "/home/kyz/.codex/generated_images/019f7fa6-79b7-7ee0-8660-5d23ddcf61e5/"
    "exec-eaa632b6-c063-4405-8995-c80d9c916f95.png"
)
TOKEN_PATTERNS = (
    re.compile(rb"[A-Za-z0-9_-]{20,30}\.[A-Za-z0-9_-]{5,10}\.[A-Za-z0-9_-]{20,80}"),
    re.compile(rb"mfa\.[A-Za-z0-9_-]{40,120}"),
)


def git(*args: str) -> str:
    result = subprocess.run(
        ["git", *args],
        cwd=ROOT,
        check=False,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        text=True,
    )
    return result.stdout.rstrip()


def run(command: list[str], *, capture: bool = False) -> str:
    result = subprocess.run(
        command,
        cwd=ROOT,
        check=False,
        stdout=subprocess.PIPE if capture else None,
        stderr=subprocess.STDOUT if capture else None,
        text=True,
    )
    output = result.stdout.rstrip() if capture and result.stdout else ""
    if result.returncode != 0:
        if output:
            print(output)
        raise SystemExit(result.returncode)
    return output


def token_locations() -> list[str]:
    found: list[str] = []
    for path in sorted(ROOT.rglob("*")):
        if not path.is_file() or ".git" in path.parts:
            continue
        try:
            if path.stat().st_size > 2_000_000:
                continue
            data = path.read_bytes()
        except OSError:
            continue
        if any(pattern.search(data) for pattern in TOKEN_PATTERNS):
            found.append(str(path.relative_to(ROOT)))
    return found


def audit() -> None:
    entries = sorted(
        str(path.relative_to(ROOT)) + ("/" if path.is_dir() else "")
        for path in ROOT.iterdir()
        if path.name != ".git"
    )
    print("Repository entries:")
    print("\n".join(entries) if entries else "(none)")
    print("\nCurrent branch:")
    print(git("branch", "--show-current") or "(detached or unborn)")
    print("\nBranches:")
    print(git("branch", "--all") or "(none)")
    print("\nStatus:")
    print(git("status", "--short", "--branch") or "(clean)")
    print("\nConfigured remotes:")
    print(git("remote", "-v") or "(none)")
    print("\nPossible Discord-token locations (content redacted):")
    locations = token_locations()
    print("\n".join(locations) if locations else "(none detected)")


def read_imagegen_skill() -> None:
    print(IMAGEGEN_SKILL.read_text(encoding="utf-8"), end="")


def inspect_config() -> None:
    ignore = ROOT / ".gitignore"
    print(".gitignore:")
    print(ignore.read_text(encoding="utf-8") if ignore.exists() else "(missing)")
    print(".env variable names (values redacted):")
    env_file = ROOT / ".env"
    if env_file.exists():
        keys = []
        for line in env_file.read_text(encoding="utf-8").splitlines():
            stripped = line.strip()
            if not stripped or stripped.startswith("#") or "=" not in stripped:
                continue
            keys.append(stripped.split("=", 1)[0].removeprefix("export ").strip())
        print("\n".join(keys) if keys else "(none)")
    else:
        print("(missing)")
    print("git check-ignore .env:")
    print(git("check-ignore", "-v", ".env") or "NOT IGNORED")


def install_icon() -> None:
    destination = ROOT / "assets" / "icon.png"
    destination.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(GENERATED_ICON, destination)
    print(f"installed {destination.relative_to(ROOT)} ({destination.stat().st_size} bytes)")


def go_files() -> list[str]:
    return [str(path.relative_to(ROOT)) for path in sorted(ROOT.rglob("*.go")) if ".git" not in path.parts]


def format_go() -> None:
    files = go_files()
    if files:
        run(["gofmt", "-w", *files])
    print(f"formatted {len(files)} Go files")


def test_go() -> None:
    run(["go", "test", "./..."])


def build_go() -> None:
    (ROOT / "bin").mkdir(exist_ok=True)
    run(["go", "build", "-o", "bin/disbot", "./cmd/disbot"])
    print("built bin/disbot")


def verify() -> None:
    unformatted = run(["gofmt", "-l", *go_files()], capture=True)
    if unformatted:
        print("unformatted Go files:")
        print(unformatted)
        raise SystemExit(1)
    ignored = run(["git", "check-ignore", ".env"], capture=True)
    if ignored != ".env":
        raise SystemExit(".env is not ignored")
    locations = token_locations()
    unexpected = [path for path in locations if path != ".env"]
    if unexpected:
        print("possible token outside .env:")
        print("\n".join(unexpected))
        raise SystemExit(1)
    test_go()
    build_go()
    print("verification passed; credential detected only in ignored .env")


def run_bot(args: list[str]) -> None:
    if not args:
        raise SystemExit("run-bot requires a command")
    run(["go", "run", "./cmd/disbot", *args])


def open_install() -> None:
    invite_url = run(["go", "run", "./cmd/disbot", "invite"], capture=True).strip()
    if not invite_url.startswith("https://discord.com/oauth2/authorize?"):
        raise SystemExit("refusing unexpected install URL")
    run(["xdg-open", invite_url])
    print("opened the least-privilege Discord bot installation page")


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for chunk in iter(lambda: handle.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


def repo_plan() -> None:
    candidates: list[dict[str, str]] = []
    for path in sorted(ROOT.rglob("*")):
        if not path.is_file() or ".git" in path.parts:
            continue
        relative = str(path.relative_to(ROOT))
        ignored = subprocess.run(
            ["git", "check-ignore", "-q", "--", relative], cwd=ROOT, check=False
        ).returncode == 0
        if ignored:
            continue
        data = path.read_bytes() if path.stat().st_size <= 2_000_000 else b""
        if any(pattern.search(data) for pattern in TOKEN_PATTERNS):
            raise SystemExit(f"refusing to plan possible credential: {relative}")
        candidates.append({"path": relative, "sha256": sha256(path)})
    plan = {
        "version": 1,
        "branch": "master",
        "remote": "origin",
        "commit_message": "Add Discord image archiver",
        "files": candidates,
    }
    plan_path = ROOT / ".repo-push-plan.json"
    plan_path.write_text(json.dumps(plan, indent=2) + "\n", encoding="utf-8")
    plan_path.chmod(0o600)
    print("Push plan (ignored checkpoint):")
    print(f"branch: {plan['branch']}")
    print(f"remote: {plan['remote']}")
    print(f"commit: {plan['commit_message']}")
    print("files:")
    for item in candidates:
        print(f"  {item['path']}  sha256={item['sha256']}")
    print("excluded and untracked: .env")


def repo_push() -> None:
    plan_path = ROOT / ".repo-push-plan.json"
    plan = json.loads(plan_path.read_text(encoding="utf-8"))
    if plan.get("version") != 1 or plan.get("branch") != "master" or plan.get("remote") != "origin":
        raise SystemExit("invalid repository push plan")
    files = plan.get("files", [])
    paths: list[str] = []
    for item in files:
        relative = item["path"]
        if relative == ".env" or relative.startswith(".git/"):
            raise SystemExit(f"refusing unsafe planned path: {relative}")
        path = ROOT / relative
        if not path.is_file() or sha256(path) != item["sha256"]:
            raise SystemExit(f"planned file changed; rerun make repo-plan: {relative}")
        data = path.read_bytes() if path.stat().st_size <= 2_000_000 else b""
        if any(pattern.search(data) for pattern in TOKEN_PATTERNS):
            raise SystemExit(f"refusing possible credential: {relative}")
        paths.append(relative)
    current_branch = git("branch", "--show-current")
    if current_branch != "master":
        run(["git", "switch", "-c", "master"])
    run(["git", "add", "--", *paths])
    run(["git", "diff", "--cached", "--check"])
    run(["git", "commit", "-m", plan["commit_message"]])
    run(["git", "push", "-u", "origin", "master"])
    print("pushed reviewed files to origin/master; .env was not staged")


def main() -> None:
    command = sys.argv[1] if len(sys.argv) > 1 else ""
    if command == "audit":
        audit()
        return
    if command == "read-imagegen-skill":
        read_imagegen_skill()
        return
    if command == "inspect-config":
        inspect_config()
        return
    if command == "install-icon":
        install_icon()
        return
    if command == "format":
        format_go()
        return
    if command == "test":
        test_go()
        return
    if command == "build":
        build_go()
        return
    if command == "verify":
        verify()
        return
    if command == "run-bot":
        run_bot(sys.argv[2:])
        return
    if command == "open-install":
        open_install()
        return
    if command == "repo-plan":
        repo_plan()
        return
    if command == "repo-push":
        repo_push()
        return
    raise SystemExit(f"unknown command: {command}")


if __name__ == "__main__":
    main()
