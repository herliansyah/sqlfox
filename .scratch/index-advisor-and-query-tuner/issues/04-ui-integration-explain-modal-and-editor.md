# Issue: UI Surface Integration in Visual EXPLAIN Modal & Query Editor

Status: resolved
Type: grilling
Blocked by: 02, 03

## Question
How should the index advisor interface seamlessly with the Query Editor, Execution Plan (EXPLAIN) modal, and Table Viewer without cluttering the UI?

Specifically:
1. In the Visual EXPLAIN modal, how should the new `💡 Index Advisor` tab display the recommended DDL, estimated benefit (e.g. converting `Seq Scan / ALL` to `ref / range`), and the Index Bloat Warning banner?
2. What one-click action buttons should be available (e.g. `📋 Copy Safe DDL`, `⚡ Run in Editor`, `🔄 Compare with Existing Indexes`)?
3. In the Query Editor toolbar, should an ambient indicator badge or subtle hint appear when an executed query triggers a full table scan or takes > 500ms?

## Answer
1. **Visual EXPLAIN Modal Integration**: Added a dedicated `💡 Advisor` tab in `#impact-preview-modal` alongside `Plan Grid`, `Visual Tree`, and `Raw JSON`. The tab displays a dynamic count badge (`💡 Advisor (N)`).
2. **Action Controls**: Each recommendation card features a 📋 `Copy` button (copies dialect-safe non-blocking DDL to clipboard with toast) and ⚡ `Editor` button (`applyAdvisorDDLToEditor` pastes the DDL directly into CodeMirror and closes modal).
3. **Scan Warning Link**: When a full table scan (`ALL`) is detected, the summary badge actively surfaces a clickable link: *"Check the Index Advisor tab for optimization"*.
4. **Bloat Banner**: Rendered in `#impact-advisor-bloat-container` with an amber border, warning about write degradation while keeping candidate cards visible below it.
