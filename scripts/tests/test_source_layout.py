import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch


VALIDATOR_PATH = Path(__file__).resolve().parents[1] / "validate-source-layout.py"
SPEC = importlib.util.spec_from_file_location("validate_source_layout", VALIDATOR_PATH)
VALIDATOR = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = VALIDATOR
SPEC.loader.exec_module(VALIDATOR)


class SourceLayoutTests(unittest.TestCase):
    def test_exactly_500_lines_pass_and_501_lines_fail(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / "src").mkdir()
            (root / "src" / "within.go").write_text("x\n" * 500, encoding="utf-8")
            (root / "src" / "over.go").write_text("é\n" * 501, encoding="utf-8")

            issues = VALIDATOR.validate_source_paths(root, ["src/within.go", "src/over.go"])

        self.assertEqual([(item.path, item.line_count) for item in issues], [("src/over.go", 501)])

    def test_physical_line_count_handles_lf_crlf_utf8_and_unterminated_line(self):
        self.assertEqual(VALIDATOR.count_physical_lines("α\r\nβ\r\nγ"), 3)
        self.assertEqual(VALIDATOR.count_physical_lines(b"\xc3\xa9\r\nx\n"), 2)
        self.assertEqual(VALIDATOR.count_physical_lines(""), 0)

    def test_production_scope_and_test_or_generated_exclusions(self):
        expected = {
            "src/factorio/server.go": True,
            "src/factorio/server_test.go": False,
            "src/factorio/testdata/fixture.py": False,
            "src/test-fixtures/generated.go": True,
            "ui/App/App.jsx": True,
            "ui/tests/route.test.cjs": False,
            "ui/e2e/manager.spec.cjs": False,
            "scripts/validate-source-layout.py": True,
            "scripts/test-container-lifecycle.py": False,
            "scripts/tests/test_source_layout.py": False,
            "scripts/fixtures/helper.sh": False,
            "docker/entrypoint.sh": True,
            "webpack.config.js": True,
            "tailwind.config.cjs": True,
            "playwright.config.ts": True,
            "README.md": False,
            "ui/index.css": False,
            "app/bundle.js": False,
            "node_modules/package/index.js": False,
            "ui/build/generated.js": False,
            "build/generated.py": False,
        }
        for path, expected_result in expected.items():
            with self.subTest(path=path):
                self.assertEqual(VALIDATOR.is_production_source(path), expected_result)

    def test_paths_normalize_windows_separators_and_dot_segments(self):
        self.assertEqual(VALIDATOR.normalize_relative_path(r".\ui\\App\.\App.jsx"), "ui/App/App.jsx")
        self.assertTrue(VALIDATOR.is_production_source(r"src\factorio\server.go"))
        with self.assertRaises(ValueError):
            VALIDATOR.normalize_relative_path(r"..\outside.py")
        with self.assertRaises(ValueError):
            VALIDATOR.normalize_relative_path(r"C:\outside.py")

    @patch.object(VALIDATOR.subprocess, "run")
    def test_discovery_uses_tracked_and_nonignored_untracked_git_paths(self, run):
        run.return_value.stdout = b"src/tracked.go\0ui/new.jsx\0"
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.assertEqual(
                VALIDATOR.discover_repo_paths(root),
                ["src/tracked.go", "ui/new.jsx"],
            )
        args = run.call_args.args[0]
        self.assertIn("--cached", args)
        self.assertIn("--others", args)
        self.assertIn("--exclude-standard", args)


if __name__ == "__main__":
    unittest.main()
