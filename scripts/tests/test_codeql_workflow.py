"""Keep dependency-only PRs in the CodeQL analysis matrix."""

from pathlib import Path
import re
import unittest


ROOT = Path(__file__).resolve().parents[2]
WORKFLOW = ROOT / ".github/workflows/codeql.yml"


class CodeQLWorkflowTests(unittest.TestCase):
    def test_scans_all_pull_requests_against_main_without_path_filters(self):
        workflow = WORKFLOW.read_text(encoding="utf-8")
        triggers = re.search(r"^on:\n((?:[ ]+[^\n]*\n|\n)+)", workflow, re.MULTILINE)
        self.assertIsNotNone(triggers)
        self.assertRegex(triggers.group(1), r"(?m)^  pull_request:\n    branches: \[main\]$")
        self.assertNotRegex(triggers.group(1), r"(?m)^\s*(?:paths|paths-ignore|pull_request_target):")
        self.assertIn("  push:\n    branches: [main]", triggers.group(1))
        self.assertIn("  schedule:", triggers.group(1))
        self.assertNotRegex(workflow, r"(?m)^\s*if:.*github\.actor")

    def test_retains_the_four_scans_and_uploads_real_results(self):
        workflow = WORKFLOW.read_text(encoding="utf-8")
        self.assertEqual(
            set(re.findall(r"(?m)^\s*- language: ([a-z-]+)$", workflow)),
            {"actions", "go", "javascript-typescript", "python"},
        )
        pins = re.findall(
            r"uses: github/codeql-action/(init|analyze)@([0-9a-f]{40})\b", workflow
        )
        self.assertEqual({step for step, _ in pins}, {"init", "analyze"})
        self.assertEqual(len({sha for _, sha in pins}), 1)
        self.assertIn("security-events: write", workflow)
        self.assertIn("persist-credentials: false", workflow)
        self.assertIn("upload: always", workflow)


if __name__ == "__main__":
    unittest.main()
