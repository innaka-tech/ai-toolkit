"""Schema tests: every schema is valid draft 2020-12, accepts known-good and rejects known-bad documents.
Usage: uv run --with jsonschema --with referencing python -I test_schemas.py"""
import glob, json, pathlib, sys
import jsonschema
from referencing import Registry, Resource

DIR = pathlib.Path(__file__).resolve().parents[2] / "schemas"
SCHEMAS = {p.name: json.loads(p.read_text()) for p in DIR.glob("*.schema.json")}
REG = Registry().with_resources((s["$id"], Resource.from_contents(s)) for s in SCHEMAS.values())

def validator(name):
    return jsonschema.Draft202012Validator(SCHEMAS[f"{name}.schema.json"], registry=REG, format_checker=jsonschema.FormatChecker())

T = {"id": "T-k3m9", "title": "Show invoice", "status": "done", "profile": "lite",
     "created": "2026-10-09T08:00:00Z", "updated": "2026-10-09T08:00:00Z"}
CHECK = {"cmd": "make test", "exit_code": 0, "at": "2026-10-09T08:00:00Z"}
CASES = [
    ("task", "valid task", T, True),
    ("task", "imported id F7", {**T, "id": "F7"}, True),
    ("task", "bad status", {**T, "status": "finished"}, False),
    ("task", "absolute path in paths", {**T, "paths": ["/Users/x/a.go"]}, False),
    ("task", "home path in paths", {**T, "paths": ["~/a.go"]}, False),
    ("task", "non-UTC timestamp", {**T, "created": "2026-10-09T08:00:00+07:00"}, False),
    ("task", "numeric v1 id", {**T, "id": "20260906-141827"}, False),
    ("task", "unknown field", {**T, "foo": 1}, False),
    ("task", "review pass", {**T, "review": {"passes": [{"n": 1, "by": "codex", "at": "2026-10-09T08:00:00Z", "findings": 0}]}}, True),
    ("state", "v2 state", {"schema_version": 2, "project": {"name": "shop"}}, True),
    ("state", "v1 state", {"version": 1, "context": "x"}, False),
    ("handoff", "valid handoff with check", {"at": "2026-10-09T08:00:00Z", "by": "claude-code", "outcome": "done", "check": CHECK}, True),
    ("handoff", "check missing exit_code", {"at": "2026-10-09T08:00:00Z", "by": "codex", "outcome": "done", "check": {"cmd": "x", "at": "2026-10-09T08:00:00Z"}}, False),
    ("knowledge-entry", "entry", {"text": "Use decimals for money", "date": "2026-10-09", "topic": "money"}, True),
    ("knowledge-entry", "too long", {"text": "x" * 4001, "date": "2026-10-09", "topic": "general"}, False),
    ("session", "session", {"schema_version": 1, "active_task": None}, True),
    ("result", "error without error object", {"schema": "aitk.result/v1", "ok": False, "command": "close"}, False),
    ("result", "error with fix", {"schema": "aitk.result/v1", "ok": False, "command": "close", "error": {"code": "E_DOD_KNOWLEDGE", "message": "m", "fix": "aitk close --knowledge none"}}, True),
    ("config", "full config", {"project": {"name": "shop", "default_branch": "main"}, "check": {"cmd": "make test", "timeout": "10m"},
                               "risk": {"sensitive_paths": ["migrations/**"]}, "plugins": {"enabled": ["uteke"], "settings": {"uteke": {"namespace": "shop"}}}}, True),
    ("config", "bad timeout", {"check": {"cmd": "x", "timeout": "10 minutes"}}, False),
    ("plugin", "manifest", {"schema": "aitk.plugin.manifest/v1", "name": "uteke", "version": "1.0.0", "hooks": ["brief.sections"]}, True),
    ("plugin", "unknown hook", {"schema": "aitk.plugin.manifest/v1", "name": "uteke", "version": "1", "hooks": ["exec.anything"]}, False),
]

def main():
    bad = 0
    for name, schema in sorted(SCHEMAS.items()):
        jsonschema.Draft202012Validator.check_schema(schema)
    for name, label, doc, expect in CASES:
        ok = not list(validator(name).iter_errors(doc))
        bad += ok != expect
        print(f"{'ok  ' if ok == expect else 'FAIL'} {name:16} {label}")
    print(f"{len(CASES) - bad}/{len(CASES)} schema cases pass; {len(SCHEMAS)} schemas are valid draft 2020-12")
    sys.exit(1 if bad else 0)

main()
