# START HERE TEMPLATE (generic — ai-toolkit engine)

> Template pintu masuk AI untuk SEMUA project. Salin ke root project sebagai `START_HERE.md`, isi bagian dalam kurung siku.
> Tujuan: AI baru / ganti tools paham project ≤ 5 menit tanpa halusinasi.

```markdown
# START HERE — Baca file ini dulu (2 menit)

> **Pintu masuk WAJIB untuk AI tools baru.** Setiap AI yang masuk workspace ini HARUS baca file ini + AGENTS.md sebelum bekerja.

---

## 1. Project ini apa?

**[NAMA PRODUK]** — [1 kalimat deskripsi produk & masalah yang diselesaikan].
**[PEMILIK/CLIENT]**: [siapa] · **Tim:** [peran] · **Durasi:** [X minggu/bulan] · **Status:** [fase]

## 2. Stack

[Stack singkat — framework, DB, infra]

## 3. File WAJIB dibaca (urutan penting)

| Urutan | File | Isi | Waktu |
|--------|------|-----|-------|
| 1 | **AGENTS.md** (root) | Protocol wajib + larangan (auto-load di sebagian tools) | otomatis |
| 2 | **docs/ai/development-protocol.md** | Lifecycle task (5 stage + bug hunt loop) — CARA KERJA | 2 mnt |
| 3 | **docs/ai/conventions.md** | Standar kode project | 2 mnt |
| 4 | **docs/ai/goals.md** | Goals & sub-goals — APA YANG DICAPAI | 1 mnt |
| 5 | **docs/ai/tasks/T-XXX.md** | Task aktif (self-contained: AC + constraint inline) | 2 mnt |

**Referensi (baca SAAT dibutuhkan):** [daftar dokumen arsitektur/PRD/ERD]

## 4. Grounding Rules (WAJIB — anti-halusinasi)

```
1. Task menyebut endpoint/tabel/field → VERIFIKASI ke dokumen (grep dulu), jangan dari ingatan
2. Enum/status → salin dari source of truth, jangan dari memori
3. Ragu konvensi → cek conventions.md, jangan tebak
4. Ragu struktur → tiru modul/file serupa yang sudah ada
5. DILARANG mengarang API/kolom/status/format yang tidak ada di dokumentasi
6. DILARANG mengubah keputusan yang sudah tercatat (decisions.md / dokumentasi)
7. Task tidak jelas → TANYA PM, jangan asumsi
```

## 5. Cara mulai bekerja

```bash
cat docs/ai/current-task.md        # task aktif
# cek daftar task: <file task list project>
ai-start "T-XXX: <judul>"          # mulai task
# lifecycle: IMPLEMENT → self-review (pass 1) → bug hunt (pass 2..N,
# reviewer AI BEDA session, loop sampai 0 bug) → DONE
ai-close --summary "<ringkasan>" --knowledge "<temuan durable>"
```

## 6. Larangan mutlak

- Jangan simpan secret/API key/password di repo
- Jangan pindah ke task lain sebelum task aktif DONE (0 bug pass terakhir)
- Jangan ubah keputusan bisnis/arsitektur tanpa persetujuan PM
```
