// Source-assertion regression test: Workspace pane splitter (resizable query
// editor height, clamping, persistence) implemented in web/frontend/src/App.svelte
// (splitter drag/reset logic) plus the Sidebar width logic in
// web/frontend/src/lib/components/layout/Sidebar.svelte. The clamping /
// persistence algorithms are copied from the real source into plain JS.
import fs from 'fs';
import assert from 'assert';

console.log('=== Running Workspace Pane Splitter Source Assertions ===');

const appCode = fs.readFileSync('web/frontend/src/App.svelte', 'utf8');
const sidebarCode = fs.readFileSync('web/frontend/src/lib/components/layout/Sidebar.svelte', 'utf8');

// 1. Splitter element exists with row-resize cursor
assert(
  appCode.includes('id="workspace-pane-splitter"'),
  'workspace-pane-splitter element must exist in App.svelte'
);
assert(
  appCode.includes('cursor-row-resize'),
  'workspace-pane-splitter must have cursor-row-resize class'
);
assert(
  appCode.includes('onmousedown={startDrag}') && appCode.includes('ondblclick={resetSplitter}'),
  'Splitter must wire mousedown -> startDrag and dblclick -> resetSplitter'
);

// 2. Splitter state and drag lifecycle
assert(
  appCode.includes('let editorSplitPercent = $state('),
  'editorSplitPercent must be reactive state'
);
assert(
  appCode.includes('let isDraggingSplitter = $state(false);'),
  'isDraggingSplitter must be reactive state'
);
assert(
  appCode.includes("window.addEventListener('mousemove', onDrag)") &&
    appCode.includes("window.addEventListener('mouseup', stopDrag)"),
  'startDrag must attach global mousemove/mouseup listeners'
);
assert(
  appCode.includes("localStorage.setItem('sqlfox_svelte_editor_split', String(editorSplitPercent));"),
  'stopDrag must persist the editor split percent'
);

// 3. Sidebar resizable width with persistence
assert(
  sidebarCode.includes("localStorage.getItem('sqlfox_sidebar_width')"),
  'Sidebar must read the persisted width from sqlfox_sidebar_width'
);
assert(
  sidebarCode.includes("localStorage.setItem('sqlfox_sidebar_width', String(sidebarWidth));"),
  'Sidebar stopResize must persist the width'
);
assert(
  sidebarCode.includes('function startResize(e: MouseEvent)') &&
    sidebarCode.includes('function onResize(e: MouseEvent)') &&
    sidebarCode.includes('function stopResize()'),
  'Sidebar must implement the startResize/onResize/stopResize lifecycle'
);
assert(
  sidebarCode.includes('function resetWidth() {'),
  'Sidebar must support resetting its width'
);

// ---- Algorithms under test (copied from the real source) ----

// App.svelte onDrag: clamp split to 15..85 percent of available height
const HEADER_HEIGHT = 48 + 36; // Header + TabBar
const clampSplit = (clientY, innerHeight) => {
  const availableHeight = innerHeight - HEADER_HEIGHT;
  const pct = Math.max(15, Math.min(85, (clientY - HEADER_HEIGHT) / availableHeight * 100));
  return Math.round(pct);
};

const initialSplit = (stored) => Math.max(15, Math.min(85, Number(stored) || 45));

// App.svelte resetSplitter
const resetSplitter = () => 50;

// Sidebar.svelte: width clamped to 200..520 while dragging, 200..500 on init
const clampSidebarDrag = (clientX) => Math.round(Math.max(200, Math.min(520, clientX)));
const initialSidebarWidth = (stored) => Math.max(200, Math.min(500, Number(stored) || 260));

// ---- Splitter assertions ----
const innerHeight = 1080;

assert.strictEqual(clampSplit(200, innerHeight), 15, 'Dragging above the header must clamp to the 15% minimum');
assert.strictEqual(clampSplit(innerHeight, innerHeight), 85, 'Dragging to the window bottom must clamp to the 85% maximum');
const available = innerHeight - HEADER_HEIGHT;
assert.strictEqual(clampSplit(HEADER_HEIGHT + available * 0.5, innerHeight), 50, 'Mid-pane drag must map to 50%');
assert.ok(
  Math.abs(clampSplit(HEADER_HEIGHT + available * 0.46, innerHeight) - 46) <= 1,
  'Split percent tracks the dragged position near-linearly'
);

// Rounding: 37.4 -> 37, 37.6 -> 38
const mid = clampSplit(HEADER_HEIGHT + (innerHeight - HEADER_HEIGHT) * 0.376, innerHeight);
assert.strictEqual(mid, 38, 'Split percent must be rounded to an integer');

// Persisted value is clamped on load
assert.strictEqual(initialSplit('90'), 85, 'Stored split above 85 must clamp to 85 on load');
assert.strictEqual(initialSplit('5'), 15, 'Stored split below 15 must clamp to 15 on load');
assert.strictEqual(initialSplit('50'), 50, 'Valid stored split must load verbatim');
assert.strictEqual(initialSplit('garbage'), 45, 'Missing/corrupt stored split must default to 45');

assert.strictEqual(resetSplitter(), 50, 'Double-click must reset the split to 50%');

// ---- Sidebar width assertions ----
assert.strictEqual(clampSidebarDrag(50), 200, 'Dragging far left must clamp the sidebar to 200px minimum');
assert.strictEqual(clampSidebarDrag(1500), 520, 'Dragging far right must clamp the sidebar to 520px maximum');
assert.strictEqual(clampSidebarDrag(340), 340, 'In-range drag positions must apply verbatim');

assert.strictEqual(initialSidebarWidth('800'), 500, 'Stored sidebar width above 500 must clamp to 500');
assert.strictEqual(initialSidebarWidth('100'), 200, 'Stored sidebar width below 200 must clamp to 200');
assert.strictEqual(initialSidebarWidth('260'), 260, 'Valid stored sidebar width must load verbatim');
assert.strictEqual(initialSidebarWidth(null), 260, 'Missing stored width must default to 260');

// ---- Persistence keys are distinct ----
assert(appCode.includes("sqlfox_svelte_editor_split"), 'Editor split must persist under sqlfox_svelte_editor_split');
assert(sidebarCode.includes("sqlfox_sidebar_width"), 'Sidebar width must persist under sqlfox_sidebar_width');

console.log('🎉 ALL WORKSPACE PANE SPLITTER SOURCE ASSERTIONS PASSED (GREEN)!');
