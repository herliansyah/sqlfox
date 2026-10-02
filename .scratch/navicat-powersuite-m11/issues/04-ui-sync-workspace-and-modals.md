# Issue: UI Sync Modals and Workflow

Status: resolved
Type: task

## Goal
Integrate Structure Sync, Data Transfer, and Data Sync dialog modals into SqlFox web interface with source/target connection pickers, progress displays, and diff previews.

## Requirements
- Top Navbar `📦 DB Tools ▾` menu links:
  - `🔄 Structure Sync`
  - `🚚 Data Transfer`
  - `⚖️ Data Sync`
- Native Modals:
  - `#structure-sync-dialog`: Source & Target connection selector, Compare button, Side-by-side diff list with item checkboxes, DDL Preview script, Run Migration button.
  - `#data-transfer-dialog`: Source & Target selector, Table list with select all/none, Options (`Create missing tables`, `Truncate target`), Start Transfer button with live NDJSON progress bar.
  - `#data-sync-dialog`: Source & Target selector, Table picker, Compare button, Diff summary (`+ Insert`, `~ Update`, `- Delete`), Diff data grid preview, Apply Changes button with Production challenge gating.

## Answer
Implemented in `web/dist/index.html` with:
- Top Navbar `📦 DB Tools ▾` menu items: `🔄 Structure Sync (Schema Diff)`, `🚚 Cross-Connection Data Transfer`, `⚖️ Row-Level Data Sync`.
- Dialogs 23, 24, 25 (`#structure-sync-dialog`, `#data-transfer-dialog`, `#data-sync-dialog`).
- JavaScript client controllers with streaming NDJSON progress, diff rendering, and Production challenge gating.

