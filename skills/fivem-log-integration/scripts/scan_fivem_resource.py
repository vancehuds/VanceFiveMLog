#!/usr/bin/env python3
import argparse
import glob
import json
import re
import sys
from pathlib import Path


HANDLER_PATTERNS = [
    ("RegisterNetEvent", re.compile(r"\bRegisterNetEvent\s*\(\s*(['\"])(?P<name>.+?)\1")),
    ("AddEventHandler", re.compile(r"\bAddEventHandler\s*\(\s*(['\"])(?P<name>.+?)\1")),
    ("RegisterCommand", re.compile(r"\bRegisterCommand\s*\(\s*(['\"])(?P<name>.+?)\1")),
]


def strip_lua_comment(line):
    return line.split("--", 1)[0]


def quoted_strings(text):
    return re.findall(r"['\"]([^'\"]+)['\"]", text)


def manifest_kind(token):
    if token.startswith("server_script"):
        return "server"
    if token.startswith("shared_script"):
        return "shared"
    if token.startswith("client_script"):
        return "client"
    return None


def parse_manifest(path):
    sections = {"server": [], "shared": [], "client": []}
    dependencies = []
    provides = []
    current = None

    for raw in path.read_text(encoding="utf-8", errors="ignore").splitlines():
        line = strip_lua_comment(raw)
        dep_match = re.match(r"\s*dependenc(?:y|ies)\b(.*)$", line)
        if dep_match:
            dependencies.extend(quoted_strings(dep_match.group(1)))
        provide_match = re.match(r"\s*provide\b(.*)$", line)
        if provide_match:
            provides.extend(quoted_strings(provide_match.group(1)))

        stmt = re.match(r"\s*(server_scripts?|shared_scripts?|client_scripts?)\b(.*)$", line)
        if stmt:
            current = manifest_kind(stmt.group(1))
            rest = stmt.group(2)
            sections[current].extend(quoted_strings(rest))
            if "{" not in rest or "}" in rest:
                current = None
            continue

        if current:
            sections[current].extend(quoted_strings(line))
            if "}" in line:
                current = None

    return sections, dependencies, provides


def expand_paths(root, patterns):
    files = []
    external = []
    for pattern in patterns:
        if pattern.startswith("@"):
            external.append(pattern)
            continue
        matches = sorted(Path(p) for p in glob.glob(str(root / pattern), recursive=True))
        if not matches:
            candidate = root / pattern
            if candidate.exists():
                matches = [candidate]
        for match in matches:
            if match.is_file() and match.suffix == ".lua" and match not in files:
                files.append(match)
    return files, external


def normalize_event_type(name):
    value = name.strip()
    value = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", value)
    value = re.sub(r"[^A-Za-z0-9]+", "_", value).strip("_").lower()
    value = re.sub(r"_+", "_", value)
    return value or "plugin_event"


def guess_severity(name):
    text = name.lower()
    if any(word in text for word in ("exploit", "cheat", "ban", "blocked", "denied", "failed")):
        return "error"
    if any(word in text for word in ("remove", "delete", "drop", "kick", "warn", "admin", "money", "cash", "bank")):
        return "warning"
    if any(word in text for word in ("success", "complete", "paid", "reward")):
        return "success"
    return "info"


def scan_file(path, root):
    text = path.read_text(encoding="utf-8", errors="ignore")
    rel = path.relative_to(root).as_posix()
    handlers = []
    vance_calls = []
    export_calls = []
    trigger_events = []

    for number, line in enumerate(text.splitlines(), start=1):
        for kind, pattern in HANDLER_PATTERNS:
            match = pattern.search(line)
            if match:
                name = match.group("name")
                handlers.append(
                    {
                        "file": rel,
                        "line": number,
                        "kind": kind,
                        "name": name,
                        "suggested_event_type": normalize_event_type(name),
                        "suggested_severity": guess_severity(name),
                    }
                )
        if "vancefivemlog" in line:
            vance_calls.append({"file": rel, "line": number, "text": line.strip()})
        for match in re.finditer(r"exports(?:\[['\"](?P<bracket>[^'\"]+)['\"]\]|\.(?P<dot>[A-Za-z0-9_-]+))\s*:", line):
            resource = match.group("bracket") or match.group("dot")
            export_calls.append({"file": rel, "line": number, "resource": resource, "text": line.strip()})
        for match in re.finditer(r"\bTriggerEvent\s*\(\s*(['\"])(?P<name>[^'\"]+)\1", line):
            trigger_events.append({"file": rel, "line": number, "name": match.group("name"), "text": line.strip()})

    return {
        "file": rel,
        "handlers": handlers,
        "vance_calls": vance_calls,
        "export_calls": export_calls,
        "trigger_events": trigger_events,
        "uses_source": bool(re.search(r"\bsource\b", text)),
        "mentions_qb": bool(re.search(r"\b(QBCore|qbx_core|qb-core)\b", text)),
        "mentions_esx": "ESX" in text,
        "mentions_inventory": bool(re.search(r"\b(inventory|ox_inventory|item)\b", text, re.I)),
        "mentions_money": bool(re.search(r"\b(money|cash|bank|account)\b", text, re.I)),
        "mentions_admin": bool(re.search(r"\b(admin|ban|kick|warn)\b", text, re.I)),
    }


def analyze(root):
    root = root.resolve()
    if not root.exists() or not root.is_dir():
        raise SystemExit(f"resource path is not a directory: {root}")

    manifest = None
    for name in ("fxmanifest.lua", "__resource.lua"):
        candidate = root / name
        if candidate.exists():
            manifest = candidate
            break

    manifest_sections = {"server": [], "shared": [], "client": []}
    dependencies = []
    provides = []
    external_scripts = []
    if manifest:
        manifest_sections, dependencies, provides = parse_manifest(manifest)

    server_files, server_external = expand_paths(root, manifest_sections["server"])
    shared_files, shared_external = expand_paths(root, manifest_sections["shared"])
    client_files, client_external = expand_paths(root, manifest_sections["client"])
    external_scripts = server_external + shared_external + client_external

    candidate_files = []
    for file in server_files + shared_files:
        if file not in candidate_files:
            candidate_files.append(file)

    if not candidate_files:
        candidate_files = sorted(root.rglob("*.lua"))

    scanned = [scan_file(path, root) for path in candidate_files]
    handlers = [handler for item in scanned for handler in item["handlers"]]
    vance_calls = [call for item in scanned for call in item["vance_calls"]]
    export_calls = [call for item in scanned for call in item["export_calls"]]
    trigger_events = [event for item in scanned for event in item["trigger_events"]]
    vance_export_calls = [call for call in export_calls if call["resource"] == "vancefivemlog"]
    vance_trigger_events = [event for event in trigger_events if event["name"].startswith("vancefivemlog:")]

    return {
        "resource_path": str(root),
        "resource_name": root.name,
        "manifest": str(manifest.relative_to(root)) if manifest else None,
        "dependencies": dependencies,
        "has_vance_dependency": "vancefivemlog" in dependencies,
        "provides": provides,
        "has_vance_provide": "vancefivemlog" in provides,
        "server_scripts": [path.relative_to(root).as_posix() for path in server_files],
        "shared_scripts": [path.relative_to(root).as_posix() for path in shared_files],
        "client_scripts": [path.relative_to(root).as_posix() for path in client_files],
        "external_scripts": external_scripts,
        "scanned_files": scanned,
        "event_handlers": handlers,
        "vance_calls": vance_calls,
        "vance_export_calls": vance_export_calls,
        "vance_trigger_events": vance_trigger_events,
    }


def print_markdown(report):
    print(f"# FiveM Resource Scan: {report['resource_name']}")
    print()
    print(f"- Path: `{report['resource_path']}`")
    print(f"- Manifest: `{report['manifest'] or 'not found'}`")
    print(f"- Vance dependency: `{'yes' if report['has_vance_dependency'] else 'no'}`")
    if report["provides"]:
        print(f"- Provides: {', '.join(f'`{item}`' for item in report['provides'])}")
    if report["external_scripts"]:
        print(f"- External manifest scripts: {', '.join(f'`{item}`' for item in report['external_scripts'])}")
    print()

    print("## Server/Shared Files")
    files = report["server_scripts"] + report["shared_scripts"]
    if files:
        for item in files:
            print(f"- `{item}`")
    else:
        print("- No manifest server/shared scripts found; scanned all Lua files.")
    print()

    print("## Existing VanceFiveMLog Calls")
    calls = report.get("vance_export_calls", []) + report.get("vance_trigger_events", [])
    if calls:
        for call in calls:
            print(f"- `{call['file']}:{call['line']}` {call['text']}")
    else:
        print("- None found.")
    print()

    print("## VanceFiveMLog Event Handlers")
    vance_handlers = [handler for handler in report["event_handlers"] if handler["name"].startswith("vancefivemlog:")]
    if vance_handlers:
        for handler in vance_handlers:
            print(f"- `{handler['file']}:{handler['line']}` {handler['kind']} `{handler['name']}`")
    else:
        print("- None found.")
    print()

    print("## Event And Command Handlers")
    if report["event_handlers"]:
        for handler in report["event_handlers"]:
            print(
                f"- `{handler['file']}:{handler['line']}` {handler['kind']} `{handler['name']}` "
                f"-> event_type `{handler['suggested_event_type']}`, severity `{handler['suggested_severity']}`"
            )
    else:
        print("- None found in scanned files.")
    print()

    print("## File Hints")
    for item in report["scanned_files"]:
        tags = []
        for key, label in (
            ("uses_source", "source"),
            ("mentions_qb", "Qbox/QBCore"),
            ("mentions_esx", "ESX"),
            ("mentions_inventory", "inventory"),
            ("mentions_money", "money"),
            ("mentions_admin", "admin"),
        ):
            if item[key]:
                tags.append(label)
        if tags:
            print(f"- `{item['file']}`: {', '.join(tags)}")
    print()

    print("## Next Steps")
    print("- Prefer direct `exports.vancefivemlog:Log(...)` calls in server handlers when source edits are allowed.")
    print("- Add `dependency 'vancefivemlog'` to the manifest only when the target resource should require the logger.")
    print("- Use `Config.EventBridge` when the target resource should remain unmodified.")
    print("- Ensure the logger resource folder is named `vancefivemlog`; otherwise direct export calls cannot resolve reliably.")
    print("- Use the helper from the skill reference so failed export calls print to the FiveM console and fall back to `TriggerEvent('vancefivemlog:server:Log', ...)`.")


def main():
    parser = argparse.ArgumentParser(description="Scan a FiveM resource for VanceFiveMLog integration points.")
    parser.add_argument("resource", help="Path to a FiveM resource directory")
    parser.add_argument("--json", action="store_true", help="Print machine-readable JSON")
    args = parser.parse_args()

    report = analyze(Path(args.resource))
    if args.json:
        json.dump(report, sys.stdout, indent=2)
        print()
    else:
        print_markdown(report)


if __name__ == "__main__":
    main()
