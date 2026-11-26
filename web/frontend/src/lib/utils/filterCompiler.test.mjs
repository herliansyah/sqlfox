import assert from 'node:assert/strict';
import test from 'node:test';
import { compileFilterPills } from './filterCompiler.ts';
import { buildCSVExport, formatCellValue } from '../components/grid/gridFormatters.ts';

test('compileFilterPills quotes IN/NOT IN literals safely and blocks raw subquery injection', () => {
  const safe = compileFilterPills([{ col: 'id', op: 'IN', val: '(1, 2, 3)' }], 'mysql');
  assert.equal(safe, '`id` IN (1, 2, 3)');

  const injected = compileFilterPills(
    [{ col: 'name', op: 'IN', val: "(SELECT password FROM users)" }],
    'postgres'
  );
  assert.equal(injected, `"name" IN ('SELECT password FROM users')`);

  const quotedWithEscape = compileFilterPills(
    [{ col: 'tag', op: 'NOT IN', val: "a'b, c" }],
    'mysql'
  );
  assert.equal(quotedWithEscape, "`tag` NOT IN ('a''b', 'c')");
});

test('compileFilterPills keeps leading-zero, phone, Infinity, and hex strings quoted', () => {
  assert.equal(
    compileFilterPills([{ col: 'zip', op: '=', val: '00123' }], 'mysql'),
    "`zip` = '00123'"
  );
  assert.equal(
    compileFilterPills([{ col: 'phone', op: '=', val: '+628123456' }], 'postgres'),
    `"phone" = '+628123456'`
  );
  assert.equal(
    compileFilterPills([{ col: 'score', op: '=', val: 'Infinity' }], 'mysql'),
    "`score` = 'Infinity'"
  );
  assert.equal(
    compileFilterPills([{ col: 'code', op: '=', val: '0x10' }], 'mysql'),
    "`code` = '0x10'"
  );
  assert.equal(
    compileFilterPills([{ col: 'amount', op: '>=', val: '-42.5' }], 'mysql'),
    '`amount` >= -42.5'
  );
});

test('compileFilterPills handles single-bound and two-bound BETWEEN', () => {
  assert.equal(
    compileFilterPills([{ col: 'price', op: 'BETWEEN', val: '10 AND 50' }], 'mysql'),
    '`price` BETWEEN 10 AND 50'
  );
  assert.equal(
    compileFilterPills([{ col: 'price', op: 'BETWEEN', val: '10' }], 'mysql'),
    '`price` >= 10'
  );
});

test('buildCSVExport neutralizes formula injection prefixes and preserves numeric negatives', () => {
  const csv = buildCSVExport(
    ['id', 'note', 'zip'],
    [
      [-42, '=CMD|"/C calc"!A0', '00123'],
      [1, '+1+1', '@SUM(1,1)'],
    ]
  );
  assert.ok(csv.includes('"-42"'));
  assert.ok(csv.includes(`"'=CMD|""/C calc""!A0"`));
  assert.ok(csv.includes(`"'00123"`));
  assert.ok(csv.includes(`"'+1+1"`));
  assert.ok(csv.includes(`"'@SUM(1,1)"`));
});

test('formatCellValue preserves exact float precision and avoids locale commas on numeric IDs', () => {
  assert.deepEqual(formatCellValue(37.7749295), { display: '37.7749295', type: 'number' });
  assert.deepEqual(formatCellValue(0.000123), { display: '0.000123', type: 'number' });
  assert.deepEqual(formatCellValue(1234567), { display: '1234567', type: 'number' });
});
