# TASK TEMPLATE (generic — ai-toolkit engine)

> Template task AI-executable untuk SEMUA project. Salin ke `docs/ai/tasks/T-XXX.md` di project.
> Prinsip: task SELF-CONTAINED — AI tidak perlu baca seluruh dokumentasi project. AC + constraint ditulis INLINE.

```markdown
---
id: T-XXX
modul: <Modul/Domain>
priority: P1|P2|P3
depends_on: [T-YYY]          # task yang harus DONE dulu
estimasi: 0.5 hari            # 0.5–1 hari per unit AI
status: TODO
---

# T-XXX: <Judul singkat>

## Objective
<1–2 kalimat: apa yang dibuat/diubah + hasil akhir yang diharapkan>

## Acceptance Criteria
- [ ] <kriteria terukur 1>
- [ ] <kriteria terukur 2>
- [ ] <test wajib: perintah spesifik + nama test>

## Constraints (inline — JANGAN referensi silang)
- <invariant/aturan bisnis — tuliskan isinya di sini>
- <enum/status valid — tuliskan nilai persis>
- <aturan data: uang decimal, soft delete, append-only>
- <permission: role mana yang boleh akses>

## UI Spec (hanya task frontend — deskriptif, BUKAN wireframe visual)
- Layout: <struktur halaman, urutan komponen>
- Pola navigasi: <sesuai konvensi project (app drawer/bottom nav/dll)>
- State wajib: loading / empty / error

## Referensi (untuk verifikasi, bukan dibaca ulang semua)
- <dokumen>: <section/endpoint/tabel>

## Definition of Done
- [ ] Semua AC tercentang
- [ ] Test hijau + regresi task sebelumnya hijau
- [ ] Bug hunt pass terakhir = 0 bug (log di handoff.md)
- [ ] `ai-close --summary "<ringkasan>" --knowledge "<temuan durable>"` dijalankan
```
