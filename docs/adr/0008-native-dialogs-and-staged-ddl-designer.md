# 0008: Native HTML5 Dialogs and Staged DDL Table Designer

To replace primitive blocking browser dialogs (`window.alert`, `window.confirm`) and enable full GUI database structure management (similar to HeidiSQL), SqlFox adopts the standard HTML5 `<dialog>` API and a draft-staged DDL modification pipeline.

## Considered Options

- **JavaScript Alerts and Confirms**: Thread-blocking, un-stylable, inconsistent across browser engines, and disallowed for structured security checks.
- **Heavy Frontend Component Libraries (e.g. Radix, headless UI)**: Requires Node.js build steps, Webpack/Vite bundlers, and increases asset weight, conflicting with the single-file embedded SPA model.
- **HTML5 `<dialog>` Element (`showModal()`)**: Native platform feature (zero external dependency), provides browser-level focus trapping, ESC key dismissal, and native backdrop styling.
- **Immediate DDL Execution**: Executing each column or key change instantly to the database risks partial schema alterations and unnecessary table locks.
- **Staged Draft Changes with DDL Review**: Changes to columns, primary keys, indexes, and foreign keys accumulate in memory. Upon saving, a unified `ALTER TABLE` statement is computed and presented in a `DDL Review Modal` before passing through the `Safety Guard` and `Smart Tiered Backup`.

## Consequences

- All notifications, confirmations, and safety inspections use standardized native `<dialog>` modals with consistent dark theme visual hierarchy.
- Database tables, views, and triggers can be created, altered, and dropped visually through a HeidiSQL-inspired interface.
- DDL modifications remain deterministic, auditable, and safe against accidental schema destruction.
