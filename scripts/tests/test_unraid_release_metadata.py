"""Regression coverage for release checks after the Unraid Markdown conversion."""

import importlib.util
from pathlib import Path
import unittest
import xml.etree.ElementTree as ET


SCRIPT = Path(__file__).resolve().parents[1] / "validate-unraid-template.py"
SPEC = importlib.util.spec_from_file_location("unraid_template", SCRIPT)
VALIDATOR = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(VALIDATOR)


class ReleaseMetadataTests(unittest.TestCase):
    def template(self, changes, date="2026-09-26"):
        return ET.fromstring(
            f"<Container><Date>{date}</Date><Changes><![CDATA[\n{changes}\n]]></Changes></Container>"
        )

    def test_accepts_markdown_cdata_with_history(self):
        template = self.template("### 0.19.0 (2026-09-26)\n- New feature.\n\n### 0.18.1 (2026-09-09)\n- Previous release.")
        VALIDATOR.validate_release(template, "0.19.0")

    def test_rejects_wrong_version_even_if_requested_version_occurs_later(self):
        template = self.template("### 0.18.1 (2026-09-26)\n- Old release.\n\n### 0.19.0 (2026-09-26)\n- New release.")
        with self.assertRaises(AssertionError):
            VALIDATOR.validate_release(template, "0.19.0")

    def test_rejects_date_mismatch(self):
        with self.assertRaises(AssertionError):
            VALIDATOR.validate_release(self.template("### 0.19.0 (2026-09-25)\n- New feature."), "0.19.0")

    def test_rejects_invalid_dates(self):
        for date in ("2026-02-30", "20260926", "tomorrow"):
            with self.subTest(date=date), self.assertRaises(AssertionError):
                VALIDATOR.validate_release(self.template(f"### 0.19.0 ({date})\n- New feature.", date), "0.19.0")

    def test_rejects_planned_and_empty_release_entries(self):
        for changes in ("### 0.19.0 (planned)\n- New feature.", "### 0.19.0 (2026-09-26)", "### 0.19.0 (2026-09-26)\n- New feature (planned)."):
            with self.subTest(changes=changes), self.assertRaises(AssertionError):
                VALIDATOR.validate_release(self.template(changes), "0.19.0")

    def test_rejects_non_semver_release(self):
        for version in ("v0.19.0", "0.19", "0.19.00", "0.19.0-preview"):
            with self.subTest(version=version), self.assertRaises(AssertionError):
                VALIDATOR.validate_release(self.template(f"### {version} (2026-09-26)\n- New feature."), version)


if __name__ == "__main__":
    unittest.main()
