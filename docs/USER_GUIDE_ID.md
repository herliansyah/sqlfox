# Panduan Lengkap Pengguna & Playbook DBA SqlFox

<div align="center">

**[ 🇬🇧 English Manual ](USER_GUIDE.md)** &nbsp;•&nbsp; **[ 🇮🇩 Panduan Bahasa Indonesia ](USER_GUIDE_ID.md)**

---

<p>Buku panduan standar industri dan playbook operasional untuk software engineer, data operator, dan Database Administrator (DBA) yang menggunakan SqlFox.</p>

</div>

---

## 📑 Daftar Isi

1. [Gambaran Umum & Filosofi Arsitektur](#1-gambaran-umum--filosofi-arsitektur)
   - [Model Single-Binary Bebas Dependensi CGO](#model-single-binary-bebas-dependensi-cgo)
   - [Plafon Memori Ketat (< 100MB RAM)](#plafon-memori-ketat--100mb-ram)
   - [Dukungan Mesin Database & Driver](#dukungan-mesin-database--driver)
2. [Instalasi & Mode Menjalankan Aplikasi](#2-instalasi--mode-menjalankan-aplikasi)
   - [File Binary Standalone (Linux, macOS, Windows)](#file-binary-standalone-linux-macos-windows)
   - [Instalasi via Go Tools](#instalasi-via-go-tools)
   - [Kompilasi Manual dari Source Code](#kompilasi-manual-dari-source-code)
   - [Opsi CLI & Fitur Port Hunting](#opsi-cli--fitur-port-hunting)
   - [Remote Access Guard & PIN Keamanan](#remote-access-guard--pin-keamanan)
3. [Connection Vault & Arsitektur Keamanan](#3-connection-vault--arsitektur-keamanan)
   - [Enkripsi AES-256-GCM & KDF Argon2id](#enkripsi-aes-256-gcm--kdf-argon2id)
   - [SSH Tunneling Native (Bastion / Jump Host)](#ssh-tunneling-native-bastion--jump-host)
   - [Konfigurasi Multi-Engine (MySQL, PostgreSQL, SQLite)](#konfigurasi-multi-engine-mysql-postgresql-sqlite)
   - [Tingkatan Environment (Production, Staging, Development)](#tingkatan-environment-production-staging-development)
   - [Penegakan Mode Read-Only](#penegakan-mode-read-only)
4. [Panduan Pengembang & Operator Data (Level Pengguna Umum)](#4-panduan-pengembang--operator-data-level-pengguna-umum)
   - [Navigasi Schema Explorer & Filter Cepat](#navigasi-schema-explorer--filter-cepat)
   - [Keyset Pagination & Pencarian Data Kilat](#keyset-pagination--pencarian-data-kilat)
   - [Spreadsheet CRUD & Pengeditan Sel Langsung (Inline)](#spreadsheet-crud--pengeditan-sel-langsung-inline)
   - [Preset Nilai Instan & Mutasi Baris Tanpa Primary Key](#preset-nilai-instan--mutasi-baris-tanpa-primary-key)
   - [Impor CSV Bebas Dependensi & Pemetaan Kolom](#impor-csv-bebas-dependensi--pemetaan-kolom)
   - [Kanvas Visual ERD Interaktif](#kanvas-visual-erd-interaktif)
   - [BYOK AI Copilot & Automated Error Healer](#byok-ai-copilot--automated-error-healer)
   - [Unified Operational Notes Inbox & Pengingat Follow-up](#unified-operational-notes-inbox--pengingat-follow-up)
   - [Ergonomi Ruang Kerja, Tab Context Menu & Fluid Splitter](#ergonomi-ruang-kerja-tab-context-menu--fluid-splitter)
5. [Playbook Lanjutan DBA & DevOps (Level DBA)](#5-playbook-lanjutan-dba--devops-level-dba)
   - [AST Safety Guard & Challenge Modal (Pencegah Query Rusak)](#ast-safety-guard--challenge-modal-pencegah-query-rusak)
   - [Smart Tiered Backup Otomatis Pra-DDL](#smart-tiered-backup-otomatis-pra-ddl)
   - [Inspeksi Lock Contention Pra-Eksekusi & Root Blocker Tree](#inspeksi-lock-contention-pra-eksekusi--root-blocker-tree)
   - [Processlist Real-Time & Pembatalan Query Out-of-Band](#processlist-real-time--pembatalan-query-out-of-band)
   - [Panel Tren Aktivitas Query (Processlist Concurrency Trend)](#panel-tren-aktivitas-query-processlist-concurrency-trend)
   - [Inspektur Cluster & Replikasi (PXC / Galera / Replika MySQL)](#inspektur-cluster--replikasi-pxc--galera--replika-mysql)
   - [Performance Flight Recorder (Telemetri Quad-Metric)](#performance-flight-recorder-telemetri-quad-metric)
   - [Inspektur Server Variables & Advisor Tuning Performa](#inspektur-server-variables--advisor-tuning-performa)
   - [Heuristic Index Advisor & Query Tuner (Aturan ESR)](#heuristic-index-advisor--query-tuner-aturan-esr)
   - [Structure Sync (Perbandingan DDL & Pembuat Migrasi AST)](#structure-sync-perbandingan-ddl--pembuat-migrasi-ast)
   - [Cross-Connection Data Transfer & Keyset Row Sync](#cross-connection-data-transfer--keyset-row-sync)
   - [Headless CLI Suite (`run`, `transfer`, `advise`)](#headless-cli-suite-run-transfer-advise)
   - [Task Runner Terjadwal Deklaratif Tanpa Daemon](#task-runner-terjadwal-deklaratif-tanpa-daemon)
   - [Pengujian Plafon Memori & Profiling `pprof`](#pengujian-plafon-memori--profiling-pprof)
6. [Tabel Pintasan Keyboard (Shortcuts Cheatsheet)](#6-tabel-pintasan-keyboard-shortcuts-cheatsheet)
7. [Tanya Jawab & Penyelesaian Masalah (Troubleshooting & FAQ)](#7-tanya-jawab--penyelesaian-masalah-troubleshooting--faq)

---

## 1. Gambaran Umum & Filosofi Arsitektur

SqlFox dibangun secara khusus bagi para rekayasawan perangkat lunak dan tim DBA yang membutuhkan kecepatan respon seketika, proteksi data berlapis baja, serta wawasan diagnostik mendalam tanpa kompromi konsumsi memori tinggi dari aplikasi desktop berbasis Electron atau Java Virtual Machine (JVM).

### Model Single-Binary Bebas Dependensi CGO
- SqlFox dikompilasi menjadi **satu berkas binary mandiri** yang telah menyertakan antarmuka web modern (*embedded SPA*).
- Sepenuhnya bebas dependensi eksternal: tidak memerlukan Node.js, Chromium, Java, maupun library shared C eksternal.
- Driver basis data murni Go (*pure-Go*) memungkinkan kompilasi silang instan ke berbagai arsitektur CPU (`amd64`, `arm64`) di Linux, macOS, dan Windows.

### Plafon Memori Ketat (< 100MB RAM)
- Bahkan saat terhubung ke katalog enterprise berskala raksasa (>1.000 tabel, >50.000 kolom) atau melakukan streaming jutaan baris data, SqlFox secara ketat mempertahankan penggunaan RAM di bawah **100MB RSS**.
- Rahasia efisiensi ini didorong oleh:
  - Transport streaming berpotongan (*chunked streaming*) via WebSocket/SSE.
  - Penomoran halaman berbasis *Keyset Pagination* tanpa penumpukan *buffer* baris di RAM.
  - Virtualisasi pohon skema sidebar menggunakan fitur CSS asli `content-visibility: auto`.
  - Filter pencarian *debounced* 150ms dan antrean *fail-fast timeout* 3 detik pada connection pool.

### Dukungan Mesin Database & Driver
| Mesin Database | Versi yang Didukung | Keterangan Driver |
| :--- | :--- | :--- |
| **MySQL** | 5.7, 8.0, 8.4+ | Pure-Go `go-sql-driver/mysql` dengan dukungan penuh SSL/TLS dan TLS 1.3 |
| **MariaDB** | 10.3 s/d 11.x+ | Protokol native MySQL dengan inspeksi kluster Galera bawaan |
| **PostgreSQL** | 9.6 s/d 16+ | Driver native `pgx/v5` dengan pembatalan query `pg_cancel_backend` |
| **SQLite** | 3.x | Engine murni Go tanpa CGO `modernc.org/sqlite` (lokal maupun via SSH) |

---

## 2. Instalasi & Mode Menjalankan Aplikasi

### File Binary Standalone (Linux, macOS, Windows)
Unduh arsip rilis terbaru di halaman resmi [GitHub Releases](https://github.com/herliansyah/sqlfox/releases):

```bash
# Linux amd64 (Download & ekstrak sekali jalan)
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.6.1-linux-amd64.tar.gz | tar -xz
./sqlfox

# macOS Apple Silicon (ARM64 M1/M2/M3/M4)
curl -sL https://github.com/herliansyah/sqlfox/releases/latest/download/sqlfox-v0.6.1-darwin-arm64.tar.gz | tar -xz
./sqlfox
```

Bagi pengguna Windows, unduh file arsip `.zip`, ekstrak berkas `sqlfox.exe`, lalu klik ganda untuk menjalankannya.

### Deployment via Docker & Docker Compose
SqlFox menyediakan image container multi-arsitektur resmi (`linux/amd64` dan `linux/arm64`) melalui GitHub Container Registry (GHCR):

```bash
docker run -d \
  --name sqlfox \
  -p 8080:8080 \
  -v sqlfox_data:/data \
  --add-host=host.docker.internal:host-gateway \
  ghcr.io/herliansyah/sqlfox:latest
```

#### Template Docker Compose (`docker-compose.yml`)
```yaml
services:
  sqlfox:
    image: ghcr.io/herliansyah/sqlfox:latest
    container_name: sqlfox
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      # Opsional: tentukan PIN statis (jika kosong, auto-generate ke /data/master.key)
      # - SQLFOX_PASS=pin-keamanan-anda
      - SQLFOX_LOG_LEVEL=info
    volumes:
      - sqlfox_data:/data
    extra_hosts:
      - "host.docker.internal:host-gateway"

volumes:
  sqlfox_data:
```

> **Tips Menghubungkan ke Database di Komputer Host**: Untuk terhubung ke MySQL atau PostgreSQL yang berjalan langsung di sistem operasi host Anda, isi Host koneksi dengan `host.docker.internal` (bukan `127.0.0.1` atau `localhost`).

### Instalasi via Go Tools
```bash
go install github.com/herliansyah/sqlfox/cmd/sqlfox@latest
sqlfox
```

### Kompilasi Manual dari Source Code
```bash
git clone https://github.com/herliansyah/sqlfox.git
cd sqlfox
go build -o sqlfox ./cmd/sqlfox
./sqlfox
```

### Opsi CLI & Fitur Port Hunting
SqlFox menyediakan argumen baris perintah yang fleksibel:

```bash
# Menentukan port dan host binding secara manual
./sqlfox -port 8080 -host 127.0.0.1

# Menjalankan mode headless server (tanpa membuka browser otomatis)
./sqlfox -no-browser

# Mengaktifkan log diagnostik verbose / debug ke terminal
./sqlfox -v
./sqlfox -log-level debug

# Format log terstruktur JSON
./sqlfox -log-format json

# Menyimpan log diagnostik ke file (dual-stream: terminal + file)
./sqlfox -log-file sqlfox.log -log-level debug

# Mode senyap (menonaktifkan log banner dan HTTP request di terminal)
./sqlfox -silent   # atau -q

# Mengaktifkan profil diagnostik memori loopback
SQLFOX_PPROF=1 ./sqlfox -port 9090
```

| Opsi Flag | Nilai Default | Penjelasan |
| :--- | :--- | :--- |
| `-port <nomor>` | `8080` | Port HTTP (dilengkapi fitur pencarian port berurutan otomatis) |
| `-host <ip>` | `127.0.0.1` | Alamat IP antarmuka jaringan (`0.0.0.0` untuk akses LAN dengan proteksi PIN) |
| `--lan` | `false` | Buka akses jaringan LAN & tablet (bind ke `0.0.0.0` dan tampilkan Master PIN) |
| `-log-level <lvl>`| `info` | Tingkat rincian log minimum: `debug`, `info`, `warn`, `error` |
| `-v`, `-verbose` | `false` | Pintasan cepat untuk `-log-level debug` |
| `-log-format <fmt>`| `text` | Format log terstruktur: `text` (mudah dibaca manusia) atau `json` |
| `-log-file <path>`| `""` | Lokasi berkas log persistent (menulis ke terminal dan file sekaligus) |
| `-silent`, `-q` | `false` | Mematikan tampilan log di terminal (pencatatan file tetap berjalan jika diset) |
| `-no-browser` | `false` | Menjalankan server tanpa membuka browser web bawaan sistem operasi |
| `-strict-port` | `false` | Langsung gagal jika port yang diminta sedang digunakan |
| `-multi` | `false` | Izinkan beberapa instance SqlFox berjalan bersamaan di port berbeda |

> [!NOTE]
> **Proteksi Single-Instance & Port Hunting**:
> - Jika SqlFox sudah berjalan aktif di PC ini, menjalankan kembali `./sqlfox` akan otomatis mendeteksi instance yang sudah ada, membukanya di browser, lalu keluar tanpa menyalakan server baru di port lain.
> - Jika port terisi oleh aplikasi lain (seperti Apache/Nginx), SqlFox otomatis mencari port berurutan berikutnya yang bebas (8081, 8082, dst.).
> - Untuk sengaja menjalankan lebih dari satu instance, gunakan flag `-multi` (atau variabel lingkungan `SQLFOX_MULTI_INSTANCE=1`).

### Konfigurasi Logging Saat Dobel-Klik di Windows
Saat Anda menjalankan `sqlfox.exe` dengan cara klik ganda (double-click) di Windows Explorer, jendela terminal tidak terbuka sehingga argumen flag baris perintah tidak dapat diketikkan secara langsung. SqlFox menyediakan 4 metode mudah untuk mengatur tingkat verbositas dan penyimpanan berkas log:

1. **Berkas Konfigurasi Otomatis (`sqlfox.env` atau `.env`)**:
   Buat atau letakkan file teks bernama `sqlfox.env` atau `.env` di folder yang sama dengan `sqlfox.exe` (atau di folder profil pengguna `~/.sqlfox/sqlfox.env`). SqlFox akan membaca dan menerapkannya secara otomatis saat dijalankan:
   ```ini
   # sqlfox.env
   SQLFOX_LOG_LEVEL=debug
   SQLFOX_LOG_FILE=sqlfox-debug.log
   SQLFOX_LOG_FORMAT=text
   # SQLFOX_SILENT=true
   ```
2. **Shortcut Desktop Windows**:
   - Klik kanan `sqlfox.exe` &rarr; **Show more options** &rarr; **Create shortcut** (Buat pintasan).
   - Klik kanan shortcut baru tersebut &rarr; **Properties**.
   - Pada kolom **Target**, tambahkan parameter flag di bagian akhir:
     ```
     C:\path\to\sqlfox.exe -log-level debug -log-file C:\path\to\sqlfox.log
     ```
   - Cukup dobel-klik shortcut ini setiap kali ingin menjalankan SqlFox dengan opsi log tersebut.
3. **Skrip Batch Eksekutor (`run-sqlfox.bat`)**:
   Buat file batch kecil di folder yang sama dengan `sqlfox.exe`:
   ```bat
   @echo off
   start "" "%~dp0sqlfox.exe" -v -log-file "%~dp0sqlfox-debug.log"
   ```
4. **Environment Variables Windows**:
   Atur variabel lingkungan sistem/pengguna `SQLFOX_LOG_LEVEL=debug` atau `SQLFOX_LOG_FILE=C:\logs\sqlfox.log` melalui pengaturan Windows (*System Properties* &rarr; *Environment Variables*). SqlFox akan selalu membaca nilai tersebut terlepas dari cara file dijalankan.

### Remote Access Guard & PIN Keamanan
Saat Anda menjalankan SqlFox di server jaringan lokal atau mengaksesnya via IP LAN (`http://192.168.1.50:8080`):
1. **Remote Access Guard** akan langsung menampilkan layar kunci proteksi (*Lock Screen*) pada perangkat non-localhost.
2. Pengguna wajib memasukkan **Master Password** atau **Master PIN** 6 digit yang tercetak di layar terminal induk.
3. Fitur ini melindungi akses database Anda saat dibuka dari iPad, tablet, atau laptop rekan tim di satu jaringan WiFi tanpa khawatir bocornya kredensial.
4. **Proteksi Privasi Shoulder-Surfing**: Di header aplikasi desktop, badge Master PIN secara default dimaskir (`fox-••••`). Klik ikon **👁️** untuk mengintip PIN sementara saat menghubungkan tablet, mencegah PIN terlihat orang lain di ruang kerja terbuka (*open space/coffee shop*) atau saat *screen sharing*.

---

## 3. Connection Vault & Arsitektur Keamanan

### Enkripsi AES-256-GCM & KDF Argon2id
- Seluruh host database, port, username, password, dan SSH private key tersimpan rapi di basis data SQLite lokal terenkripsi (`~/.sqlfox/vault.db`).
- Setiap catatan dienkripsi menggunakan blok sandi **AES-256-GCM** dengan vektor inisialisasi (IV) 96-bit unik per catatan.
- Kunci derivasi diolah menggunakan algoritma **Argon2id** berstandar keamanan tinggi guna menangkal serangan *brute-force* berbasis kluster GPU.
- Kredensial dapat diekspor menjadi arsip portabel (`.sqlfox-vault`) berpaspor sandi untuk mempermudah migrasi perangkat kerja atau berbagi profil koneksi tim secara aman.

### SSH Tunneling Native (Bastion / Jump Host)
SqlFox memiliki klien SSH native pure-Go bawaan di dalam binary:
- Terhubung secara aman ke instans AWS RDS, Google Cloud SQL, atau server on-premise yang berada di dalam subnet privat tanpa IP publik.
- Mendukung otentikasi kata sandi serta private key SSH **RSA** maupun **Ed25519** ber-passphrase.
- Berjalan sepenuhnya di dalam memori tanpa perlu mengeksekusi proses `ssh` eksternal di OS.

### Konfigurasi Multi-Engine (MySQL, PostgreSQL, SQLite)
1. Klik tombol **+ New Connection** di sidebar kiri.
2. Pilih dialek database yang Anda tuju:
   - **MySQL / MariaDB**: Host, Port (standar `3306`), Username, Password, Database Name, dan mode SSL.
   - **PostgreSQL**: Host, Port (standar `5432`), Username, Password, Database, SSL, dan skema awal **Search Path** (`public` atau skema kustom).
   - **SQLite**: Path berkas absolut di disk lokal (contoh `/var/db/app.db`) atau path berkas remote melalui tunnel SSH.
3. Klik **Test Connection** untuk memvalidasi koneksi sebelum menyimpannya ke Vault.

### Tingkatan Environment (Production, Staging, Development)
Setiap koneksi dapat diberi label tingkat operasional (*trust tier*):
- `Production 🔴`: Penegakan Safety Guard level tertinggi. Setiap query berisiko destruktif wajib melalui verifikasi pengetikan nama tabel manual dan backup otomatis pra-DDL.
- `Staging 🟡`: Pengecekan AST dengan dialog konfirmasi eksplisit.
- `Development 🟢`: Konfirmasi ringkas untuk kenyamanan rekayasa cepat.

Badge warna environment (`[PROD]`, `[STAGE]`, `[DEV]`) selalu terpampang jelas di header atas untuk mencegah salah eksekusi.

### Penegakan Mode Read-Only
Klik tombol **🔒 Read-Only** pada bilah menu atas kapan saja:
- Sesi aktif akan seketika berubah menjadi mode replika baca (*read-only*).
- Seluruh perintah mutasi (`INSERT`, `UPDATE`, `DELETE`, `ALTER`, `DROP`, `TRUNCATE`, `CREATE`) dicegat langsung di lapisan driver sebelum dikirim ke kabel jaringan database.
- Sangat penting saat DBA melakukan investigasi insiden, audit data sensitif, atau peninjauan log produksi.

---

## 4. Panduan Pengembang & Operator Data (Level Pengguna Umum)

### Navigasi Schema Explorer & Filter Cepat
- **Filter Bar Sidebar**: Ketik nama tabel atau view di kolom filter sidebar; hasil tersaring seketika dengan debounce 150ms.
- **Peralihan Skema PostgreSQL**: Berpindah skema dari `public` ke skema tenant tanpa perlu membuat koneksi baru.
- **Menu Klik Kanan Tabel**: Klik kanan tabel mana pun untuk membuka data (*Open Table*), melihat struktur pembuatan (*View DDL*), atau memeriksa ukuran disk (*Inspect Storage*).
- **Pencarian Skema Universal (Ctrl+P)**: Buka jendela Schema Search untuk melompat langsung ke tabel atau kolom apa pun dalam hitungan milidetik.
- **Sinkronisasi Cepat Katalog Skema (`🔄 All`)**: Klik tombol **🔄 All** di samping pemilih database (*Database Switcher*) untuk memperbarui cache autocomplete dan pencarian lintas database server secara instan.
  - **Bebas Kunci Foreign Key (Zero FK Lock)**: Memindai hanya identitas tabel dan kolom dalam satu sapuan cepat (`InspectFastCatalog`) tanpa memicu relasi berat (`KEY_COLUMN_USAGE`).
  - **Pengecekan Kesehatan Pra-Eksekusi**: Memeriksa beban thread aktif server (`ActiveThreads`) sebelum sinkronisasi dimulai—memberikan peringatan jika thread aktif melebihi 50 (waspada) atau 100 (kritis).
  - **Lencana Waktu Cache**: Menampilkan stempel waktu sinkronisasi terakhir (`#schema-cache-badge`) di sebelah tombol penyegaran skema database aktif.

### Keyset Pagination & Pencarian Data Kilat
Sistem pagination konvensional `OFFSET 1000000` memaksa server memindai dan membuang jutaan baris sebelum menampilkan data. SqlFox menerapkan **Keyset Pagination**:
- Menelusuri baris data berdasarkan indeks kunci (`WHERE id > :last_id ORDER BY id LIMIT 50`).
- Perpindahan halaman tetap kilat dan tanpa lonjakan RAM, bahkan pada tabel berukuran 100 juta baris lebih.
- Manfaatkan **Inline Filter Bar** di atas tabel untuk memfilter nilai kolom spesifik secara instan.

### Spreadsheet CRUD & Pengeditan Sel Langsung (Inline)
- **Klik ganda sel mana saja** untuk mengedit nilai baris selayaknya aplikasi spreadsheet modern.
- **Indikator Perubahan**: Sel yang diubah akan disorot warna hijau zamrud (*emerald*) dengan lencana penghitung perubahan (*staged changes*).
- **Review & Apply**: Klik tombol **Review Changes** untuk meninjau sintaks `UPDATE` yang dihasilkan sebelum dieksekusi ke database.
- **+ Add Row**: Tambahkan draf baris baru di bagian bawah tabel, isi nilainya, lalu terapkan secara massal dalam satu transaksi.
- **Penghapusan Banyak Baris**: Centang beberapa kotak centang baris lalu klik tombol Hapus; proteksi Safety Guard akan otomatis aktif memverifikasi aksi Anda.

### Preset Nilai Instan & Mutasi Baris Tanpa Primary Key
- **Preset Nilai**: Klik kanan sel tabel atau tekan tombol pintasan:
  - `Alt+N` &rarr; Mengisi nilai `NULL`
  - `Alt+E` &rarr; Mengisi string kosong `""`
  - `Alt+T` &rarr; Mengisi fungsi waktu `NOW()`
  - `Alt+D` &rarr; Mengisi tanggal saat ini (`YYYY-MM-DD`)
  - `Alt+U` &rarr; Mengisi nilai unik `UUID v4`
- **Tabel Tanpa Primary Key (PK-Less)**: Jika tabel tidak memiliki kolom Primary Key, SqlFox secara otomatis mempersempit kriteria mutasi ke tepat satu baris fisik (`singleRowWhere`: `LIMIT 1` pada MySQL, `rowid IN (SELECT rowid ... LIMIT 1)` pada SQLite, dan `(tableoid, ctid) IN (SELECT tableoid, ctid ... LIMIT 1)` pada PostgreSQL), menjamin bahwa pengeditan atau penghapusan satu baris tidak akan merusak atau mengubah baris duplikat identik lainnya.

### Virtual Grid Ergonomics Suite
SqlFox menyediakan fitur eksplorasi kisi data (*virtual grid*) tingkat lanjut yang dirancang untuk mempermudah navigasi tabel berkolom banyak:
- **Filter Cepat Klik Kanan (Gaya TablePlus)**:
  - Klik kanan sel data mana saja pada penampil tabel atau hasil query untuk memunculkan aksi filter instan: `kolom = 'nilai'`, `kolom != 'nilai'`, `kolom > 'nilai'`, `kolom < 'nilai'`, `kolom LIKE '%nilai%'`, `kolom IS NULL`, atau `IS NOT NULL`.
  - Secara otomatis menerapkan *escape* string dan tanda kutip sesuai dialek database aktif.
  - Pada penampil tabel (*Table Viewer*), klausa kriteria otomatis ditambahkan ke bilah filter (`AND kolom = 'nilai'`) dan hasil diperbarui seketika.
  - Pada editor query (*Query Editor*), kriteria otomatis digabungkan ke dalam klausa `WHERE` aktif dan query langsung dieksekusi.
- **Pemilih Visibilitas Kolom (Gaya TablePlus)**:
  - Klik tombol **Columns (N/Total)** di toolbar penampil tabel untuk memilih kolom mana yang ingin ditampilkan atau disembunyikan.
  - **Pencarian Kolom**: Ketik nama kolom pada kotak pencarian untuk menemukan kolom dengan cepat pada tabel yang memiliki puluhan kolom.
  - **Show All / Hide All**: Tampilkan atau sembunyikan semua kolom opsional dalam satu klik.
  - **Proteksi Permanen Primary Key**: Kolom Primary Key dikunci secara permanen (`🔒 PKs cannot be hidden`) untuk menjaga integritas keyset pagination dan mutasi data baris.
  - **Penyimpanan Status Per Tab**: Pengaturan visibilitas kolom disimpan ke dalam `sessionStorage` per koneksi, database, dan tabel, sehingga konfigurasi tampilan Anda tetap terjaga saat berpindah tab.
- **Inspektur Baris Vertikal / Tampilan Formulir Baris Tunggal (Gaya DBeaver)**:
  - Pilih baris data lalu tekan tombol **`Spacebar`**, atau klik kanan sel dan pilih **`📋 Inspect Record (Form View)`**.
  - Mengubah tampilan baris horizontal yang lebar (50+ kolom, data JSON bertingkat, atau teks panjang) menjadi kartu formulir dua kolom vertikal yang responsif.
  - **Paginasi Baris Langsung**: Berpindah antar baris dengan mudah menggunakan tombol **Previous / Next** atau tombol panah keyboard **ArrowLeft / ArrowRight**.
  - **Pencarian Field**: Saring field yang ditampilkan secara instan dengan input pencarian bawaan.
  - **Salin 1-Klik**: Salin seluruh data baris aktif sebagai format JSON yang rapi (`Copy JSON`) atau teks berpemisah tab (`Copy Text`).
  - **Pengeditan Terintegrasi**: Ubah nilai data secara langsung di dalam input atau textarea modal; perubahan ditandai dengan `● Modified` dan terhubung langsung ke pelacak transaksi `stagedEdits` serta bilah simpan (*Save Changes*).

### Impor CSV Bebas Dependensi & Pemetaan Kolom
1. Buka tabel yang diinginkan lalu klik tombol **Import CSV**.
2. Pilih file `.csv` Anda. SqlFox akan memindai baris awal dan memetakan header secara otomatis.
3. Sesuaikan pemetaan kolom target (*column mapping*).
4. Tentukan ukuran batch (standar 500 baris) dan strategi penanganan konflik (*Skip on Error* / *Stop on Error*).
5. Pantau bilah kemajuan impor secara real-time dengan metrik kecepatan baris per detik.

### Kanvas Visual ERD Interaktif
Buka melalui menu **📦 DB Tools &rarr; 🗺️ Visual ERD Canvas**:
- **Pemetaan Relasi Otomatis**: Menampilkan seluruh relasi Foreign Key dengan garis kurva Bézier SVG yang halus.
- **Interaksi Drag & Drop**: Geser kartu tabel secara leluasa di kanvas luas tak terbatas. Posisi kartu tersimpan otomatis per sesi.
- **Auto Layout**: Menata posisi seluruh tabel secara otomatis berdasarkan hierarki topologis relasi.
- **Fit Viewport**: Menyesuaikan skala dan memusatkan seluruh diagram skema dalam satu klik.
- **Detail Kartu**: Menampilkan daftar nama kolom, tipe data, serta lencana Primary Key (`🔑`) dan Foreign Key (`🔗`).

### BYOK AI Copilot & Automated Error Healer
SqlFox dilengkapi asisten AI dengan arsitektur **Bring-Your-Own-Key (BYOK)**:
- Mendukung provider terkemuka: **OpenAI**, **Anthropic Claude**, **Google Gemini**, **DeepSeek**, serta model lokal offline **Ollama** (`http://localhost:11434`).
- **Injeksi Konteks Cerdas**: Menyuntikkan struktur tabel aktif dan dialek database secara otomatis ke dalam konteks percakapan.
- **Eksekusi SQL 1-Klik**: Setiap blok kode SQL hasil generasi AI dilengkapi tombol `Insert & Run` dan `Replace in Editor`.
- **Chat Session Forking**: Percakapan dapat dicabangkan (*fork*) menjadi alur baru kapan saja untuk mencoba variasi pertanyaan tanpa kehilangan riwayat lama.
- **Automated Error Healer**: Jika query Anda mengalami kegagalan di editor, klik tombol **🩺 Heal Error**; AI akan menganalisis pesan galat dari database, membeberkan letak kesalahannya, dan menyajikan query perbaikan yang siap dijalankan.

### Unified Operational Notes Inbox & Pengingat Follow-up
SqlFox menghadirkan sistem catatan operasional terpadu (*Operational Notes Inbox*) langsung di dalam aplikasi, menjembatani telemetri diagnostik database dengan dokumentasi runbook pengembang:

- **Kotak Masuk Catatan Terpusat (Central Notes Inbox)**:
  - Buka laci catatan (*slide-over drawer* `#notes-inbox-drawer`) kapan saja melalui tombol **📝 Notes** di header atas.
  - Badge angka merah menampilkan jumlah pengingat yang telah jatuh tempo (*overdue*) dan butuh tindak lanjut segera.
  - Saring catatan dengan mudah lewat tab status (`All`, `Inbox`, `Resolved`, `Archived`), fitur pencarian teks instan, atau pin catatan penting (`📌`).
- **Note Composer Komprehensif**:
  - Klik **+ New Note** atau simpan langsung dari hasil query, baris Processlist, maupun tabel database.
  - Setiap catatan mengabadikan:
    - **Metadata**: Judul, status siklus hidup (`inbox`, `resolved`, `archived`), dan penanda pin.
    - **Tag Objek Database**: Soft-link berformat URI (`db://{connection}/{database}/{table}`) yang menautkan catatan secara presisi ke objek database terkait.
    - **Tagar Kustom**: Kategorisasi fleksibel (misal `#incident`, `#migration`, `#runbook`).
    - **Catatan Analisis Developer**: Area Markdown kaya untuk menuangkan investigasi manusia, langkah perbaikan, dan instruksi tim.
    - **Raw Snapshot**: Salinan read-only dari query SQL asli, penjelasan AI, atau log diagnostik dengan penyorotan sintaks dan live preview.
- **Pengingat Terjadwal & Alarm Audio Web Audio API**:
  - Jadwalkan pengingat menggunakan preset cepat (`10m`, `1h`, `Besok 09:00`, `Senin Depan`) atau pemilih tanggal & jam manual.
  - Mesin latar belakang mengevaluasi alarm setiap 30 detik.
  - Saat alarm berbunyi, SqlFox memainkan nada dering (*audio chime*) berfrekuensi D5-A5 via native **Web Audio API** (tanpa dependensi file MP3 eksternal) dan memunculkan toast interaktif non-blocking.
  - Operator dapat langsung mengambil tindakan dari notifikasi: **Buka Catatan**, **Tunda 10 Menit (Snooze)**, atau tandai **Selesai**.
- **Feed Runbook Khusus di Table Viewer**:
  - Buka sub-tab **📝 Notes & Runbook** pada tabel mana pun yang sedang dibuka.
  - Otomatis menampilkan feed catatan yang ditautkan ke tabel aktif tersebut (`db://...`), menempatkan dokumentasi skema, catatan anomali data, dan runbook tepat di samping editor data.
- **Integrasi Mention `@` di AI Copilot**:
  - Ketik `@` di kotak pesan AI Copilot untuk memunculkan dropdown pilihan catatan secara interaktif.
  - Memilih catatan akan menyisipkan token referensi `[@note:<judul>]`.
  - Saat prompt dikirim, SqlFox mengekstrak catatan terkait dan menyuntikkan raw snapshot serta catatan developer langsung ke sistem prompt context AI, memampukan AI menganalisis insiden masa lalu dan kueri terkait secara akurat.

### Ergonomi Ruang Kerja, Tab Context Menu & Fluid Splitter
SqlFox dirancang dengan fokus pada kenyamanan sesi pengodean dan investigasi database intensif:

- **Workspace Pane Splitter (Pemisah Fleksibel Editor-Grid)**:
  - Geser garis horizontal pemisah di antara Query Editor dan Virtual Data Grid untuk mengatur pembagian layar sesuai kebutuhan (memperbesar editor SQL vs memperluas tampilan tabel hasil).
  - Dilengkapi pengaman batas minimum (*Strict Boundary Guards*): Editor minimal 80px dan Grid minimal 120px untuk mencegah tombol toolbar dan paginasi terpotong.
  - **Double-Click Reset 50:50**: Klik ganda pada garis pemisah untuk mengembalikan ukuran panel ke rasio seimbang **50:50** secara instan.
  - **Penyimpanan Preferensi Global**: Ukuran panel yang Anda pilih otomatis disimpan di `localStorage` dan berlaku konsisten di seluruh tab query serta saat refresh browser.
  - **Visibilitas Kontekstual**: Splitter otomatis tersembunyi saat membuka tab non-query seperti Table Viewer spreadsheet, Processlist, atau Cluster monitor.
- **Workspace Tab Context Menu & Siklus Hidup Tab**:
  - Klik kanan pada tab workspace mana saja untuk membuka menu aksi: `Tutup Tab` (`Ctrl+W`), `Tutup Tab Lainnya`, `Tutup Tab di Sebelah Kanan`, `Tutup Semua Tab`, `Duplikat Tab` (menyalin buffer SQL dan filter), `Ubah Nama Tab...`, dan `Simpan sebagai Catatan Operasional...`.
  - **Tab Close Confirmation Guard**: Mencegah hilangnya pekerjaan dengan dialog konfirmasi aman jika tab memiliki draf SQL yang belum dieksekusi atau perubahan belum tersimpan. Tab bersih dan kosong akan langsung tertutup tanpa interupsi modal.
  - **Reset Ruang Kerja Bersih**: Menutup semua tab atau tab terakhir akan mereset ruang kerja secara anggun ke tab `Query 1.sql` baru yang bersih.

---

## 5. Playbook Lanjutan DBA & DevOps (Level DBA)

### AST Safety Guard & Challenge Modal (Pencegah Query Rusak)
Sebelum sebuah query dikirimkan ke server basis data, mesin pengurai AST SqlFox membedah struktur perintah SQL tersebut:
- **Kategori Query yang Dicegat**:
  - `DROP TABLE`, `DROP DATABASE`, `DROP SCHEMA`
  - Perintah `TRUNCATE TABLE`
  - Perintah `DELETE FROM tabel` tanpa klausa `WHERE`
  - Perintah `UPDATE tabel SET ...` massal tanpa klausa `WHERE`
  - Perintah mutasi data dengan potensi dampak di luar ambang batas aman
- **Challenge Modal**: Menampilkan preview query yang dicegat, menjabarkan risiko fatalnya, dan mewajibkan DBA mengetik nama tabel target secara manual dan presisi sebelum eksekusi diperkenankan berjalan.
- **Inspeksi Multi-Statement Senyawa**: Memeriksa setiap baris perintah dalam satu batch multi-statement secara individual. Mencegah penyelundupan perintah destruktif (misal `SELECT 1; DROP TABLE korban`) dan menolak eksekusi jika terdapat lebih dari satu perintah destruktif yang memerlukan konfirmasi challenge.
- **Sanitasi Komentar & String Literal**: Mengaburkan (*masking*) komentar baris/blok dan teks literal sebelum pengecekan regex, mencegah manipulasi kata kunci destruktif.
- **Pra-Pemrosesan CTE**: Menanggalkan klausa awalan `WITH ... AS (...)` untuk memeriksa perintah inti DML/DDL di bawah pengawasan Safety Guard.

### Smart Tiered Backup Otomatis Pra-DDL
Saat Anda hendak mengeksekusi operasi skema kritis seperti `ALTER TABLE` atau `DROP TABLE`:
1. SqlFox secara otomatis memicu proses **Smart Tiered Backup**.
2. **Struktur DDL**: Dicatat dan diarsipkan ke direktori cadangan lokal (`~/.sqlfox/backups/`).
3. **Data Baris Tabel**: Apabila footprint ukuran tabel berada di bawah 50MB, data baris ikut di-dump secara streaming ke snapshot lokal.
4. Apabila terjadi kegagalan skrip migrasi, DBA dapat memulihkan skema maupun data tabel tersebut seketika.

### Inspeksi Lock Contention Pra-Eksekusi & Root Blocker Tree
Dapat diakses melalui **⚡ Monitor &rarr; 🔒 Lock Contention & Blockers**:
- Menganalisis antrean kunci aktif pada MySQL (`sys.innodb_lock_waits`, `data_locks`) dan PostgreSQL (`pg_locks`, `pg_blocking_pids`).
- **Tampilan Root Blocker Triage**:
  - Menyorot thread biang keladi dengan lencana tegas `🚨 ROOT BLOCKER` yang menampilkan Thread ID, user database, mode kunci yang dipegang (`holding LOCK`), dan identitas resource yang terkunci.
  - **Kontainer Teks SQL Lengkap (Scrollable)**: Menampilkan query SQL lengkap (`font-mono text-[13px]`) dengan batas tinggi gulir vertikal (`max-h-48 overflow-y-auto`) agar query panjang dapat diperiksa tuntas saat insiden produksi.
  - **Tombol Mitigasi Spesifik**:
    - **Kill Query** (Kuning Kunyit): Membatalkan query aktif yang sedang berjalan tanpa memutus koneksi klien.
    - **Kill Blocker** (Merah): Menghentikan seluruh koneksi pemblokir seketika untuk meredakan badai lock contention.
  - **Hierarki Sesi yang Terblokir**: Menampilkan rincian sesi downstream yang tertahan lengkap dengan durasi menunggu yang disorot merah/putih (`waiting 45s for lock`), nama user, dan teks query yang terhenti.

### Processlist Real-Time & Pembatalan Query Out-of-Band
Dapat diakses melalui **⚡ Monitor &rarr; ⚡ Processlist**:
- Menampilkan daftar seluruh thread server, durasi berjalan, host klien, dan status query aktif dalam format tabel yang disegarkan otomatis.
- Opsi filter untuk menyembunyikan thread tidur (*Sleep*) agar DBA dapat berfokus pada thread yang sedang bekerja.
- **Pembatalan Out-of-Band**: Tombol **Kill Query** mengirimkan perintah terminasi (`KILL QUERY <id>` pada MySQL atau `SELECT pg_cancel_backend(<pid>)` pada PostgreSQL) melalui koneksi kontrol terpisah, sehingga koneksi utama aplikasi tidak terputus.
- **Modal Detail Proses**: Inspeksi teks lengkap SQL, user, host, database, state, waktu eksekusi, serta lock dependency dengan salin clipboard 1-klik (didukung fallback cerdas untuk protokol HTTP biasa tanpa SSL).

### Panel Tren Aktivitas Query (Processlist Concurrency Trend)
Dapat diakses melalui **⚡ Monitor &rarr; ⚡ Processlist** (Klik tombol **📈 Activity Trend**):
- **Telemetri Konkurensi Real-Time**: Merender grafik deret waktu (*time-series*) bergulir 60 titik secara efisien di kanvas HTML5 Canvas native tanpa beban komputasi tambahan di sisi database.
- **Visualisasi Dua Metrik**:
  - Garis biru langit (*Sky blue*) dengan arsiran gradien halus menggambarkan kueri yang sedang dieksekusi aktif (`Command != 'Sleep'`).
  - Garis putus-putus abu-abu (*Slate*) menggambarkan total seluruh thread klien yang tersambung.
- **Indikator Metrik Cepat**: Menampilkan statistik operasional langsung (`Running Now`, `Total Threads`, `Peak Concurrency`, dan `60s Avg Running`).
- **Eskalasi ke Flight Recorder**: Tombol 1-klik **Escalate to Flight Recorder** untuk langsung mengaktifkan perekaman diagnostik resolusi tinggi per 1 detik saat terjadi lonjakan query mencurigakan.
- **Desain Kolapsibel**: Tombol buka/tutup panel dengan status tersimpan otomatis di `localStorage` (`sqlfox_pl_trend_open`).

### Inspektur Cluster & Replikasi (PXC / Galera / MySQL 8.4+ / PostgreSQL)
Dapat diakses melalui **⚡ Monitor &rarr; 🌐 Cluster**:
- Dioptimalkan untuk **Percona XtraDB Cluster (PXC)**, **MariaDB Galera**, **MySQL 8.4 LTS / 9.x**, dan replikasi **PostgreSQL Primary/Standby**.
- **Universal Replication Normalizer (MySQL & MariaDB)**:
  - Otomatis mengeksekusi sintaks modern `SHOW REPLICA STATUS` dan `SHOW BINARY LOG STATUS`, dengan fallback aman ke `SHOW SLAVE STATUS` dan `SHOW MASTER STATUS` pada MySQL 5.7 / 8.0 lawas.
  - Menyelaraskan nama metrik kunci (`replica_io_running` vs `slave_io_running`, `seconds_behind_source` vs `seconds_behind_master`) di seluruh generasi MySQL.
  - Pada MariaDB, otomatis mendeteksi ketiadaan skema `sys` dan beralih ke tabel bawaan `information_schema.innodb_lock_waits`.
- **Telemetri Replikasi & Standby PostgreSQL**:
  - Otomatis memantau `pg_stat_replication` pada primary (klien streaming, write/flush/replay LSN, status sinkronisasi) dan `pg_stat_wal_receiver` pada standby.
- **Pendeteksi Flow Control Galera / PXC**: Memantau metrik jeda replikasi kluster `wsrep_flow_control_paused` secara real-time.
- **Pelacak Node Paling Lambat**: Memeriksa seluruh node rekan di dalam kluster secara otomatis untuk mengisolasi node mana yang mengalami kemacetan I/O atau beban berlebih.
- **Matriks Tri-Metric**: Mengevaluasi latensi jaringan EVS, hambatan disk/InnoDB, dan kapasitas antrean thread *applier worker*.

### Statement Digest Historis & Post-Mortem Deadlock
Dapat diakses melalui **⚡ Monitor &rarr; ⚡ Processlist** (Segmented Pill Switcher):
- **Live Threads**: Memantau dan menghentikan query yang sedang berjalan secara out-of-band.
- **Statement Digest**: Menganalisis latensi persentil dan pola query terlambat secara historis.
  - MySQL/MariaDB: Mengambil metrik dari `sys.statement_analysis` dengan fallback ke `performance_schema.events_statements_summary_by_digest`.
  - PostgreSQL: Memeriksa ekstensi `pg_stat_statements` secara adaptif lengkap dengan petunjuk aktivasi 1-klik jika ekstensi belum diaktifkan.
- **Deadlock Inspector**: Mengekstrak dan membedah blok `LATEST DETECTED DEADLOCK` dari `SHOW ENGINE INNODB STATUS` ke dalam matriks perbandingan konflik Transaction 1 vs Transaction 2.

### Programmability & Matriks Hak Akses User (MySQL & PostgreSQL)
- **Routines Explorer**: Menampilkan Stored Procedure dan Function di sidebar navigasi:
  - MySQL: Memindai `information_schema.routines` dengan aksi 1-klik untuk membuka template eksekusi `CALL sp(...)` di tab editor.
  - PostgreSQL: Memindai `pg_proc` + `pg_namespace` dengan signature tipe parameter yang aman dari overload (`func_name(type1, type2)`) serta ekstraksi DDL via `pg_get_functiondef`.
- **Manajemen User & Role**: Dapat diakses melalui **⚡ Monitor &rarr; 🛡️ Privilege Matrix**:
  - MySQL: Memindai kolom grant `mysql.user` beserta status koneksi SSL dan masa kedaluwarsa sandi.
  - PostgreSQL: Matriks role sadar dialek yang memeriksa atribut `pg_roles` (`SUPERUSER`, `CREATEDB`, `CREATEROLE`, `REPLICATION`, `BYPASSRLS`, `LOGIN`) dan keanggotaan grup (`MEMBER OF`).

### Performance Flight Recorder (Telemetri Quad-Metric)
Dapat diakses melalui **⚡ Monitor &rarr; 📈 Recorder**:
- Perekam telemetri performa on-demand berfrekuensi tinggi (interval sampling 1 detik) yang mencatat 4 metrik vital:
  1. **QPS Diferensial** (Jumlah Query per Detik)
  2. **`Threads_running`** (Thread server yang aktif mengeksekusi perintah)
  3. **Row Lock Waits** (`Innodb_row_lock_waits`)
  4. **Buffer Pool Waits** (`Innodb_buffer_pool_wait_free`)
- **Korelasi Query Lonjakan**: Klik titik lonjakan grafik (*spike*) pada scrubber linimasa untuk melihat daftar query persis yang sedang berjalan pada detik tersebut.
- **Pengaman Auto-Stop 10 Menit**: Perekam otomatis berhenti setelah 10 menit agar tidak membebani server database atau memenuhi ruang penyimpanan.

### Inspektur Server Variables & Advisor Tuning Performa
Dapat diakses melalui **⚡ Monitor &rarr; ⚙️ Server Variables**:
- Memeriksa ratusan variabel dan parameter sistem database dengan fitur pencarian dan pengelompokan kategori.
- **Tuning Advisor**: Menganalisis parameter kunci dan memberikan rekomendasi optimasi:
  - MySQL: Ukuran `innodb_buffer_pool_size`, rasio `max_connections`, dan durabilitas `innodb_flush_log_at_trx_commit`.
  - PostgreSQL: Pengaturan `shared_buffers`, `work_mem`, `maintenance_work_mem`, `effective_cache_size`, `wal_buffers`, `checkpoint_completion_target`, dan `autovacuum`.

### Heuristic Index Advisor & Query Tuner (Aturan ESR)
1. Tulis atau tempel query `SELECT` yang lambat di Query Editor.
2. Klik tombol **Explain** atau buka tab **💡 Advisor**.
3. SqlFox membedah query menggunakan **Aturan ESR** (Equality $\to$ Sort $\to$ Range):
   - Merekomendasikan indeks komposit ideal untuk menghilangkan operasi *filesort* dan *full table scan*.
   - Mendeteksi **Index Bloat** (peringatan bila terdapat >5 indeks sekunder pada tabel yang sama) serta menyarankan jalur konsolidasi indeks.
   - Menyediakan sintaks DDL ramah produksi non-blocking:
     - MySQL: `ALTER TABLE tabel ADD INDEX idx_nama (...) ALGORITHM=INPLACE, LOCK=NONE;`
     - PostgreSQL: `CREATE INDEX CONCURRENTLY idx_nama ON tabel (...);`

### Structure Sync (Perbandingan DDL & Pembuat Migrasi AST)
Dapat diakses melalui **📦 DB Tools &rarr; 🔄 Structure Sync**:
1. Pilih sesi **Source** (Sumber) dan sesi **Target** (Tujuan).
2. SqlFox memindai struktur kedua database, membedah DDL menjadi AST normal, lalu menghitung perbedaan strukturnya:
   - Tabel yang belum ada di target
   - Kolom yang baru, dihapus, atau tipe data yang berubah (dengan sintaks DDL sesuai dialek: `MODIFY COLUMN` pada MySQL vs `ALTER TABLE tabel ALTER COLUMN kolom TYPE ...; ALTER TABLE tabel ALTER COLUMN kolom SET/DROP NOT NULL;` pada PostgreSQL)
   - Perbedaan indeks dan Primary Key
   - Perbedaan relasi Foreign Key
3. Menghasilkan skrip migrasi DDL berurutan yang aman lengkap dengan pratinjau sebelum dijalankan.

### Cross-Connection Data Transfer & Keyset Row Sync
- **Data Transfer**: Melakukan streaming transfer tabel secara langsung dari satu server database ke server lain melintasi penyedia cloud yang berbeda dengan batas penggunaan memori ketat `<50MB`. Apabila opsi `truncate_target` aktif dan transfer gagal, transaksi dibatalkan (*rollback*) sehingga data tabel target tetap terjaga utuh.
- **Row-Level Data Sync**: Membandingkan baris data antar dua tabel menggunakan teknik hashing berbasis keyset delta. Mendukung Primary Key tunggal maupun komposit (dipisahkan pemisah unik `\x1f`), memetakan posisi kolom sumber ke target, menormalkan representasi tipe data lintas driver (`syncValueKey`), serta membatasi perbandingan maksimal 10.000 baris guna melindungi memori server.

### Headless CLI Suite (`run`, `transfer`, `advise`)
SqlFox menyediakan perintah CLI headless yang sangat cocok untuk skrip otomatisasi bash, cronjob, dan pipeline CI/CD:

```bash
# Menjalankan query SQL dan mencetak output JSON atau tabel
./sqlfox run -conn "my-prod-db" -query "SELECT id, status FROM orders WHERE status = 'pending'" -format json

# Menerima query via STDIN menggunakan DSN langsung (tanpa dependensi vault di server CI/CD)
echo "SELECT count(*) FROM users;" | ./sqlfox run -dsn "user:pass@tcp(10.0.0.5:3306)/app" -format csv

# Menjalankan transfer data antar-koneksi secara otomatis
./sqlfox transfer --from "source-db" --to "backup-db" --tables "orders,order_items" --batch 1000

# Menganalisis rekomendasi indeks untuk query lambat dari terminal
./sqlfox advise -conn "my-prod-db" -query "SELECT * FROM logs WHERE tenant_id = 4 AND created_at > NOW() - INTERVAL 1 DAY ORDER BY id DESC"
```

### Task Runner Terjadwal Deklaratif Tanpa Daemon
Jadwalkan pemeliharaan database rutin, dump telemetri berkala, atau pengecekan kesehatan server tanpa perlu menginstal dependensi daemon latar belakang yang berat:
- Konfigurasikan tugas menggunakan file deklaratif berformat YAML atau JSON.
- Dilengkapi mekanisme coba-ulang otomatis (*retry*) dengan *exponential backoff* saat koneksi jaringan terganggu.
- Mendukung pengalihan keluaran (*streaming output*) ke disk lokal atau webhook pemantau.

### Pengujian Plafon Memori & Profiling `pprof`
Untuk membuktikan komitmen efisiensi konsumsi memori `<100MB RAM` pada beban kerja intensif:
1. Jalankan SqlFox dengan mengaktifkan flag profiling:
   ```bash
   SQLFOX_PPROF=1 ./sqlfox -port 8080
   ```
2. Server runtime akan membuka endpoint diagnostik `pprof` yang terproteksi loopback (`http://127.0.0.1:8080/debug/pprof/`).
3. Analisis snapshot alokasi heap memori menggunakan Go tools:
   ```bash
   go tool pprof http://127.0.0.1:8080/debug/pprof/heap
   ```

### In-App Activity Console & Live Diagnostic Stream
Dapat dibuka dengan mengklik tombol **⚡ Activity** pada status bar bawah atau menekan pintasan **`Ctrl + \``**:
- **Aliran Telemetri Real-Time**: Memancarkan event runtime daemon secara langsung melalui Server-Sent Events (`GET /api/logs/stream`) tanpa dependensi eksternal dengan latensi di bawah 10ms.
- **Riwayat Cold-Start Instan**: Secara otomatis menampilkan 100 entri event historis terakhir dari buffer memori (ring buffer) begitu panel dibuka.
- **Tabel Berjalan 5 Kolom**: Menampilkan jam (HH:mm:ss.SSS), level status (`INFO`, `WARN`, `ERROR`, `DEBUG`), kategori modul (`query`, `http`, `schema`, `auth`, `ai`, `sync`), deskripsi aktivitas aplikasi, dan detail JSON interaktif yang bisa di-expand (durasi eksekusi ms, baris terdampak, status HTTP).
- **Smart Auto-Scroll & Freeze**: Tampilan otomatis bergulir ke baris terbaru di bawah, dan secara cerdas menjeda auto-scroll saat operator menggulir ke atas untuk membaca entri lama.
- **Filter Tingkat Keparahan & Pencarian Instan**: Menyaring log berdasarkan severity (`ALL`, `ERROR`, `WARN`, `INFO`, `DEBUG`) serta pencarian teks instan untuk query SQL atau endpoint HTTP.
- **Rekam & Unduh File Log**: Menyimpan riwayat log sesi (hingga 1.000 baris) dengan 1-klik unduh format teks rapi (`.log`) atau format data terstruktur (`.json`).

---

## 6. Tabel Pintasan Keyboard (Shortcuts Cheatsheet)

| Pintasan Keyboard | Cakupan Area | Kegunaan |
| :--- | :--- | :--- |
| `Ctrl + Enter` | Query Editor | Menjalankan query aktif atau teks yang sedang diblok |
| `Ctrl + Shift + Enter` | Query Editor | Mengeksekusi `EXPLAIN` dan membuka Visual Plan Tree |
| `Ctrl + T` | Workspace | Membuka Tab Query baru |
| `Ctrl + W` | Workspace | Menutup tab aktif |
| `Ctrl + K` | Universal | Membuka Command Palette universal |
| `Ctrl + P` | Universal | Membuka Schema Search (Lompat cepat ke tabel/kolom) |
| `Ctrl + B` | Universal | Menyembunyikan/menampilkan Sidebar kiri (Layar luas) |
| `Ctrl + L` | Universal | Menyembunyikan/menampilkan Sidebar AI Copilot |
| `Ctrl + \`` | Universal | Membuka/menutup In-App Activity Console |
| `Alt + N` | Spreadsheet Grid | Mengisi preset nilai `NULL` ke dalam sel aktif |
| `Alt + E` | Spreadsheet Grid | Mengisi preset string kosong `""` |
| `Alt + T` | Spreadsheet Grid | Mengisi preset fungsi waktu sekarang `NOW()` |
| `Alt + D` | Spreadsheet Grid | Mengisi preset tanggal saat ini (`YYYY-MM-DD`) |
| `Alt + U` | Spreadsheet Grid | Mengisi preset nilai unik acak `UUID v4` |
| `F1` atau `?` | Universal | Membuka Modal Pintasan Keyboard & Tautan Dokumentasi |
| `Esc` | Dialog / Modal | Menutup dialog, modal, atau menu pop-up aktif |

---

## 7. Tanya Jawab & Penyelesaian Masalah (Troubleshooting & FAQ)

#### Q: Bagaimana cara mengatasi error `port 8080 already in use`?
> **Jawaban**: Anda tidak perlu mengubah port secara manual! SqlFox memiliki fitur **Port Hunting** bawaan. Jika port 8080 sedang dipakai aplikasi lain, SqlFox secara otomatis akan mendeteksi dan menggunakan port kosong berikutnya (8081, 8082, dst.) dan menginformasikan tautan aktif di terminal.

#### Q: Browser meminta Master PIN saat saya membuka SqlFox dari tablet/iPad. Di mana saya melihatnya?
> **Jawaban**: Periksa jendela terminal komputer utama tempat Anda meluncurkan `sqlfox`. PIN 6 digit ditampilkan saat aplikasi mulai berjalan. Selain itu, pada sesi komputer utama, Anda dapat mengklik badge **🔑 PIN** di bilah navigasi atas untuk melihat dan menyalin PIN tersebut.

#### Q: Apa yang terjadi jika koneksi WiFi atau VPN terputus saat query sedang berjalan?
> **Jawaban**: SqlFox dilindungi fitur **Global Network Error Boundary**. Aplikasi akan tetap mempertahankan draf query editor Anda dan editan tabel yang belum disimpan di memori lokal peramban, sembari mencoba memulihkan koneksi secara otomatis.

#### Q: Bagaimana cara memperbarui SqlFox ke versi rilis terbaru?
> **Jawaban**: SqlFox dilengkapi fitur **In-App Self-Update**. Setiap kali ada rilis versi baru di GitHub, tombol **🚀 Update** akan muncul di status bar bawah. Klik tombol tersebut untuk membaca catatan rilis changelog dan meng-upgrade binary secara otomatis dengan satu klik tanpa perlu download manual.

---

<div align="center">

**SqlFox** • Dibuat dengan dedikasi tinggi bagi para developer dan DBA yang mengutamakan keamanan dan performa.  
Punya masukan atau butuh bantuan lebih lanjut? Kunjungi [Repositori GitHub SqlFox](https://github.com/herliansyah/sqlfox).

</div>
