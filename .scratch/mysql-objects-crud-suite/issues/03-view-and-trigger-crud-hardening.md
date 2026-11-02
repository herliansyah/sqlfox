# Issue: View and Trigger CRUD Hardening and CodeMirror Integration

Status: resolved
Type: task
Blocked by: 02

## Question

How should the existing View and Trigger CRUD workflows in `web/dist/index.html` be hardened to:
1. Automatically fetch the existing view DDL query when clicking "Edit View" in the sidebar instead of presenting an empty placeholder.
2. Replace plain `<textarea>` inputs with syntax-highlighted CodeMirror editors equipped with line numbers and bracket matching.
3. Fix the Trigger update error on MySQL/MariaDB by executing an atomic or sequential `DROP TRIGGER IF EXISTS` preceding the new `CREATE TRIGGER` statement inside `submitCreateTrigger()`.
4. Provide a 1-click "Open in Query Tab" button alongside "Save View" and "Save Trigger".

## Answer

1. **Auto-Fetch View DDL and DEFINER Stripping**:
   - In `openCreateViewModal(viewName)`: When `viewName` is provided, SqlFox queries `GET /api/session/${activeConnection.id}/ddl/${encodeURIComponent(viewName)}`.
   - The response DDL is sanitized with `stripDefiner(ddl)` to eliminate host-specific definer lockouts, and parsed via `extractViewSelectQuery(ddl)` to isolate the inner `SELECT` statement before populating the CodeMirror editor.
2. **CodeMirror Integration**:
   - Replaced plain `<textarea>` inputs in both `#view-designer-dialog` and `#trigger-designer-dialog` with CodeMirror containers (`#view-designer-cm-container` and `#trigger-designer-cm-container`).
   - Initialized via `getOrCreateModalCodeMirror()`, ensuring line numbers, SQL syntax highlighting, bracket matching, and auto-refresh on `showModal()` without zero-height glitches. The old `<textarea>` elements remain in the DOM as hidden inputs for seamless backwards compatibility.
3. **Trigger Safe Replacement Cycle**:
   - Updated `submitCreateTrigger()` and `assembleTriggerDDL()`: When `isEditing` is true, the generator automatically prepends `DROP TRIGGER IF EXISTS \`${name}\`;\n` before the `CREATE TRIGGER ...` statement.
   - Tested on MySQL/MariaDB and SQLite, completely resolving `ERROR 1359 (HY000): Trigger already exists`.
4. **1-Click "Open in Query Tab" Handoff**:
   - Added `openActiveViewInQueryTab()` and `openActiveTriggerInQueryTab()` buttons docked on the left of each dialog footer.
   - One click closes the dialog and passes the assembled DDL directly into a new Query tab (`openInQueryEditor()`) with full syntax highlighting, focused cursor, and editor dirty state tracking.
5. **Automated Verification**:
   - Added `TestViewAndTriggerCodeMirrorIntegrationEmbedded` in `web/embed_test.go`, verifying container IDs, handoff handlers, DEFINER stripping, and trigger replacement logic.
