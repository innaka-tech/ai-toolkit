# Write a plugin

A plugin is an executable named `aitk-<name>` on PATH, enabled in `aitk.toml`:

```toml
[plugins]
enabled = ["memory"]
timeout = "5s"

[plugins.settings.memory]
namespace = "shop"
```

aitk runs `aitk-memory manifest` and expects:

```json
{"schema":"aitk.plugin.manifest/v1","name":"memory","version":"1.0.0","hooks":["brief.sections","knowledge.search"]}
```

For each hook it runs `aitk-memory hook <hook>` with one JSON request on stdin (`aitk.plugin.request/v1`: hook, project, task, payload, with your settings in `payload.settings`) and reads one JSON response on stdout:

```json
{"schema":"aitk.plugin.response/v1","ok":true,"data":[{"title":"Memory","body":"…","rank":1}]}
```

| Hook | `data` |
|---|---|
| `brief.sections` | `[{title, body, rank}]` added to the brief (dropped first when over budget) |
| `close.after` | ignored |
| `doctor.checks` | `[{name, status: ok\|warn\|error, detail}]` |
| `knowledge.search` | `[{text, score, source}]` merged into search results |

Slow, failing, or invalid plugins become warnings; they never fail the command. A complete example is in [internal/plugins/testdata/aitk-demo](../../internal/plugins/testdata/aitk-demo/main.go). Schemas: [plugin.schema.json](../../schemas/plugin.schema.json).
