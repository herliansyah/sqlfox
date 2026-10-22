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

### 1.3 Manajemen & Editor Visual Objek Database (Views, Triggers, Stored Procedures, Functions)
* **Status Saat Ini**: Backlog Terbuka. SqlFox memiliki Table Designer yang baik untuk tabel relasional, namun untuk View, Trigger, Function, dan Stored Procedure, dukungan baru sebatas inspeksi DDL read-only.
* **Kebutuhan Lapangan**: DBA membutuhkan kemampuan untuk membuat, mengubah (*ALTER/REPLACE*), dan mengompilasi ulang View dan Trigger tanpa harus menulis DDL dari nol di Query Editor.
* **Rekomendasi Implementasi**:
  - Modal editor DDL terstruktur khusus untuk View dan Stored Routines dengan validasi sintaks pra-eksekusi.
  - Dukungan pemanggilan rutin berparameter (*Execute Stored Procedure with arguments*).

### 1.4 Format Ekspor Data Tambahan (Excel `.xlsx` & Streaming NDJSON)
* **Status Saat Ini**: Backlog Terbuka. Ekspor data mendukung SQL dump, CSV, JSON, dan format copy Markdown ke clipboard.
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
  - `web/dist/index.html` (24.544 baris, 1.24 MB)
* **Analisis**:
  - Demi menjaga distribusi *single-binary* tanpa node_modules saat runtime, seluruh kode antarmuka (DOM modal, canvas ERD visualizer, chart drawing, CodeMirror setup, table designer, sync engine) disatukan ke dalam satu file HTML raksasa.
  - Hal ini membuat navigasi kode frontend sulit, memperlambat code review, dan berpotensi menghambat rendering DOM pada mesin dengan spesifikasi rendah.
* **Rencana Perampingan**:
  - Pisahkan menjadi modul-modul JS logis di direktori sumber (`web/src/js/`), lalu gunakan bundler ringan (seperti esbuild) saat build time untuk menghasilkan `web/dist/index.html` teroptimasi secara deterministik.

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
