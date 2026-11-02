# Issue: Stored Routine Designer and Drop Management Suite

Status: resolved
Type: task
Blocked by: 02

## Question

How should the visual Stored Routine (Procedure & Function) Designer and lifecycle management be implemented in SqlFox:
1. Adding a `+` action button in the sidebar "Routines" section header and hover action buttons ("Edit" and "Drop") on individual routine items.
2. Creating `#routine-designer-dialog` with:
   - Routine Name, Type selector (`PROCEDURE` / `FUNCTION`), Deterministic checkbox, and Return Type (for functions).
   - An interactive Parameter Grid builder supporting adding/removing parameters with fields: `Name`, `Mode` (`IN`, `OUT`, `INOUT`), and `Data Type` (`INT`, `VARCHAR(255)`, `DECIMAL(10,2)`, etc.).
   - CodeMirror SQL body editor with intelligent boilerplate (`BEGIN ... END;`).
   - "Save & Apply" and "Open in Query Tab" actions.
3. Providing safe `DROP PROCEDURE / FUNCTION [IF EXISTS]` execution with confirmation challenge and DDL review modal.
4. Auto-populating the designer form when clicking "Edit Routine" from the sidebar.

## Answer

1. **Sidebar Affordances and Actions**:
   - Added a `+` button in the sidebar "Routines" section header invoking `openCreateRoutineModal()`.
   - Added hover buttons on every routine item:
     - "View DDL" (`viewRoutineDDL(name, type)`)
     - "Edit Routine" (`openCreateRoutineModal(name, type)`)
     - "Drop Routine" (`dropRoutine(name, type)`)
     - "Call in Query Editor" (`openRoutineCaller(name, type)`)
2. **Interactive Routine Designer Dialog (`#routine-designer-dialog`)**:
   - Implemented dialog with:
     - Name input and Type dropdown (`PROCEDURE` vs `FUNCTION`).
     - Returns type input (automatically shown for functions, hidden for procedures).
     - `DETERMINISTIC` checkbox (checked by default to prevent MySQL error 1418 with binary logging).
     - Dynamic Parameter Grid builder: adds and removes parameter rows (`Name`, `Mode` [`IN`/`OUT`/`INOUT`], `Data Type`). Mode selector is automatically hidden for functions since MySQL functions only accept input parameters.
     - CodeMirror SQL body editor initialized via `getOrCreateModalCodeMirror('routine-designer-cm-container')` with auto-refresh on showModal.
     - "Open in Query Tab" (↗️) button for full-screen editing in a new query tab.
     - "Save Routine" button running through the DDL review modal and Safety Guard.
3. **Safe Editing and Drop Cycles**:
   - Editing an existing routine pre-populates name, type, returns type, parameters, and auto-fetches DDL via `GET /api/session/:id/ddl/routine/:name?type=:type`.
   - Saving an edit prepends `DROP {PROCEDURE|FUNCTION} IF EXISTS \`name\`;\n` to safely recreate the routine on MySQL/MariaDB.
   - Dropping a routine prompts a native confirmation challenge and executes `DROP {PROCEDURE|FUNCTION} IF EXISTS \`name\`;` through the DDL review modal, automatically refreshing the schema tree on completion.
4. **Automated Verification**:
   - Added `TestRoutineDesignerSuiteEmbedded` in `web/embed_test.go`, verifying modal markup, sidebar actions, parameter grid logic, and DDL assembly.
