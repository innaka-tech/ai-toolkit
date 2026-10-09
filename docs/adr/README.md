# Architecture Decision Records

Format: [MADR 4](https://adr.github.io/madr/). One file per decision; never edit an accepted ADR's decision, supersede it with a new one.

- [ADR-0001](0001-product-name-aitk.md): Product and CLI name: `aitk`
- [ADR-0002](0002-split-private-coordination.md): Private agent coordination lives outside aitk
- [ADR-0003](0003-model-routing-out-of-scope.md): Model selection, routing, and cost are out of scope
- [ADR-0004](0004-go-single-binary.md): Rewrite the core in Go as one binary
- [ADR-0005](0005-git-as-storage.md): Git is the storage; one file per entity; session state stays local
- [ADR-0006](0006-keep-v1-layout.md): Keep the v1 layout; add `aitk.toml`
- [ADR-0007](0007-plugins-as-executables.md): Plugins are executables speaking JSON
- [ADR-0008](0008-license-and-language.md): Apache-2.0; English by default
- [ADR-0009](0009-task-ids.md): Short random task IDs
