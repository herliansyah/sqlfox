import assert from 'node:assert/strict';
import test from 'node:test';
import { compileFilterPills } from './filterCompiler.ts';

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
