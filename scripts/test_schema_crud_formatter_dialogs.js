const fs = require('fs');
const assert = require('assert');

const html = fs.readFileSync('web/dist/index.html', 'utf8');

console.log('Testing SQL Formatter, Native Dialogs, and HeidiSQL Structure CRUD...');

// 1. Check Native HTML5 Dialog Elements
const requiredDialogs = [
  'app-native-dialog',
  'safety-inspection-dialog',
  'ddl-review-dialog',
  'table-creator-dialog',
  'view-designer-dialog',
  'trigger-designer-dialog'
];

requiredDialogs.forEach(id => {
  assert(html.includes(`id="${id}"`), `Missing dialog element with id="${id}"`);
  assert(html.includes(`<dialog id="${id}"`), `Element id="${id}" must be a native <dialog> tag`);
});
console.log('✓ All 6 native HTML5 <dialog> elements present in DOM.');

// 2. Check Table Sub-Tabs & HeidiSQL Mode Views
const requiredViews = [
  'table-mode-tabs',
  'btn-mode-data',
  'btn-mode-structure',
  'btn-mode-indexes',
  'btn-mode-foreign-keys',
  'btn-mode-ddl',
  'table-structure-view',
  'table-indexes-view',
  'table-fk-view',
  'table-ddl-view'
];

requiredViews.forEach(id => {
  assert(html.includes(`id="${id}"`), `Missing view element with id="${id}"`);
});
console.log('✓ Table sub-tabs and HeidiSQL structure views present.');

// 3. Extract and unit-test formatSQL directly from index.html
const formatSQLMatch = html.match(/function formatSQL\(sql\) \{([\s\S]*?)\n    \}/);
assert(formatSQLMatch, 'formatSQL function must be defined in script');

// Create sandbox to execute formatSQL
const SQL_KEYWORDS = [
  'SELECT', 'FROM', 'WHERE', 'JOIN', 'LEFT', 'RIGHT', 'INNER', 'ON',
  'GROUP', 'BY', 'ORDER', 'HAVING', 'LIMIT', 'OFFSET', 'UNION', 'ALL',
  'INSERT', 'INTO', 'VALUES', 'UPDATE', 'SET', 'DELETE',
  'CREATE', 'TABLE', 'ALTER', 'DROP', 'VIEW', 'TRIGGER',
  'AND', 'OR', 'AS', 'ASC', 'DESC', 'NOT', 'NULL', 'IS'
];
const sandbox = new Function('SQL_KEYWORDS', `
  ${formatSQLMatch[0]}
  return formatSQL;
`)(SQL_KEYWORDS);

const rawSQL = "select id, name, email from users where id > 5 and status = 'active' order by id desc limit 10;";
const formatted = sandbox(rawSQL);

assert(formatted.includes('SELECT id,'), 'formatSQL must uppercase keywords');
assert(formatted.includes('\nFROM users'), 'formatSQL must put FROM on new line');
assert(formatted.includes('\nWHERE id > 5'), 'formatSQL must put WHERE on new line');
assert(formatted.includes('\nORDER BY id DESC'), 'formatSQL must put ORDER BY on new line');
assert(formatted.includes('\nLIMIT 10;'), 'formatSQL must put LIMIT on new line');
console.log('✓ SQL Formatter unit test passed:\n' + formatted.replace(/^/gm, '    '));

// 4. Check Keybinding for Format in CodeMirror extraKeys and global listener
assert(html.includes("'Ctrl-Shift-F': function()"), 'CodeMirror extraKeys must include Ctrl-Shift-F');
assert(html.includes("'Cmd-Shift-F': function()"), 'CodeMirror extraKeys must include Cmd-Shift-F');
assert(html.includes("e.shiftKey && (e.key === 'F' || e.key === 'f')"), 'Global keydown listener must support Ctrl+Shift+F');
console.log('✓ Format SQL keybindings registered.');

// 5. Verify Safety Check replaces alert with openSafetyInspectionModal
assert(html.includes('function inspectActiveQuery()'), 'inspectActiveQuery must be defined');
assert(html.includes('openSafetyInspectionModal(data, sql)'), 'inspectActiveQuery must call openSafetyInspectionModal');
assert(!html.includes('alert(`Safety Inspection:'), 'Old alert in inspectActiveQuery must be completely removed');
console.log('✓ Safety Guard inspection modal integration verified.');

// 6. Verify Table Designer CRUD Functions
const requiredFunctions = [
  'setTableMode',
  'renderTableStructure',
  'addStructureColumn',
  'saveStructureChanges',
  'toggleColPK',
  'removeStructureCol',
  'renderTableIndexes',
  'submitNewIndex',
  'dropTableIndex',
  'renderTableFKs',
  'submitNewFK',
  'dropTableFK',
  'openCreateTableModal',
  'submitCreateTable',
  'openCreateViewModal',
  'submitCreateView',
  'dropView',
  'openCreateTriggerModal',
  'submitCreateTrigger',
  'dropTrigger'
];

requiredFunctions.forEach(fn => {
  assert(html.includes(`function ${fn}`), `Function ${fn} must be defined`);
});
console.log('✓ All 20 Table / View / Trigger GUI CRUD functions verified.');

// 7. Verify Type Guard Palette & Default Value Presets
const requiredTypeGuardFunctions = [
  'parseColumnType',
  'formatColumnType',
  'isLengthlessType',
  'isIntegerType',
  'detectDefaultPreset',
  'formatDefaultClause',
  'renderTypeSelectOptions',
  'updateStagedColType',
  'updateStagedColDefaultPreset',
  'updateCreatorColType',
  'updateCreatorColDefaultPreset'
];

requiredTypeGuardFunctions.forEach(fn => {
  assert(html.includes(`function ${fn}`), `Type Guard function ${fn} must be defined`);
});

// Unit test Type Guard logic in sandbox
const typeGuardSandbox = new Function(`
  ${html.match(/function parseColumnType\([\s\S]*?function renderTypeSelectOptions\([^\)]*\) \{[\s\S]*?\n    \}/)[0]}
  return { parseColumnType, formatColumnType, isLengthlessType, isIntegerType, detectDefaultPreset, formatDefaultClause };
`)();

assert.deepStrictEqual(typeGuardSandbox.parseColumnType('VARCHAR(255)'), { base: 'VARCHAR', length: '255' });
assert.deepStrictEqual(typeGuardSandbox.parseColumnType('INT'), { base: 'INT', length: '' });
assert.deepStrictEqual(typeGuardSandbox.parseColumnType('decimal(10,2)'), { base: 'DECIMAL', length: '10,2' });
assert.strictEqual(typeGuardSandbox.isLengthlessType('INT'), true, 'INT must be lengthless (no int(200))');
assert.strictEqual(typeGuardSandbox.isLengthlessType('VARCHAR'), false, 'VARCHAR requires length');
assert.strictEqual(typeGuardSandbox.isIntegerType('BIGINT'), true);
assert.strictEqual(typeGuardSandbox.isIntegerType('VARCHAR'), false);
assert.strictEqual(typeGuardSandbox.formatColumnType('INT', '200'), 'INT', 'INT(200) must be prevented and normalized to INT');
assert.strictEqual(typeGuardSandbox.formatColumnType('VARCHAR', '100'), 'VARCHAR(100)');

assert.strictEqual(typeGuardSandbox.detectDefaultPreset(null), 'none');
assert.strictEqual(typeGuardSandbox.detectDefaultPreset('NULL'), 'NULL');
assert.strictEqual(typeGuardSandbox.detectDefaultPreset('CURRENT_TIMESTAMP'), 'CURRENT_TIMESTAMP');
assert.strictEqual(typeGuardSandbox.detectDefaultPreset('0'), '0');
assert.strictEqual(typeGuardSandbox.detectDefaultPreset("''"), 'empty_string');
assert.strictEqual(typeGuardSandbox.detectDefaultPreset('custom_val'), 'custom');

assert.strictEqual(typeGuardSandbox.formatDefaultClause('NULL', 'VARCHAR'), 'DEFAULT NULL');
assert.strictEqual(typeGuardSandbox.formatDefaultClause('CURRENT_TIMESTAMP', 'DATETIME'), 'DEFAULT CURRENT_TIMESTAMP');
assert.strictEqual(typeGuardSandbox.formatDefaultClause('0', 'INT'), 'DEFAULT 0');
assert.strictEqual(typeGuardSandbox.formatDefaultClause('active', 'VARCHAR'), "DEFAULT 'active'");
console.log('✓ Type Guard Palette and Default Value Presets unit tests passed.');

// 8. Verify Cross-Database Autocomplete logic
const computeMatch = html.match(/function computeSQLCompletions\(fullText, cursorIndex, schema\) \{([\s\S]*?)\n    \}/);
assert(computeMatch, 'computeSQLCompletions must be defined');

const acSandbox = new Function('SQL_KEYWORDS', 'SQL_FUNCTIONS', `
  let multiDBCatalogCache = {
    finance: {
      database_name: 'finance',
      tables: [
        { name: 'invoices', columns: [{ name: 'id', type: 'INT' }, { name: 'amount', type: 'DECIMAL(10,2)' }] },
        { name: 'clients', columns: [{ name: 'id', type: 'INT' }, { name: 'company', type: 'VARCHAR(100)' }] }
      ]
    },
    sales: {
      database_name: 'sales',
      tables: [
        { name: 'orders', columns: [{ name: 'id', type: 'INT' }, { name: 'total', type: 'DECIMAL(10,2)' }] }
      ]
    }
  };
  ${computeMatch[0]}
  return { computeSQLCompletions, multiDBCatalogCache };
`)(SQL_KEYWORDS, ['COUNT()', 'SUM()', 'NOW()']);

const testSchema = {
  database_name: 'core',
  databases: ['core', 'finance', 'sales'],
  tables: [{ name: 'users', columns: [{ name: 'id' }, { name: 'email' }] }]
};

// Test 8a: Typing "FROM " should suggest databases with kind "DB"
const fromRes = acSandbox.computeSQLCompletions('SELECT * FROM ', 14, testSchema);
const dbSuggestions = fromRes.list.filter(c => c.kind === 'DB');
assert(dbSuggestions.some(c => c.text === 'finance'), 'Should suggest database "finance" in FROM clause');
assert(dbSuggestions.some(c => c.text === 'sales'), 'Should suggest database "sales" in FROM clause');

// Test 8b: Typing "finance." should suggest tables in finance with kind "TBL"
const finDotRes = acSandbox.computeSQLCompletions('SELECT * FROM finance.', 22, testSchema);
assert(finDotRes.hasDot, 'finance. must have hasDot=true');
const tblSuggestions = finDotRes.list.filter(c => c.kind === 'TBL');
assert(tblSuggestions.some(c => c.text === 'invoices'), 'finance. must suggest table "invoices"');
assert(tblSuggestions.some(c => c.text === 'clients'), 'finance. must suggest table "clients"');

// Test 8c: Typing "finance.invoices." should suggest columns in finance.invoices with kind "COL"
const finColRes = acSandbox.computeSQLCompletions('SELECT finance.invoices.', 24, testSchema);
assert(finColRes.hasDot, 'finance.invoices. must have hasDot=true');
const colSuggestions = finColRes.list.filter(c => c.kind === 'COL');
assert(colSuggestions.some(c => c.text === 'amount'), 'finance.invoices. must suggest column "amount"');
assert(colSuggestions.some(c => c.text === 'id'), 'finance.invoices. must suggest column "id"');
console.log('✓ Cross-Database autocompletion verified (DB candidates, db.table, db.table.col).');

// 9. Verify HeidiSQL-Style Session Switcher
const requiredSessionElements = [
  'session-switcher-btn',
  'session-switcher-name',
  'session-switcher-env-badge',
  'session-dropdown-menu',
  'session-search-input',
  'connections-tree'
];

requiredSessionElements.forEach(id => {
  assert(html.includes(`id="${id}"`), `Missing element with id="${id}"`);
});

assert(html.includes('function toggleSessionDropdown'), 'toggleSessionDropdown must be defined');
assert(html.includes('function filterSessionList'), 'filterSessionList must be defined');
console.log('✓ HeidiSQL Session Switcher elements and handlers verified.');

console.log('\n✅ ALL SCHEMA CRUD, FORMATTER, DIALOGS, TYPE GUARD, AUTOCOMPLETE, AND SESSION SWITCHER TESTS PASSED!');

