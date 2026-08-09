# STANDARD DEVELOPMENT PROTOCOL (ai-toolkit engine level)

> Berlaku untuk SEMUA project yang memakai ai-toolkit.
> Provider-agnostic: tidak mengikat AI tertentu (Hermes/Codex/Claude/OpenCode/AGY/Kiro/dll).
> Project boleh menambah detail spesifik, TIDAK boleh mengurangi aturan ini.

**Lokasi file:**
- Standar ini: `~/.ai-toolkit/conventions/STANDARD_DEVELOPMENT.md`
- Template task: `~/.ai-toolkit/conventions/templates/TASK_TEMPLATE.md`
- Template konvensi kode: `~/.ai-toolkit/conventions/templates/CONVENTIONS_TEMPLATE.md`
- Template START_HERE: `~/.ai-toolkit/conventions/templates/START_HERE_TEMPLATE.md`

**Cara pakai di project baru:**
1. `ai-init` (otomatis: referensi standar engine masuk AGENTS.md, buat docs/ai/, ai-state.json)
2. Salin `START_HERE_TEMPLATE.md` → `START_HERE.md` di root, isi detail project
3. Salin `CONVENTIONS_TEMPLATE.md` → `docs/ai/conventions.md`, isi stack project
4. Salin `TASK_TEMPLATE.md` → `docs/ai/templates/TASK_TEMPLATE.md`
5. Buat `docs/ai/development-protocol.md` (project-specific tambahan) + `docs/ai/goals.md`
6. Mulai task pertama: `ai-start "T-001: ..."`

## 1. Task Lifecycle (5 stage + bug hunt loop)

```
TODO → IN_PROGRESS → IMPLEMENTED → REVIEWED → BUG_HUNT (pass 2..N, loop) → DONE
```

| Stage | Arti | Syarat masuk |
|-------|------|--------------|
| TODO | Belum dikerjakan | Task dibuat + AC lengkap |
| IN_PROGRESS | Sedang dikerjakan | `ai-start "<task id>"` |
| IMPLEMENTED | Kode selesai + test hijau | Semua AC tercentang, lint OK |
| REVIEWED | Self-review pass 1 selesai | Bug pass 1 = 0 |
| BUG_HUNT | Bug hunting pass 2..N | Reviewer AI BERBEDA session dari implementer |
| DONE | 1 pass penuh 0 bug | Bukti log pass di handoff |

**ATURAN WAJIB:**
1. AI TIDAK boleh pindah ke task berikutnya sebelum task aktif `DONE`.
2. Bug hunt diulang terus sampai **1 pass penuh menemukan 0 bug** — tanpa batas siklus.
3. Pass ke-N menemukan bug ≥ pass ke-N-1 (tidak progres) → ESKALASI ke PM: pecah task / ganti AI / manual.
4. Reviewer pass 2+ WAJIB AI/session berbeda dari implementer (fresh eyes — blind spot tidak berulang).
5. Setiap bug ditemukan & di-fix → **jalankan ulang semua pass sebelumnya** (regresi).

## 2. Rotasi Fokus Bug Hunt (tiap pass sudut berbeda)

| Pass | Fokus | Cek utama |
|------|-------|-----------|
| 1 | Self-review (implementer) | AC, lint, test, constraint |
| 2 | Flow logic | State machine, transisi, chain proses |
| 3 | Edge case | qty 0/negatif, duplikat, null, konflik, pagination |
| 4 | Cross-module | Event, permission 403, data leak antar role |
| 5 | Security & data | OWASP, soft delete, append-only, encrypt |
| 6+ | Rotasi ulang | Kombinasi + regresi semua pass sebelumnya |

## 3. Bukti per Task (wajib, bukan klaim)

Setiap task `DONE` harus punya blok bukti di `docs/ai/handoff.md`:

```
T-XXX: <judul>
Implement: <AI/tool> · Review: <AI/tool berbeda>
Pass 1 (self): X bug · Pass 2 (flow): Y bug · Pass 3 (edge): Z bug ...
→ DONE: N pass, M bug fixed, regression hijau
```

## 4. Definition of Done (semua wajib)

- [ ] Semua Acceptance Criteria tercentang
- [ ] Test suite hijau (task + regresi task sebelumnya)
- [ ] Log bug hunt pass terakhir = 0 bug
- [ ] `ai-close --summary "<ringkasan>" --knowledge "<temuan durable>"` dijalankan
- [ ] Task berikutnya TIDAK dimulai sebelum ini

## 5. Integrasi ai-toolkit

| Momen | Aksi |
|-------|------|
| Mulai task | `ai-start "<task id>"` → current-task.md + ai-state.json |
| Selesai implement | centang AC, jalankan test |
| Review pass 1 | self-review, fix, catat hasil |
| Bug hunt 2..N | `ai-task review` / `ai-task pass "<hasil>"` (reviewer beda session) |
| Selesai | `ai-close --summary --knowledge` |
| Status | `ai-task list` / `ai-status` |

## 6. Grounding Rules (WAJIB — anti-halusinasi untuk AI baru)

Setiap AI yang masuk workspace (tools baru / ganti tools / resume session) WAJIB:

1. Baca `START_HERE.md` (jika ada di root project) → `AGENTS.md` → `docs/ai/development-protocol.md` → `docs/ai/conventions.md` → `docs/ai/goals.md` → task aktif.
2. **Verifikasi, jangan ingat**: endpoint/tabel/field yang disebut task → grep ke ARD/ERD dulu. Enum status → salin dari state machine, jangan dari memori.
3. **DILARANG mengarang**: API path, kolom DB, status enum, format nomor, config key yang tidak ada di dokumentasi.
4. **DILARANG mengubah keputusan** yang sudah tercatat di decisions.md / dokumentasi project — task berisi referensi inline.
5. Ragu konvensi → cek conventions.md. Ragu struktur → tiru modul serupa yang sudah ada.
6. Konteks lama hilang (session baru) → baca `docs/ai/handoff.md` + `docs/ai/current-task.md` + `docs/ai/knowledge.md` SEBELUM mulai.
7. Kalau task tidak jelas → TANYA PM, jangan asumsi.

## 7. Onboarding Checklist (AI tools baru / ganti tools)

```
[ ] Baca START_HERE.md (jika ada)
[ ] Baca AGENTS.md (auto-load di sebagian tools)
[ ] Baca development-protocol.md (cara kerja: lifecycle + bug hunt)
[ ] Baca conventions.md (standar kode)
[ ] Baca goals.md (tujuan project + prioritas)
[ ] Baca current-task.md + handoff.md + knowledge.md (konteks sesi)
[ ] Baca task aktif docs/ai/tasks/T-XXX.md (self-contained)
[ ] Jalankan test suite (regresi) — pastikan baseline hijau
[ ] Mulai: ai-start "<task id>"
```
