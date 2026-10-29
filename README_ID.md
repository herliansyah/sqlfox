<div align="center">

  <a href="https://github.com/herliansyah/sqlfox">
    <img src="docs/assets/logo.svg" alt="SqlFox Logo" width="120" height="120" />
  </a>

  <h1>SqlFox</h1>

  <p><b>Database Manager &amp; Headless CLI Ringan untuk PostgreSQL, MySQL, MariaDB, dan SQLite</b></p>

[![GitHub Release](https://img.shields.io/github/v/release/herliansyah/sqlfox?style=flat-square&color=FF6B35&logo=github)](https://github.com/herliansyah/sqlfox/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/herliansyah/sqlfox?style=flat-square&color=00ADD8&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-F59E0B?style=flat-square)](https://opensource.org/licenses/MIT)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-4B5563?style=flat-square&logo=linux)](https://github.com/herliansyah/sqlfox/releases)
[![Arch](https://img.shields.io/badge/arch-amd64%20%7C%20arm64-6366F1?style=flat-square)](https://github.com/herliansyah/sqlfox/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/herliansyah/sqlfox/ci.yml?branch=master&label=CI&style=flat-square&logo=githubactions)](https://github.com/herliansyah/sqlfox/actions)

<br />

[ 🇬🇧 English ](README.md) &nbsp;•&nbsp; [ 🇮🇩 Bahasa Indonesia ](README_ID.md) &nbsp;•&nbsp; [ User Guide (EN) ](docs/USER_GUIDE.md) &nbsp;•&nbsp; [ Panduan Pengguna (ID) ](docs/USER_GUIDE_ID.md)

</div>

---

**SqlFox** adalah aplikasi manajemen basis data dan CLI otomatisasi mandiri (*single-binary*) dengan antarmuka web modern bawaan. Dirancang untuk developer, engineer, dan tim database administrator yang mengutamakan performa tinggi, proteksi keamanan data, dan efisiensi operasional tanpa beban aplikasi desktop tradisional.

SqlFox beroperasi tanpa dependensi runtime eksternal (tanpa Electron, tanpa JVM, bebas CGO), menjaga pagu konsumsi memori yang ketat (**< 100MB RAM**), waktu start sub-detik (**< 1 detik**), dan menegakkan sistem pengaman deterministik (*Safety Guard*) sebelum operasi mutasi menyentuh server database.

> Dokumentasi teknis dan panduan operasional lengkap tersedia di **[Panduan Pengguna & Playbook DBA](docs/USER_GUIDE_ID.md)** serta **[Spesifikasi Arsitektur](ARCHITECTURE.md)**.

### Ringkasan Kemampuan Utama

#### Arsitektur & Performa
- **Binary Mandiri**: Distribusi berkas tunggal tanpa dependensi instalasi. Dikompilasi menggunakan driver murni Go (`CGO_ENABLED=0`) untuk platform Linux, macOS, dan Windows.
- **Plafon Memori Ketat**: Beroperasi di bawah batas konsumsi `< 100MB RAM` pada skema skala besar (1.000+ tabel, 50.000+ kolom).
- **Virtual Data Grid**: Virtualisasi baris DOM dengan tinggi seragam (32px) yang mempertahankan `< 500` node DOM aktif pada dataset 100.000+ baris data, meniadakan lag peramban.
- **Streaming Kueri Rendah Latensi**: Pengiriman data hasil kueri berbasis NDJSON (`/api/session/{id}/execute-stream`) dengan pembersihan instan 20 baris pertama untuk Time to First Row (TTFR) di bawah 50ms, didukung terminasi via `AbortController`.
- **Bilah Paginasi Bawah Terpadu**: Navigasi baris terpusat dengan loncat halaman langsung, pilihan ukuran per halaman (25, 50, 100, 200), kalkulasi total baris on-demand, dan pembungkusan wildcard `%...%` otomatis pada filter.

#### Proteksi Keamanan & Tata Kelola Mutasi
- **Inspeksi AST Deterministik**: Membedah struktur sintaks kueri sebelum dikirim ke database untuk mencegat operasi destruktif (`UPDATE`/`DELETE` tanpa `WHERE`, `DROP`, `TRUNCATE`).
- **Estimasi Mutasi Pra-Eksekusi**: Mengubah perintah `UPDATE` dan `DELETE` menjadi probe `SELECT COUNT(*)` terikat batas waktu 500ms untuk menghitung baris terdampak secara presisi sebelum konfirmasi eksekusi.
- **Afirmasi Bertingkat (Tiered Affirmation)**: Proteksi adaptif dengan eksekusi senyap untuk mutasi satu baris PK di Development, dialog 1-klik untuk 1–100 baris, dan Challenge Modal ketik manual untuk mutasi massal atau seluruh operasi di Staging/Production.
- **Penguraian Komentar Eksekutabel**: Mengurai komentar tereksekusi MySQL (`/*! ... */`) menjadi perintah SQL terbuka untuk menutup celah bypass keamanan.
- **Smart Backup Otomatis Pra-DDL**: Mencadangkan struktur DDL dan data baris tabel (<50MB) ke penyimpanan lokal sebelum eksekusi perubahan skema dijalankan.

#### Operasi DBA & Tanggap Insiden
- **Processlist Real-Time**: Pemantauan thread server aktif dengan auto-refresh berkala (1s, 2s, 5s), filter status, dan terminasi query out-of-band (`KILL QUERY`, `KILL CONNECTION`).
- **Terminasi Batch & Proteksi Thread Kebal**: Penghentian kueri macet atau koneksi menganggur secara massal dengan kebijakan fail-closed dan pelindung otomatis untuk thread sistem dan koneksi operator sendiri.
- **Triage Root Blocker**: Mengisolasi thread pemblokir utama saat terjadi badai lock contention menggunakan data `sys.innodb_lock_waits` dan `pg_locks`, menampilkan SQL lengkap scrollable dan tombol mitigasi terpisah.
- **Advisor Indeks Heuristik**: Rekomendasi indeks berbasis aturan ESR (Equality, Sort, Range) dengan deteksi index bloat (>5 indeks sekunder) dan sintaks DDL non-blocking (`ALGORITHM=INPLACE` pada MySQL, `CONCURRENTLY` pada PostgreSQL).
- **Perekam Telemetri (Flight Recorder)**: Perekam performa on-demand per 1 detik yang mencatat QPS, `Threads_running`, row lock waits, dan buffer pool waits, dikorelasikan langsung dengan teks query aktif.
- **Inspektur Replikasi**: Telemetri adaptif untuk MySQL 8.4 LTS / 9.x modern (`SHOW REPLICA STATUS`), MySQL 5.7 / 8.0 lawas, replikasi MariaDB Galera (Flow Control), dan status PostgreSQL standby (`pg_stat_replication`).

#### Sinkronisasi Skema & Data
- **Structure Sync**: Perbandingan DDL berbasis AST antar-database, menghasilkan ringkasan perbedaan terstruktur dan skrip migrasi berurutan yang aman.
- **Cross-Connection Data Transfer**: Streaming transfer data antar-koneksi langsung dengan batas buffer memori ketat (<50MB) dan perlindungan rollback transaksi.
- **Row-Level Data Sync**: Rekonsiliasi data baris per baris berbasis primary key yang menghitung delta insert, update, dan delete secara presisi.
- **Visual Table Designer**: Manajemen struktur tabel in-place tanpa lonjakan kursor pengetikan, dilengkapi builder ENUM/SET interaktif, pembuat indeks komposit visual, dan validasi kompatibilitas Foreign Key.
- **Kanvas Visual ERD**: Visualisasi diagram relasi antar-tabel dengan garis kurva Bézier SVG dinamis, auto-layout, dan navigasi viewport.

#### Keamanan Kredensial & Akses Jaringan
- **Connection Vault Terenkripsi**: Kredensial dan private key SSL klien (`SSLKey`) dienkripsi secara persisten dengan **AES-256-GCM** dan KDF **Argon2id**.
- **Perlindungan Ingress Ketat**: Binding loopback default (`127.0.0.1`), proteksi DNS rebinding, dan validasi Same-Origin identik bita-demi-bita penangkal serangan CSRF lintas-port.
- **Akses Jaringan Lokal (LAN)**: Akses multi-perangkat via flag `--lan` yang dilindungi autentikasi Master PIN dan karantina otomatis terhadap brute force.
- **AI Copilot Sadar Privasi (BYOK)**: Integrasi AI cloud (Claude, OpenAI, Gemini, DeepSeek) atau lokal (Ollama) dengan garansi privasi data—hanya DDL skema yang dikirim, data baris tabel tidak pernah keluar dari mesin lokal.

---

### Panduan Cepat Penggunaan

#### Berkas Binary Siap Pakai

Unduh paket binary sesuai sistem operasi Anda di halaman [GitHub Releases](https://github.com/herliansyah/sqlfox/releases):

| Sistem Operasi | Arsitektur CPU | Format Berkas |
| :--- | :--- | :--- |
| **Linux** | `x86_64` (amd64), `ARM64` | `.tar.gz` |
| **macOS** | Apple Silicon (`arm64`), Intel (`amd64`) | `.tar.gz` |
| **Windows** | `x86_64` (amd64), `ARM64` | `.zip` |

**Linux (amd64):**
```bash
curl -sL https://github.com/herliansyah/sqlfox/releases/download/v0.10.13/sqlfox-v0.10.13-linux-amd64.tar.gz | tar -xz
./sqlfox
```

**macOS (Apple Silicon):**
```bash
curl -sL https://github.com/herliansyah/sqlfox/releases/download/v0.10.13/sqlfox-v0.10.13-darwin-arm64.tar.gz | tar -xz
./sqlfox
```

#### Docker Container

```bash
docker run -d \
  --name sqlfox \
  -p 8080:8080 \
  -v sqlfox_data:/data \
  --add-host=host.docker.internal:host-gateway \
  ghcr.io/herliansyah/sqlfox:latest
```

Atau menggunakan Docker Compose:
```bash
docker compose up -d
```

#### Instalasi via Go Toolchain / Kompilasi Source Code

```bash
# Install menggunakan Go toolchain
go install github.com/herliansyah/sqlfox/cmd/sqlfox@latest

# Kompilasi dari source code
git clone https://github.com/herliansyah/sqlfox.git
cd sqlfox
go build -o sqlfox ./cmd/sqlfox
```

---

### Opsi Perintah CLI

SqlFox dapat beroperasi sebagai server web desktop interaktif maupun sebagai suite CLI otomatisasi tanpa tampilan grafis (headless):

```text
Penggunaan:
  sqlfox [command] [flags]
  sqlfox [server-flags]    (menjalankan web UI desktop)

Perintah (Commands):
  query     Eksekusi query atau script SQL dengan streaming format (table, json, ndjson, csv)
  transfer  Transfer data langsung antar-koneksi database (streaming <50MB RAM)
  job       Jalankan pipeline otomatisasi database deklaratif
  advise    Analisis query SQL untuk rekomendasi indeks ESR dan peringatan bloat
  help      Tampilkan bantuan perintah

Opsi Server:
  -port int          Port server (default 8080, otomatis mencari port jika sibuk)
  -host string       Interface jaringan yang di-bind (default "127.0.0.1")
  -lan               Aktifkan akses jaringan LAN & tablet (bind 0.0.0.0 dan tampilkan Master PIN)
  -pass string       Master password enkripsi vault (atau baca dari ~/.sqlfox/master.key)
  -db string         Lokasi database vault konfigurasi (default: ~/.sqlfox/config.db)
  -no-browser        Jangan membuka browser secara otomatis saat mulai
  -strict-port       Keluar jika port yang ditentukan sedang terpakai
  -log-level string  Tingkat verbosity log: debug, info, warn, error, silent (default "info")
  -log-file string   Target file untuk menyimpan output log secara persisten
  -log-format string Format log: text, json (default "text")
  -silent, -q        Bungkam output konsol terminal (log tetap ditulis ke file jika diset)
```

#### Contoh Penggunaan CLI

```bash
# Menjalankan server web desktop di port 9090 tanpa membuka browser otomatis
sqlfox -port 9090 -no-browser

# Menjalankan kueri non-interaktif dan mencetak output format JSON
sqlfox query --dsn "root:pass@tcp(127.0.0.1:3306)/mydb" --sql "SELECT id, name FROM users" --format json

# Menerima kueri via pipeline STDIN pada runner CI/CD
echo "SELECT count(*) FROM orders WHERE status = 'pending';" | sqlfox query --dsn "$DATABASE_URL" --format csv

# Menganalisis saran indeks pada kueri yang lambat
sqlfox advise --dsn "$DATABASE_URL" --sql "SELECT * FROM orders WHERE status = 'pending' ORDER BY created_at"
```

---

### Arsitektur Sistem

```
┌────────────────────────────────────────────────────────────────────────┐
│ Browser Pengguna / Tablet / Klien LAN                                  │
│        ▲                                                               │
│        │ Local HTTP REST & SSE (Terproteksi Master PIN & Origin Guard) │
│        ▼                                                               │
│ Go Backend Process (< 100MB RAM, Single Static Binary)                 │
│   ├── Safety Guard: Analisis AST Deterministik & Probe Baris Terdampak │
│   ├── Session Manager: Deduplikasi Singleflight & Control Pool         │
│   ├── Virtual Data Grid: Windowing Baris DOM 32px & Streaming NDJSON   │
│   ├── AI Firewall: Sanitasi Konteks Khusus Skema DDL (BYOK)            │
│   └── Connection Vault: Enkripsi AES-256-GCM & Argon2id                │
└────────┬─────────────────────────────────────────────┬─────────────────┘
         │                                             │
         │ TLS / Socket Lokal (MySQL, PG, SQLite)      │ HTTPS (Hanya Metadata DDL)
         ▼                                             ▼
[ Server Basis Data Target ]                  [ API Provider AI ]
(MySQL, MariaDB, PostgreSQL, SQLite)          (Claude, OpenAI, Gemini, Ollama)
```

---

### Lisensi

Didistribusikan di bawah lisensi terbuka [MIT](LICENSE).
