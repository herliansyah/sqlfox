# Issue: Scheduled Event Designer and Lifecycle Management Suite

Status: resolved
Type: task
Blocked by: 02

## Question

How should the visual Scheduled Event Designer and full lifecycle management be implemented in SqlFox:
1. Adding a `+` action button in the sidebar "Events" section header and hover action buttons ("Edit" and "Drop") on individual event items.
2. Creating `#event-designer-dialog` with:
   - Event Name, Status (`ENABLED` / `DISABLED`), and On Completion (`PRESERVE` / `NOT PRESERVE`).
   - Schedule configuration mode: Recurring Interval (`EVERY [amount] [MINUTE|HOUR|DAY|WEEK|MONTH]`) with optional `STARTS` / `ENDS` timestamps OR One-time execution (`AT [timestamp]`).
   - CodeMirror SQL body editor with boilerplate `DO\nBEGIN\n  -- Statements\nEND;`.
   - "Save & Apply" and "Open in Query Tab" actions.
3. Providing safe `DROP EVENT [IF EXISTS]` execution with confirmation challenge and DDL review modal.
4. Auto-populating the designer form when clicking "Edit Event" from the sidebar.

## Answer

1. **Sidebar Affordances and Actions**:
   - Added a `+` button in the sidebar "Events" section header invoking `openCreateEventModal()`.
   - Added hover buttons on each event item:
     - "View DDL" (`viewEventDDL(name)`)
     - "Edit Event" (`openCreateEventModal(name)`)
     - "Drop Event" (`dropEvent(name)`)
     - "Toggle Status" (`toggleEventStatusAction(name, enable)`)
2. **Interactive Event Designer Dialog (`#event-designer-dialog`)**:
   - Implemented dialog with:
     - Event Name input and Status selector (`ENABLE` / `DISABLE`).
     - `PRESERVE` checkbox (`ON COMPLETION PRESERVE`, preventing MySQL from auto-deleting the event once its schedule expires).
     - Radio selector for Schedule mode:
       - **Recurring (`EVERY`)**: Amount input, unit dropdown (`SECOND`, `MINUTE`, `HOUR`, `DAY`, `WEEK`, `MONTH`, `YEAR`), optional `STARTS` timestamp, optional `ENDS` timestamp.
       - **One-Time (`AT`)**: Explicit execution timestamp input.
     - CodeMirror SQL action statement editor (`DO ...`) initialized via `getOrCreateModalCodeMirror('event-designer-cm-container')` with auto-refresh on showModal.
     - "Open in Query Tab" (↗️) button for seamless full-screen editor handoff.
     - "Save Event" button running through the DDL review modal and Safety Guard.
3. **Safe Editing and Drop Cycles**:
   - Editing an existing event pre-populates name, status, preservation, schedule mode, interval or timestamp, and fetches DDL via `GET /api/session/:id/ddl/event/:name`.
   - Saving an edit prepends `DROP EVENT IF EXISTS \`name\`;\n` to safely replace the event on MySQL/MariaDB.
   - Dropping an event prompts a native confirmation challenge and executes `DROP EVENT IF EXISTS \`name\`;` through the DDL review modal, automatically refreshing the schema tree on completion.
4. **Automated Verification**:
   - Added `TestEventDesignerSuiteEmbedded` in `web/embed_test.go`, verifying modal markup, sidebar actions, schedule toggle logic, and DDL assembly.
