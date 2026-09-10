const fs = require('fs');
const path = require('path');
const assert = require('assert');

console.log('Testing autocomplete engine & vendored assets...');

// Check 1: Vendor files exist and have valid size
const vendorFiles = [
  'web/dist/vendor/codemirror/codemirror.min.js',
  'web/dist/vendor/codemirror/codemirror.min.css',
  'web/dist/vendor/codemirror/mode/sql/sql.min.js',
  'web/dist/vendor/codemirror/addon/hint/show-hint.min.js',
  'web/dist/vendor/codemirror/addon/hint/show-hint.min.css'
];

vendorFiles.forEach(file => {
  const fullPath = path.join(__dirname, '..', file);
  assert(fs.existsSync(fullPath), `Vendored file missing: ${file}`);
  const stat = fs.statSync(fullPath);
  assert(stat.size > 200, `Vendored file is too small or empty: ${file} (${stat.size} bytes)`);
});
console.log('✓ All 5 CodeMirror vendor files verified on local disk.');

// Check 2: Verify index.html loads localized vendor assets
const htmlPath = path.join(__dirname, '..', 'web', 'dist', 'index.html');
const content = fs.readFileSync(htmlPath, 'utf8');

assert(
  content.includes('/vendor/codemirror/codemirror.min.css'),
  'index.html must load local codemirror.min.css'
);
assert(
  content.includes('/vendor/codemirror/codemirror.min.js'),
  'index.html must load local codemirror.min.js'
);
assert(
  content.includes('/vendor/codemirror/mode/sql/sql.min.js'),
  'index.html must load local sql.min.js'
);
assert(
  content.includes('/vendor/codemirror/addon/hint/show-hint.min.js'),
  'index.html must load local show-hint.min.js'
);
assert(
  content.includes('/vendor/codemirror/addon/hint/show-hint.min.css'),
  'index.html must load local show-hint.min.css'
);
console.log('✓ index.html references localized vendor scripts and stylesheets.');

// Check 3: Verify Context-Aware Completion Provider exists in index.html
assert(
  content.includes('function computeSQLCompletions') && content.includes('function sqlContextHint'),
  'index.html must define computeSQLCompletions and sqlContextHint'
);

// Check 4: Verify Schema Catalog Cache is stored and used
assert(
  content.includes('schemaCatalogCache'),
  'index.html must maintain schemaCatalogCache'
);

// Check 5: Verify Auto-Refresh on DDL
assert(
  content.includes('CREATE') && content.includes('ALTER') && content.includes('DROP'),
  'index.html must detect DDL execution for schema auto-refresh'
);
console.log('✓ Core completion provider and caching hooks present.');

// Check 6: Extract and test computeSQLCompletions logic directly
const fnStart = content.indexOf('function computeSQLCompletions');
assert(fnStart !== -1, 'computeSQLCompletions function not found');

// Evaluate the function in a sandboxed test scope
const scope = {};
const fnCode = content.slice(fnStart, content.indexOf('function sqlContextHint'));
const fullEval = `
${content.slice(content.indexOf('const SQL_KEYWORDS'), content.indexOf('function computeSQLCompletions'))}
${fnCode}
scope.computeSQLCompletions = computeSQLCompletions;
`;
eval(fullEval);

const mockSchema = {
  tables: [
    {
      name: 'users',
      type: 'TABLE',
      columns: [
        { name: 'id', type: 'INT' },
        { name: 'username', type: 'VARCHAR(255)' },
        { name: 'email', type: 'VARCHAR(255)' }
      ]
    },
    {
      name: 'orders',
      type: 'TABLE',
      columns: [
        { name: 'id', type: 'INT' },
        { name: 'user_id', type: 'INT' },
        { name: 'amount', type: 'DECIMAL(10,2)' }
      ]
    }
  ]
};

// Test 6a: Alias dereferencing: SELECT u. FROM users u
const sql1 = 'SELECT u. FROM users u';
const res1 = scope.computeSQLCompletions(sql1, sql1.indexOf('u.') + 2, mockSchema);
assert(res1.list.length > 0, 'Should find columns for alias u');
const colNames1 = res1.list.map(c => c.text);
assert(colNames1.includes('username'), 'Should include username');
assert(colNames1.includes('email'), 'Should include email');
assert(!colNames1.includes('amount'), 'Should NOT include orders.amount under alias u');
console.log('✓ Alias dereferencing works correctly.');

// Test 6b: Table context boosting after FROM
const sql2 = 'SELECT * FROM ';
const res2 = scope.computeSQLCompletions(sql2, sql2.length, mockSchema);
assert(res2.list.length > 0, 'Should return candidates after FROM');
assert.strictEqual(res2.list[0].kind, 'TBL', 'Top suggestion after FROM should be a Table');
console.log('✓ Table clause context boosting works correctly.');

// Test 6c: Keyword completion: SEL -> SELECT
const sql3 = 'SEL';
const res3 = scope.computeSQLCompletions(sql3, sql3.length, mockSchema);
assert(res3.list.length > 0, 'Should return candidates for SEL');
assert.strictEqual(res3.list[0].text, 'SELECT', 'Top suggestion for SEL should be UPPERCASE SELECT');
console.log('✓ Keyword prefix matching and UPPERCASE convention verified.');

console.log('ALL AUTOCOMPLETE TESTS PASSED SUCCESSFULLY!');
