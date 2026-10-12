<div align="center">

  <a href="https://github.com/herliansyah/sqlfox">
    <img src="docs/assets/logo.svg" alt="SqlFox Logo" width="128" height="128" />
  </a>

  <h1>SqlFox</h1>

  <p><b>High-Performance Modern Database Manager &amp; Viewer with AI Copilot &amp; Safety Guard</b></p>

[![GitHub Release](https://img.shields.io/github/v/release/herliansyah/sqlfox?style=flat-square&color=FF6B35&logo=github)](https://github.com/herliansyah/sqlfox/releases)
[![Go Version](https://img.shields.io/github/go-mod/go-version/herliansyah/sqlfox?style=flat-square&color=00ADD8&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-F59E0B?style=flat-square)](https://opensource.org/licenses/MIT)
[![Sponsor: Saweria](https://img.shields.io/badge/Sponsor-Saweria-F97316?style=flat-square&logo=coffeescript)](https://saweria.co/herliansyah26)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-4B5563?style=flat-square&logo=linux)](https://github.com/herliansyah/sqlfox/releases)
[![Arch](https://img.shields.io/badge/arch-amd64%20%7C%20arm64-6366F1?style=flat-square)](https://github.com/herliansyah/sqlfox/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/herliansyah/sqlfox/ci.yml?branch=master&label=CI&style=flat-square&logo=githubactions)](https://github.com/herliansyah/sqlfox/actions)
[![GitHub Stars](https://img.shields.io/github/stars/herliansyah/sqlfox?style=flat-square&color=EAB308&logo=star)](https://github.com/herliansyah/sqlfox/stargazers)

<br />

**[ 🇬🇧 English ](#-english)** &nbsp;•&nbsp; **[ 🇮🇩 Bahasa Indonesia ](#-bahasa-indonesia)** &nbsp;•&nbsp; **[ 📖 User Guide (EN) ](docs/USER_GUIDE.md)** &nbsp;•&nbsp; **[ 📖 Panduan Pengguna (ID) ](docs/USER_GUIDE_ID.md)**

<br />

</div>

---

<a name="english"></a>
## 🇬🇧 English

**SqlFox** is a fast, lightweight, single-binary database management tool with an embedded modern web UI. Designed for developers and DBA teams who prioritize safety, speed, and intelligence without the bloat of traditional heavy desktop clients.

> 📖 **Official Documentation**: Read the full **[English User Guide & DBA Playbook](docs/USER_GUIDE.md)** or **[Panduan Bahasa Indonesia](docs/USER_GUIDE_ID.md)**.

Operates with a strict memory ceiling (**< 100MB RAM**), sub-second launch time (**< 1s**), zero CGO dependencies, and instant browser/LAN accessibility.

### 🌟 Key Highlights

- ⚡ **Zero-Install Single Binary**: Run a single standalone executable. No electron, no heavy JVM, no complex installation wizard required.
- 🗺️ **Visual ERD Interactive Canvas**: Drag-and-drop table relationship visualizer with smooth cubic Bézier SVG lines, auto-layout, fit-all viewport, and per-session positioning cache.
- 🔄 **Schema & Data Synchronization Suite**: AST-based Structure Sync (side-by-side DDL comparison & safe migration generation), direct memory-bounded Cross-Connection Data Transfer (<50MB buffer), and Row-Level Data Sync (keyset delta reconciliation).
- 📊 **Spreadsheet CRUD & Smart Seeding**: Inline "+ Add Row" drafting, multi-row deletion with Safety Guard challenge, instant column filtering, streaming SQL dumps (<10MB buffer), and FK-Aware Smart Seed Dumps (referential topological closure).
- 🛡️ **Deterministic Safety Guard**: Intercepts accidental destructive queries (`DROP TABLE`, `TRUNCATE`, unqualified `UPDATE`/`DELETE` without `WHERE`) via AST evaluation with a mandatory Challenge Modal.
- 🔐 **Encrypted Connection Vault**: Database credentials and SSH keys encrypted at rest with **AES-256-GCM** and **Argon2id**. Protected by a Master PIN / Password.
- 📱 **Remote & LAN Access Guard**: Access your database workspace from your tablet, phone, or team members on the same network with PIN verification.
- 🤖 **BYOK AI Copilot & Error Healer**: Bring Your Own Key (OpenAI, Claude, Gemini, DeepSeek, or local Ollama). Explains schemas, generates optimized queries, and automatically analyzes and heals SQL execution errors. Features rich Markdown rendering with 1-click execution blocks (`Insert & Run`, `Replace`) and chat session branching/forking.
- 🏎️ **Keyset Pagination & Out-of-Band Cancellation**: High-speed row streaming without memory spikes and instantaneous query termination (`KILL QUERY`) through dedicated control sessions.
- 💾 **Smart Pre-DDL Backups**: Automatic schema and table dumps before dangerous alter/drop operations.
- 🌐 **Cluster & Replication Inspector**: Real-time diagnostic suite for Percona XtraDB Cluster (PXC 5.7/8.0), Galera, and MySQL replication. Auto-detects Flow Control pauses, auto-probes peer nodes to pinpoint the slowest bottleneck node, and evaluates a Tri-Metric Matrix (EVS network latency, InnoDB disk stalls, apply worker capacity).
- 📈 **Performance Flight Recorder**: On-demand diagnostic flight recorder capturing Quad-Metric telemetry (differential QPS, `Threads_running`, Row Lock Waits, Buffer Pool Waits) and correlating active non-idle queries with an interactive timeline scrubber and 10-minute auto-stop safety guard.
- 💻 **Headless CLI Suite**: Execute queries and scripts non-interactively with streaming output formats (`table`, `json`, `ndjson`, `csv`), pipe SQL via STDIN, direct DSN targeting for CI/CD runners, cross-connection data migrations (`sqlfox transfer`), and fail-closed AST safety guardrails.
- 🏋️ **Enterprise Hardening & Memory Ceiling (<100MB)**: Benchmarked against 1,000+ tables, 50,000 columns, and 1,000,000-row streams with a strict <100MB RAM budget. Features native CSS `content-visibility` sidebar virtualization, 150ms search debounce, 3s fail-fast query cancellation on saturated connection pools, loopback-guarded `pprof` diagnostics (`SQLFOX_PPROF=1`), and a global network error boundary with auto-reconnection and draft state preservation.
- 💡 **Heuristic Index Advisor & Query Tuner**: Deterministic rule-based index recommendations using the ESR rule (Equality -> Sort -> Range), Index Bloat detection (>5 secondary indexes) with unsuppressed candidate advice and consolidation paths, safe non-blocking DDL (`ALGORITHM=INPLACE, LOCK=NONE` on MySQL 5.7/8.0, `CONCURRENTLY` on PostgreSQL), and headless CLI query tuning via `sqlfox advise`.
- 🎨 **Value Presets & PK-Less Editing**: Quick value presets (`NULL`, `Empty`, `NOW`, `Date`, `UUID`) with floating pill and hotkeys (`Alt+N`, `Alt+E`, `Alt+T`, `Alt+D`, `Alt+U`), type-aware cell editors (`ENUM`, `BOOLEAN`, Foreign Key pickers), and full-row criteria matching enabling safe editing and deletion on tables without primary keys.
- 📜 **Universal Mutation Logging & Audit Source Attribution**: Comprehensive audit ledger persistence for all data and schema mutations across the platform with origin tracking (`source`). Attributions include `editor`, `grid_cell_edit`, `grid_insert`, `grid_delete`, `script_runner`, `csv_import`, `mock_data`, `sync_transfer`, and `schema_sync`, with visual badges and dropdown source filtering.
- 🩺 **Visual EXPLAIN Tree & Status Bar Telemetry**: Dedicated Target Query card with client-side SQL formatting, segmented navigation tabs (`📊 Plan Grid`, `🌳 Visual Tree`, `💡 Advisor`, `{ } Raw Plan`), and bottom telemetry displaying real-time engine flavor, server uptime, ping latency, and memory consumption.
- 🚀 **In-App Self-Update & Changelog Viewer**: Upgrade your single binary directly from the web UI with 1-click in-place replacement, full Markdown changelog inspection from GitHub Releases, 24-hour rate-limit guarded checks, and graceful auto-restart.
- 📋 **Structured Diagnostic Logging & Observability**: Configurable verbosity tiers (`debug`, `info`, `warn`, `error`, `silent`), dual output formats (`text`, `json`), persistent atomic append file sinks (`-log-file`), and automatic `sqlfox.env` loader for seamless Windows double-click configuration.
- 📝 **Unified Operational Notes Inbox & Follow-up Lifecycle**: Central inbox for captured SQL snippets, AI explanations, and developer runbooks. Features soft-link database object tagging (`db://conn/db/table`), scheduled reminder alarms with Web Audio API chime notifications, non-blocking snooze/done actions, table-filtered runbook views, and seamless AI Copilot `@` mention autocomplete context injection.
- 🔌 **Multi-Engine Parity & Resilience**: Resilient dual-syntax probes for MySQL 8.4 LTS / 9.x (`SHOW REPLICA STATUS`, `SHOW BINARY LOG STATUS`), MariaDB fallback diagnostics, full PostgreSQL Stored Function & Procedure introspection (`pg_proc`), Statement Digest (`pg_stat_statements`), Replication Inspector (`pg_stat_replication`), and atomic Structure Sync DDL.
- 📐 **Fluid Workspace & Live Concurrency Telemetry**: Draggable Workspace Pane Splitter separating Query Editor and Data Grid with 50:50 double-click reset, tab lifecycle suite with dirty buffer close confirmation guard, Master PIN shoulder-surfing privacy masking, and an ephemeral 60-point Processlist Activity Trend canvas.
- 🔄 **Fast Catalog Sync & Root Blocker Incident Triage**: Synchronize cross-database autocomplete and search caches in a single fast sweep (`InspectFastCatalog`) without foreign key metadata lock contention (`KEY_COLUMN_USAGE`), backed by a pre-flight server health check on active thread load (`Threads_running`), cache timestamp badge, and a high-contrast Root Blocker triage display with scrollable SQL queries and dedicated Kill Query vs Kill Blocker actions.

---

### 🚀 Quick Start

#### Method 1: Download Prebuilt Standalone Binary
Grab the pre-compiled binary for your OS and architecture from [GitHub Releases](https://github.com/herliansyah/sqlfox/releases):

| Operating System | Architecture | Package Format |
| :--- | :--- | :--- |
| **Linux** | `x86_64` (amd64), `ARM64` | `.tar.gz` |
| **macOS** | Apple Silicon (`arm64`), Intel (`amd64`) | `.tar.gz` |
| **Windows** | `x86_64` (amd64), `ARM64` | `.zip` |

**Linux / macOS (One-liner download & run):**
```bash
# Linux amd64
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.6.1-linux-amd64.tar.gz | tar -xz
./sqlfox

# macOS Apple Silicon
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.6.1-darwin-arm64.tar.gz | tar -xz
./sqlfox
```

#### Method 2: Run with Docker or Docker Compose
Run the official ultra-lightweight (~20MB) multi-arch container image directly:

```bash
docker run -d \
  --name sqlfox \
  -p 8080:8080 \
  -v sqlfox_data:/data \
  --add-host=host.docker.internal:host-gateway \
  ghcr.io/herliansyah/sqlfox:latest
```

Or deploy with Docker Compose:
```bash
docker compose up -d
```

#### Method 3: Install with Go
```bash
go install github.com/herliansyah/sqlfox/cmd/sqlfox@latest
sqlfox
```

#### Method 4: Build from Source
```bash
git clone https://github.com/herliansyah/sqlfox.git
cd sqlfox
go build -o sqlfox ./cmd/sqlfox
./sqlfox
```

---

### ⚙️ Command Line Options & Headless CLI

SqlFox operates both as an interactive desktop server and as a non-interactive headless CLI automation suite:

```text
Usage:
  sqlfox [command] [flags]
  sqlfox [server-flags] (launches desktop web UI)

Commands:
  query     Execute SQL query or script with streaming output formats (table, json, ndjson, csv)
  transfer  Perform direct cross-connection data transfer between databases
  job       Execute declarative database automation jobs or dry-run checks
  advise    Analyze SQL query and generate heuristic index recommendations and bloat warnings
  help      Show help for commands

Server Flags (Default Mode):
  -port int
        Port to listen on (default 8080, with automatic scanning if busy)
  -host string
        Host interface to bind (default "127.0.0.1"; use --lan or "0.0.0.0" for LAN access)
  -lan
        Enable LAN and tablet access (binds to 0.0.0.0 and displays Master PIN)
  -pass string
        Master password for vault encryption (leave blank to auto-generate or reuse ~/.sqlfox/master.key)
  -db string
        Path to config vault database (default: ~/.sqlfox/config.db)
  -no-browser
        Do not automatically launch web browser
  -strict-port
        Fail immediately if specified port is in use
  -multi
        Allow multiple SqlFox instances to run concurrently
  -version
        Print SqlFox version and exit
  -log-level string
        Logging verbosity: debug, info, warn, error, silent (or SQLFOX_LOG_LEVEL)
  -v, -verbose
        Enable verbose debug logging (shortcut for -log-level=debug)
  -log-file string
        Target file for log output sink (or SQLFOX_LOG_FILE)
  -log-format string
        Log output format: text, json (default text or SQLFOX_LOG_FORMAT)
  -silent, -q
        Suppress terminal console output (logs still written to file if -log-file is set)
```

#### Examples

```bash
# Run desktop web UI on a custom port without automatically opening a browser:
sqlfox -port 9000 -no-browser

# Bind strictly to localhost only (no LAN access):
sqlfox -host 127.0.0.1

# Headless CLI: execute a query and output as JSON:
sqlfox query --dsn "root:pass@tcp(127.0.0.1:3306)/mydb" --sql "SELECT id, name FROM users" --format json

# Headless CLI: analyze index recommendations for a query:
sqlfox advise --dsn "root:pass@tcp(127.0.0.1:3306)/mydb" --sql "SELECT * FROM orders WHERE status = 'pending' ORDER BY created_at"
```

---

### 🏛️ Architecture & Security

```
┌─────────────────────────────────────────────────────────┐
│ User Browser / Tablet / LAN Client                      │
│        ▲                                                │
│        │ Local HTTP REST & SSE (Protected by Master PIN)│
│        ▼                                                │
│ Go Backend Process (<100MB RAM, Single Binary)          │
│   ├── Safety Guard: Deterministic AST Query Analyzer    │
│   ├── Session Manager & Out-of-Band KILL QUERY Pool     │
│   ├── AI Firewall: Context-Window Sanitizer (BYOK)      │
│   └── Connection Vault: Encrypted via AES-256-GCM       │
│   └── Headless CLI Runner & Declarative Task Engine     │
└────────┬──────────────────────────────────┬─────────────┘
         │                                  │
         │ Direct TLS or SSH Tunnel         │ HTTPS (DDL Metadata Only)
         ▼                                  ▼
[ Remote / Local Database ]         [ AI Provider API ]
(MySQL, MariaDB, PostgreSQL, SQLite) (OpenAI, Claude, Gemini, Ollama)
```

1. **Safety Guard**: Prevents accidental queries like `UPDATE users SET status = 'inactive';` without a `WHERE` clause. Prompts with the exact row estimate and requires explicit confirmation.
2. **BYOK Privacy Guarantee**: SqlFox does NOT send database rows or business data to AI providers. Only the sanitized schema DDL relevant to your query prompt is transmitted.
3. **No External Runtimes**: Zero CGO, pure-Go SQLite, no NodeJS or Python server needed at runtime.

---

### 💖 Support & Sponsorship

If you find SqlFox useful and would like to support its ongoing development, consider sponsoring the project:
- ☕ **[Sponsor via Saweria (Buy a Coffee)](https://saweria.co/herliansyah26)**

---

<br />

---

<a name="bahasa-indonesia"></a>
## 🇮🇩 Bahasa Indonesia

**SqlFox** adalah aplikasi database management and viewer generasi baru yang super cepat, ringan, dan hadir dalam format **single-binary** dengan antarmuka web modern bawaan (*embedded*). Dirancang khusus untuk developer, software engineer, dan tim DBA yang mengutamakan keamanan data (*Safety Guard*), efisiensi performa, dan kecerdasan AI tanpa beban aplikasi desktop tradisional yang berat.

> 📖 **Dokumentasi Resmi**: Baca **[Panduan Lengkap Pengguna & Playbook DBA (Bahasa Indonesia)](docs/USER_GUIDE_ID.md)** atau **[English User Guide](docs/USER_GUIDE.md)**.

Berjalan dengan konsumsi memori sangat hemat (**< 100MB RAM**), waktu start kilat (**< 1 detik**), bebas dependensi CGO, serta bisa langsung diakses dari browser maupun perangkat lain di jaringan lokal (LAN/WiFi).

### 🌟 Fitur Utama

- ⚡ **Jalankan Tanpa Install (Single Binary)**: Cukup download dan jalankan berkas binary tunggal mandiri. Tanpa Electron, tanpa JVM yang membebani laptop.
- 🗺️ **Visual ERD Canvas Interaktif**: Visualisasi relasi antar-tabel dengan drag-and-drop, garis koneksi Bézier SVG mulus, penataan otomatis (*Auto Layout*), penyesuaian layar (*Fit All*), dan penyimpanan posisi per sesi.
- 🔄 **Sinkronisasi Skema & Data (Navicat Power Suite)**: Perbandingan struktur DDL berdampingan (*Structure Sync*) beserta migrasi aman, transfer streaming antar-koneksi langsung (*Data Transfer* dengan batas buffer <50MB), dan rekonsiliasi data per baris berbasis primary key (*Row-Level Data Sync*).
- 📊 **Spreadsheet Grid CRUD & Smart Seed**: Penambahan baris draf instan ("+ Add Row"), penghapusan banyak baris dengan proteksi Safety Guard, filter kolom instan, streaming SQL dump langsung (<10MB buffer), dan *FK-Aware Smart Seed Dump* dengan resolusi graf relasi rekursif (*topological closure*).
- 🛡️ **Safety Guard Terintegrasi**: Menganalisis sintaks SQL (*AST parser*) secara deterministik untuk mencegat query destruktif berbahaya (`DROP TABLE`, `TRUNCATE`, `DELETE`/`UPDATE` massal tanpa `WHERE`) dengan dialog konfirmasi aman (*Challenge Modal*).
- 🔐 **Connection Vault Terenkripsi**: Menyimpan kredensial database dan SSH key dengan standar kriptografi **AES-256-GCM** dan **Argon2id**. Dilengkapi PIN / Master Password otomatis yang praktis.
- 📱 **Akses Multi-Device (Remote LAN Guard)**: Buka SqlFox dari tablet, iPad, atau HP melalui koneksi WiFi lokal dengan otentikasi PIN keamanan.
- 🤖 **AI Copilot & Error Healer (BYOK)**: Gunakan API Key Anda sendiri (OpenAI, Claude, Gemini, DeepSeek, atau Ollama lokal). Mampu membuat query dari bahasa manusia, optimasi indeks, memperbaiki query error secara otomatis (*Error Healer*), formatting Markdown kaya dengan tombol eksekusi SQL instan (*Insert & Run*, *Replace*), serta *Chat Session Forking* untuk eksplorasi prompt independen.
- 🏎️ **Keyset Pagination & Out-of-Band Cancel**: Tampilan data super responsif tanpa lonjakan RAM, serta fitur pembatalan query instan (*KILL QUERY*) lewat secondary connection tanpa memutus koneksi utama.
- 💾 **Smart Pre-DDL Backup**: Backup otomatis skema dan data tabel sebelum eksekusi DDL penting dijalankan.
- 🌐 **Cluster & Replication Inspector**: Diagnostik real-time untuk Percona XtraDB Cluster (PXC 5.7/8.0), Galera, dan replikasi MySQL. Mendeteksi pembekuan Flow Control, auto-probe node rekan untuk mengisolasi node terlelet yang membebani cluster, serta Tri-Metric Detective (latensi jaringan EVS, stall harddisk/InnoDB, dan kejenuhan worker).
- 📈 **Performance Flight Recorder**: Perekam telemetri performa database on-demand (Quad-Metric: QPS, `Threads_running`, Row Lock Waits, Buffer Pool Waits) yang menghubungkan titik lonjakan (*spike*) grafik langsung dengan query aktif yang sedang berjalan di detik tersebut, dilengkapi pengaman auto-stop 10 menit.
- 💻 **Headless CLI Suite**: Eksekusi query dan script terminal secara otomatis/non-interaktif dengan format output streaming (`table`, `json`, `ndjson`, `csv`), dukungan piping via STDIN, direct DSN untuk runner CI/CD tanpa vault, transfer data antar-koneksi (`sqlfox transfer`), serta perlindungan fail-closed AST Safety Guard.
- 🏋️ **Enterprise Hardening & Plafon Memori (<100MB RAM)**: Teruji tahan banting pada skema masif 1.000+ tabel, 50.000 kolom, dan streaming 1.000.000 baris di bawah pagu <100MB RAM. Dilengkapi virtualisasi sidebar native CSS `content-visibility`, filter pencarian *debounced* 150ms, *fail-fast timeout* 3 detik untuk pembatalan query saat pool jenuh, profil memori pprof terproteksi loopback (`SQLFOX_PPROF=1`), serta *Global Network Error Boundary* dengan rekoneksi otomatis tanpa menghapus draf query atau staged edits.
- 💡 **Heuristic Index Advisor & Query Tuner**: Rekomendasi indeks deterministik berbasis aturan ESR (Equality $\to$ Sort $\to$ Range) untuk meniadakan filesort dan full table scan, peringatan proteksi *Index Bloat* (>5 indeks sekunder) tanpa memutus saran indeks, safe non-blocking DDL (`ALGORITHM=INPLACE, LOCK=NONE` pada MySQL 5.7/8.0, `CONCURRENTLY` pada PostgreSQL), serta CLI headless `sqlfox advise`.
- 🎨 **Value Presets & PK-Less Editing**: Preset nilai cepat (`NULL`, `Empty`, `NOW`, `Date`, `UUID`) dengan floating pill dan shortcut (`Alt+N`, `Alt+E`, `Alt+T`, `Alt+D`, `Alt+U`), editor sel cerdas berbasis tipe (`ENUM` dropdown, toggle `BOOLEAN`, pemilih relasi Foreign Key), serta pencocokan kriteria baris penuh (*full-row criteria*) yang memungkinkan pengeditan dan penghapusan aman pada tabel tanpa primary key.
- 📜 **Universal Mutation Logging & Audit Source Attribution**: Pencatatan histori audit persisten untuk seluruh mutasi data dan skema di seluruh platform dengan pelacakan asal (`source`: `editor`, `grid_cell_edit`, `grid_insert`, `grid_delete`, `script_runner`, `csv_import`, `mock_data`, `sync_transfer`, `schema_sync`), lengkap dengan badge visual dan filter dropdown.
- 🩺 **Visual EXPLAIN Tree & Telemetri Status Bar**: Kartu Target Query terdedikasi dengan formatting SQL otomatis, tab navigasi modern (`📊 Plan Grid`, `🌳 Visual Tree`, `💡 Advisor`, `{ } Raw Plan`), serta telemetri status bar bawah yang menampilkan mesin database, waktu uptime server, latensi ping, dan konsumsi memori.
- 🚀 **In-App Self-Update & Changelog Viewer**: Update binary SqlFox langsung dari antarmuka web dengan 1 klik (in-place atomic replacement), inspeksi changelog rilis Markdown dari GitHub Releases, proteksi cache 24 jam untuk rate limit, dan auto-restart mulus.
- 📋 **Logging Diagnostik & Observabilitas Terstruktur**: Konfigurasi tingkatan verbosity (`debug`, `info`, `warn`, `error`, `silent`), dual format (`text`, `json`), target file persisten append (`-log-file`), dan auto-load file `sqlfox.env` untuk konfigurasi instan saat double-klik di Windows.
- 📝 **Operational Notes Inbox & Pengingat Terjadwal**: Kotak masuk terpusat untuk catatan operasional, snapshot SQL, penjelasan AI, dan runbook developer. Dilengkapi penandaan objek database relasional (`db://conn/db/table`), alarm pengingat tindak lanjut dengan notifikasi audio Web Audio API, aksi snooze/selesai non-blocking, feed runbook terfilter per tabel, serta injeksi konteks prompt instan melalui mention `@` di AI Copilot.
- 🔌 **Ketahanan & Paritas Multi-Engine**: Dual-syntax probe adaptif untuk MySQL 8.4 LTS / 9.x (`SHOW REPLICA STATUS`, `SHOW BINARY LOG STATUS`), fallback diagnostik MariaDB, introspeksi fungsi & prosedur PostgreSQL (`pg_proc`), kueri teratas (`pg_stat_statements`), monitoring replikasi (`pg_stat_replication`), dan sintaks DDL Structure Sync yang presisi.
- 📐 **Ergonomi Ruang Kerja & Telemetri Konkurensi**: Workspace Pane Splitter fleksibel antara Query Editor dan Data Grid dengan reset 50:50 klik ganda, suite siklus hidup tab dengan pengaman konfirmasi draf kotor (*dirty buffer guard*), proteksi privasi PIN Master dari *shoulder surfing*, dan kanvas grafik tren aktivitas kueri 60 titik di Processlist.
- 🔄 **Sinkronisasi Cepat Katalog Skema & Root Blocker Triage**: Perbarui cache autocomplete dan pencarian lintas database dalam satu sapuan kilat (`InspectFastCatalog`) tanpa memicu kunci metadata foreign key (`KEY_COLUMN_USAGE`), didukung evaluasi kesehatan pra-eksekusi beban thread server (`Threads_running`), lencana stempel waktu cache, dan tampilan Root Blocker triage dengan query SQL scrollable serta tombol terminasi terpisah (Kill Query vs Kill Blocker).

---

### 🚀 Panduan Instalasi & Penggunaan Cepat

#### Cara 1: Unduh File Binary Siap Pakai
Unduh paket binary sesuai sistem operasi dan arsitektur Anda di halaman [GitHub Releases](https://github.com/herliansyah/sqlfox/releases):

| Sistem Operasi | Arsitektur CPU | Format Berkas |
| :--- | :--- | :--- |
| **Linux** | `x86_64` (amd64), `ARM64` | `.tar.gz` |
| **macOS** | Apple Silicon (`arm64`), Intel (`amd64`) | `.tar.gz` |
| **Windows** | `x86_64` (amd64), `ARM64` | `.zip` |

**Linux / macOS (Download & ekstrak sekali jalan):**
```bash
# Linux amd64
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.6.1-linux-amd64.tar.gz | tar -xz
./sqlfox

# macOS Apple Silicon
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.6.1-darwin-arm64.tar.gz | tar -xz
./sqlfox
```

#### Cara 2: Jalankan via Docker / Docker Compose
Jalankan image container multi-arsitektur resmi yang sangat ringan (~20MB):

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

#### Cara 3: Install Menggunakan Go
```bash
go install github.com/herliansyah/sqlfox/cmd/sqlfox@latest
sqlfox
```

#### Cara 4: Kompilasi Manual dari Source Code
```bash
git clone https://github.com/herliansyah/sqlfox.git
cd sqlfox
go build -o sqlfox ./cmd/sqlfox
./sqlfox
```

---

### ⚙️ Parameter & Opsi Perintah CLI

SqlFox dapat beroperasi sebagai server web desktop interaktif maupun sebagai suite CLI otomatisasi tanpa tampilan grafis (headless):

```text
Penggunaan:
  sqlfox [command] [flags]
  sqlfox [server-flags] (menjalankan web UI desktop)

Perintah (Commands):
  query     Eksekusi query atau script SQL dengan streaming format (table, json, ndjson, csv)
  transfer  Transfer data langsung antar-koneksi database (streaming <50MB RAM)
  job       Jalankan pipeline otomatisasi database deklaratif (run, check)
  advise    Analisis query SQL untuk rekomendasi indeks dan peringatan bloat
  help      Tampilkan bantuan perintah

Opsi Server (Mode Default):
  -port int
        Port server (default 8080, otomatis mencari port berikutnya jika bentrok)
  -host string
        Interface jaringan yang di-bind (default "127.0.0.1"; gunakan --lan atau "0.0.0.0" untuk akses LAN)
  -lan
        Aktifkan akses jaringan LAN & tablet (bind ke 0.0.0.0 dan tampilkan Master PIN)
  -pass string
        Master password enkripsi vault (kosongkan untuk generate PIN otomatis atau baca dari ~/.sqlfox/master.key)
  -db string
        Lokasi file database vault konfigurasi (default: ~/.sqlfox/config.db)
  -no-browser
        Jangan otomatis membuka browser saat aplikasi mulai
  -strict-port
        Langsung keluar jika port yang ditentukan sedang terpakai
  -version
        Tampilkan versi SqlFox dan keluar
  -log-level string
        Tingkat verbosity log: debug, info, warn, error, silent (atau SQLFOX_LOG_LEVEL)
  -v, -verbose
        Shortcut untuk mode verbose full debug (-log-level=debug)
  -log-file string
        Target file untuk menyimpan output log secara persisten (atau SQLFOX_LOG_FILE)
  -log-format string
        Format log: text, json (default text atau SQLFOX_LOG_FORMAT)
  -silent, -q
        Bungkam output konsol terminal (log tetap ditulis ke file jika -log-file ditentukan)
```

#### Contoh Penggunaan

```bash
# Menjalankan di port tertentu tanpa auto-buka browser:
sqlfox -port 9090 -no-browser

# Menjalankan dengan full debug logging ke file:
sqlfox -v -log-file=./sqlfox-debug.log

# Menjalankan mode hening (silent) dan hanya simpan error ke file:
sqlfox -log-level=error -log-file=./sqlfox-error.log -silent

# Headless CLI: jalankan query dan tampilkan output dalam format JSON:
sqlfox query --dsn "root:pass@tcp(127.0.0.1:3306)/mydb" --sql "SELECT id, name FROM users" --format json

# Headless CLI: dapatkan rekomendasi indeks untuk optimasi query:
sqlfox advise --dsn "root:pass@tcp(127.0.0.1:3306)/mydb" --sql "SELECT * FROM orders WHERE status = 'pending' ORDER BY created_at"
```

#### 🪟 Menjalankan di Windows via Double-Klik (sqlfox.exe)

Untuk mengatur log saat menjalankan `sqlfox.exe` tanpa membuka Command Prompt:
1. **File `sqlfox.env` (Paling Praktis)**: Buat file bernama `sqlfox.env` di folder yang sama dengan `sqlfox.exe` (atau di `%USERPROFILE%\.sqlfox\sqlfox.env`):
   ```ini
   SQLFOX_LOG_LEVEL=debug
   SQLFOX_LOG_FILE=sqlfox.log
   SQLFOX_LOG_FORMAT=text
   ```
2. **Windows Shortcut**: Klik kanan `sqlfox.exe` ➔ *Create shortcut* ➔ Properties ➔ Tambahkan parameter di kolom Target: `sqlfox.exe -v -log-file=sqlfox.log`.
3. **Batch File**: Buat `start.bat` berisi: `sqlfox.exe -v -log-file=sqlfox.log`.
4. **Environment Variables**: Set `SQLFOX_LOG_LEVEL=debug` dan `SQLFOX_LOG_FILE=C:\path\sqlfox.log` di System Environment Variables Windows.

---

### 🛡️ Keamanan & Privasi

1. **Pencegahan Human Error**: Fitur **Safety Guard** aktif secara default untuk lingkungan *Production*. Query seperti `UPDATE akun SET saldo = 0;` tanpa klausa `WHERE` akan dicegat sebelum menyentuh database.
2. **Privasi AI (BYOK)**: SqlFox **TIDAK PERNAH** mengirimkan data baris tabel atau data pelanggan ke server AI. Hanya struktur DDL skema terkait yang dikirimkan untuk konteks pembuatan query.
3. **Penyimpanan Kredensial Aman**: Password database dan private key SSH dienkripsi menggunakan algoritma standar industri **AES-256-GCM** dengan derivasi kunci **Argon2id**.

---

### 💖 Dukungan & Sponsor

Jika Anda merasa SqlFox bermanfaat dan ingin mendukung pengembangan berkelanjutannya:
- ☕ **[Sponsor via Saweria](https://saweria.co/herliansyah26)**

---

### 📄 Lisensi

Proyek ini dilisensikan di bawah lisensi [MIT](LICENSE).

<div align="center">
  <sub>Dibangun dengan ❤️ oleh <a href="https://github.com/herliansyah">herliansyah</a> untuk komunitas database & developer.</sub>
</div>
