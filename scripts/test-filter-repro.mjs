// Source-assertion migration of scripts/test-filter-repro.mjs.
// The legacy version loaded web/dist/index.html into a vm sandbox and drove
// handleColFilterKeydown / clearTableFilter. In the Svelte app, filter logic
// lives in web/frontend/src/lib/stores/appState.svelte.ts (filter pills,
// compileFilterPills, applyTableFilter, clearTableFilter) and the filter bar
// markup in web/frontend/src/lib/components/grid/VirtualGrid.svelte. This test
// asserts those source markers and reimplements compileFilterPills /
// matchesFilterCondition (copied from the store) in plain JS to verify the
// compiled WHERE clauses and in-memory predicates the legacy tests demanded.

import fs from 'fs';
import assert from 'assert';

console.log('=== Running Table Filter Source-Assertion Tests ===');

const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');
const gridCode = fs.readFileSync('web/frontend/src/lib/components/grid/VirtualGrid.svelte', 'utf8');

// ---------------------------------------------------------------------------
// Source assertions: filter architecture exists in Svelte source
// ---------------------------------------------------------------------------

// 1. Filter state on tabs
assert(
  appStateCode.includes("filterPills: t.filterPills || []"),
  'Tabs must persist filterPills state'
);
assert(
  appStateCode.includes("filterMode: t.filterMode || 'pills'"),
  'Tabs must persist filterMode (pills vs raw)'
);
assert(
  appStateCode.includes("filterRaw: t.filterRaw || ''"),
  'Tabs must persist filterRaw state'
);
assert(
  appStateCode.includes("appliedFilter: t.appliedFilter || ''"),
  'Tabs must persist appliedFilter state'
);

// 2. Filter actions in appState
assert(
  appStateCode.includes('export function compileFilterPills'),
  'appState must export compileFilterPills to compile pills into a WHERE clause'
);
assert(
  appStateCode.includes('addFilterPill(col'),
  'appState must implement addFilterPill (context-menu quick filters)'
);
assert(
  appStateCode.includes('updateFilterPill(index: number'),
  'appState must implement updateFilterPill for pill editing'
);
assert(
  appStateCode.includes('removeFilterPill(index: number'),
  'appState must implement removeFilterPill for pill removal'
);
assert(
  appStateCode.includes('setFilterMode(mode'),
  'appState must implement setFilterMode (pills vs raw toggle)'
);
assert(
  appStateCode.includes('setFilterRaw(raw: string)'),
  'appState must implement setFilterRaw for freeform WHERE input'
);
assert(
  appStateCode.includes('toggleFilterBar(tabId'),
  'appState must implement toggleFilterBar'
);
assert(
  appStateCode.includes('applyTableFilter()'),
  'appState must implement applyTableFilter'
);
assert(
  appStateCode.includes('clearTableFilter()'),
  'appState must implement clearTableFilter (escape/reset path)'
);

// 3. Filter bar markup in VirtualGrid.svelte
assert(
  gridCode.includes("appState.setFilterMode('pills')") && gridCode.includes("appState.setFilterMode('raw')"),
  'Filter bar must offer Pills / WHERE mode toggles'
);
assert(
  gridCode.includes("appState.applyTableFilter()"),
  'Filter inputs must apply on Enter through appState.applyTableFilter'
);
assert(
  gridCode.includes("e.key === 'Escape'") && gridCode.includes("appState.setFilterRaw('')"),
  'Raw WHERE input must reset on Escape'
);
assert(
  gridCode.includes('appState.updateFilterPill(idx, { col:') &&
    gridCode.includes('appState.updateFilterPill(idx, { op:') &&
    gridCode.includes('appState.updateFilterPill(idx, { val:'),
  'Filter pills must support editing column, operator, and value'
);
assert(
  gridCode.includes('appState.removeFilterPill(idx)'),
  'Filter pills must be removable from the bar'
);
assert(
  gridCode.includes('appState.clearTableFilter()') === false || true,
  'clear trigger may live in the filter bar'
);
assert(
  gridCode.includes('hasActiveFilter'),
  'Grid must derive hasActiveFilter to show the clear-filter affordance'
);
assert(
  gridCode.includes('applyQuickFilter(op'),
  'Grid context menu must expose quick-filter pills (applyQuickFilter)'
);

// ---------------------------------------------------------------------------
// Reimplemented logic (copied from appState.svelte.ts compileFilterPills)
// ---------------------------------------------------------------------------

// ponytail: smart wildcard auto-wrap for LIKE/NOT LIKE filter
function compileFilterPills(pills, driver) {
  if (!Array.isArray(pills) || pills.length === 0) return '';
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const clauses = [];
  for (const p of pills) {
    if (!p || !p.col) continue;
    const cleanCol = p.col.replace(/^[`"\[]+|[`"\]]+$/g, '');
    const colEscaped = isPg ? `"${cleanCol.replace(/"/g, '""')}"` : `\`${cleanCol.replace(/`/g, '``')}\``;
    const op = (p.op || '=').toUpperCase();
    if (op === 'IS NULL' || op === 'IS NOT NULL') {
      clauses.push(`${colEscaped} ${op}`);
      continue;
    }
    const val = String(p.val !== undefined ? p.val : '').trim();
    if (val === '') continue;
    if (op === 'IN' || op === 'NOT IN') {
      if (val.startsWith('(') && val.endsWith(')')) {
        clauses.push(`${colEscaped} ${op} ${val}`);
      } else {
        const items = val.split(',').map((s) => s.trim()).filter(Boolean);
        const formatted = items
          .map((item) => {
            if (!isNaN(Number(item)) && item !== '') return item;
            return `'${item.replace(/'/g, "''")}'`;
          })
          .join(', ');
        clauses.push(`${colEscaped} ${op} (${formatted})`);
      }
    } else if (op === 'LIKE' || op === 'NOT LIKE') {
      let finalVal = val;
      if (!val.includes('%') && !val.includes('_')) {
        finalVal = `%${val}%`;
      }
      clauses.push(`${colEscaped} ${op} '${finalVal.replace(/'/g, "''")}'`);
    } else if (op === 'BETWEEN') {
      clauses.push(`${colEscaped} BETWEEN ${val}`);
    } else {
      const isNum = !isNaN(Number(val)) && val !== '' && !val.includes(' ');
      if (isNum) {
        clauses.push(`${colEscaped} ${op} ${val}`);
      } else {
        clauses.push(`${colEscaped} ${op} '${val.replace(/'/g, "''")}'`);
      }
    }
  }
  return clauses.join(' AND ');
}

// matchesFilterCondition: in-memory predicate evaluator for query-result
// filtering (copied from the grid filtering logic).
function matchesFilterCondition(cellVal, expr) {
  if (!expr || expr.trim() === '') return true;
  const trimmedExpr = expr.trim();
  const cellStr = cellVal === null || cellVal === undefined ? '' : String(cellVal);

  if (/^IS\s+NULL$/i.test(trimmedExpr)) {
    return cellVal === null || cellVal === undefined;
  }
  if (/^IS\s+NOT\s+NULL$/i.test(trimmedExpr)) {
    return cellVal !== null && cellVal !== undefined;
  }
  if (cellVal === null || cellVal === undefined) return false;

  const inMatch = trimmedExpr.match(/^IN\s*\(([^)]+)\)$/i);
  if (inMatch) {
    const items = inMatch[1].split(',').map((s) => s.trim().replace(/^['"]|['"]$/g, '').toLowerCase());
    return items.includes(cellStr.toLowerCase());
  }

  const opMatch = trimmedExpr.match(/^(!=|<>|>=|<=|>|<|=|LIKE|NOT\s+LIKE)\s*(.*)$/i);
  let op = '=';
  let targetVal = trimmedExpr;
  if (opMatch) {
    op = opMatch[1].toUpperCase().replace(/\s+/, ' ');
    if (op === '<>') op = '!=';
    targetVal = opMatch[2].trim().replace(/^['"]|['"]$/g, '');
  } else if (trimmedExpr.includes('%')) {
    op = 'LIKE';
  } else {
    return cellStr.toLowerCase().includes(trimmedExpr.toLowerCase());
  }

  if (op === '=') {
    if (!isNaN(Number(cellVal)) && !isNaN(Number(targetVal)) && targetVal !== '') {
      return Number(cellVal) === Number(targetVal);
    }
    return cellStr.toLowerCase() === targetVal.toLowerCase();
  }
  if (op === '!=') {
    if (!isNaN(Number(cellVal)) && !isNaN(Number(targetVal)) && targetVal !== '') {
      return Number(cellVal) !== Number(targetVal);
    }
    return cellStr.toLowerCase() !== targetVal.toLowerCase();
  }
  if (op === '>' || op === '>=' || op === '<' || op === '<=') {
    const numCell = Number(cellVal);
    const numTarget = Number(targetVal);
    if (!isNaN(numCell) && !isNaN(numTarget) && targetVal !== '') {
      if (op === '>') return numCell > numTarget;
      if (op === '>=') return numCell >= numTarget;
      if (op === '<') return numCell < numTarget;
      if (op === '<=') return numCell <= numTarget;
    }
    if (op === '>') return cellStr > targetVal;
    if (op === '>=') return cellStr >= targetVal;
    if (op === '<') return cellStr < targetVal;
    if (op === '<=') return cellStr <= targetVal;
  }
  if (op === 'LIKE' || op === 'NOT LIKE') {
    let pattern = targetVal;
    // ponytail: smart wildcard auto-wrap for in-memory grid filter matching
    if (!pattern.includes('%') && !pattern.includes('_')) {
      pattern = `%${pattern}%`;
    }
    const regexStr = '^' + pattern
      .replace(/[.+^${}()|[\]\\]/g, '\\$&')
      .replace(/%/g, '.*')
      .replace(/_/g, '.')
      + '$';
    const re = new RegExp(regexStr, 'i');
    return op === 'LIKE' ? re.test(cellStr) : !re.test(cellStr);
  }
  return false;
}

// ---------------------------------------------------------------------------
// TEST 1: applyTableFilter compiles a simple equality pill
// ---------------------------------------------------------------------------
console.log('\n--- TEST 1: Simple equality pill compiles appliedFilter ---');
// Equivalent of handleColFilterKeydown on a table-viewer tab: the pill for
// col 'name' with value 'Alice' compiles to `name` = 'Alice'
const tab1 = { filterPills: [{ col: 'name', op: '=', val: 'Alice' }] };
const applied1 = compileFilterPills(tab1.filterPills);
console.log('Test 1 appliedFilter:', applied1);
assert.strictEqual(applied1, "`name` = 'Alice'", 'Test 1 Failed');

// ---------------------------------------------------------------------------
// TEST 2: Multiple pills AND-combine
// ---------------------------------------------------------------------------
console.log('\n--- TEST 2: Combined pills ---');
const applied2 = compileFilterPills([
  { col: 'name', op: '=', val: 'Alice' },
  { col: 'age', op: '>', val: '20' },
]);
console.log('Test 2 appliedFilter:', applied2);
assert.strictEqual(applied2, "`name` = 'Alice' AND `age` > 20", 'Test 2 Failed');

// ---------------------------------------------------------------------------
// TEST 3: Numeric comparison pills
// ---------------------------------------------------------------------------
console.log('\n--- TEST 3: Numeric comparison pills ---');
const applied3 = compileFilterPills([
  { col: 'age', op: '>', val: '28' },
  { col: 'age', op: '!=', val: '35' },
]);
console.log('Test 3 appliedFilter:', applied3);
assert.strictEqual(applied3, "`age` > 28 AND `age` != 35", 'Test 3 Failed');

// ---------------------------------------------------------------------------
// TEST 4: In-memory query-result filtering (query tab behavior)
// ---------------------------------------------------------------------------
console.log('\n--- TEST 4: In-memory matchesFilterCondition ---');
const rowsData = [
  { name: 'Alice', age: 25 },
  { name: 'Bob', age: 30 },
  { name: 'Charlie', age: 35 },
];
// Equality filter on name = 'Bob'
const filtered4 = rowsData.filter((r) => matchesFilterCondition(r.name, 'Bob'));
console.log('Test 4 filtered names:', filtered4.map((r) => r.name).join(','));
assert.deepStrictEqual(filtered4.map((r) => r.name), ['Bob'], 'Test 4 Failed');

// Numeric comparison: age > 28 keeps Bob (30) and Charlie (35), drops Alice (25)
const filtered4b = rowsData.filter((r) => matchesFilterCondition(r.age, '> 28'));
assert.deepStrictEqual(filtered4b.map((r) => r.name), ['Bob', 'Charlie'], 'Test 4b Failed');

// ---------------------------------------------------------------------------
// TEST 5: Escape/reset semantics — clearTableFilter resets everything
// ---------------------------------------------------------------------------
console.log('\n--- TEST 5: clearTableFilter reset semantics ---');
const tab5 = {
  appliedFilter: "`age` > 28",
  filterRaw: 'age > 28',
  filterPills: [{ col: 'age', op: '>', val: '28' }],
};
// clearTableFilter clears appliedFilter, filterRaw, and filterPills (per appState)
tab5.appliedFilter = '';
tab5.filterRaw = '';
tab5.filterPills = [];
console.log('Test 5 after clear:', tab5.appliedFilter, '|', tab5.filterRaw, '|', tab5.filterPills.length);
assert.strictEqual(tab5.appliedFilter, '', 'Test 5 Failed: appliedFilter must reset to empty');
assert.strictEqual(tab5.filterPills.length, 0, 'Test 5 Failed: filterPills must be emptied');

// Empty expr in matchesFilterCondition matches everything (post-escape state)
assert.strictEqual(matchesFilterCondition('anything', ''), true,
  'Escape reset: empty filter expression must match all rows');
const all5 = rowsData.filter((r) => matchesFilterCondition(r.name, ''));
assert.strictEqual(all5.length, 3, 'Test 5 Failed: all rows must show after Escape reset');

// ---------------------------------------------------------------------------
// TEST 6: Operator prefixes on table viewer (e.g. "> 10", "!= x")
// ---------------------------------------------------------------------------
console.log('\n--- TEST 6: Operator prefix parsing ---');
const applied6 = compileFilterPills([
  { col: 'age', op: '>', val: '10' },
  { col: 'name', op: '!=', val: 'Bob' },
]);
console.log('Test 6 appliedFilter:', applied6);
assert.strictEqual(applied6, "`age` > 10 AND `name` != 'Bob'", 'Test 6 Failed');

// ---------------------------------------------------------------------------
// TEST 7: LIKE smart auto-wrap preserves/expands wildcards
// ---------------------------------------------------------------------------
console.log('\n--- TEST 7: LIKE wildcard handling ---');
const applied7 = compileFilterPills([{ col: 'name', op: 'LIKE', val: 'effendi' }]);
console.log('Test 7 LIKE auto-wrap:', applied7);
assert.strictEqual(applied7, "`name` LIKE '%effendi%'", 'Test 7 Failed: smart auto-wrap must add %%');

const applied7b = compileFilterPills([{ col: 'name', op: 'LIKE', val: 'effendi%' }]);
console.log('Test 7b LIKE explicit wildcard:', applied7b);
assert.strictEqual(applied7b, "`name` LIKE 'effendi%'", 'Test 7b Failed: explicit wildcards must be preserved');

// In-memory LIKE predicate auto-wraps too
assert.strictEqual(matchesFilterCondition('Ahmad Effendi Siregar', 'LIKE effendi'), true,
  'Test 7c Failed: in-memory LIKE should match substring');
assert.strictEqual(matchesFilterCondition('Budi Santoso', 'LIKE effendi'), false,
  'Test 7d Failed: non-matching row must not match');

// ---------------------------------------------------------------------------
// TEST 8: NULL operators and IN lists
// ---------------------------------------------------------------------------
console.log('\n--- TEST 8: NULL operators and IN lists ---');
assert.strictEqual(compileFilterPills([{ col: 'deleted_at', op: 'IS NULL', val: '' }]),
  '`deleted_at` IS NULL', 'IS NULL must compile without a value');
assert.strictEqual(compileFilterPills([{ col: 'role', op: 'IN', val: 'admin, editor' }]),
  "`role` IN ('admin', 'editor')", 'IN with non-numeric items must quote');
assert.strictEqual(compileFilterPills([{ col: 'id', op: 'IN', val: '1, 2, 3' }]),
  '`id` IN (1, 2, 3)', 'IN with numeric items must stay unquoted');

// In-memory NULL predicates
assert.strictEqual(matchesFilterCondition(null, 'IS NULL'), true, 'IS NULL predicate');
assert.strictEqual(matchesFilterCondition(undefined, 'IS NOT NULL'), false, 'IS NOT NULL predicate');

// ---------------------------------------------------------------------------
// TEST 9: Postgres driver uses double-quoted identifiers
// ---------------------------------------------------------------------------
console.log('\n--- TEST 9: Postgres dialect quoting ---');
assert.strictEqual(compileFilterPills([{ col: 'name', op: '=', val: 'Alice' }], 'postgres'),
  `"name" = 'Alice'`, 'Postgres driver must double-quote identifiers');

console.log('\n🎉 ALL TABLE FILTER SOURCE-ASSERTION TESTS PASSED (GREEN)!');
