#!/usr/bin/env python3
"""Reject oversized imperative production source files.

The validator deliberately applies one hard physical-line limit to the
repository's imperative source. It is not a general code-quality or god-object
detector.
"""

from __future__ import annotations

import posixpath
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable


MAX_SOURCE_LINES = 500
REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
CODE_ROOTS = {"src", "ui", "scripts", "docker"}
UI_CODE_EXTENSIONS = {".js", ".jsx", ".cjs"}
SCRIPT_EXTENSIONS = {".lua", ".py", ".sh", ".ps1"}
GO_TEST_SUFFIX = "_test.go"
TEST_DIRECTORY_NAMES = {
    "test",
    "tests",
    "testing",
    "testdata",
    "test-data",
    "test_data",
    "test-fixture",
    "test_fixtures",
    "test-fixtures",
    "test_fixture",
    "fixtures",
    "fixture",
    "__tests__",
    "__fixtures__",
    "e2e",
}
EXCLUDED_PATH_NAMES = {"build", "coverage", ".git", "dist", "generated", "node_modules", "vendor"}
ROOT_CONFIG_PREFIXES = ("webpack", "tailwind", "playwright")
ROOT_CONFIG_EXTENSIONS = {".js", ".jsx", ".cjs", ".mjs", ".ts", ".tsx"}
TEST_FILE_PATTERNS = (
    re.compile(r"^test_.*\.py$", re.IGNORECASE),
    re.compile(r"^test[-_].*\.(?:py|lua|sh|ps1)$", re.IGNORECASE),
    re.compile(r".*_test\.(?:py|sh|lua|ps1)$", re.IGNORECASE),
    re.compile(r".*\.test\.(?:js|jsx|cjs|lua|py|sh|ps1)$", re.IGNORECASE),
    re.compile(r".*\.spec\.(?:js|jsx|cjs)$", re.IGNORECASE),
)


@dataclass(frozen=True)
class SourceIssue:
    path: str
    line_count: int | None
    detail: str


def normalize_relative_path(path: str | Path) -> str:
    """Normalize Git-style or Windows-style paths to a safe repo-relative form."""
    value = str(path).replace("\\", "/")
    if value.startswith("/") or re.match(r"^[A-Za-z]:", value):
        raise ValueError(f"path must be repository-relative: {path}")
    normalized = posixpath.normpath(value)
    if normalized in ("", "."):
        return ""
    if normalized == ".." or normalized.startswith("../"):
        raise ValueError(f"path escapes the repository root: {path}")
    return normalized


def _is_test_path(parts: tuple[str, ...], filename: str) -> bool:
    if any(part.casefold() in TEST_DIRECTORY_NAMES for part in parts):
        return True
    return any(pattern.fullmatch(filename) for pattern in TEST_FILE_PATTERNS)


def is_production_source(path: str | Path) -> bool:
    """Return whether a repo-relative path is governed by the source budget."""
    normalized = normalize_relative_path(path)
    if not normalized:
        return False
    parts = tuple(normalized.split("/"))
    filename = parts[-1]
    suffix = Path(filename).suffix.casefold()
    if any(part.casefold() in EXCLUDED_PATH_NAMES for part in parts):
        return False

    if len(parts) == 1:
        stem = filename.casefold()
        return (
            suffix in ROOT_CONFIG_EXTENSIONS
            and stem.startswith(ROOT_CONFIG_PREFIXES)
        )

    if parts[0] not in CODE_ROOTS:
        return False

    if parts[0] == "src" and suffix == ".go":
        return not filename.casefold().endswith(GO_TEST_SUFFIX)

    if parts[0] == "ui" and suffix in UI_CODE_EXTENSIONS:
        return not _is_test_path(parts[1:-1], filename)

    if suffix in SCRIPT_EXTENSIONS:
        return not _is_test_path(parts[1:-1], filename)

    return False


def count_physical_lines(content: bytes | str) -> int:
    """Count LF, CRLF, or CR-delimited lines, including an unterminated last line."""
    text = content.decode("utf-8-sig") if isinstance(content, bytes) else content
    normalized = text.replace("\r\n", "\n").replace("\r", "\n")
    if not normalized:
        return 0
    return normalized.count("\n") + (not normalized.endswith("\n"))


def discover_repo_paths(repository_root: Path = REPOSITORY_ROOT) -> list[str]:
    """Read tracked and non-ignored untracked paths without walking build/dependency trees."""
    result = subprocess.run(
        ["git", "ls-files", "--cached", "--others", "--exclude-standard", "-z"],
        cwd=repository_root,
        check=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    paths = result.stdout.decode("utf-8", errors="strict").split("\0")
    return sorted({normalize_relative_path(path) for path in paths if path})


def validate_source_paths(
    repository_root: Path,
    paths: Iterable[str | Path],
    line_limit: int = MAX_SOURCE_LINES,
) -> list[SourceIssue]:
    issues = []
    for candidate in sorted({normalize_relative_path(path) for path in paths}):
        if not candidate or not is_production_source(candidate):
            continue
        source_path = repository_root.joinpath(*candidate.split("/"))
        if not source_path.is_file():
            continue
        try:
            line_count = count_physical_lines(source_path.read_bytes())
        except UnicodeDecodeError as error:
            issues.append(SourceIssue(candidate, None, f"not valid UTF-8 ({error})"))
            continue
        if line_count > line_limit:
            issues.append(SourceIssue(candidate, line_count, f"exceeds {line_limit} lines"))
    return issues


def validate_repository(repository_root: Path = REPOSITORY_ROOT) -> list[SourceIssue]:
    return validate_source_paths(repository_root, discover_repo_paths(repository_root))


def main() -> int:
    try:
        issues = validate_repository()
    except (OSError, subprocess.CalledProcessError, UnicodeDecodeError) as error:
        print(f"Source layout validation could not run: {error}", file=sys.stderr)
        return 2

    if issues:
        for issue in issues:
            if issue.line_count is None:
                print(f"{issue.path}: {issue.detail}")
            else:
                print(f"{issue.path}: {issue.line_count} physical lines ({issue.detail})")
        return 1

    print(f"Source layout OK: imperative production files are at most {MAX_SOURCE_LINES} physical lines.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
