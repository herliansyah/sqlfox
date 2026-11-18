# SqlFox Future Roadmap & Architectural Backlog

Dokumen ini mendokumentasikan analisis backlog fitur masa depan (*missing features*) dan evaluasi perampingan kode (*bloat analysis*) berdasarkan audit komprehensif codebase SqlFox. Dokumen ini dirancang sebagai panduan pengembangan jangka menengah/panjang dengan tetap menjunjung prinsip **Lazy Senior Developer (Ponytail Mode)**: *"The best code is the code never written; boring over clever; minimum code that works"*.

---

## 1. Backlog Fitur yang Perlu Ditambahkan (Missing Features)

Fitur-fitur esensial yang diharapkan oleh developer dan DBA dari sebuah Database Manager modern, namun belum diimplementasikan:

### 1.1 In-Process SSH Tunneling & Bastion Host Support
* **Status Saat Ini**: Di ADR 0020, kode in-process SSH dialer dihilangkan demi menjaga kesederhanaan binary dan merekomendasikan port-forwarding eksternal (`ssh -N -L`).
* **Kebutuhan Lapangan**: Banyak developer bekerja dengan database cloud (AWS RDS, GCP Cloud SQL, DigitalOcean Managed Database) yang berada di dalam private VPC tanpa IP publik, di mana operator tidak memiliki akses terminal langsung atau menggunakan Windows tanpa tooling SSH bawaan yang mudah.
* **Rekomendasi Implementasi**:
  - Pertimbangkan penyediaan opsi koneksi SSH Bastion opsional pada form koneksi (`use_ssh: bool`, `ssh_host`, `ssh_port`, `ssh_user`, `ssh_auth_type`).
  - Implementasikan dialer SSH standar menggunakan library Go murni (`golang.org/x/crypto/ssh`) secara lazy (hanya di-dial jika `use_ssh == true`).

### 1.2 Kontrol Transaksi Visual di Toolbar UI (Commit & Rollback Controls)
* **Status Saat Ini**: **Terimplementasi (v0.10.7)**
* **Detail Implementasi**:
  - Backend menyematkan status `tx_active` pada setiap `ExecuteResponse`.
  - Frontend menampilkan pill status transaksi berkedip (`[⚡ TX Active]`) di toolbar editor query saat transaksi terbuka.
  - Tersedia tombol cepat visual `Commit` dan `Rollback` serta shortcut keyboard global `Ctrl+Shift+C` dan `Ctrl+Shift+R`.
  - Dilengkapi dialog konfirmasi pengaman pada `closeTab` jika tab ditutup saat transaksi masih uncommitted.

### 1.3 Manajemen & Editor Visual Objek Database (Views, Triggers, Stored Procedures, Functions, Scheduled Events)
* **Status Saat Ini**: **Terimplementasi (v0.11.0)**
* **Detail Implementasi**:
  - Dialog desainer visual interaktif untuk Views, Triggers, Stored Routines (Procedures & Functions), dan Scheduled Events dengan editor CodeMirror bawaan.
  - Builder grid parameter dinamis untuk input/output rutin terprogram.
  - Pemilih interval penjadwalan visual untuk events (`EVERY`, `AT`, `STARTS`, `ENDS`).
  - Handoff 1-klik "Open in Query Tab" dan sanitasi definer SQL dengan preservasi komentar multibaris.

### 1.4 Format Ekspor Data Tambahan (Excel `.xlsx` & Streaming NDJSON)
* **Status Saat Ini**: 
  - **Streaming NDJSON**: **Terimplementasi (v0.10.12, ADR 0058)** via endpoint `/api/session/{id}/execute-stream` dengan immediate 20-row micro-chunk flush.
  - **Format Excel `.xlsx`**: Backlog Terbuka. Ekspor data saat ini mendukung SQL dump, CSV, JSON, NDJSON streaming, dan format copy Markdown/TSV ke clipboard.
* **Kebutuhan Lapangan**: Pengguna bisnis, manajer produk, dan analis data kerap meminta hasil query dalam format spreadsheet Excel (`.xlsx`) asli untuk pengolahan formula dan laporan.
* **Rekomendasi Implementasi**:
  - Tambahkan parser generator Excel streaming tanpa CGO (menggunakan library Go murni seperti `excelize`).
  - Opsi format export pada data grid: CSV, Excel (.xlsx), Markdown, dan NDJSON.

### 1.5 Persistensi Tab Query dan Draft SQL Lintas Reload Browser
* **Status Saat Ini**: **Terimplementasi**
* **Detail Implementasi**:
  - State metadata tab dan draft query disimpan secara otomatis ter-debounce (250ms) ke browser `localStorage` dengan scoping per-koneksi (`sqlfox_tabs_<conn_id>`).
  - Memulihkan tab aktif dan teks draft query saat halaman browser di-refresh.

### 1.6 Penerjemahan Skema / DDL Lintas-Engine pada Data Transfer
* **Status Saat Ini**: **Terimplementasi (v0.10.7)**
* **Detail Implementasi**:
  - Mengimplementasikan `normalizeColumnType(col.Type, targetDriver)` dan `buildCreateTableDDL(tbl, targetDriver)` pada `internal/server/handlers_sync.go`.
  - Menghilangkan *clause* spesifik MySQL seperti `AUTO_INCREMENT` dan storage engine options saat mentransfer data ke target SQLite atau PostgreSQL, serta menormalisasi tipe data standar (`int(11)` -> `INTEGER`, `mediumtext` -> `TEXT`, `datetime` -> `TIMESTAMP`, dll).

### 1.7 Kebijakan Retensi Otomatis & Pembersihan Log Audit (`audit_logs`)
* **Status Saat Ini**: **Terimplementasi (v0.10.7)**
* **Detail Implementasi**:
  - Menambahkan metode `PurgeAuditLogs(retentionDays, maxEntries)` pada `internal/vault/audit.go` serta indeks performa `idx_audit_logs_executed`.
  - Dijalankan secara otomatis saat inisialisasi vault database (`OpenVault`) dengan kebijakan retensi 30 hari dan pembatasan batas atas (*ceiling limit*) maksimal 50.000 log terbaru untuk mencegah kebocoran ruang disk (*disk leak*).

### 1.8 Mutasi In-Place Query Grid & Submenu Kontekstual Terkelompok
* **Status Saat Ini**: **Terimplementasi (v0.14.0 - v0.14.1)**
* **Detail Implementasi**:
  - Memungkinkan pengeditan sel langsung, penambahan draf baris, dan penghapusan baris pada hasil query SELECT arbitrer yang merujuk pada tabel tunggal dengan primary key lengkap.
  - Pengelompokan aksi menu konteks menjadi hierarki submenu (`Quick Filter`, `Insert Value`, `Mutations`, `Copy / Export`) dengan pembatasan batas viewport agar tidak terpotong tepi layar.

### 1.9 Mode Aplikasi Desktop Mandiri & Integrasi Launcher OS
* **Status Saat Ini**: **Terimplementasi (v0.15.0)**
* **Detail Implementasi**:
  - Dukungan mode jendela desktop frameless menggunakan argumen `--app` Chromium/Edge di Linux, macOS, dan Windows tanpa runtime Electron.
  - Perintah CLI `sqlfox desktop install/uninstall/status` untuk mendaftarkan launcher Freedesktop dan ikon 512x512 ke sistem.
  - Perintah `sqlfox stop` dan endpoint `/api/system/shutdown` untuk manajemen proses latar belakang.

### 1.10 Verifikasi Tanda Tangan Rantai Pasok Ed25519 & Paritas Safety Guard Script Runner
* **Status Saat Ini**: **Terimplementasi (v0.16.0, ADR 0060)**
* **Detail Implementasi**:
  - Verifikasi berkas manifest checksum rilis via tanda tangan terpisah Ed25519 (`checksums.txt.sig`) menggunakan public key tertanam (fail-closed) sebelum binary diperbarui.
  - Script runner SQL berukuran besar mewarisi AST Safety Guard dan Smart Tiered Backup pre-DDL, memblokir DDL/DML destruktif tanpa konfirmasi eksplisit (`confirm=true`).
  - Pengikatan enkripsi AES-256-GCM ke identitas connection dan field (AAD) di Connection Vault untuk mencegah relokasi ciphertext.

### 1.11 Penggantian Proses In-Place (`syscall.Exec`) & Pencegahan Konflik Port Saat Self-Update
* **Status Saat Ini**: **Terimplementasi (v0.16.1)**
* **Detail Implementasi**:
  - Pada sistem POSIX, restart self-update mengeksekusi `syscall.Exec` untuk menimpa citra proses langsung di memori, mempertahankan PID asli serta meniadakan perebutan file lock dan tabrakan port (`EADDRINUSE`).
  - Pada Windows, listener socket dan lock file dilepas secara bersih sebelum meluncurkan proses anak.
  - Menyuntikkan flag `--no-browser` otomatis saat restart dan memantau status kesiapan server via polling `/api/auth/status` sebelum memuat ulang antarmuka web.

### 1.12 Enkripsi Transport Ingress TLS/HTTPS & Sertifikat Otomatis Ephemeral
* **Status Saat Ini**: **Terimplementasi (v0.17.2, ADR 0061)**
* **Detail Implementasi**:
  - Dukungan listener TLS murni Go (`--tls`, `--tls-cert`, `--tls-key`, `SQLFOX_TLS=1`) tanpa dependensi CGO atau reverse proxy eksternal.
  - Pembangkitan sertifikat ECDSA P-256 self-signed otomatis langsung di memori dengan Subject Alternative Names (SANs) untuk `localhost`, `127.0.0.1`, `::1`, dan alamat IP LAN lokal terdeteksi.
  - Terminal banner menampilkan indikator URL HTTPS yang aman saat startup.

### 1.13 Arsitektur Frontend Modular & Peningkatan Engine Editor CodeMirror 6
* **Status Saat Ini**: **Terimplementasi (v0.17.2, ADR 0061, ADR 0062)**
* **Detail Implementasi**:
  - Port penuh UI ke Svelte 5 Runes + TypeScript di `web/frontend/` (menggantikan 15 modul JavaScript domain legacy di `web/src/js/`).
  - Pipeline build otomatis (`scripts/build_web.js` → `npm run build:svelte`) yang menjalankan Vite + Tailwind dan menghasilkan bundle ke `web/dist/index.html`.
  - Migrasi dari CodeMirror 5 ke CodeMirror 6 modern dengan `@codemirror/lang-sql`, *compartment* dialek dinamis (MySQL, PostgreSQL, SQLite), palet tema gelap kontras tinggi, dan integrasi autocompletion skema katalog.

---

## 2. Analisis Fitur Bloated & Potensi Perampingan (Over-Engineering Review)

Fitur-fitur yang sudah ada di codebase namun menambah kompleksitas berlebih, memperbesar ukuran file, atau keluar dari filosofi aplikasi inti:

### 2.1 GitHub Cloud Sync untuk Operational Notes
* **File Terkait**:
  - `internal/server/handlers_cloud_sync.go` (359 baris)
  - `internal/vault/cloud_sync.go` (305 baris)
* **Analisis**:
  - SqlFox adalah tool manajemen database. Fitur sinkronisasi catatan operasional ke remote repository GitHub via personal access token dan debounce timer (30 detik) mengimplementasikan manipulasi Git tree tingkat rendah di dalam binary.
  - Fitur ini menambah *attack surface* (penyimpanan GitHub PAT token), overhead timer background, serta potensi kegagalan rate limit API GitHub.
* **Rencana Perampingan**:
  - Pertahankan opsi ekspor/impor arsip lokal (`.sqlfox-vault` / ZIP) yang sudah ada via `internal/vault/bundle.go`.
  - Pindahkan fungsionalitas auto-sync GitHub ke integrasi opsional atau plugin terpisah.

### 2.2 Telemetri Kluster Percona XtraDB (PXC) / Galera di Driver MySQL
* **File Terkait**:
  - Bagian dari `internal/driver/mysql/mysql.go` (~600+ baris terkait EVS latency, flow control pauses, peer node scanning).
* **Analisis**:
  - `mysql.go` membengkak hingga lebih dari 2.600 baris kode karena memuat logika diagnostik mendalam untuk kluster multi-master spesifik (Percona XtraDB Cluster dan Galera).
  - Mayoritas pengguna menjalankan MySQL standar (5.7, 8.0, 8.4) atau MariaDB tunggal/replikasi standar.
* **Rencana Perampingan**:
  - Ekstrak diagnostik PXC / Galera ke dalam submodule terpisah (`internal/driver/mysql/pxc.go` atau `diagnostics/pxc`) agar file inti driver MySQL tetap ramping, mudah diuji, dan fokus pada eksekusi query umum.

### 2.3 Performance Flight Recorder Berbasis Polling dari Desktop Client
* **File Terkait**:
  - `internal/server/flight_recorder.go` (215 baris)
  - `internal/vault/flight_recorder.go` (280 baris)
* **Analisis**:
  - Flight recorder melakukan polling berkala (setiap 1–5 detik) terhadap `SHOW GLOBAL STATUS` dan processlist, lalu menyimpan setiap tick ke basis data lokal SQLite.
  - Pengawasan metrik semacam ini pada arsitektur produksi seharusnya menjadi tanggung jawab APM dedicated server-side (Prometheus, Grafana, Datadog, Percona PMM). Menjalankan APM poller berkala dari laptop atau tablet developer dapat membebani database produksi yang sedang sibuk.
* **Rencana Perampingan**:
  - Batasi flight recorder hanya untuk pengujian lokal/staging atau pastikan durasi otomatis berhenti lebih ketat (default 2 menit) untuk mencegah polling background yang tertinggal.

### 2.4 Monolit Frontend Single-File SPA (24.500+ Baris HTML)
* **File Terkait**:
  - `web/src/` (15 modul domain JS), `scripts/build_web.js`, `web/dist/index.html`
* **Status Saat Ini**: **Terimplementasi (ADR 0061, ADR 0062)**
* **Detail Implementasi**:
  - Kode antarmuka telah dimigrasikan sepenuhnya ke aplikasi Svelte 5 di `web/frontend/` (runes reactivity, TypeScript, Tailwind), menggantikan monolit HTML/JS legacy.
  - Build pipeline otomatis (`scripts/build_web.js` → `npm run build:svelte`) menjalankan Vite + Tailwind, menghilangkan layout shift (CLS) dan merampingkan pemeliharaan kode.
  - Editor dimodernisasi ke CodeMirror 6 dengan bundling offline via esbuild (`scripts/build_codemirror.js`).

### 2.5 Operational Notes dengan Web Audio API Alarm & Snooze
* **File Terkait**:
  - `internal/vault/operational_notes.go`
  - `internal/server/handlers_notes.go`
  - `web/dist/index.html` (fungsi `playReminderChime`, interval polling `/api/notes/reminders/due` tiap 30 detik)
* **Analisis**:
  - SqlFox menyertakan sistem alarm reminder berbasis Web Audio API (chime synthesizer oscillator), snooze interval, dan background polling berkala.
  - Untuk tool database manager yang berorientasi performa tinggi dan ringan, sistem task scheduler/reminder alarm di dalam browser terasa over-engineering dan menambah polling noise.
* **Rencana Perampingan**:
  - Hapus background audio chime dan polling scheduler 30 detik, pertahankan fungsionalitas catatan query / runbook sebagai snippet markdown biasa yang terikat ke tabel atau query tab.
