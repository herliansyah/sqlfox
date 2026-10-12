import fs from 'fs';
import assert from 'assert';

const html = fs.readFileSync('web/dist/index.html', 'utf8');

console.log("=== Testing Connection Hover & Export Dropdown Fixes ===");

// 1. Grid toolbar overflow check
assert(!html.includes('id="grid-toolbar" class="h-8 bg-app-surface/40 border-b border-app-border px-3 flex items-center justify-between text-xs text-slate-400 flex-shrink-0 overflow-x-auto'),
  'FAIL: grid-toolbar must not have overflow-x-auto');
assert(html.includes('id="grid-toolbar" class="h-8 bg-app-surface/40 border-b border-app-border px-3 flex items-center justify-between text-xs text-slate-400 flex-shrink-0 z-20 gap-3"'),
  'FAIL: grid-toolbar must have z-20 and non-clipping overflow');
console.log("✅ Test 1 Passed: grid-toolbar does not clip dropdowns");

// 2. Export CSV dropdown element exists and is styled properly
assert(html.includes('id="export-csv-menu"'), 'FAIL: export-csv-menu must exist');
assert(html.includes('id="export-csv-dropdown-container"'), 'FAIL: export-csv-dropdown-container must exist');
console.log("✅ Test 2 Passed: export dropdown container and menu exist");

// 3. Connection tree hover buttons must not use hidden alongside group-hover
assert(!html.includes('class="hidden group-hover:inline-block hover:text-white px-1 text-xs" title="Edit Connection"'),
  'FAIL: connection tree edit button must not use hidden group-hover:inline-block');
assert(html.includes('class="opacity-0 group-hover:opacity-100 hover:text-white px-1 text-xs transition" title="Edit Connection"'),
  'FAIL: connection tree edit button must use opacity-0 group-hover:opacity-100 transition');
console.log("✅ Test 3 Passed: connection tree edit button uses opacity-0 group-hover:opacity-100");

// 4. Session switcher bar does not have edit button (prevents misclick with dropdown arrow)
assert(!html.includes('id="session-switcher-edit-btn"'), 'FAIL: session-switcher-edit-btn must be removed to avoid misclick with dropdown arrow');
console.log("✅ Test 4 Passed: session switcher bar does not have misclickable edit button");

// 5. Header active connection badge has edit button and click affordance
assert(html.includes("badgeEl.onclick = () => openEditConnModal(conn.id)"), 'FAIL: badgeEl must open edit connection modal on click');
assert(html.includes("title=\"Edit Connection: ${escapeHtml(conn.name)}\""), 'FAIL: badgeEl must have edit button on hover');
console.log("✅ Test 5 Passed: header connection badge has hover/click edit affordance");

console.log("\n🎉 ALL TESTS PASSED!");
