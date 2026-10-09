"""Runs the migration prototype on tests/fixtures/v1-project and asserts the exact outcome.
Usage: uv run --with jsonschema --with pyyaml --with referencing python -I test_migration.py"""
import pathlib, sys, tempfile
sys.path.insert(0, str(pathlib.Path(__file__).parent))
import migrate_proto as m

FIX = pathlib.Path(__file__).resolve().parents[2] / "tests/fixtures/v1-project"
EXPECT = {"tasks": 3, "handoffs": 3, "knowledge": 4, "adrs": 2}

with tempfile.TemporaryDirectory() as d:
    out = pathlib.Path(d)
    rep = m.migrate(FIX, out, m.Rng(0))
    tasks = sorted(p.name for p in (out / "docs/ai/tasks").glob("*.md"))
    legacy = out / "docs/ai/_legacy"
    checks = [
        ("no errors", rep["errors"] == []),
        *[(f"count {k} == {v}", rep["counts"].get(k) == v) for k, v in EXPECT.items()],
        ("hand-written F7 task kept its id", any(n.startswith("F7-") for n in tasks)),
        ("notes.md without status is not a task", not any(n.startswith("notes") for n in tasks)),
        ("machine-specific config dropped", any("PROJECT_ROOT" in w for w in rep["warnings"])),
        ("aitk.toml has no absolute path", "/Users/" not in (out / "aitk.toml.json").read_text()),
        ("uteke namespace -> plugin settings", '"namespace": "demo-shop"' in (out / "aitk.toml.json").read_text()),
        ("v1 state preserved verbatim", (legacy / "ai-state.json").read_bytes() == (FIX / "ai-state.json").read_bytes()),
        ("multi-line knowledge kept", "on an indented second line" in (out / "docs/ai/knowledge/_inbox.md").read_text()),
        ("undated decision migrated", (out / "docs/adr/0002-initial-ai-protocol-setup.md").exists()),
        ("user doc untouched (not copied/rewritten)", not (out / "docs/ai/uat-checklist.md").exists()),
    ]
bad = 0
for label, ok in checks:
    bad += not ok
    print(f"{'ok  ' if ok else 'FAIL'} {label}")
print(f"{len(checks) - bad}/{len(checks)} migration checks pass")
sys.exit(1 if bad else 0)
