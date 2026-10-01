# 0016: Grid Inline Edit Lifecycle and Staged Apply Workflow

To resolve multi-cell input contention and provide flexible persistence workflows matching professional database management tools (such as HeidiSQL and DBeaver), SqlFox adopts a single-active-cell state machine alongside a configurable Staged Apply / Immediate Persist architecture.

## Context & Problem

Previously, the Virtual Data Grid's inline cell editor replaced `<td>` innerHTML with an `<input>` element without maintaining an active cell lifecycle reference. The input only listened to `Enter` and `Escape` keyboard events:
- Clicking another cell or clicking outside did not blur or commit the active edit, leaving multiple input fields concurrently open.
- Edits were either committed immediately on `Enter` or lost, without support for staging multiple edits across rows and columns before applying them atomically.
- The backend `/api/session/{id}/cell-edit` endpoint only accepted single-cell updates without database qualification or identifier quoting.

## Considered Options

- **Third-Party Grid Libraries (e.g. AG Grid, Handsontable)**: Introduces heavy multi-megabyte bundle footprints and complex build pipelines, violating the single-file embedded SPA and zero-dependency principles.
- **Immediate Persist Only (Auto-Save)**: Every cell edit immediately triggers an HTTP roundtrip to update the database. While convenient, it can lead to accidental updates on production environments and lacks transactional batch review.
- **Staged Batch Apply Only**: All edits must be explicitly staged and applied via a button. While safe, it adds friction for quick single-value edits.
- **Configurable Dual-Mode Architecture**:
  - **`📝 Staged` (Default)**: In-memory change accumulation in `stagedEdits`. Cells display visual dirty state (`●` badge + amber highlight). The grid toolbar exposes `Apply (N)` and `Discard` actions.
  - **`⚡ Auto` (Immediate Persist)**: Cells automatically persist on input blur or Enter key submission.
  - **Confirmation Gate (`Confirm`)**: An optional pre-flight confirmation dialog (`showNativeConfirm`) protecting against unintended mutations in both modes.

## Decision

1. **Single-Active-Cell State Machine**: An `activeEditCell` reference tracks the currently open cell. Initiating an edit on any other cell or clicking outside triggers `commitInlineEdit(activeEditCell)`, cleanly closing the prior input before proceeding.
2. **Dual-Mode Persistence**: The Grid Toolbar includes a toggle switch between `📝 Staged` and `⚡ Auto` mode, alongside a `Confirm` checkbox, persisted locally via `localStorage`.
3. **Backend Batch Support**: `POST /api/session/{id}/cell-edit` is enhanced to handle single-cell payloads as well as batch arrays (`edits: [...]`) with backtick identifier quoting and target database scoping.

## Consequences

- Exactly one cell is ever in active edit mode at any moment.
- Users can choose between zero-friction immediate updates or safe batch reviews.
- Zero external runtime dependencies added.
