# SqlFox Svelte 5 Complete Migration Checklist & Implementation Roadmap

> **Status Dokumen**: Living Document  
> **Target**: 100% Paritas Fitur Legacy ke Svelte 5 Runes (`web/frontend/`) tanpa regresi, zero-bloat, dan ultra-responsif.
> **Status: SELESAI** — paritas fitur 149/149 tercapai dan **sumber legacy telah dipensiunkan**: `web/src/js/` (15 modul), `web/src/index.html`, dan `web/dist/index.legacy.html` dihapus dari repo. Bundle aktif adalah hasil build Svelte (`npm run build:svelte`) yang ter-embed via `go:embed dist/*`. Infrastruktur (CI, release workflow, Dockerfile, `scripts/release.sh`, test suite `web/embed_test.go` + `scripts/test-*.mjs`) kini sepenuhnya menarget Svelte.

---

## 📌 Rekapitulasi Status Paritas

| Kategori Modul | Total Fitur | Svelte 5 Implemented | Pending | Progres Paritas |
| :--- | :---: | :---: | :---: | :---: |
| 1. Core State & Remote Guard | 8 | 8 | 0 | 100.0% |
| 2. Connections & Session Management | 11 | 11 | 0 | 100.0% |
| 3. Schema Explorer & Catalog | 16 | 16 | 0 | 100.0% |
| 4. Query Worksheet & CodeMirror 6 | 12 | 12 | 0 | 100.0% |
| 5. Deterministic Safety Guard | 7 | 7 | 0 | 100.0% |
| 6. Virtual Data Grid & CRUD Suite | 19 | 19 | 0 | 100.0% |
| 7. Dedicated Table Viewer & Designers | 13 | 13 | 0 | 100.0% |
| 8. Programmability Object Designers | 6 | 6 | 0 | 100.0% |
| 9. DBA Telemetry & Monitoring Suite | 16 | 16 | 0 | 100.0% |
| 10. Database Tools, Sync & Migration | 6 | 6 | 0 | 100.0% |
| 11. AI Copilot & Assistant | 8 | 8 | 0 | 100.0% |
| 12. Productivity, Snippets, CSV & Matrix | 10 | 10 | 0 | 100.0% |
| 13. Visual ERD Interactive Canvas | 8 | 8 | 0 | 100.0% |
| 14. Operational Notes Collaboration | 5 | 5 | 0 | 100.0% |
| 15. System Lifecycle & Activity Console | 4 | 4 | 0 | 100.0% |
| **TOTAL** | **149** | **149** | **0** | **100.0%** |

---

## 📋 Ceklis Lengkap Fitur Legacy & Status Implementasi

### 1. Core State, Security & Remote Guard (`01_core_state.js`)
- [x] State Management Reaktif Svelte 5 Runes (`appState.svelte.ts` dengan `$state`, `$derived`, `$effect`)
- [x] Remote Access Guard / PIN Lock Screen (`RemoteLockModal.svelte`) - autentikasi token PIN
- [x] Header Master PIN Pill (`Header.svelte`) - reveal/mask dan 1-click copy PIN
- [x] Environment Pill & Pulse Warning (`Header.svelte` - Production, Staging, Development)
- [x] Global Toast Notification System (`ToastContainer.svelte`)
- [x] HTML5 Native Dialog System (`showNativeAlert`, `showNativeConfirm`, `showNativePrompt` via `NativeDialogModal.svelte`)
- [x] Global Network Error Boundary & Health Watcher (`ReconnectBanner.svelte` & ping loop berkala ke daemon)
- [x] Pre-Execution DDL Review Modal (`DdlReviewModal.svelte`)

### 2. Connection Management & Session Vault (`02_connections.js`, `13_app_settings_modals.js`)
- [x] Penyimpanan & Pemuatan Profil Koneksi (LocalStorage + API client)
- [x] Modal Tambah Koneksi Baru (`ConnectionModal.svelte`)
- [x] Modal Edit Koneksi (`ConnectionModal.svelte`)
- [x] Hapus Profil Koneksi dengan konfirmasi
- [x] Test Connection action (`handleTest` dengan latensi round-trip)
- [x] Multi-driver preset: MySQL (3306), PostgreSQL (5432), SQLite (`./data.db`)
- [x] Parameter SSH Tunneling (SSH Host, Port, User)
- [x] Mode SSL/TLS Encryption (`disabled`, `require`, `verify-ca`, `verify-full`)
- [x] Opsi Read-Only Connection (isolasi hanya query SELECT)
- [x] Filter pencarian cepat di dropdown koneksi header
- [x] Pengelompokan koneksi berbasis Tag / Environment Folder

### 3. Schema Explorer & Database Catalog (`03_schema.js`)
- [x] Dropdown Pemilih Database Aktif di Header & Sidebar
- [x] Filter pencarian live database di dropdown
- [x] Dual View Mode Sidebar (Tampilan Objek vs Daftar Semua Database)
- [x] Section Collapsible Skema (Tables, Views, Routines, Triggers)
- [x] Filter pencarian objek skema (live filter tabel, view, routine)
- [x] Format ringkas estimasi jumlah baris tabel (`1.2k`, `1.5M`)
- [x] Fast background catalog caching (`loadFastCatalogBackground`)
- [x] Table Context Menu dasar:
  - [x] Open Data Grid
  - [x] View Structure
  - [x] View CREATE TABLE DDL
  - [x] Copy Table Name
- [x] Table Context Menu lanjutan:
  - [x] Rename Table (`RenameTableModal.svelte`)
  - [x] Truncate Table (dengan konfirmasi Safety Guard)
  - [x] Drop Table (dengan konfirmasi Safety Guard)
  - [x] Duplicate Table (struktur atau data via `DuplicateTableModal.svelte`)
  - [x] Export Table SQL Dump (`exportTableDump`)
  - [x] Generate Table Mock Data / Smart Seed
- [x] Schema Tree Prefix Grouping (ADR 0058: auto-group prefix `auth_*`, `shop_*`)
- [x] Dialog Buat Database Baru (`CreateDatabaseModal.svelte`: nama, charset, collation)
- [x] Aksi Drop Database dengan dialog afirmasi keamanan
- [x] Katalog Karakter Set & Collation MySQL/MariaDB (`utf8mb4_unicode_ci`, dll.)
- [x] Refresh All Catalogs dengan Active Threads Safety Guard (`RefreshAllModal.svelte`)

### 4. Query Worksheet & CodeMirror 6 Engine (`04_editor_tabs.js`, `09_query_execution.js`)
- [x] CodeMirror 6 Action Modern (`actions/codemirror.ts`) — shared by worksheet + designer editors
- [x] Multi-Dialect Syntax Highlighting (MySQL, PostgreSQL, SQLite) via dynamic dialect compartment
- [x] Eksekusi Query via tombol & shortcut `Ctrl+Enter` / `Cmd+Enter` (selection → statement at cursor)
- [x] Run All (whole worksheet) via `Ctrl+Alt+Enter` / `F5` dan tombol toolbar
- [x] In-Flight Query Cancellation button (`cancelCurrentTab`)
- [x] Draggable Workspace Pane Splitter (persistensi + double-click 50:50 reset)
- [x] Multi-Tab Workspace Manager (`TabBar.svelte`): New Tab (`Ctrl+T`), Close (`Ctrl+W`)
- [x] Tab Context Menu: Duplicate Tab, Close Other Tabs, Close Tab
- [x] Explain Plan Drawer (`explainCurrentTab` & visual output, `Ctrl+Shift+Enter`)
- [x] Schema-Aware SQL Autocomplete / IntelliSense (tabel, kolom, alias, `db.tabel`, `db.tabel.kolom`)
- [x] Pure Client-Side SQL Formatter (`formatSQL`, tombol toolbar & `Ctrl+Shift+F`)
- [x] Tab Bar Overflow Dropdown (menu penampung tab berlebih)
- [x] Tab-Specific Database Scope Pill (ADR 0055: setiap tab bisa target DB beda, ikut ke eksekusi & suggest)
- [x] Multi-Statement Execution Splitter & Multi-Result Tabs (Result 1, Result 2, dst.)
- [x] Execute Statement at Cursor (`Ctrl+Shift+Enter` → EXPLAIN statement terpilih)
- [x] Snippet autocomplete (`snip:` prefix) + parameter substitution `{{nama:default}}`
- [x] Per-tab undo/selection/scroll state saat berpindah tab

### 5. Deterministic Safety Guard & Transaction Control (`09_query_execution.js`)
- [x] Deterministic Safety Guard™ Modal (`SafetyModal.svelte`)
- [x] Target Database Affirmation Typing (ketik ulang nama tabel/DB)
- [x] Risk Level Badge (`CRITICAL`, `HIGH`, `MEDIUM`) & Blast Radius Metrics
- [x] Visual Transaction Management Controls:
  - [x] Tombol `START TRANSACTION` / `BEGIN` (`handleStartTransaction`)
  - [x] Tombol `COMMIT` (`handleCommit`)
  - [x] Tombol `ROLLBACK` (`handleRollback`)
  - [x] Badge status transaksi aktif (uncommitted changes indicator: `TX Active` animated pill)
- [x] Pre-Flight Impact Preview Modal (`ImpactPreviewModal.svelte`: simulasi mutasi AST & table locks)
- [x] Rich Visual Query Plan Tree Renderer (diagram pohon kartu node visual untuk EXPLAIN di `ImpactPreviewModal`)
- [x] Query Stream 5k Batch Pager (ADR 0058: chunked paging untuk query jutaan baris)

### 6. Advanced Virtual Data Grid & CRUD Suite (`10_grid_crud.js`)
- [x] High-Performance Virtual Windowing Data Grid (`VirtualGrid.svelte`)
- [x] Sorting kolom (`ASC` / `DESC` / reset)
- [x] Column Visibility Picker (search, toggle kolom, show all, hide all)
- [x] TablePlus-Style Inline Filter Bar (`addFilterPill`, `removeFilterPill`, `clearFilterPills`)
- [x] Inline Cell Editing dengan Staged Edits Buffer (`stageEdit`, `commitStagedEdits`)
- [x] Staged Edits Review Pill ("X Staged Edits: Save All / Discard")
- [x] Insert Row Modal (`InsertRowModal.svelte`) dengan default value otomatis
- [x] Delete Selected Row (dengan pengecekan Primary Key & konfirmasi)
- [x] Pewarnaan tipe data cell (NULL, number, boolean, json, text)
- [x] Persistent Cell Detail Drawer (panel samping baca text/JSON panjang)
- [x] Vertical Record Inspector Modal (`RecordInspectorModal.svelte` dengan navigasi Next/Prev)
- [x] Ekspor hasil grid ke CSV, JSON, SQL
- [x] Cell Context Menu dengan Quick Filter (=, !=, >, <, LIKE)
- [x] Filter Bar Pushdown lanjutan (operator: `<`, `<=`, `>=`, `NOT LIKE`, `IN`, `NOT IN`, `BETWEEN`, `IS NULL`, `IS NOT NULL`)
- [x] JSON Cell Dedicated Modal (`JsonCellModal.svelte`: validator, prettifier, tree/editor)
- [x] Foreign Key Hover & Peek Popover (`FkPeekPopover.svelte`: intip data baris berelasi tanpa pindah tab)
- [x] Multi-Row Checkbox Selection & Batch Deletion
- [x] Grid In-Place Draft Rows (tambah beberapa baris kosong langsung di grid sebelum disimpan)
- [x] Submenu Lengkap Context Menu Grid:
  - [x] Copy Cell Value
  - [x] Copy Row as TSV (kompatibel Excel / spreadsheet)
  - [x] Copy Row as Markdown Table
  - [x] Copy as INSERT SQL statement
  - [x] Open in JSON Editor
  - [x] Duplicate Record

### 7. Dedicated Table Viewer & Schema Designers (`06_table_designer.js`, `08_programmability_designers.js`)
- [x] Dedicated Table Viewer Tab (`TableViewer.svelte`)
- [x] Sub-Tab Data Grid
- [x] Sub-Tab Structure (`TableStructure.svelte`: kolom, tipe, nullable, PK, default, extra)
- [x] Sub-Tab Indexes (`TableIndexes.svelte`: index name, kolom, unique, tipe)
- [x] Sub-Tab Foreign Keys (`TableForeignKeys.svelte`: constraint name, kolom, ref table & col)
- [x] Sub-Tab DDL (`TableDDL.svelte`: `CREATE TABLE` asli dari server)
- [x] Sub-Tab Diagnostics (`TableDiagnostics.svelte`: estimasi baris, data size, index count, properti)
- [x] Visual Table Column Alteration Designer (tambah, ubah tipe, ganti nama, hapus kolom dengan pratinjau `ALTER TABLE`)
- [x] Column Data Type Presets & Guard (INT, VARCHAR, TEXT, DATETIME, JSON, ENUM, SET)
- [x] Floating ENUM / SET Value Builder Popover (`enum-builder-popover`)
- [x] Visual Index Designer (tambah/edit index: INDEX, UNIQUE, FULLTEXT, SPATIAL, BTREE/HASH)
- [x] Visual Foreign Key Designer (tambah FK, cascade options: CASCADE, SET NULL, RESTRICT, NO ACTION)
- [x] Table Partitions Viewer (inspeksi nama partisi, metode, estimasi baris, data length)

### 8. Programmability Object Designers (`08_programmability_designers.js`)
- [x] Read Definition Routines di Tab Baru (`openRoutineTab`)
- [x] Table Creator Dialog (`TableCreatorModal.svelte`: desainer visual tabel baru dengan preview DDL real-time & open in editor)
- [x] View Designer Dialog (`ViewDesignerModal.svelte`: CodeMirror query editor, algorithm, check option, live DDL preview)
- [x] Trigger Designer Dialog (`TriggerDesignerModal.svelte`: timing BEFORE/AFTER, event INSERT/UPDATE/DELETE, CodeMirror body, live DDL preview)
- [x] Stored Routine / Procedure Designer (`RoutineDesignerModal.svelte`: parameter IN/OUT/INOUT, return type, deterministic, CodeMirror body, live DDL preview)
- [x] Event Scheduler Designer (`EventDesignerModal.svelte`: jadwal EVERY/AT, interval, status ENABLE/DISABLE, on completion preserve, CodeMirror body, live DDL preview)

### 9. DBA Telemetry & Monitoring Suite (`05_dba_telemetry.js`)
- [x] Dedicated Processlist Tab (`ProcesslistView.svelte`)
- [x] Real-time Thread Table: ID, User, Host, DB, Command, Time, State, Info
- [x] Auto-Refresh Polling Control (Paused, 1s, 2s, 5s)
- [x] Live Processlist Filter (User, Host, DB, Command, State, Info)
- [x] Kill Thread Action dengan konfirmasi
- [x] Lock Dependency Tree & Lock Inspector (Deadlock Inspector sub-tab, conflicting transactions & lock modes)
- [x] Processlist Batch Kill Operations (Multi-selection thread checkboxes & Kill Selected batch action)
- [x] Processlist Quick Filter Chips (ADR 0052: All, Active, Sleep, Locked, Slow >5s)
- [x] Processlist Column Visibility Management (ADR 0054)
- [x] Thread Detail Modal (`ThreadDetailModal`: format SQL, copy SQL/JSON, EXPLAIN query, kirim ke AI)
- [x] Process Activity Trend Panel (`ProcessTrendPanel.svelte`: grafik rolling buffer QPS, koneksi aktif, thread over time)
- [x] Cluster & Replication Inspector (Primary/Replica topology, Seconds_Behind_Master, GTID)
- [x] Server Variables Inspector & Tuning Advisor (inspeksi global variables, live edit, tuning recommendation)
- [x] Performance Flight Recorder & Statement Digest (Statement Digest sub-tab: top slow queries, latency, reset digest)
- [x] InnoDB Deadlock Inspector (Deadlock Inspector sub-tab: parser otomatis log deadlock terkini)
- [x] Session Audit Log Drawer (`AuditDrawer.svelte`: timeline query yang dieksekusi selama sesi)

### 10. Database Tools, Sync & Migration (`11_tools_sync.js`)
- [x] SQL Dump & Backup Manager (`DumpModal.svelte`: database/table level, schema only, data only, schema+data)
- [x] Smart Seed / Mock Data Generator (`MockDataModal.svelte`: dummy data realistis berbasis skema)
- [x] Large SQL Script Runner (`RunSqlModal.svelte`: streaming runner untuk file SQL raksasa dengan progress bar)
- [x] Structure Synchronization Engine (`StructureSyncModal.svelte`: visual diff skema antar koneksi & generator skrip ALTER)
- [x] Cross-Connection Data Transfer (`DataTransferModal.svelte`: streaming pipe transfer tabel antar server)
- [x] Row-Level Data Sync (`DataSyncModal.svelte`: komparasi checksum baris antar tabel beda server & sync)

### 11. AI Copilot & Natural Language Assistant (`12_ai_copilot.js`)
- [x] Integrasi Dasar AI Chat (`SqlEditor.svelte` -> `api.aiChat`)
- [x] AI Prompt Input & Query Context Passing
- [x] 1-Click Insert AI Generated SQL into Worksheet
- [x] Persistent AI Copilot Sidebar / Drawer (`AiDrawer.svelte`: panel chat samping dengan streaming response)
- [x] Chat Session History & Management (simpan, restore, buat percakapan baru)
- [x] Context Mentions (`@table`, `@column`, `@note` dalam prompt)
- [x] Quick Action: "Explain Query", "Fix SQL Syntax Error", "Optimize Performance"
- [x] AI Settings Modal (`AiSettingsModal.svelte`: BYOK OpenAI, Claude, Gemini, Ollama lokal, Custom API Endpoint)
- [x] Global Read-Only Lock Mode (toggle gembok proteksi mutasi di header)

### 12. Productivity, Shortcuts, CSV & Privilege Matrix (`13_app_settings_modals.js`)
- [x] Command Palette Modal (`CommandPaletteModal.svelte`: `Ctrl+K` quick open)
- [x] Shortcuts Discovery Modal (`ShortcutsModal.svelte`: `?`)
- [x] Global Keyboard Navigation (`Ctrl+Enter`, `Ctrl+K`, `Ctrl+B`, `Ctrl+T`, `Ctrl+W`, `Esc`)
- [x] Query Snippets Manager (`SnippetsModal.svelte`: pustaka template query dengan variabel dinamis `{{param}}`)
- [x] Snippet Parameter Prompt Modal (modal parameter dinamis terintegrasi, mendukung default `{{param:default}}`)
- [x] CSV Import & Visual Column Mapping (`CsvImportModal.svelte`: unggah file CSV, deteksi kolom, visual mapper, batch insert)
- [x] Database Storage & Health Inspector (`DbDiagnosticsModal.svelte`: fragmentasi tabel, engine overview, optimize/analyze)
- [x] Query Profiler Integration (`QueryProfilerPanel.svelte`: `SHOW PROFILE` stage timing breakdown)
- [x] Visual Privilege Matrix (`PrivilegeMatrixModal.svelte`: matriks hak akses user DB & generator `GRANT`/`REVOKE`)
- [x] Unified Settings Modal (`SettingsModal.svelte`: cloud sync GitHub Gist, notes export/import bundle, LAN PIN)

### 13. Visual ERD Interactive Canvas (`14_erd_canvas.js`)
- [x] Interactive Canvas Modal / Tab (`ErdCanvas.svelte`, integrasi tab & header)
- [x] Komponen Kartu Tabel (nama tabel, daftar kolom, primary/foreign key badge, collapse/expand)
- [x] Garis Relasi Foreign Key Otomatis (konektor Cubic Bezier SVG dengan label badge)
- [x] Kontrol Kanvas Interaktif (Pan, Zoom in/out, Reset 100%, Fit All)
- [x] Auto-Layout Algorithm (topological depth layout berdasar dependensi FK)
- [x] Node Coordinates Persistence (simpan posisi kartu tabel ke `localStorage`)
- [x] Minimap Viewport di sudut kanvas (HTML5 Canvas dengan bounding box & viewport rect)
- [x] Ekspor Diagram ERD (download gambar SVG & Mermaid erDiagram ke clipboard)

### 14. Operational Notes & Team Collaboration (`07_notes_collaboration.js`)
- [x] Operational Notes Inbox Drawer (`NotesDrawer.svelte`: inbox, filter due/pinned/resolved)
- [x] Note Composer & Editor Modal (`NoteComposerModal.svelte` dengan dukungan Markdown)
- [x] Query Snapshot Capture (ADR-0044: kaitkan query SQL, parameter & plan ke catatan)
- [x] Table Viewer Filtered Feed (tab catatan spesifik untuk tabel yang sedang dibuka)
- [x] Background Follow-up Reminder Engine (Web Audio chime alarm saat pengingat jatuh tempo & snooze)

### 15. System Lifecycle, Self-Update & Live Activity (`15_system_lifecycle.js`)
- [x] Self-Update Engine Dialog (`UpdateModal.svelte`: cek rilis baru di GitHub & 1-click update)
- [x] In-App Changelog Viewer (render catatan rilis resmi)
- [x] Activity Console Drawer (`ActivityConsoleDrawer.svelte`: live SSE streaming log backend, query lambat, error)
- [x] Export Activity Log (download log format teks/JSON)

---

## 🚀 Rencana Masa Depan (Fitur Baru Setelah Migrasi Svelte 5 Selesai)
*Fitur-fitur inovasi ini dicatat secara resmi dan akan diimplementasikan setelah 100% paritas tercapai:*
1. **Dark/Light/Cyberpunk Dynamic Theme Engine** — Pemilih tema kustom dengan CSS variables.
2. **Multi-Tab Split View (Side-by-Side SQL Worksheet)** — Kemampuan split 2 editor query bersebelahan.
3. **WASM-Powered Local SQLite Engine** — Eksekusi query SQLite langsung di browser tanpa koneksi server.
4. **Interactive Data Pivot & Charting** — Visualisasi hasil query ke grafik bar/line/pie langsung di atas grid.
5. **Git-Backed Query Versioning** — Commit query langsung ke repository Git lokal.
