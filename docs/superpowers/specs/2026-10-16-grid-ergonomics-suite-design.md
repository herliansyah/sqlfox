# Virtual Grid Ergonomics Suite Design Spec

**Topic:** Virtual Grid Ergonomics Suite: Right-Click Quick-Filters, Column Visibility Picker & Vertical Record Inspector (Form View)  
**Date:** 2026-10-16  
**Status:** Approved  
**Target Milestone:** Milestone 38 (Virtual Grid Ergonomics Suite)  

---

## 1. Overview & Goals

SqlFox features a high-performance Virtual Data Grid (`@tanstack/virtual` + custom DOM streaming) capable of rendering 100,000+ rows with keyset pagination and inline cell mutations. However, everyday DBA and developer workflows on wide or dense tables face two ergonomic bottlenecks:
1. **Filtering friction:** Users must manually click "+ AND", pick column names from dropdowns, and type filter values even when the exact value is already visible on screen.
2. **Horizontal scrolling fatigue on wide tables:** Tables with 30–80 columns (e.g. `users`, `orders`, `system_logs`) require extensive horizontal scrolling and truncate long text or JSON payloads, making single-record inspection tedious.

This design introduces the **Virtual Grid Ergonomics Suite**, delivering three tightly-coupled, zero-bloat capabilities inspired by TablePlus, DBeaver, and HeidiSQL:
* **Grid Right-Click Quick-Filters (TablePlus-style):** Instant contextual filtering (`=`, `!=`, `>`, `<`, `LIKE`, `IS NULL`) directly from cell context menus into the existing TablePlus Filter Bar.
* **Column Visibility Picker (HeidiSQL-style):** Toolbar dropdown with column checkboxes to show or hide non-critical columns with sticky Primary Key protection.
* **Vertical Record Inspector / Single Record Form View (DBeaver-style):** Modal dialog (`Spacebar` trigger) displaying an individual row's columns vertically with instant search, formatted JSON/text, row pager navigation (`Prev`/`Next`), and direct integration with SqlFox's `stagedEdits`.

---

## 2. Architecture & Design Principles

* **Zero Backend Overhead:** All three components operate 100% on the client-side (embedded web UI in `web/dist/index.html`). No new server endpoints, no Go daemon changes, and zero additional memory footprint on the remote database.
* **Reuse Over Rewrite:**
  * Quick-Filters compile directly into the existing `activeTab.filterPills` array and trigger `applyTableFilter()`, fully reusing the existing AST safety token validation and server-side pushdown.
  * Form View edits read from and write directly into `stagedEdits` (`Map`), ensuring zero discrepancy between Grid and Form mutations.
* **Bounded Persistence:**
  * Hidden column preferences are cached per-table in `sessionStorage` (`sqlfox-hidden-cols:{db}:{table}`).
  * Form View is rendered dynamically into a reusable native HTML5 `<dialog id="record-inspector-dialog">`.

---

## 3. Detailed Component Specifications

### 3.1. Grid Right-Click Quick-Filters

#### Trigger & Menu Augmentation
In `openGridContextMenu(e, rIdx)`:
When right-clicking a cell with column index `cIdx` on column `col = currentColumns[cIdx]`:
Extract `rawVal` from `stagedEdits.get(stagedKey)` or `currentResults[rIdx][cIdx]`.
A new `#grid-ctx-filter-section` is rendered in `#grid-context-menu`:

1. **If `rawVal !== null && rawVal !== undefined`:**
   * `🔍 Filter where col = 'val'`
   * `🚫 Filter where col != 'val'`
   * If type category is numeric or temporal (`INT`, `FLOAT`, `DECIMAL`, `DATE`, `DATETIME`, `TIMESTAMP`):
     * `▶ Filter where col > 'val'`
     * `◀ Filter where col < 'val'`
   * If type category is text (`VARCHAR`, `TEXT`, `CHAR`, `UUID`):
     * `* Filter where col LIKE '%val%'`
2. **If `rawVal === null || rawVal === undefined`:**
   * `∅ Filter where col IS NULL`
   * `✓ Filter where col IS NOT NULL`

#### Execution Workflow
```javascript
function applyQuickFilterFromCell(colName, op, val) {
  const activeTab = tabs.find(t => t.id === activeTabId);
  if (!activeTab || !activeTab.isTableViewer) return;
  
  if (filterBarMode === 'raw') {
    setFilterBarMode('pills');
  }
  if (!Array.isArray(activeTab.filterPills)) {
    activeTab.filterPills = [];
  }
  
  activeTab.filterPills.push({
    col: colName,
    column: colName,
    op: op,
    operator: op,
    val: val,
    value: val
  });
  
  renderFilterPills(activeTab);
  applyTableFilter();
  closeGridContextMenu();
}
```

---

### 3.2. Column Visibility Picker

#### Toolbar Element
Located in the Table Viewer toolbar (`#table-viewer-toolbar`), alongside the Filter Bar:
```html
<div class="relative inline-block">
  <button type="button" id="btn-col-picker" onclick="toggleColumnPickerDropdown(event)"
          class="hover:bg-slate-800 text-slate-300 hover:text-white px-2 py-1 rounded border border-slate-700 text-xs transition flex items-center space-x-1.5"
          title="Show or hide table columns">
    <span>👁️</span><span>Columns</span>
    <span id="col-picker-count-badge" class="text-[10px] text-slate-400 font-mono">(15/15)</span>
    <span class="text-[9px] text-slate-500">▾</span>
  </button>
  
  <!-- Column Picker Dropdown Menu -->
  <div id="col-picker-dropdown" class="hidden absolute left-0 mt-1 z-50 bg-slate-900 border border-slate-700 rounded-lg shadow-2xl p-2 text-xs text-slate-200 min-w-64 max-h-80 flex flex-col ring-2 ring-black/50">
    <div class="pb-1.5 mb-1.5 border-b border-slate-800 flex items-center justify-between gap-1.5">
      <input type="text" id="col-picker-search" oninput="filterColumnPickerList(this.value)" placeholder="Search column..."
             class="w-full bg-slate-950 border border-slate-700 rounded px-2 py-0.5 text-xs text-white placeholder-slate-500 focus:outline-none focus:border-sky-500 font-mono" />
      <button type="button" onclick="showAllColumns()" class="text-[10px] text-sky-400 hover:text-sky-300 whitespace-nowrap px-1">All</button>
    </div>
    <div id="col-picker-list" class="overflow-y-auto flex-1 space-y-1 max-h-60 pr-1">
      <!-- Checkbox items rendered dynamically -->
    </div>
  </div>
</div>
```

#### State & Grid Rendering Rules
* State per tab: `activeTab.hiddenColumns = new Set()`
* Storage key: `sqlfox-hidden-cols:${activeGridDB || ''}:${activeGridTable || ''}`
* Primary Key Guard: Checkbox for Primary Key column is locked (`disabled`, icon `🔒 PK`), preventing users from hiding PK and breaking inline mutations.
* Selective Rendering:
  * In `renderGridHeaders()`:
    ```javascript
    if (activeTab?.hiddenColumns?.has(col.name)) return '';
    ```
  * In `renderGridRows()`:
    ```javascript
    if (activeTab?.hiddenColumns?.has(cols[cIdx]?.name)) return '';
    ```
  * Badge displays `(${total - hidden}/${total})` with amber styling if any columns are hidden.

---

### 3.3. Vertical Record Inspector (Single Record Form View)

#### Triggers
* **Spacebar Hotkey:** When any cell or row in `#results-table` is selected (`activeGridSelectedRow >= 0`), pressing `Space` (when not inside an active cell input or textarea) opens the inspector.
* **Context Menu:** `#grid-context-menu` ➜ `📋 Inspect Record (Form View)` (hint: `Space`).
* **Row Number Action:** Clicking row index `#` or row inspector button opens the inspector.

#### Modal Layout & Features (`<dialog id="record-inspector-dialog">`)
1. **Header Navigation:**
   * Title: `📋 Record Details: ${tableName}`
   * Row Pager: `[◀ Prev]` Button, `Row ${rIdx + 1} of ${totalRows}` Badge, `[Next ▶]` Button.
   * Hotkeys: `ArrowLeft` / `ArrowRight` cycles through rows (disabled while focused on `<input>` or `<textarea>`).
   * Close: `[✕]` or `Escape`.
2. **Toolbar:**
   * Search Input: `🔍 Filter fields...` filters visible field cards in real time.
   * Actions: `[📋 Copy JSON]` (serializes row to JSON object), `[📋 Copy Text]` (serializes to `col: val` text).
3. **Field Card Layout (Two-Column):**
   * Left: Column Name, PK badge (`🔑`), FK badge (`🔗`), Data Type pill (`VARCHAR(255)`, `DATETIME`, etc.).
   * Right:
     * If `NULL`: formatted `∅ NULL` italic badge.
     * If JSON: pretty-printed syntax-highlighted block with `[✏️ Edit JSON]` trigger.
     * If Multiline Text: auto-expanding textarea.
     * Direct Edit: Inputs bind directly to `onRecordFieldChange(colName, newVal)`, syncing to `stagedEdits` and displaying amber `● Modified` indicator.

---

## 4. Error Handling & Edge Cases

1. **Zero Rows:** If `currentResults.length === 0`, pressing `Spacebar` displays non-blocking toast: *"No records to inspect"*.
2. **PK-Less Tables:** Editing in Form View falls back to SqlFox's `row_criteria` synthesis (`synthesizeSingleRowWhere`), preventing execution errors.
3. **Empty Grid Guard:** The PK column (or first column if no PK) cannot be unchecked in Column Picker, preventing accidental 0-column grids.
4. **Special Characters:** `compileFilterPills` handles quotes (`'`) safely via `.replace(/'/g, "''")`.
5. **Keyboard Trap Guard:** Form View hotkeys (`Space`, `ArrowLeft`, `ArrowRight`) are strictly guarded with `if (e.target.matches('input, textarea, select')) return;`.

---

## 5. Automated Verification Plan

### 5.1. Automated Headless Test (`scripts/test-grid-ergonomics-suite.mjs`)
* Validates right-click menu populates filter actions and appends pill to `activeTab.filterPills`.
* Validates Column Visibility Picker toggles columns and updates `hiddenColumns` set and badge counter.
* Validates `Spacebar` opens `#record-inspector-dialog`, cycles rows via Prev/Next, and writes to `stagedEdits`.

### 5.2. Go Embedding & Regressions
* `go test -v ./web` (verifies embedded asset integrity and bundle compilation).
* `go test -v ./...` (verifies zero regressions across backend packages).
