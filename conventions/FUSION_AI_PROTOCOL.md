# Fusion-AI Coordination Protocol v1

Standar koordinasi antar agent Anas: **Novara** (MacBook) ↔ **JAKA** (Smarter 10.3.3.241).

## Scope & Data Boundaries (WAJIB — batas koordinasi)

**Yang BOLEH dikoordinasikan Novara ↔ JAKA:**
1. **Yapindo Transportama & turunannya**: PCP Express, PCP Transport, PCP Logistics, Wilog (infra, deploy, monitoring, DB, network, cron, alert).
2. **Ide umum IT & teknologi** (bukan project spesifik).

**Yang TIDAK BOLEH pernah di-share ke JAKA:**
- ❌ Project spesifik lain (mis. ber5/Triks, close-ecommerce, project client lain)
- ❌ Mitra Kualitas Abadi / Catalyst Consulting (semua hal)
- ❌ Data/task/detail project di luar scope di atas

**Aturan:**
- Novara: jangan pernah kirim handoff/report/protokol/context project di luar scope ke JAKA.
- JAKA: kalau menerima konten di luar scope → tolak halus, balas `🤖 [jaka] info — Di luar scope koordinasi (hanya Yapindo/PCP/Wilog + ide IT umum).` dan jangan proses.
- Event log tetap mencatat penolakan tersebut (transparan).
- Ide IT umum boleh didiskusikan, tapi detail project lain tidak pernah.

## Network & Access

| Agent | Host | SSH | Hermes | Telegram |
|---|---|---|---|---|
| Novara | MacBook (10.10.7.3) | local | v2.x (gateway launchd) | Bot sendiri → home DM |
| JAKA | Smarter (10.3.3.241) | `ssh smarter` | v0.19 (venv: `/home/anas.fikri/.hermes/hermes-agent/venv/bin`) | Bot sendiri → Colloseum |

**PENTING:** `ssh wireguard` = Hub (10.3.3.225), BUKAN JAKA. JAKA selalu `ssh smarter`.

## Coordination Channel

- **Group Telegram: Colloseum** (`-1004377389254`) — semua komunikasi antar agent + notifikasi ke Anas.
- No thread baru. No DM.

## Sending Messages (Agent → Colloseum)

### JAKA → Colloseum (via own bot, no webhook needed)
```bash
# Script (recommended)
~/.ai-toolkit/scripts/ai-agent-report-jaka <from> <event> <message>

# Raw (hermes send)
export PATH="/home/anas.fikri/.hermes/hermes-agent/venv/bin:$PATH"
hermes send -t telegram:-1004377389254 "message"
```

### Novara → Colloseum
```bash
# Script — webhook route agent-coord (HMAC V2)
~/.ai-toolkit/scripts/ai-agent-report <from> <event> <message>

# Raw curl (must sign)
# POST http://10.10.7.3:8644/webhooks/agent-coord
# Headers: X-Webhook-Timestamp: <unix>  X-Webhook-Signature-V2: <hex hmac-sha256("<ts>.<body>")>
```

## Handoff (Task Passing)

```bash
ai-handoff <to-agent> <task> [--project NAME] [--scope tags]
# → writes ~/.ai-toolkit/bridge/inbox/<task-id>.json + appends coordination-events.jsonl
```

Remote (JAKA side): `ai-handoff-remote <to-agent> <task> [--project NAME]`

## Event Log

Append-only JSONL di `~/.ai-toolkit/logs/coordination-events.jsonl` (kedua agent).
Format:
```json
{"ts":"...","event":"handoff_sent|report|done|alert","from":"novara|jaka","to":"...","task_id":"...","message":"..."}
```

## Message Format (Colloseum)

```
🤖 [<agent>] <event-type>
<message>
```
Event types: `deploy`, `alert`, `handoff`, `done`, `test`, `info`.

## Standardized Event Templates

| Event | Format |
|---|---|
| Deploy selesai | `🤖 [jaka] deploy — <project> <env>: <status> (<durasi>)` |
| Alert | `🤖 [jaka] alert — <service>: <severity> <detail>` |
| Handoff | `🤖 [novara] handoff → jaka: <task> (project: <name>)` |
| Done | `🤖 [jaka] done — <task_id>: <ringkasan hasil>` |
| Info | `🤖 [novara] info — <pesan>` |

## Guardrails

- JAKA → no infra changes without Anas approval (post plan to Colloseum first).
- Novara → no prod push without Anas review.
- Approval gate: Anas reacts/mentions in Colloseum.
- Event log append-only, never delete.

## Turn-Taking Protocol (WAJIB — anti rebutan)

**Prinsip: satu giliran bicara per pesan. Agent yang menerima handoff/harapan menunggu sampai pekerjaan selesai, BARU balas.**

### Aturan dasar
1. **Require-mention enforced** (sudah aktif di kedua agent): agent TIDAK pernah auto-reply ke pesan agent lain di Colloseum kecuali di-mention (`@Novara` / `@R0g0_bot`) atau di-reply langsung.
2. **Siapa yang mulai**: 
   - Anas mention `@novara` → Novara kerja, balas ke Colloseum, **berhenti**.
   - Anas mention `@jaka` → JAKA kerja, balas, **berhenti**.
   - `🤖 [jaka] handoff → novara: <task>` → NOVARA yang lanjut. JAKA diam sampai Novara selesai & balas.
   - `🤖 [novara] handoff → jaka: <task>` → JAKA yang lanjut. Novara diam.
3. **Saat menerima handoff**: agent penerima kerja, lalu balas dengan `🤖 [<agent>] done — <task_id>: <hasil>`. Agent pengirim TIDAK membalas pesan `done` itu (tugas sudah berakhir) kecuali ada error yang butuh follow-up — dan kalau pun, mention agent yang dimaksud.
4. **Saat nunggu**: agent yang menunggu TIDAK posting apa pun. Diam = menunggu. Hanya Anas yang bisa interupsi.
5. **Kalau dua-duanya kena trigger bersamaan** (misal Anas mention keduanya dalam satu pesan): Novara jalan duluan (coding), JAKA tunggu sampai Novara balas, baru JAKA lanjut (infra). Urutan default: **Novara → JAKA**.
6. **Reply vs new message**: balasan harus berupa reply ke pesan yang dituju (bukan pesan baru berdiri sendiri) supaya jelas rantai percakapannya.
7. **Error/follow-up**: kalau penerima handoff nemu masalah, dia balas dengan `🤖 [<agent>] alert — <task_id>: <detail>` dan mention agent pengirim (`@novara` / `@R0g0_bot`). Baru agent pengirim boleh balas.

### Alur handoff lengkap (contoh)
```
1. [novara] handoff → jaka: Cek health PG (project: ber5)   ← Novara selesai bicara
2. [jaka] (kerja diam-diam, tanpa posting)
3. [jaka] done — handoff-xxx: PG sehat, 0 error               ← JAKA selesai bicara
4. (Colloseum diam sampai Anas kasih instruksi berikutnya)
```

### Yang TIDAK boleh
- ❌ Balas pesan agent lain tanpa di-mention/di-reply
- ❌ Balas pesan `done` dengan konfirmasi (noise)
- ❌ Nge-post status "lagi kerja..." — cuma hasil akhir
- ❌ Rebutan: dua agent kerja task yang sama
