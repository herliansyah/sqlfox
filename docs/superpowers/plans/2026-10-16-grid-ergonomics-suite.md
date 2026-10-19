# Virtual Grid Ergonomics Suite Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Virtual Grid Ergonomics Suite (Grid Right-Click Quick-Filters, Column Visibility Picker, and Single Record Vertical Form View) directly in `web/dist/index.html`.

**Architecture:** Client-side UI enhancements extending the existing Virtual Data Grid, TablePlus Filter Bar, and `stagedEdits` system without backend API mutations. Zero extra dependencies, pure DOM & standard JavaScript.

**Tech Stack:** Native HTML5, Tailwind CSS classes, vanilla JavaScript in `web/dist/index.html`, pure Node.js (`fs`, `vm`, `assert`) for test verification.

**Spec:** [docs/superpowers/specs/2026-10-16-grid-ergonomics-suite-design.md](file:///home/ian/sqlfox-shad/docs/superpowers/specs/2026-10-16-grid-ergonomics-suite-design.md)

## Global Constraints

- Zero external npm dependencies and zero CGO dependencies.
- 100% client-side UI execution with zero backend API overhead.
- Strict memory ceiling (<100MB RAM) and sub-second binary startup (<1s).
- Preserve compatibility with existing TablePlus Filter Bar, Keyset Pagination, and `stagedEdits`.
- All automated tests written in zero-dependency Node.js (`fs`, `vm`, `assert`).

## Review Focus

1. Cell value containing single quotes (e.g. `O'Reilly`) or `NULL` correctly sanitized without breaking filter SQL pushdown.
2. Tables without a Primary Key supported in Form View via existing `row_criteria` without throwing undefined PK errors.
3. Column picker prevents hiding all columns by enforcing sticky lock on Primary Key (or first column if PK absent).
4. ArrowLeft / ArrowRight hotkeys inside Record Inspector ignored when user is actively focused in an `<input>` or `<textarea>`.
5. Column visibility preferences persisted in `sessionStorage` and restored when switching tabs or reopening tables.

---

### Task 1: Zero-Dependency Automated Test Harness

**Files:**
- Create: `scripts/test-grid-ergonomics-suite.mjs`
- Test: `node scripts/test-grid-ergonomics-suite.mjs`

**Interfaces:**
- Consumes: `web/dist/index.html`
- Produces: Executable Node.js test script verifying HTML DOM elements and JavaScript functions for Quick-Filter, Column Picker, and Record Inspector.

- [ ] **Step 1: Write the test harness script**

Create `scripts/test-grid-ergonomics-suite.mjs` with assertions for:
1. Context menu filter section (`#grid-ctx-filter-section`, `#grid-ctx-filter-options`).
2. Column picker toolbar button (`#btn-col-picker`, `#col-picker-dropdown`, `#col-picker-list`).
3. Record inspector dialog (`#record-inspector-dialog`, `#record-inspector-fields`, `#record-inspector-pager`).
4. VM sandbox testing `applyQuickFilterFromCell()`, `toggleColumnVisibility()`, and `openRecordInspector()`.

- [ ] **Step 2: Run test to verify it fails initially**

Run: `node scripts/test-grid-ergonomics-suite.mjs`  
Expected: FAIL with assertion error indicating missing elements or functions.

- [ ] **Step 3: Commit the test harness**

```bash
git add scripts/test-grid-ergonomics-suite.mjs
git commit -m "test: add automated test harness for grid ergonomics suite"
```

---

### Task 2: Grid Right-Click Quick-Filters (TablePlus-Style)

**Files:**
- Modify: `web/dist/index.html` (around lines 4366+ `#grid-context-menu` and lines 15930+ `openGridContextMenu`)
- Test: `node scripts/test-grid-ergonomics-suite.mjs`

**Interfaces:**
- Consumes: `currentColumns`, `currentResults`, `stagedEdits`, `activeTab.filterPills`, `applyTableFilter()`
- Produces: `applyQuickFilterFromCell(colName, op, val)`

- [ ] **Step 1: Add Filter Section to `#grid-context-menu` in HTML**

In `web/dist/index.html`, add `<div id="grid-ctx-filter-section" class="py-1">` above the edit actions with a header `🔍 Quick Filter` and `<div id="grid-ctx-filter-options"></div>`.

- [ ] **Step 2: Update `openGridContextMenu()` to populate dynamic filter actions**

In JavaScript inside `openGridContextMenu(e, rIdx)`:
- Extract `colName = col.name`, `rawVal = staged ? staged.newVal : currentResults[rIdx][cIdx]`, and `colType`.
- If `rawVal === null || rawVal === undefined`:
  - Add button `∅ Filter: ${colName} IS NULL`
  - Add button `✓ Filter: ${colName} IS NOT NULL`
- Else:
  - Add button `🔍 Filter: ${colName} = '${val}'`
  - Add button `🚫 Filter: ${colName} != '${val}'`
  - If numeric or temporal: add `▶ Filter: ${colName} > '${val}'` and `◀ Filter: ${colName} < '${val}'`
  - If text: add `* Filter: ${colName} LIKE '%${val}%'`

- [ ] **Step 3: Implement `applyQuickFilterFromCell(colName, op, val)`**

Implement helper:
- Ensure active tab is table viewer.
- Switch `filterBarMode` to `'pills'` if currently in `'raw'` mode.
- Push `{ col: colName, column: colName, op: op, operator: op, val: val, value: val }` to `activeTab.filterPills`.
- Call `renderFilterPills(activeTab)`.
- Call `applyTableFilter()`.
- Close context menu.

- [ ] **Step 4: Run test to verify Quick-Filter assertions pass**

Run: `node scripts/test-grid-ergonomics-suite.mjs`  
Expected: Quick-filter assertions pass.

- [ ] **Step 5: Commit**

```bash
git add web/dist/index.html
git commit -m "feat(grid): right-click quick-filters on data grid cells"
```

---

### Task 3: Column Visibility Picker (Show/Hide Columns)

**Files:**
- Modify: `web/dist/index.html` (Table Viewer toolbar around line 812, `renderGridHeaders()`, and `renderGridRows()`)
- Test: `node scripts/test-grid-ergonomics-suite.mjs`

**Interfaces:**
- Consumes: `activeTab.hiddenColumns`, `sessionStorage`, `renderGridHeaders()`, `renderGridRows()`
- Produces: `toggleColumnPickerDropdown()`, `toggleColumnVisibility()`, `showAllColumns()`

- [ ] **Step 1: Add Column Visibility Picker button & dropdown to toolbar**

In `web/dist/index.html` toolbar (next to TablePlus Filter Bar), add:
- `#btn-col-picker` with `#col-picker-count-badge`
- `#col-picker-dropdown` with search box `#col-picker-search`, `All` button, and checklist container `#col-picker-list`.

- [ ] **Step 2: Implement dropdown logic & sessionStorage persistence**

Implement:
- `toggleColumnPickerDropdown(e)`
- `filterColumnPickerList(query)`
- `toggleColumnVisibility(colName, isVisible)`:
  - Updates `activeTab.hiddenColumns` (Set).
  - Persists to `sessionStorage.setItem('sqlfox-hidden-cols:' + db + ':' + table, JSON.stringify([...hiddenColumns]))`.
  - Re-renders headers & rows: `renderGridHeaders()`, `renderGridRows()`.
  - Updates badge count: `(visibleCount/totalCount)`.
- `showAllColumns()`: Clears `activeTab.hiddenColumns` and re-renders.

- [ ] **Step 3: Update `renderGridHeaders()` and `renderGridRows()` to skip hidden columns**

- In `renderGridHeaders()`: check `if (activeTab?.hiddenColumns?.has(col.name)) return '';`
- In `renderGridRows()`: check `if (activeTab?.hiddenColumns?.has(cols[cIdx]?.name)) return '';`
- Guard PK column from being unchecked (lock icon `🔒`).

- [ ] **Step 4: Run test to verify Column Visibility Picker passes**

Run: `node scripts/test-grid-ergonomics-suite.mjs`  
Expected: Column picker assertions pass.

- [ ] **Step 5: Commit**

```bash
git add web/dist/index.html
git commit -m "feat(grid): column visibility picker with show and hide controls"
```

---

### Task 4: Vertical Record Inspector (Single Record Form View)

**Files:**
- Modify: `web/dist/index.html` (dialog markup and event handlers)
- Test: `node scripts/test-grid-ergonomics-suite.mjs`

**Interfaces:**
- Consumes: `currentResults`, `currentColumns`, `stagedEdits`, `synthesizeSingleRowWhere`
- Produces: `openRecordInspector(rIdx)`, `navigateRecordInspector(delta)`, `onRecordFieldChange(colName, newVal)`

- [ ] **Step 1: Add `<dialog id="record-inspector-dialog">` to HTML**

Add dialog markup with:
- Header with title `#record-inspector-title`, pager `#record-inspector-pager` with `Prev`/`Next` buttons and row indicator, and close button.
- Toolbar with field search input `#record-inspector-search`, `Copy JSON`, and `Copy Text` buttons.
- Content container `#record-inspector-fields` with two-column responsive styling.

- [ ] **Step 2: Implement `openRecordInspector(rIdx)` and field rendering**

Implement:
- Graceful check: if `!currentResults || currentResults.length === 0`, show toast *"No records to inspect"*.
- State tracking: `activeInspectorRowIdx = rIdx`.
- Render field rows with column name, PK/FK/Type badges, and type-aware inputs (multiline for text, pretty-print for JSON, toggle for bool).
- Wire `Spacebar` keydown handler on data grid table.
- Wire context menu item `📋 Inspect Record (Form View)` to `openRecordInspector(activeGridRightClickedRow)`.

- [ ] **Step 3: Implement row navigation and `stagedEdits` integration**

Implement:
- `navigateRecordInspector(delta)`: moves `rIdx` by -1 or +1 bounded to `[0, currentResults.length - 1]`.
- Hotkey navigation: `ArrowLeft` and `ArrowRight` (guarded with `!e.target.matches('input, textarea, select')`).
- `onRecordFieldChange(colName, newVal)`:
  - Resolves cell coordinates `(rIdx, cIdx)`.
  - Updates `stagedEdits.set(stagedKey, { rIdx, cIdx, db, table, colName, pkCol, pkVal, oldVal, newVal })`.
  - Highlights field card with amber `● Modified`.
  - Updates global toolbar `💾 Save Changes` counter.
- Copy actions: `copyRecordAsJSON()` and `copyRecordAsText()`.

- [ ] **Step 4: Run test to verify Record Inspector passes**

Run: `node scripts/test-grid-ergonomics-suite.mjs`  
Expected: All test suites pass 100%.

- [ ] **Step 5: Commit**

```bash
git add web/dist/index.html
git commit -m "feat(grid): vertical record inspector single record form view"
```

---

### Task 5: End-to-End Verification & Documentation Updates

**Files:**
- Modify: `CHANGELOG.md`, `MILESTONES.md`, `docs/USER_GUIDE.md`, `docs/USER_GUIDE_ID.md`
- Verify: `go test -v ./web`, `go test -v ./...`, `node scripts/test-grid-ergonomics-suite.mjs`

- [ ] **Step 1: Run complete verification suite**

```bash
node scripts/test-grid-ergonomics-suite.mjs
go test -v ./web
go test -v ./...
```
Expected: 100% PASS across all scripts and Go tests.

- [ ] **Step 2: Document milestone & release notes**

Update `CHANGELOG.md`, `MILESTONES.md`, `docs/USER_GUIDE.md`, and `docs/USER_GUIDE_ID.md` detailing Milestone 38 deliverables.

- [ ] **Step 3: Commit documentation**

```bash
git add CHANGELOG.md MILESTONES.md docs/USER_GUIDE.md docs/USER_GUIDE_ID.md
git commit -m "docs: document milestone 38 virtual grid ergonomics suite"
```
