# Playbook: Large-Scale Database Management (500GB+ Tables & High-Concurrency OLTP)

Panduan operasional dan arsitektur bagi DBA dan Software Engineer saat menangani database skala besar (~500GB) pada **MySQL 8.x, MySQL 5.7, MariaDB, dan SQLite**, serta prinsip skalabilitas untuk database masa depan (PostgreSQL, Oracle).

---

## 1. Anatomi Masalah: Mengapa Database Jumbo Terkunci?

Pada database berukuran ratusan gigabyte dengan beban transaksi tinggi, kegagalan operasional paling fatal hampir selalu bersumber dari dua hal: **Metadata Lock (MDL) Contention** dan **Unbounded Long Transactions**.

```
[Transaksi Lambat / SELECT Besar]
         │  (Memegang shared read lock / MDL)
         ▼
[Tabel: 500GB atau Master Kecil]
         ▲
         │  (Menunggu exclusive MDL lock)
[ALTER TABLE / DDL Baru]
         ▲
         │  (SEMUA transaksi & query baru antre di belakang DDL!)
[Antrean Query Web / Aplikasi] ──► 💥 CONNECTION POOL EXHAUSTION & DOWNTIME!
```

### Jebakan "Tabel Master Kecil vs. Tabel Transaksi Jumbo"
> ⚠️ **Peringatan Kritis**: Jangan pernah meremehkan perubahan skema pada tabel master yang hanya berisi 10 baris!
>
> Jika tabel `order_status` (hanya 5 baris) memiliki relasi Foreign Key dari tabel `orders` (~500GB, 200 juta baris):
> 1. Setiap transaksi yang menulis atau membaca `orders` dengan foreign key check akan meminta shared lock ke `order_status`.
> 2. Ketika Anda menjalankan `ALTER TABLE order_status ...`, operasi ini meminta **Exclusive Metadata Lock** pada `order_status`.
> 3. DDL akan tertahan menunggu transaksi `orders` selesai.
> 4. Akibatnya, seluruh transaksi baru yang menyentuh `orders` akan terblokir! Dalam 5 detik, aplikasi produksi bisa lumpuh total.

---

## 2. Aturan Emas DDL di Lingkungan Produksi

### Aturan 1: Wajib Fail-Fast Session Lock Timeout
Secara default, parameter `lock_wait_timeout` pada MySQL adalah **31.536.000 detik (1 tahun)**. Jika DDL tidak dapat memperoleh lock segera, ia akan terus menunggu dan menyumbat seluruh traffic database.

* **Solusi**: Selalu set lock timeout sesi menjadi **2 s/d 5 detik** sebelum mengeksekusi DDL:
  ```sql
  -- MySQL / MariaDB
  SET SESSION lock_wait_timeout = 3;
  ALTER TABLE my_table ADD COLUMN notes VARCHAR(255);
  ```
* **Hasil**: Jika ada transaksi yang sedang membaca/menulis tabel, DDL akan langsung gagal seketika (*fail-fast* dengan error `Lock wait timeout exceeded`), **tanpa menumpuk antrean query lain**.
* **Eksekusi di SqlFox**: SqlFox secara otomatis mengisolasi eksekusi DDL pada *dedicated single connection* (`*sql.Conn`), menginjeksi timeout fail-fast, dan mereset koneksi agar pool aplikasi tetap higienis.

### Aturan 2: Inspeksi Transaksi Aktif Sebelum Eksekusi (Pre-Flight Check)
Sebelum menekan tombol jalankan DDL:
1. Periksa apakah ada transaksi berjalan lama (*uncommitted transaction*) yang menyentuh tabel target:
   ```sql
   -- Cek transaksi InnoDB aktif di MySQL
   SELECT 
       trx_mysql_thread_id, 
       trx_state, 
       trx_started, 
       TIMESTAMPDIFF(SECOND, trx_started, NOW()) AS duration_sec,
       trx_query 
   FROM information_schema.innodb_trx;
   ```
2. Jika ditemukan transaksi yang berjalan > 10 detik, cari thread ID-nya dan hentikan jika diperlukan (atau jadwalkan DDL saat transaksi mereda).

### Aturan 3: Pahami Kemampuan Online DDL Native vs. Tool OSC
Tidak semua `ALTER TABLE` bisa berjalan tanpa mengunci tabel.

| Operasi | MySQL 8.0+ | MySQL 5.7 / MariaDB | Rekomendasi 500GB |
| :--- | :--- | :--- | :--- |
| **Tambah kolom di akhir** | `INSTANT` (0 detik lock) | `INPLACE` (Concurrent DML) | Aman via native ALTER |
| **Tambah kolom di tengah** | `INSTANT` (>= 8.0.29) | `COPY` (Kunci tabel penuh!) | Gunakan `gh-ost` di 5.7 |
| **Ubah tipe kolom (misal INT ke BIGINT)** | `COPY` (Rebuild penuh) | `COPY` (Rebuild penuh) | **Wajib `gh-ost`** |
| **Tambah Indeks Sekunder** | `INPLACE` (Concurrent DML) | `INPLACE` | Aman, jalankan saat low traffic |
| **Drop Kolom / Rename** | `INSTANT` (>= 8.0.29) | `COPY` / `INPLACE` | Cek dependensi view/FK |

---

## 3. Playbook Migrasi Tabel Jumbo dengan `gh-ost`

Untuk perubahan skema yang membutuhkan *full table rewrite* pada tabel ratusan gigabyte, gunakan **`gh-ost`** (GitHub's Online Schema Change). `gh-ost` tidak menggunakan trigger database sehingga tidak menambah beban I/O pada transaksi OLTP.

### Arsitektur Alur Kerja `gh-ost`:
1. `gh-ost` membuat ghost table (`_my_table_gho`).
2. Menerapkan `ALTER` pada ghost table yang masih kosong.
3. Membaca perubahan data secara real-time via binlog stream.
4. Meng-copy baris data lama secara bertahap (*chunked batches*).
5. Melakukan cut-over atomic rename dalam hitungan milidetik.

### Sintaks Eksekusi Standar `gh-ost`:
```bash
gh-ost \
  --host="192.168.1.100" \
  --port=3306 \
  --user="dba_user" \
  --ask-pass \
  --database="production_db" \
  --table="orders" \
  --alter="ADD COLUMN tracking_code VARCHAR(64) DEFAULT NULL" \
  --chunk-size=2000 \
  --max-load="Threads_running=30" \
  --critical-load="Threads_running=60" \
  --throttle-control-replicas="192.168.1.101:3306" \
  --max-lag-millis=1500 \
  --allow-on-master \
  --execute
```

* **Parameter Penjaga Beban**:
  * `--max-load="Threads_running=30"`: Jika thread server melebihi 30, migrasi otomatis istirahat (*throttle*).
  * `--critical-load="Threads_running=60"`: Mencegah CPU spike.
  * `--max-lag-millis=1500`: Menunda copy jika replikasi slave mulai mengalami replication lag.

---

## 4. Playbook Menjalankan Query Berat & Analitik

Pada database 500GB, query `SELECT` yang ceroboh bisa membebani buffer pool InnoDB dan menaikkan thread wait.

### 1. Pisahkan Connection Write vs. Read Replica
* Jangan pernah menjalankan query laporan, agregasi bulanan, atau data dump di database Master/Primary.
* Di SqlFox `Connection Vault`, buat dua profil terpisah:
  * `Primary (Production - Write)`: Dibatasi hanya untuk mutasi operasional.
  * `Replica (Production - Read-Only)`: Diberikan untuk query analisis dan investigasi data.

### 2. Gunakan Keyset Pagination (Bukan OFFSET Besar)
* ❌ **Buruk**: `SELECT * FROM orders ORDER BY id LIMIT 1000000, 50;`
  *(Database tetap membaca 1.000.050 baris ke memori lalu membuang 1.000.000 baris pertama)*.
*  **Benar (Keyset Pagination)**:
  ```sql
  SELECT * FROM orders 
  WHERE id > 1000000 
  ORDER BY id ASC 
  LIMIT 50;
  ```
  *(Database langsung melompat ke node indeks B-Tree yang tepat dalam < 1 milidetik)*.

### 3. Wajibkan `EXPLAIN` (Impact Preview)
Sebelum menjalankan query kompleks pada tabel jutaan baris:
* Periksa apakah `type` bernilai `ALL` (Full Table Scan).
* Jika `rows` yang dipindai > 100.000 dan tidak menggunakan indeks, tambahkan indeks komposit yang sesuai atau tambahkan filter rentang waktu bertingkat (`created_at BETWEEN ...`).

### 4. Kurangi Lock Sesi Saat Membaca Data
Jika terpaksa harus mengekstrak data dari server utama tanpa menghalangi transaksi lain:
```sql
-- Membaca snapshot data tanpa meminta shared lock
SET TRANSACTION ISOLATION LEVEL READ COMMITTED;
-- atau jika hanya butuh aproksimasi cepat di MySQL:
SET SESSION TRANSACTION ISOLATION LEVEL READ UNCOMMITTED;
```

---

## 5. Ringkasan Checklist Sebelum Melakukan Perubahan

- [ ] **1. Identifikasi Ukuran Tabel & Relasi**: Apakah tabel ini berukuran besar (> 10GB) atau merupakan tabel master yang menjadi foreign key tabel jumbo?
- [ ] **2. Cek Transaksi Berjalan**: Pastikan tidak ada transaksi menggantung di `information_schema.innodb_trx`.
- [ ] **3. Pasang Fail-Fast Lock Timeout**: Pastikan `SET lock_wait_timeout = 3` aktif.
- [ ] **4. Tentukan Tool DDL**:
  - Jika native `INSTANT` didukung: jalankan via SqlFox DDL Designer.
  - Jika memerlukan table copy: gunakan generator perintah `gh-ost`.
- [ ] **5. Pisahkan Jalur Baca**: Pastikan query audit/analitik diarahkan ke Read Replica.

---

## 6. Fitur Pengamanan & Resiliensi SqlFox pada Skala Ekstrem

SqlFox secara internal telah dilengkapi mekanisme pertahanan bawaan untuk menjaga stabilitas saat menangani database berskala masif (1.000+ tabel, jutaan baris, dan dataset ratusan gigabyte):

1. **Fail-Fast Out-of-Band Query Cancellation (3s Timeout)**:
   - Tombol pembatalan query (`Cancel`) menggunakan koneksi kontrol terpisah dengan batas waktu 3 detik.
   - Jika koneksi database target sedang jenuh total (*connection pool saturation*), pembacaan stream memori lokal di SqlFox segera dihentikan seketika untuk mencegah kebocoran RAM klien, dan antarmuka menampilkan pesan diagnostik yang jelas.
2. **Pagu Memori Bounded Chunk Streaming (<100MB RAM RSS)**:
   - Data dibaca dan di-stream secara bertahap (100–200 baris per frame WebSocket/SSE).
   - Telah dibenchmark mampu membaca hingga 1.000.000 baris data tanpa memicu lonjakan memori melebihi plafon 100MB RAM.
3. **Virtualisasi Sidebar Pohon Skema (1.000+ Tabel)**:
   - Menggunakan optimasi CSS native `content-visibility: auto; contain-intrinsic-size: 0 28px;` sehingga browser melewati fase perhitungan layout untuk tabel yang berada di luar layar.
   - Dilengkapi filter pencarian dengan *debounce* 150ms sehingga pencarian di antara ribuan tabel tidak menyebabkan freeze pada UI.
4. **Proteksi Diagnostik Profiling (`SQLFOX_PPROF=1`)**:
   - Jika diperlukan analisis alokasi memori saat menangani beban berat, profil `net/http/pprof` dapat diaktifkan melalui environment variable `SQLFOX_PPROF=1`.
   - Endpoint ini otomatis terkunci dan menolak akses dari IP non-localhost (`403 Forbidden`) demi menjaga keamanan credential vault di jaringan LAN.
5. **Global Network Error Boundary & Retensi State**:
   - Jika server mengalami restart atau glitch jaringan sesaat, bilah notifikasi melayang otomatis mengaktifkan rekoneksi berjenjang (*exponential backoff* 1s, 2s, 4s, 8s).
   - Seluruh tab query editor, skrip SQL yang belum disimpan, dan draf edit sel di grid tidak akan terhapus.

