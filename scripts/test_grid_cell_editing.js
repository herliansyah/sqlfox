const fs = require('fs');
const assert = require('assert');

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// Phase 1 Feedback Loop: Test Grid Cell Editing Best Practices & Apply Mode

// 1. Check if active editing cell is tracked and closed before opening a new cell
const hasCloseOrCommitPrevEdit = html.includes('activeEditCell') || html.includes('finishInlineEdit') || html.includes('commitInlineEdit');
assert(hasCloseOrCommitPrevEdit, 'BUG: activeEditCell or finishInlineEdit is missing; multiple cells can remain open simultaneously!');

// 2. Check if blur event or click outside closes/saves the editing cell
const hasBlurHandler = html.includes("input.addEventListener('blur'") || html.includes('handleCellBlur');
assert(hasBlurHandler, 'BUG: input blur handler is missing; clicking outside cell does not close/save!');

// 3. Check for Grid Edit Mode toggle / Apply / Discard buttons
const hasApplyBtn = html.includes('btn-grid-apply') || html.includes('applyGridChanges');
const hasDiscardBtn = html.includes('btn-grid-discard') || html.includes('discardGridChanges');
const hasEditModeToggle = html.includes('grid-edit-mode') || html.includes('toggleGridEditMode') || html.includes('gridAutoCommit');

assert(hasApplyBtn, 'BUG: Missing Apply Changes button for staged edits in data grid toolbar!');
assert(hasDiscardBtn, 'BUG: Missing Discard Changes button for staged edits in data grid toolbar!');
assert(hasEditModeToggle, 'BUG: Missing Grid Edit Mode setting / toggle (Immediate Persist vs Staged Apply)!');

// 4. Check backend Go handler supports batch edits
const goCode = fs.readFileSync('internal/server/handlers_schema.go', 'utf8');
assert(goCode.includes('Edits') && goCode.includes('totalAffected'), 'BUG: Go backend handleCellEdit does not support batch edits!');

console.log('PASS: Grid inline editing best practices and Staged Apply features verified!');
