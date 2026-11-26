# KONVENSI KODE GENERIC (engine level)

> Template konvensi kode untuk SEMUA project ai-toolkit. Salin ke `docs/ai/conventions.md` di project, lalu isi detail spesifik stack project (framework, bahasa, struktur modul).
> Prinsip: 5 AI = 1 gaya kode. Semua AI wajib ikut file ini.

## Template (isi sesuai project)

### 1. Struktur
- <struktur folder project — modular? monolith? domain?>
- <larangan import lintas modul / layer>
- <pola komunikasi antar modul: event? service?>

### 2. Penamaan
| Item | Aturan | Contoh |
|------|--------|--------|
| Tabel | <aturan> | <contoh> |
| Model | <aturan> | <contoh> |
| Migration | <aturan> | <contoh> |
| Endpoint | <aturan> | <contoh> |
| Event | <aturan> | <contoh> |
| Status/enum | <aturan> | <contoh> |
| Pesan error | <aturan> | <contoh> |

### 3. Data sensitif (uang/tanggal/angka)
- <tipe kolom uang — decimal? integer cents?>
- <pembulatan — aturan>
- <timezone>
- <formula wajib jika ada>

### 4. Integritas data
- <tabel transaksi: boleh DELETE? atau hanya status?>
- <soft delete untuk master?>
- <append-only untuk audit?>
- <penomoran dokumen>

### 5. Keamanan (minimal)
- <auth & authorization>
- <validasi input>
- <larangan khusus (v-html, hardcode config)>
- <audit dependency>

### 6. Test
- <framework>
- <naming>
- <test wajib per modul kritis>

### 7. Git
- <format commit message>
- <branch per task?>

---

## Aturan universal (berlaku semua project)

1. **TIDAK boleh hardcode config** — semua nilai dinamis dari config/settings project
2. **TIDAK boleh DELETE data transaksi** — hanya ubah status (audit trail)
3. **TIDAK boleh simpan secret** di repo
4. **Uang & angka** — hindari float, pakai tipe presisi
5. Ragu → cek konvensi project, jangan tebak
