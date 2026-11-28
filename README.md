# aitk

**Bahasa Indonesia** · [English](README.en.md)

**Asisten development untuk AI coding agent: kontinuitas, standar, persona SDLC, dan kendali mutu.**
aitk menyimpan status kerja proyek di git sebagai file teks biasa yang bisa dibaca semua agen: Claude Code, Codex, OpenCode, Gemini CLI, Copilot, Cursor, Windsurf, Cline, Roo Code, Aider, Kiro, Junie, Qwen Code, atau manusia. Agen mana pun bisa melanjutkan pekerjaan agen lain, beberapa agen bisa bekerja paralel tanpa bentrok, dan sebuah task baru dianggap *selesai* setelah diverifikasi mesin. Agen bekerja dengan cara ini sendiri: Anda tidak perlu menjelaskan aitk di prompt.

```bash
curl -fsSL https://raw.githubusercontent.com/innaka-tech/ai-toolkit/main/install.sh | sh
aitk setup             # sekali per mesin: AI tool Anda langsung mengenali proyek aitk
cd repo-anda
aitk init              # blok AGENTS.md, aitk.toml, docs/ai/
aitk adapters sync     # file proyek untuk AI tool yang Anda pakai (commit untuk tim)
```

Lalu cukup minta pekerjaannya ke agen ("tambahkan export CSV"). Agen membaca brief, mengambil task, menjalankan check, dan menutup task lewat Definition of Done.

## Fitur utama

| Kebutuhan | Fitur |
|---|---|
| Agen tidak lupa antar sesi | `aitk brief`: satu ringkasan terbatas (≤ 4k token) berisi task aktif, handoff terakhir, konvensi, dan knowledge yang relevan |
| "Selesai" benar-benar selesai | `aitk close` menolak sampai check lolos di kode terakhir dan semua acceptance criteria terpenuhi |
| Perbaikan tidak merusak bagian lain | Bug fix wajib punya test regresi; `aitk impact` menunjukkan kode yang terdampak dan yang belum tercakup test |
| Standar development yang sama di semua proyek | `aitk standards`: 13 standar bawaan, bisa ditimpa per proyek |
| Peran SDLC seperti tim sungguhan | Persona PM, analyst, architect, DBA, UX, QA, security, DevOps, auditor, writer, dan team lead |
| Mengoreksi, bukan hanya mengusulkan | Persona audit dengan `fix` langsung memperbaiki temuan (test dulu); sisanya jadi task |
| Otomatis tanpa slash command | `[assistant]` di `aitk.toml`: karakter, gaya bicara, dan persona dipilih sendiri di setiap sesi |
| Agen paralel tidak bentrok | Klaim task, satu worktree dan branch per task |
| Aturan tidak dilewati | Gate berjalan di git hooks, CI, server MCP, dan `aitk run`, bukan di prompt |
| Semua tool dikonfigurasi sekaligus | `aitk setup` dan `aitk adapters sync` |

## Cara kerja

```
1. aitk brief                  → agen membaca satu brief
2. aitk task next --start      → atau: aitk task new "<judul>" --ac "Given …, when …, then …"
3. kerjakan; aitk check        → menjalankan test, mencatat bukti; aitk impact menunjukkan dampak perubahan
4. aitk close --summary "…" --knowledge "…|none"
```

`close` menerapkan **Definition of Done** sesuai profil risiko perubahan, yang dipilih otomatis dari diff:

| Profil | Kapan | Syarat |
|---|---|---|
| `lite` | ≤ 3 file dan ≤ 100 baris | check lolos di kode terakhir, summary, knowledge; kriteria opsional, tapi yang ditulis wajib terpenuhi |
| `standard` | default | + semua acceptance criteria terpenuhi, dan review sampai satu pass menemukan 0 (`[quality] review = "standard"`, aktif di proyek baru) |
| `strict` | menyentuh `risk.sensitive_paths` (auth, migrasi, infra…) | + analisis risiko (STRIDE), checklist OWASP ASVS, audit keamanan lolos, dan dua review pass (yang terakhir tanpa temuan, salah satunya pass bersih dari tool yang tidak ikut mengerjakan task) |

**Review sampai 0, baru pindah task.** Agen mencatat `aitk review pass --findings N`, memperbaiki temuan, dan mengulang sampai satu pass menemukan 0; sebelum itu task tertahan di `in_review`. Selama task aktif belum selesai, `aitk task start` untuk task lain ditolak (`E_TASK_ACTIVE`): selesaikan, atau parkir dengan `aitk task block <id> --reason "…"`. Setelah tiga pass atau lebih, kalau pass terakhir masih menemukan masalah dan tidak lebih sedikit dari pass sebelumnya, brief menyuruh agen berhenti dan bertanya ke Anda.

Di semua profil, task bertag `bug` wajib menyertakan perubahan file test, dan penerimaan oleh manusia bisa diwajibkan (`uat.required`). Saat `close` menolak, pesan error-nya menyebut persis perintah berikutnya (`fix: aitk task update T-k3m9 --ac-done 2`), jadi model kecil pun bisa pulih sendiri.

## Asisten development

### Standar bawaan

`aitk standards [topik]` menyajikan standar ke setiap proyek dan setiap agen, tanpa menyalin file:

| Topik | Isi |
|---|---|
| `character` | Cara agen bekerja: perubahan terkecil yang berfungsi, tidak memangkas keamanan, mengoreksi temuan, menutup jawaban dengan Skipped/Risk |
| `sdlc` | Fase, persona per fase, Definition of Ready dan Done, kelas perubahan |
| `engineering` | Desain, kode, dependensi, logging |
| `testing` | Apa yang wajib dites, checklist edge case, coverage |
| `security` | Analisis risiko (STRIDE), kontrol OWASP Top 10 / ASVS L2, tingkat keparahan, insiden |
| `api` | Kontrak OpenAPI, error RFC 9457, pagination, idempotency, versioning |
| `database` | Integritas, migrasi expand/contract, query, backup dan restore |
| `operations` | Deploy, observability (RED/USE, SLO), backup dan DR, hardening |
| `git` | Branch, Conventional Commits, pull request, SemVer, changelog |
| `review` | Urutan review, format temuan, independensi |
| `documentation` | Dokumen apa menyimpan apa |
| `ai-agents` | Izin dan pengawasan agen, fitur LLM (OWASP LLM Top 10) |
| `ux` | WCAG 2.1 AA dan aturan usability |

File `docs/ai/standards/<topik>.md` menimpa topik bawaan atau menambah topik baru (misalnya aturan domain logistik Anda). `aitk debt` mendaftar komentar `shortcut: <batasan>, <kapan upgrade>` yang mencatat utang teknis yang disengaja. Lihat [standar](docs/how-to/standards.md).

### Persona SDLC

| Fase | Persona |
|---|---|
| Semua | `/aitk-team`: menentukan fase permintaan lalu menjalankan persona yang tepat, fase demi fase |
| Discover | `/aitk-pm`: masalah, metrik hasil (`aitk goal add`), scope MoSCoW |
| Analyse | `/aitk-analyst`: alur, tabel status, aturan bisnis, kriteria Given/when/then |
| Design | `/aitk-architect` (opsi dengan TCO, ADR), `/aitk-dba`, `/aitk-ux` |
| Plan dan Build | `/aitk-plan`, `/aitk-next`, `/aitk-fix` (test dulu) |
| Verify | `/aitk-review` (reviewer independen), `/aitk-qa`, `/aitk-security`, `/aitk-hunt` |
| Accept | `/aitk-uat`: skrip UAT; hanya manusia yang menerima |
| Release dan Operate | `/aitk-release`, `/aitk-devops`, `/aitk-auditor` (scorecard proyek terhadap semua standar), `/aitk-writer` |

Persona audit (`dba`, `ux`, `qa`, `security`, `devops`, `auditor`) menerima kata `fix` di akhir argumen untuk langsung memperbaiki temuan; tanpa `fix`, setiap temuan menjadi task dengan kriteria yang bisa direproduksi. Perintah lain: `/aitk-start`, `/aitk-check`, `/aitk-impact`, `/aitk-close`, `/aitk-handover`, `/aitk-status`.

Semua perintah terpasang native di Claude Code, Codex, OpenCode, Gemini CLI, Qwen Code, Cursor, Copilot, dan Windsurf, dan disajikan sebagai prompt MCP. File markdown di `docs/ai/commands/` menjadi slash command Anda sendiri di semua tool tim. `aitk prompt <nama> <teks>` mencetak perintah apa pun untuk skrip (`aitk prompt aitk-hunt payments | claude -p`). Lihat [slash command dan persona](docs/how-to/commands.md).

### Karakter dan gaya bicara

- **Karakter lean senior engineer** (diadaptasi dari [Ponytail](https://github.com/DietrichGebert/ponytail), MIT): perubahan terkecil yang benar-benar berfungsi, tidak pernah memangkas validasi, error handling, keamanan, atau aksesibilitas, dan setiap jawaban ditutup dengan **Skipped:** dan **Risk:**. Mode: `lite`, `full`, `ultra`, `off` (`/aitk-lean`).
- **Caveman**: jawaban singkat dan hemat token (`/aitk-caveman lite|full|off`). Kode, perintah, path, error, peringatan, pertanyaan untuk Anda, dan baris Skipped/Risk tidak pernah dipotong.

### Otomatis di setiap sesi

Setiap agen selalu mulai dari `aitk brief`, jadi apa pun yang ada di brief berlaku tanpa slash command:

```toml
# aitk.toml
[assistant]
character = "full"    # lite | full | ultra | off
talk = "normal"       # normal | lite | caveman
auto_persona = true   # brief memilih persona dan standar yang dibutuhkan task
```

| Sinyal | Persona yang dijalankan sebelum close |
|---|---|
| Belum ada task aktif | `aitk-team` |
| Task bertag `bug` | `aitk-fix` |
| Task bertag `security` atau berprofil strict | `aitk-security` |
| Migrasi, `.sql`, `schema.prisma` berubah | `aitk-dba` |
| File UI berubah (`.tsx`, `.vue`, `.css`, Blade, …) | `aitk-ux` |
| Dockerfile, compose, CI, Terraform, Helm, k8s berubah | `aitk-devops` |
| OpenAPI/Swagger atau `routes/` berubah | `aitk-architect` |
| Kode berubah tanpa perubahan test | `aitk-qa` |

Satu orang bisa memakai gaya sendiri tanpa mengubah file tim: `AITK_TALK=caveman`, `AITK_CHARACTER=ultra`. Yang sengaja tetap manual: review independen untuk task strict (harus tool lain) dan penerimaan UAT (hanya manusia). Lihat [asisten otomatis](docs/how-to/assistant.md).

## Bagaimana agen tahu harus berbuat apa

Tidak ada yang perlu menyuruh agen memakai aitk. Empat lapisan memastikannya:

| Lapisan | Fungsi | Dipasang oleh |
|---|---|---|
| **Instruksi tingkat user** | Bagian pendek di instruksi user setiap AI tool: repository dengan `aitk.toml` dikelola aitk dan agen mulai dari `aitk brief`. Berlaku di semua repository aitk di mesin itu, termasuk clone baru. | `aitk setup` (sekali per mesin; `--remove` membatalkan) |
| **AGENTS.md** | Blok aitk di repository: lima langkah, hanya perintah shell, dibaca native oleh kebanyakan agen. | `aitk init` |
| **File khusus tool** | Untuk tool yang tidak membaca AGENTS.md sendiri: file aturan yang menunjuk ke sana, server MCP, dan hook sesi yang menjalankan `aitk brief`. | `aitk adapters sync` |
| **Server MCP** | 22 tool (`brief`, `task_next`, `check`, `impact`, `close`, `standards`, `debt`, …) dan setiap slash command sebagai prompt. Instruksinya menyuruh agen memanggil `brief` lebih dulu. | dipasang oleh kedua perintah di atas |

| Tool | Repository (`adapters sync`) | Mesin (`setup`) |
|---|---|---|
| Claude Code | `CLAUDE.md` → `@AGENTS.md`, `.mcp.json`, SessionStart hook, Agent Skill | `~/.claude/CLAUDE.md`, Agent Skill tingkat user, commands, MCP via `claude mcp add --scope user` |
| Codex CLI | `AGENTS.md` (native) | `~/.codex/AGENTS.md`, MCP di `~/.codex/config.toml`, prompts, Agent Skill |
| OpenCode | `AGENTS.md` (native), MCP di `opencode.json` | `~/.config/opencode/AGENTS.md`, MCP di `opencode.json` |
| Gemini CLI | `.gemini/settings.json`: AGENTS.md sebagai konteks, MCP, SessionStart hook | `~/.gemini/GEMINI.md`, MCP di `settings.json` |
| GitHub Copilot (VS Code) | `.github/copilot-instructions.md`, `.vscode/mcp.json` | |
| Cursor | `.cursor/rules/aitk.mdc` (selalu aktif), `.cursor/mcp.json` | |
| Windsurf | `.windsurf/rules/aitk.md` (selalu aktif) | `~/.codeium/windsurf/memories/global_rules.md` |
| Cline | `.clinerules` | |
| Roo Code | `.roo/rules/aitk.md`, `.roo/mcp.json` | |
| Kiro | `.kiro/steering/aitk.md`, `.kiro/settings/mcp.json` | `~/.kiro/steering/aitk.md` |
| Aider | `.aider.conf.yml`: `read: [AGENTS.md]` | |
| Junie | `.junie/guidelines.md` | |
| Qwen Code | `.qwen/settings.json`: AGENTS.md sebagai konteks, MCP | `~/.qwen/QWEN.md` |
| Antigravity CLI (`agy`) | `AGENTS.md` (native) | `~/.gemini/antigravity-cli/AGENTS.md`, Agent Skill, MCP via `agy mcp add` |
| jcode | `AGENTS.md` (native) | `~/.jcode/prompt-overlay.md`, MCP di `~/.jcode/mcp.json` |
| pi | `AGENTS.md` (native) | `~/.pi/agent/AGENTS.md`, Agent Skill, template prompt (tanpa MCP: pi memakai CLI aitk) |
| Amp, Zed, Jules, Factory, dan lainnya | `AGENTS.md` (native) | |
| tool lain apa pun | `AGENTS.md`; git hooks memaksakan aturan untuk semua orang | |

`adapters sync` mengonfigurasi tool yang terdeteksi; `--tool all` menulis file untuk semua tool, bagi tim yang anggotanya memakai tool berbeda. Pengaturan dan teks Anda sendiri dipertahankan: aitk hanya mengubah bagian bertanda miliknya atau entri `aitk` (file JSON mungkin di-indentasi ulang). File yang tidak aman untuk diedit (tidak terbaca, read-only, JSON berkomentar, penanda yang diedit manual) dilewati dan dilaporkan, tidak pernah ditimpa.

## Semua tersimpan di repository Anda

```
AGENTS.md                 # satu blok aitk; teks Anda sendiri tidak disentuh
aitk.toml                 # perintah check, path sensitif, [assistant], pengaturan run, target deploy, plugin
docs/ai/tasks/            # satu file per task (frontmatter YAML, divalidasi JSON Schema)
docs/ai/handoff/          # satu file per sesi: siapa (tool apa) mengerjakan apa, hasil check, langkah berikutnya
docs/ai/knowledge/        # temuan yang bertahan, per topik
docs/ai/standards/        # standar proyek Anda sendiri (opsional)
docs/ai/conventions.md    # konvensi kode proyek, tampil di setiap brief
docs/adr/                 # keputusan arsitektur (MADR)
```

Tanpa server, tanpa akun, bisa offline. Task aktif disimpan di direktori git tiap worktree, tidak pernah di file bersama yang di-commit.

## Autopilot

```bash
aitk task next                                   # apa yang siap, sesuai urutan dependensi
aitk run --agent claude-code --commit --max-tasks 5
```

`aitk run` mengerjakan task yang siap satu per satu dengan agen headless: `claude-code`, `codex`, `opencode`, `gemini-cli`, atau perintah apa pun (`--cmd`). aitk tidak menerima begitu saja klaim agen bahwa task selesai. aitk memulihkan apa yang tidak boleh diubah agen sendiri (penerimaan user, review, profil risiko, tag, kriteria), menjalankan check dan audit yang diwajibkan sendiri, lalu menerapkan Definition of Done. Pekerjaan yang butuh manusia menunggu; pekerjaan yang belum selesai diserahkan sebagai blocked dengan handoff (dan, dengan `--commit`, di-stash); run berhenti saat macet. Lihat [seberapa jauh verifikasinya](docs/how-to/autopilot.md#how-far-the-verification-goes).

## Lebih dari daftar task

- **Bug hunting**: `aitk audit` menjalankan scanner dependensi (osv-scanner, terpasang bersama aitk), static analysis, dan scan secret; `aitk audit --tasks` mengubah temuan menjadi task perbaikan, satu per paket rentan beserta versi yang memperbaikinya.
- **Kerja paralel**: `aitk work <id>` membuat worktree dan branch, mengklaim task, dan memulainya di sana. Tiga agen di tiga task, di-merge kembali tanpa konflik, termasuk dalam test suite.
- **Pindah tool di tengah task**: `aitk switch codex` menulis handoff dan menjalankan Codex dengan brief.
- **Gate**: `aitk hooks install` menambahkan scan secret, validasi schema, Conventional Commits dengan trailer `AI-Task`/`AI-Tool`, dan check sebelum push. Hook Anda yang sudah ada tetap berjalan. `aitk ci` menjalankan gate yang sama di CI (GitHub Action: `uses: innaka-tech/ai-toolkit/action@v2`).
- **Rencana dari tool lain**: impor Spec Kit (fase dan penanda `[P]` menjadi dependensi; task selesai dicentang di `tasks.md`), OpenSpec, BMAD, Superpowers, GitHub Issues, atau checklist markdown apa pun, lalu jalankan dengan verifikasi aitk.
- **Standar delivery**: checklist OWASP ASVS untuk pekerjaan berisiko, penerimaan oleh manusia (`aitk uat accept`), `aitk release` untuk SemVer, changelog, dan tag dari Conventional Commits, konvensi proyek, goal beserta progresnya.
- **Laporan**: `aitk report --since 7d` menunjukkan apa yang dikerjakan, oleh tool apa, beserta buktinya.
- **Plugin**: executable `aitk-<nama>` apa pun di PATH bisa menambah isi brief, pemeriksaan doctor, dan pencarian knowledge (JSON lewat stdin/stdout).
- **Kendali diri**: aitk menyuruh agen berhenti setelah check gagal berulang, menolak kredensial di apa pun yang dicatatnya, dan menjaga status `done` di belakang Definition of Done.
- **Pemulihan diri**: file aitk yang diedit manual atau konflik diperbaiki sebelum perintah berikutnya (peringatan `self-healed:`), dan `aitk doctor --fix` memulihkan adapter dan pengaturan yang menyimpang. Tidak ada yang dibuang.
- **Migrasi dari v1**: `aitk migrate` tanpa kehilangan data dan idempotent. Setiap file asli disimpan byte demi byte di `docs/ai/_legacy/`.

## Instalasi

| Cara | Perintah |
|---|---|
| Skrip (macOS, Linux) | `curl -fsSL https://raw.githubusercontent.com/innaka-tech/ai-toolkit/main/install.sh \| sh` (memverifikasi SHA-256, dan tanda tangan cosign bila cosign terpasang) |
| Go | `go install github.com/innaka-tech/ai-toolkit/v2/cmd/aitk@latest` |
| Manual / Windows | unduh dari [Releases](https://github.com/innaka-tech/ai-toolkit/releases) dan cek `checksums.txt` |

Satu binary statis, tanpa dependensi runtime selain git. Skrip juga memasang [osv-scanner](https://google.github.io/osv-scanner/) untuk `aitk audit` (lewati dengan `AITK_NO_SCANNER=1`; di tempat lain jalankan `aitk audit install`). Lalu jalankan `aitk setup` sekali.

## Dokumentasi

Dokumentasi rinci masih berbahasa Inggris.

- [Tutorial: proyek pertama dalam 10 menit](docs/tutorial.md)
- Panduan: [setup AI tool](docs/how-to/ai-tools.md) · [slash command dan persona](docs/how-to/commands.md) · [standar](docs/how-to/standards.md) · [asisten otomatis](docs/how-to/assistant.md) · [autopilot: aitk run](docs/how-to/autopilot.md) · [keamanan, UAT, konvensi](docs/how-to/quality.md) · [rilis proyek Anda](docs/how-to/release.md) · [Spec Kit, BMAD, Superpowers](docs/how-to/planning-tools.md) · [kerja paralel](docs/how-to/parallel.md) · [gate dan CI](docs/how-to/ci.md) · [migrasi dari v1](docs/how-to/migrate-v1.md) · [menulis plugin](docs/how-to/plugins.md)
- Referensi: [CLI](docs/reference/cli.md) · [spesifikasi](docs/spec/README.md) · [JSON Schema](schemas/) · [changelog](CHANGELOG.md)
- Penjelasan: [keputusan arsitektur](docs/adr/README.md)

## Status

v2 adalah penulisan ulang toolkit bash v1 (masih tersedia di tag [`v1.0.0`](https://github.com/innaka-tech/ai-toolkit/releases/tag/v1.0.0); perintah `ai-*` tetap berfungsi: diteruskan ke `aitk` di proyek v2 dan menjalankan v1 tanpa perubahan di proyek v1).

- **Migrasi v2**: divalidasi pada 24 proyek v1 sungguhan. Semua termigrasi tanpa kehilangan data, `aitk doctor` melaporkan nol error, dan setiap brief muat dalam 4k token dan tampil dalam sekitar 100 ms.
- **Run otonom**: divalidasi langsung dengan Claude Code dan Codex, yang menyelesaikan task sungguhan lewat `aitk run`, termasuk bug fix dengan test regresi.
- **Review**: fitur otonom melewati tiga putaran review independen ditambah pass oleh Codex dan OpenCode.
- **CI**: seluruh test suite berjalan dengan race detector di Linux, macOS, dan Windows.
- **Dogfooding**: aitk dikembangkan dengan aitk.

## Kontribusi

Lihat [CONTRIBUTING.md](CONTRIBUTING.md). Masalah keamanan: [SECURITY.md](SECURITY.md). Berlisensi [Apache-2.0](LICENSE).
