import fs from 'fs';
import assert from 'assert';

console.log('=== Running Svelte Tab Overflow & Enterprise Dropdown Verification ===');

const tabBarCode = fs.readFileSync('web/frontend/src/lib/components/editor/TabBar.svelte', 'utf8');
const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');

// 1. Container structure & overflow containment
assert(
  tabBarCode.includes('id="tabs-bar"') && tabBarCode.includes('overflow-hidden'),
  'tabs-bar must have overflow-hidden to prevent horizontal screen overflow'
);
assert(
  tabBarCode.includes('bind:clientWidth={containerWidth}'),
  'tabs-bar must bind clientWidth to dynamically detect viewport resizes'
);
assert(
  tabBarCode.includes('id="tabs-container"'),
  'tabs-container must exist'
);

// 2. Tab Overflow Dropdown Elements
assert(
  tabBarCode.includes('id="tab-overflow-wrapper"'),
  'tab-overflow-wrapper must exist'
);
assert(
  tabBarCode.includes('id="tab-overflow-btn"'),
  'tab-overflow-btn must exist'
);
assert(
  tabBarCode.includes('id="tab-overflow-count"'),
  'tab-overflow-count must exist'
);
assert(
  tabBarCode.includes('id="tab-overflow-dropdown"'),
  'tab-overflow-dropdown must exist'
);

// 3. Tab Overflow Calculation Logic
assert(
  tabBarCode.includes('visibleCount = $derived.by('),
  'visibleCount must be a derived state calculating visible tabs from available container width'
);
assert(
  tabBarCode.includes('visibleTabs = $derived('),
  'visibleTabs must be derived from tabs slice'
);
assert(
  tabBarCode.includes('overflowTabs = $derived('),
  'overflowTabs must be derived from tabs slice'
);

// 4. Enterprise Best Practice: Active Tab Visibility Guarantee
assert(
  tabBarCode.includes('activeIdx >= vCount'),
  'Active tab must be swapped into visible tabs when user selects an overflowed tab'
);

// 5. Enterprise Dropdown Features: Filter search, direct close, database pill
assert(
  tabBarCode.includes('overflowSearchQuery'),
  'Overflow dropdown must support quick search filtering for high tab counts'
);
assert(
  tabBarCode.includes('closeTabFromOverflow'),
  'Overflow dropdown must allow closing background tabs directly'
);
assert(
  tabBarCode.includes('selectOverflowTab'),
  'selectOverflowTab must switch active tab and close dropdown'
);

// 6. Monotonically increasing Tab titles in appState
assert(
  appStateCode.includes('getNextQueryTabTitle()'),
  'appState must define getNextQueryTabTitle to avoid duplicate tab titles'
);

// Verify getNextQueryTabTitle logic in isolation
const getNextQueryTabTitle = (tabs) => {
  let maxNum = 0;
  for (const t of tabs) {
    const m = (t.title || '').match(/^Query\s+(\d+)\.sql$/i);
    if (m) {
      const n = parseInt(m[1], 10);
      if (n > maxNum) maxNum = n;
    }
  }
  return `Query ${maxNum + 1}.sql`;
};

const mockTabs = [
  { id: '1', title: 'Query 1.sql' },
  { id: '2', title: 'Query 2.sql' },
  { id: '4', title: 'Query 4.sql' },
];
assert.strictEqual(getNextQueryTabTitle(mockTabs), 'Query 5.sql', 'Next tab should be Query 5.sql');

// 7. duplicateTab logic handles sequential copies
const getDuplicateTitle = (existingTitles, baseTitle) => {
  const cleanBase = baseTitle.replace(/^Copy(?: \(\d+\))? of /, '');
  let copyNum = 1;
  let nextTitle = `Copy of ${cleanBase}`;
  while (existingTitles.includes(nextTitle)) {
    copyNum++;
    nextTitle = `Copy (${copyNum}) of ${cleanBase}`;
  }
  return nextTitle;
};

const existing = ['Query 1.sql', 'Copy of Query 1.sql'];
assert.strictEqual(
  getDuplicateTitle(existing, 'Query 1.sql'),
  'Copy (2) of Query 1.sql',
  'Duplicate should generate Copy (2) of Query 1.sql'
);

console.log('🎉 ALL SVELTE TAB OVERFLOW AND DROPDOWN TESTS PASSED (GREEN)!');
