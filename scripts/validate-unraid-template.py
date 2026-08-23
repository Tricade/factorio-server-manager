#!/usr/bin/env python3
"""Validate the repository's Unraid Community Applications metadata."""

from pathlib import Path
import struct
import sys
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[1]
PROFILE_PATH = ROOT / "ca_profile.xml"
TEMPLATE_PATH = ROOT / "templates" / "factorio-server-manager.xml"
APPDATA_ROOT = "/mnt/user/appdata/factorio-server-manager/"
RAW_REPOSITORY_ROOT = "https://raw.githubusercontent.com/Tricade/factorio-server-manager/main/"


def parse(path: Path) -> ET.Element:
    try:
        return ET.parse(path).getroot()
    except (OSError, ET.ParseError) as error:
        raise AssertionError(f"cannot parse {path.relative_to(ROOT)}: {error}") from error


def required_text(root: ET.Element, tag: str) -> str:
    value = (root.findtext(tag) or "").strip()
    assert value, f"missing or empty <{tag}>"
    return value


def assert_png(path: Path, expected_size: tuple[int, int] | None = None) -> None:
    contents = path.read_bytes()
    assert contents.startswith(b"\x89PNG\r\n\x1a\n"), f"{path.relative_to(ROOT)} is not a PNG file"
    assert len(contents) >= 24 and contents[12:16] == b"IHDR", f"{path.relative_to(ROOT)} has no PNG IHDR"
    size = struct.unpack(">II", contents[16:24])
    if expected_size is not None:
        assert size == expected_size, f"unexpected dimensions for {path.relative_to(ROOT)}: {size}"


def assert_local_raw_asset(url: str) -> Path:
    assert url.startswith(RAW_REPOSITORY_ROOT), f"asset must use this repository's raw HTTPS URL: {url}"
    relative = url.removeprefix(RAW_REPOSITORY_ROOT)
    path = ROOT / relative
    assert path.is_file(), f"raw asset does not exist locally: {relative}"
    return path


def main() -> int:
    profile = parse(PROFILE_PATH)
    assert profile.tag == "CommunityApplications", "ca_profile.xml has the wrong root element"
    profile_text = required_text(profile, "Profile")
    assert "Maintained by Tricade" in profile_text, "ca_profile.xml is missing maintainer attribution"
    profile_icon = assert_local_raw_asset(required_text(profile, "Icon"))
    assert profile_icon == ROOT / "branding" / "tricade-maintainer.png", (
        "ca_profile.xml must use the maintainer profile icon"
    )
    assert_png(profile_icon, (512, 512))
    required_text(profile, "WebPage")
    for photo in profile.findall("Photo"):
        assert_png(assert_local_raw_asset((photo.text or "").strip()))

    template = parse(TEMPLATE_PATH)
    assert template.tag == "Container", "template has the wrong root element"
    assert template.attrib.get("version") == "2", "template must use Container version 2"

    for tag in (
        "Name",
        "Repository",
        "Overview",
        "Description",
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
    assert required_text(template, "Name") == "Factorio Server Control"
    assert "modern fork" in required_text(template, "Overview").lower()
    assert required_text(template, "TemplateURL").endswith("/templates/factorio-server-manager.xml")
    assert required_text(template, "Network") == "bridge"
    assert required_text(template, "Privileged") == "false"
    assert "--restart=unless-stopped" in required_text(template, "ExtraParams")
    assert "--stop-timeout=180" in required_text(template, "ExtraParams")
    template_icon = assert_local_raw_asset(required_text(template, "Icon"))
    assert template_icon == ROOT / "icon.png", "template must use the canonical PNG icon"
    for screenshot in template.findall("Screenshot"):
        assert_png(assert_local_raw_asset((screenshot.text or "").strip()))

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
        assert matching[0].attrib.get("Required") == "true", f"{target} must be required"

    cookie_config = next(config for config in configs if config.attrib.get("Target") == "FSM_COOKIE_SECURE")
    assert cookie_config.attrib.get("Default") == "false", "direct Unraid HTTP requires a non-Secure session cookie"
    rcon_config = next(config for config in configs if config.attrib.get("Target") == "RCON_PASS")
    assert rcon_config.attrib.get("Default") == "", "RCON must default to generated credentials"
    assert rcon_config.attrib.get("Mask") == "true", "RCON input must be masked"

    assert not list(ROOT.glob("docker/*.xml")), "Docker templates belong only in templates/ for CA scanning"
    assert not (ROOT / "icon.svg").exists(), "stale SVG icon should not be published"
    print("Unraid Community Applications metadata is structurally valid.")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except AssertionError as error:
        print(f"Unraid template validation failed: {error}", file=sys.stderr)
        raise SystemExit(1) from error
