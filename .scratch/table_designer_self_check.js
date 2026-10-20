// table_designer_self_check.js - Small runnable assertion self-check for Table Designer logic
const assert = require('assert');
const fs = require('fs');

const html = fs.readFileSync('web/dist/index.html', 'utf8');

// Helper to extract a function by name from HTML
function extractFunction(name) {
  const match = html.match(new RegExp(`function ${name}\\s*\\([\\s\\S]*?\\n    }`, 'm'));
  if (!match) throw new Error(`Function ${name} not found in index.html`);
  return new Function(`return (${match[0]});`)();
}

const isLengthlessType = global.isLengthlessType = extractFunction('isLengthlessType');
const parseColumnType = global.parseColumnType = extractFunction('parseColumnType');
const parseEnumOptionList = global.parseEnumOptionList = extractFunction('parseEnumOptionList');
const renderEnumPillSummary = global.renderEnumPillSummary = extractFunction('renderEnumPillSummary');
const formatColumnType = global.formatColumnType = extractFunction('formatColumnType');
const validateTableColumns = global.validateTableColumns = extractFunction('validateTableColumns');

// EscapeHtml dummy for renderEnumPillSummary
global.escapeHtml = (s) => String(s);

console.log('Running Table Designer logic self-checks...');

// 1. Check parseColumnType
assert.deepStrictEqual(parseColumnType('VARCHAR(255)'), { base: 'VARCHAR', length: '255' });
assert.deepStrictEqual(parseColumnType("ENUM('active','inactive')"), { base: 'ENUM', length: "'active','inactive'" });
assert.deepStrictEqual(parseColumnType('BIGINT'), { base: 'BIGINT', length: '' });

// 2. Check parseEnumOptionList with various quoting and delimiters
assert.deepStrictEqual(parseEnumOptionList("'active', 'inactive', 'pending'"), ['active', 'inactive', 'pending']);
assert.deepStrictEqual(parseEnumOptionList("active, inactive, pending"), ['active', 'inactive', 'pending']);
assert.deepStrictEqual(parseEnumOptionList("ENUM('draft','published')"), ['draft', 'published']);
assert.deepStrictEqual(parseEnumOptionList("'a''b', \"c\"\"d\""), ["a'b", 'c"d']);

// 3. Check renderEnumPillSummary
assert.strictEqual(renderEnumPillSummary(''), 'Set values...');
assert.strictEqual(renderEnumPillSummary("'open','closed'"), "'open','closed'");
assert.strictEqual(renderEnumPillSummary("'a','b','c','d'"), "4 values ('a',...)");

// 4. Check formatColumnType
assert.strictEqual(formatColumnType('INT', '11'), 'INT'); // lengthless
assert.strictEqual(formatColumnType('VARCHAR', '100'), 'VARCHAR(100)');
assert.strictEqual(formatColumnType('ENUM', "'a','b'"), "ENUM('a','b')");

// 5. Check validateTableColumns
const validCols = [
  { name: 'id', type: 'BIGINT', is_primary_key: true },
  { name: 'status', type: "ENUM('active','inactive')" },
  { name: 'title', type: 'VARCHAR(255)' }
];
assert.deepStrictEqual(validateTableColumns(validCols, false), []);

// Duplicate column name detection
const dupCols = [
  { name: 'title', type: 'VARCHAR(255)' },
  { name: 'TITLE', type: 'VARCHAR(100)' }
];
const dupErrors = validateTableColumns(dupCols, false);
assert.strictEqual(dupErrors.length, 1);
assert(dupErrors[0].includes('Duplicate column name "TITLE"'));

// Empty enum options detection
const emptyEnumCols = [
  { name: 'status', type: 'ENUM()' }
];
const enumErrors = validateTableColumns(emptyEnumCols, false);
assert.strictEqual(enumErrors.length, 1);
assert(enumErrors[0].includes('At least 1 option value must be defined'));

console.log('✅ All Table Designer self-checks passed cleanly!');
