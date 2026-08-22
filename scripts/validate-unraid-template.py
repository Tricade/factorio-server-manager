#!/usr/bin/env python3
"""Validate the repository's Unraid Community Applications metadata."""

from pathlib import Path
import sys
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[1]
PROFILE_PATH = ROOT / "ca_profile.xml"
TEMPLATE_PATH = ROOT / "templates" / "factorio-server-manager.xml"
APPDATA_ROOT = "/mnt/user/appdata/factorio-server-manager/"


def parse(path: Path) -> ET.Element:
    try:
        return ET.parse(path).getroot()
    except (OSError, ET.ParseError) as error:
        raise AssertionError(f"cannot parse {path.relative_to(ROOT)}: {error}") from error


def required_text(root: ET.Element, tag: str) -> str:
    value = (root.findtext(tag) or "").strip()
    assert value, f"missing or empty <{tag}>"
    return value


def main() -> int:
    profile = parse(PROFILE_PATH)
    assert profile.tag == "CommunityApplications", "ca_profile.xml has the wrong root element"
    required_text(profile, "Profile")
    required_text(profile, "Icon")
    required_text(profile, "WebPage")

    template = parse(TEMPLATE_PATH)
    assert template.tag == "Container", "template has the wrong root element"
    assert template.attrib.get("version") == "2", "template must use Container version 2"

    for tag in (
        "Name",
        "Repository",
        "Overview",
        "Category",
        "Icon",
        "Project",
        "Support",
        "TemplateURL",
    ):
        required_text(template, tag)

    all_text = " ".join(template.itertext()).lower()
    assert "your_github" not in all_text and "example-app" not in all_text, "template contains starter placeholders"
    assert required_text(template, "Repository") == "ghcr.io/tricade/factorio-server-manager:latest"
    assert required_text(template, "TemplateURL").endswith("/templates/factorio-server-manager.xml")

    configs = template.findall("Config")
    targets = [config.attrib.get("Target", "") for config in configs]
    assert len(targets) == len(set(targets)), "Config targets must be unique"
    assert "/opt/factorio" not in targets, "combined /opt/factorio must not overlap split mounts"

    expected_paths = {
        "/opt/fsm-data": "data",
        "/opt/factorio/saves": "saves",
        "/opt/factorio/mods": "mods",
        "/opt/factorio/config": "config",
    }
    for target, directory in expected_paths.items():
        matching = [config for config in configs if config.attrib.get("Target") == target]
        assert len(matching) == 1, f"expected exactly one Config for {target}"
        default = matching[0].attrib.get("Default", "")
        assert default == APPDATA_ROOT + directory, f"unexpected host path for {target}: {default}"

    assert not list(ROOT.glob("docker/*.xml")), "Docker templates belong only in templates/ for CA scanning"
    print("Unraid Community Applications metadata is structurally valid.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except AssertionError as error:
        print(f"Unraid template validation failed: {error}", file=sys.stderr)
        raise SystemExit(1) from error
