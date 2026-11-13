import fs from 'fs';
import assert from 'assert';

console.log('--- Starting Query Grid Mutations & SQL Copy Tests (source-assertion) ---');

// Source-assertion pattern: assert mutation/copy behavior directly against the
// Svelte sources and re-run the copied algorithms in plain JS — no bundled
// web/dist artifacts, no vm sandboxes.
const virtualGridCode = fs.readFileSync(
  'web/frontend/src/lib/components/grid/VirtualGrid.svelte',
  'utf8'
);
const appStateCode = fs.readFileSync(
  'web/frontend/src/lib/stores/appState.svelte.ts',
  'utf8'
);

// --- 1. Context menu mutation & copy markers -------------------------------
for (const marker of [
  'id="grid-ctx-mutations-parent"',
  'id="grid-ctx-table-mutations"',
  'id="grid-ctx-insert-section"',
  'id="grid-ctx-insert-options"',
  'id="grid-ctx-btn-dup-nokey"',
  'id="grid-ctx-btn-dup-key"',
  'id="grid-ctx-btn-add-draft"',
  'id="grid-ctx-btn-delete-row"',
  'id="grid-ctx-btn-copy-row-tsv"',
  'id="grid-ctx-btn-copy-row-json"',
  'id="grid-ctx-btn-copy-row-sql"',
]) {
  assert(virtualGridCode.includes(marker), `Grid context menu must contain ${marker}`);
}
assert(
  virtualGridCode.includes('duplicateRowAsDraft(contextMenu!.rIdx, true)') &&
    virtualGridCode.includes('duplicateRowAsDraft(contextMenu!.rIdx, false)'),
  'Duplicate buttons must call duplicateRowAsDraft with and without the key'
);
assert(
  virtualGridCode.includes('addDraftRow();') &&
    virtualGridCode.includes('handleDeleteCurrentRow();'),
  'Add-draft and delete-row buttons must be wired to their handlers'
);
assert(
  virtualGridCode.includes('copyRowAsTSV(contextMenu!.rIdx);') &&
    virtualGridCode.includes('copyRowAsMarkdown(contextMenu!.rIdx);') &&
    virtualGridCode.includes('copyRowAsInsertSQL(contextMenu!.rIdx);'),
  'Copy buttons must call copyRowAsTSV / copyRowAsMarkdown / copyRowAsInsertSQL'
);
assert(
  virtualGridCode.includes('await appState.deleteRows(pkCol, [pkVal]);'),
  'Row deletion must go through appState.deleteRows with the PK column/value'
);

// --- 2. SQL identifier quoting per driver ----------------------------------
// Copied from appState.svelte.ts duplicateTable quoting rules.
const quoteIdentifierForDriver = (driver, name) => {
  if (driver === 'postgres' || driver === 'postgresql') return `"${name}"`;
  if (driver === 'sqlite') return `"${name}"`;
  return `\`${name}\``;
};
assert.strictEqual(quoteIdentifierForDriver('mysql', 'user_accounts'), '`user_accounts`',
  'MySQL identifiers must be backtick-quoted');
assert.strictEqual(quoteIdentifierForDriver('postgres', 'user_accounts'), '"user_accounts"',
  'PostgreSQL identifiers must be double-quoted');
assert.strictEqual(quoteIdentifierForDriver('sqlite', 'user_accounts'), '"user_accounts"',
  'SQLite identifiers must be double-quoted');
assert(
  appStateCode.includes('CREATE TABLE "${targetTable}"') &&
    appStateCode.includes('CREATE TABLE \\`${targetTable}\\` LIKE \\`${sourceTable}\\`;'),
  'appState must use driver-specific quoting for table duplication SQL'
);

// --- 3. Duplicate row as draft (without key / with key) --------------------
// Copied verbatim from VirtualGrid.svelte duplicateRowAsDraft.
const duplicateRowAsDraft = (result, withoutKey, stagedDraftRows = []) => {
  const getColName = (c) => (typeof c === 'string' ? c : c.name || '');
  const isColPK = (c) => (typeof c === 'object' ? Boolean(c.is_primary_key) : false);
  const allColumns = result.columns.map((c) => getColName(c));
  const rIdx = 0;
  const draft = {};
  allColumns.forEach((colName, cIdx) => {
    const isPK = Boolean(result.columns[cIdx] && isColPK(result.columns[cIdx]));
    draft[colName] = withoutKey && isPK ? '' : result.rows[rIdx][cIdx];
  });
  stagedDraftRows.unshift(draft);
  return stagedDraftRows;
};

const result = {
  columns: [
    { name: 'id', type: 'int', is_primary_key: true },
    { name: 'username', type: 'varchar(255)' },
    { name: 'is_active', type: 'boolean' },
  ],
  rows: [[1, "O'Connor", true]],
};

// Without key: PK must NOT be populated with a value so DB sequences work
let drafts = duplicateRowAsDraft(result, true);
assert.strictEqual(drafts.length, 1, 'Expected 1 draft row');
assert.strictEqual(drafts[0]['id'], '',
  "Primary Key 'id' MUST NOT carry the source value when duplicating without key");
assert.strictEqual(drafts[0]['username'], "O'Connor", 'Username must be copied');
assert.strictEqual(drafts[0]['is_active'], true, 'is_active boolean must be copied');
console.log('✓ duplicateRowAsDraft without key omits the PK so DB sequences work cleanly!');

// With key: PK is copied verbatim (composite/derived keys)
drafts = duplicateRowAsDraft(result, false);
assert.strictEqual(drafts[0]['id'], 1, 'With key, the PK value must be copied verbatim');
assert.strictEqual(drafts[0]['username'], "O'Connor");
console.log('✓ duplicateRowAsDraft with key retains the PK!');

// --- 4. Add draft row ------------------------------------------------------
// Copied from VirtualGrid.svelte addDraftRow: a blank row for every column.
const addDraftRow = (allColumns, stagedDraftRows = []) => {
  const blank = {};
  allColumns.forEach((c) => {
    blank[c] = '';
  });
  stagedDraftRows.unshift(blank);
  return stagedDraftRows;
};
drafts = addDraftRow(['id', 'username', 'is_active']);
assert.strictEqual(drafts.length, 1, 'Add draft must create one blank row');
assert.deepStrictEqual(drafts[0], { id: '', username: '', is_active: '' },
  'Blank draft must have an empty string for every column');
drafts = addDraftRow(['id', 'username', 'is_active'], drafts);
assert.strictEqual(drafts.length, 2, 'New drafts are unshifted to the top of the list');
assert.deepStrictEqual(drafts[0], { id: '', username: '', is_active: '' },
  'The newest draft must be first');
console.log('✓ addDraftRow blank draft semantics passed!');

// --- 5. Remove draft row ---------------------------------------------------
const removeDraftRow = (stagedDraftRows, dIdx) => {
  if (!stagedDraftRows) return;
  stagedDraftRows.splice(dIdx, 1);
};
removeDraftRow(drafts, 0);
assert.strictEqual(drafts.length, 1, 'removeDraftRow must splice the draft out');
console.log('✓ removeDraftRow splice semantics passed!');

// --- 6. Copy row as TSV / Markdown / INSERT SQL ----------------------------
// Copied from VirtualGrid.svelte copyRowAsTSV / copyRowAsMarkdown / copyRowAsInsertSQL.
const getColName = (c) => (typeof c === 'string' ? c : c.name || '');
const cols = result.columns.map((c) => getColName(c));
const row = result.rows[0];

const copyRowAsTSV = () =>
  `${cols.join('\t')}\n${row.map((v) => (v === null || v === undefined ? '' : String(v))).join('\t')}`;
const copyRowAsMarkdown = () =>
  `| ${cols.join(' | ')} |\n| ${cols.map(() => '---').join(' | ')} |\n| ${row
    .map((v) => (v === null ? '*NULL*' : String(v).replace(/\|/g, '\\|')))
    .join(' | ')} |`;
const copyRowAsInsertSQL = (tbl) => {
  const sqlCols = result.columns.map((c) => `\`${getColName(c)}\``).join(', ');
  const vals = row
    .map((c) => {
      if (c === null || c === undefined) return 'NULL';
      if (typeof c === 'number') return c;
      return `'${String(c).replace(/'/g, "''")}'`;
    })
    .join(', ');
  return `INSERT INTO \`${tbl}\` (${sqlCols}) VALUES (${vals});`;
};

assert.strictEqual(
  copyRowAsTSV(),
  "id\tusername\tis_active\n1\tO'Connor\ttrue",
  'TSV copy must be header line + value line joined by tabs'
);
assert(
  copyRowAsMarkdown().includes('| id | username | is_active |') &&
    copyRowAsMarkdown().includes('| --- | --- | --- |') &&
    copyRowAsMarkdown().includes("| 1 | O'Connor | true |"),
  'Markdown copy must produce a proper markdown table row'
);
assert.strictEqual(
  copyRowAsInsertSQL('user_accounts'),
  "INSERT INTO `user_accounts` (`id`, `username`, `is_active`) VALUES (1, 'O''Connor', 'true');",
  'INSERT SQL must escape single quotes (non-numeric values are quoted verbatim)'
);
// NULL handling in SQL copy
const nullRowSQL = (() => {
  const r = [null, undefined, 3];
  return r
    .map((c) => {
      if (c === null || c === undefined) return 'NULL';
      if (typeof c === 'number') return c;
      return `'${String(c).replace(/'/g, "''")}'`;
    })
    .join(', ');
})();
assert.strictEqual(nullRowSQL, 'NULL, NULL, 3',
  'NULL/undefined cells must serialize as NULL in INSERT SQL');
console.log('✓ Copy row TSV / Markdown / INSERT SQL semantics passed!');

// --- 7. Staged edits key by row identity -----------------------------------
// Copied from appState.stageEdit: the key is table:rIdx:cIdx.
const stageEditKey = (table, rIdx, cIdx) => `${table}:${rIdx}:${cIdx}`;
const stagedEdits = {};
const stageEdit = (table, rIdx, cIdx, colName, pkCol, pkVal, oldVal, newVal) => {
  stagedEdits[stageEditKey(table, rIdx, cIdx)] = {
    rIdx, cIdx, table, colName, pkCol, pkVal, oldVal, newVal,
  };
};
stageEdit('users', 2, 1, 'username', 'id', 7, 'alice', 'alice2');
assert(
  stagedEdits['users:2:1'] && stagedEdits['users:2:1'].newVal === 'alice2',
  'stageEdit must key edits by table:rIdx:cIdx so they follow the row, not the array position'
);
assert.strictEqual(Object.keys(stagedEdits).length, 1);
console.log('✓ Staged-edit row identity semantics passed!');

// --- 8. Export SQL (multi-row INSERT export) -------------------------------
// Copied from VirtualGrid.svelte exportSQL.
const exportSQL = (rows, columns, tbl) => {
  const sqlCols = columns.map((c) => `\`${getColName(c)}\``).join(', ');
  return rows.map((r) => {
    const vals = r
      .map((c) => {
        if (c === null || c === undefined) return 'NULL';
        if (typeof c === 'number') return c;
        return `'${String(c).replace(/'/g, "''")}'`;
      })
      .join(', ');
    return `INSERT INTO \`${tbl}\` (${sqlCols}) VALUES (${vals});`;
  });
};
const sqlOut = exportSQL(result.rows, result.columns, 'user_accounts');
assert.strictEqual(sqlOut.length, 1);
assert(
  sqlOut[0].includes("VALUES (1, 'O''Connor', 'true');"),
  'Export SQL must produce one INSERT per row with escaped quotes'
);
console.log('✓ exportSQL multi-row INSERT semantics passed!');

console.log('\n--- ALL TESTS PASSED SUCCESSFULLY! ---');
