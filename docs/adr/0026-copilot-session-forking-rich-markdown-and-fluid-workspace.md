# 0026: Copilot Session Forking, Rich Markdown Blocks, and Fluid Workspace Ergonomics

To elevate AI Copilot from a passive text chatbot into an interactive execution assistant and ensure fluid ergonomics across modern responsive screen sizes, SqlFox implements Chat Session Forking, rich Markdown formatting with interactive SQL block toolbars, Notes live preview, and fluid workspace layout constraints.

## Context & Problem

1. **Passive & Flat AI Chat UX**:
   - Copilot answers were displayed as flat or unformatted text blocks. Complex multi-table schema explanations, step-by-step query optimization breakdowns, and SQL snippets were difficult to parse visually.
   - Using AI-generated SQL required manual selection, copying, switching tabs, pasting into the editor, and triggering execution manually.
2. **Linear Chat Bottleneck**:
   - AI conversations were strictly linear. When a developer received an interesting suggestion and wanted to explore an alternative hypothesis or different index strategy, they had to either overwrite their prompt sequence or create an empty chat session from scratch, losing preceding schema context.
3. **Internal Notes Markdown Blindspot**:
   - The persistent `Notes.md` tab was restricted to a raw `<textarea>`, offering no rendered preview of formatted markdown notes, query plans, or saved incident logs.
4. **Layout Collapse During Sidebar Resizing**:
   - Dragging the left sidebar or the AI copilot drawer on smaller viewports or laptops could compress the center query editor and virtual data grid below usable dimensions without a safety floor.
   - Diagnostic and telemetry cards across Cluster Inspector and Flight Recorder used rigid column layouts that clipped or broke on narrow viewports.
5. **Residual Stateless Wrappers**:
   - `safety.BackupManager` and `ai.Firewall` remained as single-method wrapper structs instantiated with boilerplate constructors despite carrying no long-lived state.

## Decision

1. **Rich Markdown Formatting & Interactive SQL Blocks**:
   - Implement zero-dependency client-side Markdown rendering supporting headings, ordered/unordered lists, blockquotes, bold/italic formatting, inline code, and fenced code blocks.
   - Attach interactive action toolbars to all rendered SQL code blocks:
     - `📋 Copy`: Copies the SQL snippet to the system clipboard.
     - `▶ Insert & Run`: Injects the SQL statement into the active query tab and immediately dispatches execution.
     - `🔄 Replace`: Replaces the current query editor content with the SQL block.
2. **Chat Session Forking (`ForkSession`)**:
   - Add a "Fork" action (`🔀 Branch`) to Copilot message blocks.
   - Creates a new, isolated conversation session copying all message history up to the selected assistant message.
   - Enables developers to explore alternate optimization branches without polluting or losing the original thread.
3. **Internal Notes Live Preview Mode**:
   - Introduce an `✏️ Edit` / `👁️ Preview` segmented toggle button in the `Notes.md` tab header.
   - Renders formatted Markdown live while allowing instant switching back to editing mode.
4. **Fluid Workspace Ergonomics & Auto-Fit Grids**:
   - Enforce a minimum width floor (400px) on the center workspace during sidebar drag-resizing, preventing viewport collapse.
   - Refactor metric cards and telemetry dashboards across Cluster Inspector, Flight Recorder, and Storage Diagnostics to responsive CSS Grid `repeat(auto-fit, minmax(...))` layouts.
5. **Ponytail Stateless Simplification**:
   - Prune `type BackupManager struct` and `NewBackupManager` in `internal/safety/backup.go` into pure function `safety.BackupTable(ctx, sess, backupDir, tableName)`.
   - Prune `type Firewall struct` and `NewFirewall` in `internal/ai/firewall.go` into pure function `ai.BuildSchemaPrompt(schema, query, maskIdentifiers)`.
   - Remove redundant `package-lock.json`.

## Consequences

- **Execution Flow**: Developers can review AI query recommendations, test them with a single click (`▶ Insert & Run`), or branch conversation threads instantly.
- **Ergonomics**: Full responsiveness across 13" laptop screens, widescreen monitors, and tablet displays without clipping or unusable column compression.
- **Code Cleanliness**: Eliminated unnecessary struct wrappers and boilerplate instantiations across server handlers and background workers.
