const fs = require('fs');
const assert = require('assert');

console.log('Running feedback loop for Snippet hang & Table Viewer empty tabs bugs...');

const html = fs.readFileSync('web/dist/index.html', 'utf8');

let failures = [];

// 1. Check for unclosed dialog tag before snippets-manager-dialog
const dbCreatorIdx = html.indexOf('id="database-creator-dialog"');
const snippetsMgrIdx = html.indexOf('id="snippets-manager-dialog"');

if (dbCreatorIdx !== -1 && snippetsMgrIdx !== -1) {
  const between = html.slice(dbCreatorIdx, snippetsMgrIdx);
  if (!between.includes('</dialog>')) {
    failures.push('Bug 1: <dialog id="database-creator-dialog"> is not closed before <dialog id="snippets-manager-dialog"> (causes showModal DOMException / app hang)');
  }
}

// 2. Check for undefined variable currentDatabase
if (html.includes('currentDatabase')) {
  // Check if currentDatabase is declared with let/const/var
  const isDeclared = /let\s+currentDatabase\b|const\s+currentDatabase\b|var\s+currentDatabase\b/.test(html);
  if (!isDeclared) {
    failures.push('Bug 2: `currentDatabase` is referenced in loadAndRenderTableNote but never declared (ReferenceError on Notes tab)');
  }
}

// 3. Check if editor-container is hidden during isTableViewer mode
const renderTabsMatch = html.match(/function renderTabs\(\) \{([\s\S]*?)\n    \}/);
if (renderTabsMatch) {
  const code = renderTabsMatch[1];
  if (!code.includes('editor-container') || !code.includes('classList.add(\'hidden\')') && !code.includes('classList.toggle')) {
    failures.push('Bug 3: renderTabs() does not toggle editor-container visibility for isTableViewer tabs');
  }
}

// 4. Check if loadTableRows passes ?db= and checks res.ok
const loadTableRowsMatch = html.match(/async function loadTableRows\([^)]*\)\s*\{([\s\S]*?)\n    \}/);
if (loadTableRowsMatch) {
  const code = loadTableRowsMatch[1];
  if (!code.includes('res.ok') && !code.includes('!res.ok')) {
    failures.push('Bug 4: loadTableRows() does not check res.ok, silently failing with blank grid on HTTP error');
  }
  if (!code.includes('db=')) {
    failures.push('Bug 5: loadTableRows() does not pass target db in query params');
  }
}

// 5. Check if renderTableDDL passes ?db= and checks res.ok
const renderTableDDLMatch = html.match(/async function renderTableDDL\([^)]*\)\s*\{([\s\S]*?)\n    \}/);
if (renderTableDDLMatch) {
  const code = renderTableDDLMatch[1];
  if (!code.includes('res.ok') && !code.includes('!res.ok')) {
    failures.push('Bug 6: renderTableDDL() does not check res.ok, silently failing with empty DDL on HTTP error');
  }
  if (!code.includes('db=')) {
    failures.push('Bug 7: renderTableDDL() does not pass target db in query params');
  }
}

console.log('--- TEST RESULTS ---');
if (failures.length > 0) {
  console.log(`RED: Found ${failures.length} issues:`);
  failures.forEach(f => console.log('  ❌ ' + f));
  process.exit(1);
} else {
  console.log('GREEN: All snippet and table tabs checks passed!');
  process.exit(0);
}
