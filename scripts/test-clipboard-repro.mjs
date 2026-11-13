import fs from 'fs';
import assert from 'assert';

// Source-assertion pattern: instead of mocking a browser DOM, assert the source
// wiring that performs the copies in the Svelte port.
const virtualGridCode = fs.readFileSync('web/frontend/src/lib/components/grid/VirtualGrid.svelte', 'utf8');
const appStateCode = fs.readFileSync('web/frontend/src/lib/stores/appState.svelte.ts', 'utf8');

// TEST 1: TSV copy exists and serializes tab-separated values through navigator.clipboard
assert(
  /function copyRowAsTSV\(rIdx: number\)/.test(virtualGridCode),
  'VirtualGrid.svelte must define copyRowAsTSV(rIdx)'
);
assert(
  /function copyRowAsTSV[\s\S]{0,600}?navigator\.clipboard\.writeText\(`\$\{cols\}\\n\$\{vals\}`\)/.test(virtualGridCode),
  'copyRowAsTSV must write tab-separated header+values to navigator.clipboard'
);
assert(
  /function copyRowAsTSV[\s\S]{0,600}?\.join\('\\t'\)/.test(virtualGridCode),
  'copyRowAsTSV must join columns and values with tabs (TSV format)'
);
assert(
  /function copyRowAsTSV[\s\S]{0,700}?appState\.showToast\('Copied row as TSV', 'info'\)/.test(virtualGridCode),
  'copyRowAsTSV must surface success feedback (legacy "Copied TSV" alert equivalent)'
);
console.log('✅ Test 1 Passed: copyRowAsTSV writes TSV via navigator.clipboard with user feedback');

// TEST 2: Markdown copy exists and produces a GitHub-flavored markdown table
assert(
  /function copyRowAsMarkdown\(rIdx: number\)/.test(virtualGridCode),
  'VirtualGrid.svelte must define copyRowAsMarkdown(rIdx)'
);
assert(
  /function copyRowAsMarkdown[\s\S]{0,900}?const header = `\| \$\{colNames\.join\(' \| '\)\} \|`;/.test(virtualGridCode),
  'copyRowAsMarkdown must build a | separated header row'
);
assert(
  /function copyRowAsMarkdown[\s\S]{0,900}?const separator = `\| \$\{colNames\.map\(\(\) => '---'\)\.join\(' \| '\)\} \|`;/.test(virtualGridCode),
  'copyRowAsMarkdown must emit the markdown --- separator row'
);
assert(
  /function copyRowAsMarkdown[\s\S]{0,1200}?navigator\.clipboard\.writeText\(`\$\{header\}\\n\$\{separator\}\\n\$\{row\}`\)/.test(virtualGridCode),
  'copyRowAsMarkdown must write header/separator/row to navigator.clipboard'
);
assert(
  /function copyRowAsMarkdown[\s\S]{0,1300}?appState\.showToast\('Copied row as Markdown Table', 'info'\)/.test(virtualGridCode),
  'copyRowAsMarkdown must surface success feedback (legacy "Copied Markdown" alert equivalent)'
);
assert(
  /function copyRowAsMarkdown[\s\S]{0,1300}?v === null \? '\*NULL\*'/.test(virtualGridCode),
  'copyRowAsMarkdown must render NULL cells as *NULL*'
);
console.log('✅ Test 2 Passed: copyRowAsMarkdown writes markdown tables via navigator.clipboard');

// TEST 3: Copy affordances reachable from the grid context menu
assert(virtualGridCode.includes('<span>Copy Row as TSV</span>'), 'context menu must offer "Copy Row as TSV"');
assert(virtualGridCode.includes('<span>Copy Row as Markdown</span>'), 'context menu must offer "Copy Row as Markdown"');
assert(
  /copyRowAsTSV\(contextMenu!\.rIdx\)/.test(virtualGridCode),
  'context menu must invoke copyRowAsTSV on the active row'
);
assert(
  /copyRowAsMarkdown\(contextMenu!\.rIdx\)/.test(virtualGridCode),
  'context menu must invoke copyRowAsMarkdown on the active row'
);
console.log('✅ Test 3 Passed: copy actions are reachable from the grid context menu');

// TEST 4: Null/undefined cell values are serialized as empty strings in TSV
assert(
  /function copyRowAsTSV[\s\S]{0,600}?v === null \|\| v === undefined \? ''/.test(virtualGridCode),
  'copyRowAsTSV must map null/undefined cells to empty strings'
);
console.log('✅ Test 4 Passed: null-safe cell serialization');

// TEST 5: appState-level clipboard usage remains async-API based (no execCommand fallback needed)
assert(
  appStateCode.includes('navigator.clipboard.writeText'),
  'appState clipboard flows must use navigator.clipboard.writeText'
);
assert(
  !virtualGridCode.includes('document.execCommand'),
  'VirtualGrid must not rely on the deprecated document.execCommand copy fallback'
);
console.log('✅ Test 5 Passed: clipboard strategy is the modern async clipboard API');

console.log('ALL CLIPBOARD TESTS PASSED!');
