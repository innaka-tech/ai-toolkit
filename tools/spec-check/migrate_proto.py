"""Prototype of `aitk migrate` used to validate the v2 spec against real v1 projects.

Read-only on the source project. Writes the would-be v2 tree to OUT_DIR, validates every
structured artifact against schemas/, and checks the losslessness guarantees in
docs/spec/migration-v1.md. Replaced by the Go implementation in Phase 2.

Usage: uv run --with jsonschema --with pyyaml python -I migrate_proto.py OUT_DIR PROJECT...
Exit code: 0 when every project passes, 1 otherwise.
"""
import datetime as dt
import json
import pathlib
import random
import re
import subprocess
import sys

import jsonschema
import yaml

SCHEMAS = pathlib.Path(__file__).resolve().parents[2] / "schemas"
TOKEN_BYTES = 4
STATUS_MAP = [
    (r"^(active|in[_ ]progress|eksekusi)", "in_progress"),
    (r"^implemented", "implemented"),
    (r"^(review|reviewed|bug[_ ]hunt)", "in_review"),
    (r"^(completed|done|✅)", "done"),
    (r"^blocked", "blocked"),
    (r"^(cancell?ed)", "cancelled"),
]
ID_RE = re.compile(r"^[A-Za-z][A-Za-z0-9-]{0,31}$")
DATE_RE = re.compile(r"(\d{4}-\d{2}-\d{2})(?:[T ](\d{2}:\d{2}(?::\d{2})?)Z?)?")
CROCKFORD = "0123456789abcdefghjkmnpqrstvwxyz"


def load_schema(name):
    schema = json.loads((SCHEMAS / name).read_text())
    return schema


REGISTRY = {}
for p in SCHEMAS.glob("*.schema.json"):
    s = json.loads(p.read_text())
    REGISTRY[s["$id"]] = s


def validator(name):
    from referencing import Registry, Resource

    reg = Registry().with_resources((k, Resource.from_contents(v)) for k, v in REGISTRY.items())
    schema = load_schema(name)
    return jsonschema.Draft202012Validator(schema, registry=reg, format_checker=jsonschema.FormatChecker())


V = {n: validator(f"{n}.schema.json") for n in ["state", "task", "handoff", "config", "knowledge-entry"]}


def slugify(text, maxlen=48):
    s = re.sub(r"[^a-z0-9]+", "-", text.lower()).strip("-")
    return (s[:maxlen].rstrip("-") or "untitled")


def iso(date, time=None):
    t = (time or "00:00:00")
    if len(t) == 5:
        t += ":00"
    return f"{date}T{t}Z"


def last_commit_date(root, rel):
    try:
        out = subprocess.run(["git", "-C", str(root), "log", "-1", "--format=%cI", "--", rel],
                             capture_output=True, text=True, timeout=10).stdout.strip()
        if out:
            return dt.datetime.fromisoformat(out).astimezone(dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    except Exception:
        pass
    p = root / rel
    ts = p.stat().st_mtime if p.exists() else 0
    return dt.datetime.fromtimestamp(ts, dt.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def map_status(raw):
    v = re.sub(r"[*`]", "", raw or "").strip().lower()
    for pat, out in STATUS_MAP:
        if re.search(pat, v):
            return out
    return "todo"


class Rng:
    def __init__(self, seed):
        self.r = random.Random(seed)
        self.used = set()

    def task_id(self):
        n = 4
        while True:
            cand = "T-" + "".join(self.r.choice(CROCKFORD) for _ in range(n))
            if cand not in self.used:
                self.used.add(cand)
                return cand
            n += 1


def split_sections(text):
    """Split Markdown on level-2 headings. Returns (preamble, [(heading, body)])."""
    parts = re.split(r"(?m)^(## .*)$", text)
    pre = parts[0]
    secs = [(parts[i], parts[i + 1]) for i in range(1, len(parts), 2)]
    return pre, secs


def parse_template_task(text):
    """v1 template current-task: 'Status: X' and 'Task ID: Y' lines + ## Objective."""
    st = re.search(r"(?m)^Status: *(.+)$", text)
    tid = re.search(r"(?m)^Task ID: *(.+)$", text)
    if not (st and tid):
        return None
    obj = re.search(r"(?ms)^## Objective\s*\n(.*?)(?=^## |\Z)", text)
    title_src = (obj.group(1).strip().splitlines() or [""])[0] if obj else ""
    goal = re.search(r"(?m)^Goal ID: *(G-[A-Za-z0-9.-]{1,16})\s*$", text)
    started = re.search(r"(?m)^Started At: *(\S+)$", text)
    updated = re.search(r"(?m)^Updated At: *(\S+)$", text)
    return {"status_raw": st.group(1).strip(), "v1_id": tid.group(1).strip(), "title": title_src,
            "goal": goal.group(1) if goal else None,
            "started": started.group(1) if started else None, "updated": updated.group(1) if updated else None}


def body_without_header(text):
    """Drop v1 header lines that move into frontmatter; keep the rest verbatim."""
    keep = [l for l in text.splitlines(keepends=True)
            if not re.match(r"^(# Current Task|Status:|Task ID:|Started At:|Updated At:|Project Root:)", l)]
    return "".join(keep).lstrip("\n")


def valid_ts(s):
    return bool(s and re.match(r"^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$", s))


def write_md(path, front, body):
    path.parent.mkdir(parents=True, exist_ok=True)
    fm = yaml.safe_dump(front, sort_keys=False, allow_unicode=True).strip()
    path.write_text(f"---\n{fm}\n---\n\n{body.rstrip()}\n")


def migrate(root, out, rng):
    rep = {"project": str(root), "errors": [], "warnings": [], "counts": {}}
    ai = root / "docs/ai"
    out.mkdir(parents=True, exist_ok=True)
    now = "2026-10-09T00:00:00Z"
    legacy_bytes = {}

    def keep_legacy(rel):
        p = root / rel
        if p.exists() and p.is_file():
            legacy_bytes[rel] = p.read_bytes()

    def check(kind, obj, where):
        for e in V[kind].iter_errors(obj):
            rep["errors"].append(f"{where}: {kind} schema: {e.message}")

    # --- config
    env = {}
    pe = root / ".ai-toolkit/project.env"
    if pe.exists():
        keep_legacy(".ai-toolkit/project.env")
        for line in pe.read_text(errors="replace").splitlines():
            m = re.match(r'^([A-Z_]+)="?(.*?)"?$', line.strip())
            if m:
                env[m.group(1)] = m.group(2)
    name = slugify(env.get("PROJECT_NAME") or root.name, 62)
    cfg = {"project": {"name": name}}
    if env.get("DEFAULT_BRANCH"):
        cfg["project"]["default_branch"] = env["DEFAULT_BRANCH"]
    targets = {k: env[f"DEPLOY_{k.upper()}"] for k in ["default", "staging", "production"] if env.get(f"DEPLOY_{k.upper()}")}
    if targets or env.get("POST_DEPLOY_CHECK"):
        cfg["deploy"] = {}
        if targets:
            cfg["deploy"]["targets"] = targets
        if env.get("POST_DEPLOY_CHECK"):
            cfg["deploy"]["post_check"] = env["POST_DEPLOY_CHECK"]
    settings = {}
    if env.get("UTEKE_NAMESPACE"):
        settings["uteke"] = {"namespace": env["UTEKE_NAMESPACE"]}
    if env.get("CODEBASE_PROJECT"):
        settings["codebase-memory"] = {"project": env["CODEBASE_PROJECT"]}
    if settings:
        cfg["plugins"] = {"settings": settings}
    dropped = [k for k in env if k == "PROJECT_ROOT" or k.startswith("BROWSER_MCP_")]
    if dropped:
        rep["warnings"].append("dropped machine-specific config: " + ", ".join(sorted(dropped)))
    check("config", cfg, "aitk.toml")
    (out / "aitk.toml.json").write_text(json.dumps(cfg, indent=2))

    # --- state
    st1 = {}
    sp = root / "ai-state.json"
    if sp.exists():
        keep_legacy("ai-state.json")
        try:
            st1 = json.loads(sp.read_text())
        except Exception as e:
            rep["warnings"].append(f"ai-state.json unreadable: {e}")
    state = {"schema_version": 2, "project": {"name": name}, "aitk": {"migrated_from": 1, "migrated_at": now}}
    ctx = (st1.get("context") or "").strip()
    if ctx and "update this summary" not in ctx:
        state["project"]["summary"] = ctx[:280]
    check("state", state, "ai-state.json")
    (out / "ai-state.json").write_text(json.dumps(state, indent=2))

    # --- tasks
    tasks = []
    def add_template_task(text, src):
        t = parse_template_task(text)
        if not t:
            return False
        tid = rng.task_id()
        title = (t["title"] or f"Migrated task {t['v1_id']}")[:120]
        if len(title) < 3:
            title = f"Migrated task {t['v1_id']}"
        created = t["started"] if valid_ts(t["started"]) else last_commit_date(root, src)
        updated = t["updated"] if valid_ts(t["updated"]) else created
        front = {"id": tid, "title": title, "status": map_status(t["status_raw"]), "profile": "standard",
                 "created": created, "updated": updated, "legacy": {"v1_id": t["v1_id"], "status": t["status_raw"], "source": src}}
        if t["goal"]:
            front["goal"] = t["goal"]
        check("task", front, src)
        write_md(out / f"docs/ai/tasks/{tid}-{slugify(title)}.md", front, body_without_header(text))
        tasks.append(tid)
        return True

    ct = ai / "current-task.md"
    if ct.exists():
        keep_legacy("docs/ai/current-task.md")
        if not add_template_task(ct.read_text(errors="replace"), "docs/ai/current-task.md"):
            rep["warnings"].append("current-task.md is free-form: kept in _legacy, no task created")
    td = ai / "tasks"
    unparsed = 0
    if td.is_dir():
        for f in sorted(td.glob("*.md")):
            rel = f"docs/ai/tasks/{f.name}"
            keep_legacy(rel)
            text = f.read_text(errors="replace")
            if f.name.endswith(".previous.md"):
                if not add_template_task(text, rel):
                    unparsed += 1
                continue
            prefix = f.stem.split("-")[0]
            h1 = re.search(r"(?m)^# +(.+)$", text)
            stl = re.search(r"(?mi)^\**status:?\**:? *(.+)$", text)
            if ID_RE.match(prefix) and h1 and stl:
                title = h1.group(1).strip()[:120]
                front = {"id": prefix, "title": title if len(title) >= 3 else f"Task {prefix}",
                         "status": map_status(stl.group(1) if stl else ""), "profile": "standard",
                         "created": last_commit_date(root, rel), "updated": last_commit_date(root, rel),
                         "legacy": {"source": rel, **({"status": stl.group(1).strip()} if stl else {})}}
                check("task", front, rel)
                write_md(out / f"docs/ai/tasks/{prefix}-{slugify(f.stem[len(prefix):] or title)}.md", front, text)
                tasks.append(prefix)
            else:
                unparsed += 1
    if unparsed:
        rep["warnings"].append(f"{unparsed} task file(s) not parseable: moved to _legacy/tasks")
    rep["counts"]["tasks"] = len(tasks)

    # --- handoffs
    hp = ai / "handoff.md"
    n_ho = 0
    if hp.exists():
        keep_legacy("docs/ai/handoff.md")
        pre, secs = split_sections(hp.read_text(errors="replace"))
        prev = last_commit_date(root, "docs/ai/handoff.md")
        seen = set()
        for head, body in secs:
            m = DATE_RE.search(head[:40])
            at = iso(m.group(1), m.group(2)) if m else prev
            prev = at
            stamp = at.replace("-", "").replace(":", "")
            k = 0
            while (stamp, k) in seen:
                k += 1
            seen.add((stamp, k))
            front = {"at": at, "by": "unknown", "outcome": "legacy"}
            check("handoff", front, f"handoff {head[:40]}")
            write_md(out / f"docs/ai/handoff/{stamp}-legacy{'-' + str(k) if k else ''}.md", front, head + body)
            n_ho += 1
    rep["counts"]["handoffs"] = n_ho

    # --- knowledge
    kp = ai / "knowledge.md"
    entries = []
    if kp.exists():
        keep_legacy("docs/ai/knowledge.md")
        text = kp.read_text(errors="replace")
        fallback = last_commit_date(root, "docs/ai/knowledge.md")[:10]
        cur_date, cur = fallback, None
        for line in text.splitlines():
            if line.startswith("#"):
                m = DATE_RE.search(line)
                cur_date = m.group(1) if m else cur_date
                if cur:
                    entries.append(cur); cur = None
            elif line.startswith("- "):
                if cur:
                    entries.append(cur)
                cur = {"text": line[2:], "date": cur_date}
            elif cur and re.match(r"^\s{2,}\S", line):
                cur["text"] += "\n" + line
            elif cur and not line.strip():
                entries.append(cur); cur = None
        if cur:
            entries.append(cur)
        lines = []
        for e in entries:
            parsed = {"text": e["text"][:4000], "date": e["date"], "topic": "general", "source": "migrated"}
            if len(e["text"]) > 4000:
                rep["warnings"].append(f"knowledge entry > 4000 chars ({len(e['text'])}); stored in full on disk, truncated in parsed form")
            if len(parsed["text"].strip()) < 3:
                continue
            check("knowledge-entry", parsed, "knowledge")
            lines.append(f"- {e['text']} <!-- aitk:k date={e['date']} -->")
        kd = out / "docs/ai/knowledge"
        kd.mkdir(parents=True, exist_ok=True)
        (kd / "_inbox.md").write_text("\n".join(lines) + ("\n" if lines else ""))
        # losslessness: every bullet text must appear verbatim
        out_text = (kd / "_inbox.md").read_text()
        missing = [e for e in entries if e["text"] not in out_text and len(e["text"].strip()) >= 3]
        if missing:
            rep["errors"].append(f"knowledge: {len(missing)} entries lost")
    rep["counts"]["knowledge"] = len(entries)

    # --- decisions -> ADR
    dp = ai / "decisions.md"
    n_adr = 0
    if dp.exists():
        keep_legacy("docs/ai/decisions.md")
        _, secs = split_sections(dp.read_text(errors="replace"))
        existing = len(list((root / "docs/adr").glob("[0-9][0-9][0-9][0-9]-*.md"))) if (root / "docs/adr").is_dir() else 0
        for i, (head, body) in enumerate(secs, start=existing + 1):
            title = re.sub(r"^## *", "", head)
            m = DATE_RE.match(title)
            date = m.group(1) if m else last_commit_date(root, "docs/ai/decisions.md")[:10]
            title_clean = DATE_RE.sub("", title, count=1).strip(" —-") or "Decision"
            p = out / f"docs/adr/{i:04d}-{slugify(title_clean)}.md"
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text(f"---\nstatus: accepted\ndate: {date}\n---\n\n# {title_clean}\n{body}")
            if (head + body).strip() and title_clean not in p.read_text():
                rep["errors"].append(f"adr {i}: content lost")
            n_adr += 1
    rep["counts"]["adrs"] = n_adr

    # --- legacy copy + lossless check
    for rel, data in legacy_bytes.items():
        dst = out / "docs/ai/_legacy" / rel.replace("docs/ai/", "")
        dst.parent.mkdir(parents=True, exist_ok=True)
        dst.write_bytes(data)
        if dst.read_bytes() != data:
            rep["errors"].append(f"legacy copy mismatch: {rel}")

    # --- portability: no absolute paths written by migration in generated structured data
    for p in [q for q in out.rglob("*.json") if "_legacy" not in q.parts] + list((out / "docs/ai/tasks").glob("*.md")):
        head = p.read_text(errors="replace").split("\n---\n", 1)[0] if p.suffix == ".md" else p.read_text()
        if re.search(r'"(/Users|/home)/|^\s*\w+: (/Users|/home)/', head, re.M):
            rep["errors"].append(f"absolute path in generated metadata: {p.relative_to(out)}")

    v1_bytes = sum(len(v) for k, v in legacy_bytes.items() if k.startswith("docs/ai/"))
    rep["counts"]["v1_tokens_docs_ai"] = v1_bytes // TOKEN_BYTES
    return rep


def main():
    out_root = pathlib.Path(sys.argv[1])
    projects = [pathlib.Path(p) for p in sys.argv[2:]]
    fail = 0
    print(f"{'project':38} {'tasks':>5} {'hand':>5} {'know':>5} {'adr':>4} {'v1 tok':>7}  result")
    for i, root in enumerate(projects):
        rep = migrate(root, out_root / f"{i:02d}-{root.name}", Rng(i))
        c = rep["counts"]
        ok = not rep["errors"]
        fail += not ok
        print(f"{str(root)[-38:]:38} {c.get('tasks',0):5} {c.get('handoffs',0):5} {c.get('knowledge',0):5} {c.get('adrs',0):4} {c.get('v1_tokens_docs_ai',0):7}  {'PASS' if ok else 'FAIL'}")
        for e in rep["errors"][:5]:
            print("    ERROR", e)
        for w in rep["warnings"]:
            print("    warn ", w[:140])
    print(f"\n{len(projects) - fail}/{len(projects)} projects pass")
    sys.exit(1 if fail else 0)


if __name__ == "__main__":
    main()
