# Issue: CodeMirror Modal Embedding and Hybrid Tab Handoff Architecture

Status: resolved
Type: prototype
Blocked by: 01

## Question

How should the embedded CodeMirror SQL editor be initialized and managed inside HTML5 `<dialog>` modals (such as `view-designer-dialog`, `trigger-designer-dialog`, `routine-designer-dialog`, and `event-designer-dialog`) without suffering from zero-height or unpainted line-number artifacts when the dialog opens (`showModal()`), and how should the 1-click "Open in Query Tab" (↗️) transition seamlessly transfer the assembled DDL into a fresh Query tab with active CodeMirror focus and dirty state tracking?

## Answer

1. **Modal CodeMirror Lifecycle & Zero-Height Elimination**:
   - Provide a shared factory function `getOrCreateModalCodeMirror(containerId, initialValue, mode = 'text/x-mysql')`.
   - Store the instance on `container._cmInstance`. When opening an existing modal again, reuse the instance via `.setValue(initialValue)` rather than tearing down and reconstructing DOM nodes.
   - Set container height explicitly (e.g. `class="h-52 border border-slate-700 rounded bg-slate-950 overflow-hidden"`). Because global `.CodeMirror` uses `height: 100% !important;`, it fills the container cleanly.
   - In the modal opener, invoke `setTimeout(() => { if (cm) { cm.refresh(); cm.focus(); } }, 50);` immediately after `dlg.showModal()`. This ensures the browser computes layout geometry before CodeMirror measures line heights and gutter widths, eliminating unpainted line numbers and hidden cursor artifacts.
2. **Hybrid Tab Handoff Architecture (`openInQueryEditor`)**:
   - A universal function `openInQueryEditor(assembledDDL, tabTitle)`:
     1. Closes the active modal dialog via `dlg.close()`.
     2. Calls `addNewQueryTab()`, allocating a fresh tab with an inherited active database context.
     3. Updates the tab title (e.g. `View: v_active` or `Procedure: sp_calc`).
     4. Sets editor content via `setEditorSQL(assembledDDL)`.
     5. Focuses the main `cmEditor` and positions the cursor at the end of the text.
3. **Verified Assemblers & DEFINER Sanitizer**:
   - Verified via prototype check `cm_modal_prototype_check.js`:
     - Universal `stripDefiner(sql)` removes all `DEFINER = user@host` clauses from extracted DDL.
     - `assembleTriggerDDL`: generates `DROP TRIGGER IF EXISTS` + `CREATE TRIGGER` for edits.
     - `assembleRoutineDDL`: generates parameter modes (`IN`, `OUT`, `INOUT` for procedures, bare types for functions), `RETURNS` type, and `DETERMINISTIC` characteristics.
     - `assembleEventDDL`: generates `EVERY ...` (recurring) or `AT ...` (one-time) with `ON COMPLETION PRESERVE`.
