import fs from 'fs';
import assert from 'assert';

console.log('=== Testing Connection Hover & Export Dropdown Fixes (source assertions) ===');
// Source-assertion pattern: assert against the Svelte component sources.
const virtualGridCode = fs.readFileSync('web/frontend/src/lib/components/grid/VirtualGrid.svelte', 'utf8');
const headerCode = fs.readFileSync('web/frontend/src/lib/components/layout/Header.svelte', 'utf8');
const sidebarCode = fs.readFileSync('web/frontend/src/lib/components/layout/Sidebar.svelte', 'utf8');
const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');

// TEST 1 (legacy 1+2): Grid toolbar must not clip the export dropdown, and the
// export menu + dropdown container must exist in VirtualGrid.svelte
assert(
  !/id="grid-toolbar"[^>]*overflow-x-auto/.test(virtualGridCode),
  'FAIL: grid-toolbar must not have overflow-x-auto (dropdown clipping)'
);
assert(
  /id="grid-toolbar"[^>]*gap-2/.test(virtualGridCode),
  'FAIL: grid-toolbar must use gap-based layout with no clipping overflow'
);
assert(virtualGridCode.includes('id="export-csv-menu"'), 'FAIL: export-csv-menu must exist');
assert(virtualGridCode.includes('id="export-csv-dropdown-container"'), 'FAIL: export-csv-dropdown-container must exist');
assert(
  /id="export-csv-dropdown-container"[^>]*class="relative"/.test(virtualGridCode),
  'FAIL: export dropdown container must be position:relative so the menu anchors to it'
);
console.log('✅ Test 1 Passed: grid-toolbar does not clip dropdowns; export menu + container exist');

// TEST 2 (legacy 3): Connection tree hover edit button must use opacity-based
// group-hover reveal, never hidden + group-hover:inline-block
assert(
  !/class="hidden group-hover:inline-block[^"]*"[\s\S]{0,200}?title="Edit Connection"/.test(headerCode) &&
  !/class="hidden group-hover:inline-block[^"]*"[\s\S]{0,200}?title="Edit Connection"/.test(sidebarCode),
  'FAIL: connection tree edit button must not use hidden group-hover:inline-block'
);
assert(
  headerCode.includes('opacity-0 group-hover:opacity-100') && headerCode.includes('title="Edit Connection"'),
  'FAIL: header connection tree edit button must use opacity-0 group-hover:opacity-100 transition'
);
assert(
  /opacity-0 group-hover:opacity-100[\s\S]{0,300}?title="Edit Connection"/.test(headerCode),
  'FAIL: hover-reveal classes must be on the Edit Connection button'
);
console.log('✅ Test 2 Passed: connection tree edit button uses opacity-0 group-hover:opacity-100');

// TEST 3 (legacy 4): Header active-connection badge must open the edit modal on
// click with a hover edit affordance (no misclickable separate edit btn next to
// the session switcher)
assert(
  !headerCode.includes('session-switcher-edit-btn'),
  'FAIL: session-switcher-edit-btn must stay removed to avoid misclick with dropdown arrow'
);
assert(
  headerCode.includes('appState.openEditConnectionModal(conn)'),
  'FAIL: connection entries must open the edit-connection modal via appState.openEditConnectionModal(conn)'
);
assert(
  /appState\.openEditConnectionModal\(conn\)[\s\S]{0,300}?title="Edit Connection"/.test(headerCode),
  'FAIL: edit affordance must carry the "Edit Connection" title/hover tooltip'
);
assert(
  appStateCode.includes('openEditConnectionModal(conn: ConnectionProfile)'),
  'FAIL: appState must implement openEditConnectionModal(conn: ConnectionProfile)'
);
console.log('✅ Test 3 Passed: header connection rows expose openEditConnectionModal with hover edit affordance');

// TEST 4 (legacy 5 continued): the modal itself labels the editing state
const modalCode = fs.readFileSync('web/frontend/src/lib/components/modals/ConnectionModal.svelte', 'utf8');
assert(
  modalCode.includes('Edit Connection: ${name}'),
  'FAIL: ConnectionModal must show "Edit Connection: <name>" while editing'
);
console.log('✅ Test 4 Passed: edit-connection modal renders per-connection edit title');

console.log('🎉 ALL TESTS PASSED!');
