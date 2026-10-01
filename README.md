<div align="center">

# 🦊 SqlFox

### High-Performance Modern Database Manager & Viewer with AI Copilot & Safety Guard

[![GitHub Release](https://img.shields.io/github/v/release/herliansyah/sqlfox?style=flat-square&color=FF6B35&logo=github)](https://github.com/herliansyah/sqlfox/releases)
[![npm version](https://img.shields.io/npm/v/sqlfox?style=flat-square&color=CB3837&logo=npm)](https://www.npmjs.com/package/sqlfox)
[![npm downloads](https://img.shields.io/npm/dm/sqlfox?style=flat-square&color=2E7D32&logo=npm)](https://www.npmjs.com/package/sqlfox)
[![Go Version](https://img.shields.io/github/go-mod/go-version/herliansyah/sqlfox?style=flat-square&color=00ADD8&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-F59E0B?style=flat-square)](https://opensource.org/licenses/MIT)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-4B5563?style=flat-square&logo=linux)](https://github.com/herliansyah/sqlfox/releases)
[![Arch](https://img.shields.io/badge/arch-amd64%20%7C%20arm64-6366F1?style=flat-square)](https://github.com/herliansyah/sqlfox/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/herliansyah/sqlfox/ci.yml?branch=master&label=CI&style=flat-square&logo=githubactions)](https://github.com/herliansyah/sqlfox/actions)
[![GitHub Stars](https://img.shields.io/github/stars/herliansyah/sqlfox?style=flat-square&color=EAB308&logo=star)](https://github.com/herliansyah/sqlfox/stargazers)

<br />

**[ 🇬🇧 English ](#-english)** &nbsp;•&nbsp; **[ 🇮🇩 Bahasa Indonesia ](#-bahasa-indonesia)**

<br />

</div>

---

<a name="english"></a>
## 🇬🇧 English

**SqlFox** is a fast, lightweight, single-binary database management tool with an embedded modern web UI. Designed for developers and DBA teams who prioritize safety, speed, and intelligence without the bloat of traditional heavy desktop clients.

Operates with a strict memory ceiling (**< 100MB RAM**), sub-second launch time (**< 1s**), zero CGO dependencies, and instant browser/LAN accessibility.

### 🌟 Key Highlights

- ⚡ **Zero-Install Instant Run**: Launch via `npx sqlfox` or run a single standalone binary. No electron, no heavy JVM, no installation wizard required.
- 🛡️ **Deterministic Safety Guard**: Intercepts accidental destructive queries (`DROP TABLE`, `TRUNCATE`, unqualified `UPDATE`/`DELETE` without `WHERE`) via AST evaluation with a mandatory Challenge Modal.
- 🔐 **Encrypted Connection Vault**: Database credentials and SSH keys encrypted at rest with **AES-256-GCM** and **Argon2id**. Protected by a Master PIN / Password.
- 📱 **Remote & LAN Access Guard**: Access your database workspace from your tablet, phone, or team members on the same network with PIN verification.
- 🤖 **BYOK AI Copilot & Error Healer**: Bring Your Own Key (OpenAI, Claude, Gemini, DeepSeek, or local Ollama). Explains schemas, generates optimized queries, and automatically analyzes and heals SQL execution errors.
- 🏎️ **Keyset Pagination & Out-of-Band Cancellation**: High-speed row streaming without memory spikes and instantaneous query termination (`KILL QUERY`) through dedicated control sessions.
- 💾 **Smart Pre-DDL Backups**: Automatic schema and table dumps before dangerous alter/drop operations.
- 🌐 **Cluster & Replication Inspector**: Real-time diagnostic suite for Percona XtraDB Cluster (PXC 5.7/8.0), Galera, and MySQL replication. Auto-detects Flow Control pauses, auto-probes peer nodes to pinpoint the slowest bottleneck node, and evaluates a Tri-Metric Matrix (EVS network latency, InnoDB disk stalls, apply worker capacity).
- 📈 **Performance Flight Recorder**: On-demand diagnostic flight recorder capturing Quad-Metric telemetry (differential QPS, `Threads_running`, Row Lock Waits, Buffer Pool Waits) and correlating active non-idle queries with an interactive timeline scrubber and 10-minute auto-stop safety guard.
- 🔌 **Multi-Engine Support**: MySQL 5.7 / 8.x, MariaDB, and local pure-Go SQLite (direct connection or over SSH tunnel).

---

### 🚀 Quick Start

#### Method 1: Using `npx` (No Installation Needed)
If you have Node.js (16+) installed, run SqlFox immediately:
```bash
npx sqlfox
```

#### Method 2: Global Install via `npm`
```bash
npm install -g sqlfox

# Then start anywhere:
sqlfox
```

#### Method 3: Download Prebuilt Standalone Binary
Grab the pre-compiled binary for your OS and architecture from [GitHub Releases](https://github.com/herliansyah/sqlfox/releases):

| Operating System | Architecture | Package Format |
| :--- | :--- | :--- |
| **Linux** | `x86_64` (amd64), `ARM64` | `.tar.gz` |
| **macOS** | Apple Silicon (`arm64`), Intel (`amd64`) | `.tar.gz` |
| **Windows** | `x86_64` (amd64), `ARM64` | `.zip` |

**Linux / macOS (One-liner download & run):**
```bash
# Linux amd64
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.1.0-linux-amd64.tar.gz | tar -xz
./sqlfox

# macOS Apple Silicon
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.1.0-darwin-arm64.tar.gz | tar -xz
./sqlfox
```

#### Method 4: Install with Go
```bash
go install github.com/herliansyah/sqlfox/cmd/sqlfox@latest
sqlfox
```

#### Method 5: Build from Source
```bash
git clone https://github.com/herliansyah/sqlfox.git
cd sqlfox
go build -o sqlfox ./cmd/sqlfox
./sqlfox
```

---

### ⚙️ Command Line Options

```text
Usage of sqlfox:
  -port int
        Port to listen on (default 8080, with automatic scanning if busy)
  -host string
        Host interface to bind (default "0.0.0.0" for LAN/Tablet access, "127.0.0.1" for local only)
  -pass string
        Master password for vault encryption (leave blank to auto-generate or reuse ~/.sqlfox/master.key)
  -db string
        Path to config vault database (default: ~/.sqlfox/config.db)
  -no-browser
        Do not automatically launch web browser
  -strict-port
        Fail immediately if specified port is in use
  -version
        Print SqlFox version and exit
```

#### Examples

```bash
# Run on a custom port without automatically opening a browser:
sqlfox -port 9000 -no-browser

# Bind strictly to localhost only (no LAN access):
sqlfox -host 127.0.0.1

# Set a custom master password for the vault:
sqlfox -pass "my-secure-master-password"
```

---

### 🏛️ Architecture & Security

```
┌─────────────────────────────────────────────────────────┐
│ User Browser / Tablet / LAN Client                      │
│        ▲                                                │
│        │ Local HTTP / WS (Protected by Master PIN)      │
│        ▼                                                │
│ Go Backend Process (<100MB RAM, Single Binary)          │
│   ├── Safety Guard: Deterministic AST Query Analyzer    │
│   ├── Session Manager & Out-of-Band KILL QUERY Pool     │
│   ├── AI Firewall: Context-Window Sanitizer (BYOK)      │
│   └── Connection Vault: Encrypted via AES-256-GCM       │
└────────┬──────────────────────────────────┬─────────────┘
         │                                  │
         │ Direct TLS or SSH Tunnel         │ HTTPS (DDL Metadata Only)
         ▼                                  ▼
[ Remote / Local Database ]         [ AI Provider API ]
(MySQL, MariaDB, SQLite)            (OpenAI, Claude, Gemini, Ollama)
```

1. **Safety Guard**: Prevents accidental queries like `UPDATE users SET status = 'inactive';` without a `WHERE` clause. Prompts with the exact row estimate and requires explicit confirmation.
2. **BYOK Privacy Guarantee**: SqlFox does NOT send database rows or business data to AI providers. Only the sanitized schema DDL relevant to your query prompt is transmitted.
3. **No External Runtimes**: Zero CGO, pure-Go SQLite, no NodeJS or Python server needed at runtime.

---

<br />

---

<a name="bahasa-indonesia"></a>
## 🇮🇩 Bahasa Indonesia

**SqlFox** adalah aplikasi database management and viewer generasi baru yang super cepat, ringan, dan hadir dalam format **single-binary** dengan antarmuka web modern bawaan (*embedded*). Dirancang khusus untuk developer, software engineer, dan tim DBA yang mengutamakan keamanan data (*Safety Guard*), efisiensi performa, dan kecerdasan AI tanpa beban aplikasi desktop tradisional yang berat.

Berjalan dengan konsumsi memori sangat hemat (**< 100MB RAM**), waktu start kilat (**< 1 detik**), bebas dependensi CGO, serta bisa langsung diakses dari browser maupun perangkat lain di jaringan lokal (LAN/WiFi).

### 🌟 Fitur Utama

- ⚡ **Jalankan Tanpa Install**: Jalankan langsung lewat `npx sqlfox` atau download file binary tunggal. Tanpa Electron, tanpa JVM yang membebani laptop.
- 🛡️ **Safety Guard Terintegrasi**: Menganalisis sintaks SQL (*AST parser*) secara deterministik untuk mencegat query destruktif berbahaya (`DROP TABLE`, `TRUNCATE`, `DELETE`/`UPDATE` massal tanpa `WHERE`) dengan dialog konfirmasi aman (*Challenge Modal*).
- 🔐 **Connection Vault Terenkripsi**: Menyimpan kredensial database dan SSH key dengan standar kriptografi **AES-256-GCM** dan **Argon2id**. Dilengkapi PIN / Master Password otomatis yang praktis.
- 📱 **Akses Multi-Device (Remote LAN Guard)**: Buka SqlFox dari tablet, iPad, atau HP melalui koneksi WiFi lokal dengan otentikasi PIN keamanan.
- 🤖 **AI Copilot & Error Healer (BYOK)**: Gunakan API Key Anda sendiri (OpenAI, Claude, Gemini, DeepSeek, atau Ollama lokal). Mampu membuat query dari bahasa manusia, optimasi indeks, serta menganalisis dan memperbaiki query yang error secara otomatis (*Error Healer*).
- 🏎️ **Keyset Pagination & Out-of-Band Cancel**: Tampilan data super responsif tanpa lonjakan RAM, serta fitur pembatalan query instan (*KILL QUERY*) lewat secondary connection tanpa memutus koneksi utama.
- 💾 **Smart Pre-DDL Backup**: Backup otomatis skema dan data tabel sebelum eksekusi DDL penting dijalankan.
- 🌐 **Cluster & Replication Inspector**: Diagnostik real-time untuk Percona XtraDB Cluster (PXC 5.7/8.0), Galera, dan replikasi MySQL. Mendeteksi pembekuan Flow Control, auto-probe node rekan untuk mengisolasi node terlelet yang membebani cluster, serta Tri-Metric Detective (latensi jaringan EVS, stall harddisk/InnoDB, dan kejenuhan worker).
- 📈 **Performance Flight Recorder**: Perekam telemetri performa database on-demand (Quad-Metric: QPS, `Threads_running`, Row Lock Waits, Buffer Pool Waits) yang menghubungkan titik lonjakan (*spike*) grafik langsung dengan query aktif yang sedang berjalan di detik tersebut, dilengkapi pengaman auto-stop 10 menit.
- 🔌 **Dukungan Multi Database**: MySQL 5.7 / 8.x, MariaDB, dan SQLite (koneksi langsung maupun melalui SSH Tunneling).

---

### 🚀 Panduan Instalasi & Penggunaan Cepat

#### Cara 1: Menggunakan `npx` (Langsung Jalankan tanpa Install)
Bagi pengguna Node.js (v16+):
```bash
npx sqlfox
```

#### Cara 2: Install Global via `npm`
```bash
npm install -g sqlfox

# Jalankan kapan saja dengan perintah:
sqlfox
```

#### Cara 3: Unduh File Binary Siap Pakai
Unduh paket binary sesuai sistem operasi dan arsitektur Anda di halaman [GitHub Releases](https://github.com/herliansyah/sqlfox/releases):

| Sistem Operasi | Arsitektur CPU | Format Berkas |
| :--- | :--- | :--- |
| **Linux** | `x86_64` (amd64), `ARM64` | `.tar.gz` |
| **macOS** | Apple Silicon (`arm64`), Intel (`amd64`) | `.tar.gz` |
| **Windows** | `x86_64` (amd64), `ARM64` | `.zip` |

**Linux / macOS (Download & ekstrak sekali jalan):**
```bash
# Linux amd64
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.1.0-linux-amd64.tar.gz | tar -xz
./sqlfox

# macOS Apple Silicon
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.1.0-darwin-arm64.tar.gz | tar -xz
./sqlfox
```

#### Cara 4: Install Menggunakan Go
```bash
go install github.com/herliansyah/sqlfox/cmd/sqlfox@latest
sqlfox
```

#### Cara 5: Kompilasi Manual dari Source Code
```bash
git clone https://github.com/herliansyah/sqlfox.git
cd sqlfox
go build -o sqlfox ./cmd/sqlfox
./sqlfox
```

---

### ⚙️ Parameter & Opsi Perintah CLI

```text
Penggunaan sqlfox:
  -port int
        Port server (default 8080, otomatis mencari port berikutnya jika bentrok)
  -host string
        Interface jaringan yang di-bind (default "0.0.0.0" untuk akses LAN/Tablet, "127.0.0.1" untuk lokal saja)
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
```

#### Contoh Penggunaan

```bash
# Menjalankan di port tertentu tanpa auto-buka browser:
sqlfox -port 9090 -no-browser

# Hanya izinkan akses dari laptop lokal (tutup akses LAN):
sqlfox -host 127.0.0.1

# Menggunakan master password sendiri:
sqlfox -pass "password-rahasia-saya"
```

---

### 🛡️ Keamanan & Privasi

1. **Pencegahan Human Error**: Fitur **Safety Guard** aktif secara default untuk lingkungan *Production*. Query seperti `UPDATE akun SET saldo = 0;` tanpa klausa `WHERE` akan dicegat sebelum menyentuh database.
2. **Privasi AI (BYOK)**: SqlFox **TIDAK PERNAH** mengirimkan data baris tabel atau data pelanggan ke server AI. Hanya struktur DDL skema terkait yang dikirimkan untuk konteks pembuatan query.
3. **Penyimpanan Kredensial Aman**: Password database dan private key SSH dienkripsi menggunakan algoritma standar industri **AES-256-GCM** dengan derivasi kunci **Argon2id**.

---

### 📄 Lisensi

Proyek ini dilisensikan di bawah lisensi [MIT](LICENSE).

<div align="center">
  <sub>Dibangun dengan ❤️ oleh <a href="https://github.com/herliansyah">herliansyah</a> untuk komunitas database & developer.</sub>
</div>
