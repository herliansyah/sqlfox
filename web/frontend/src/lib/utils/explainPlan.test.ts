import assert from 'node:assert/strict';
import test from 'node:test';
import { parseExplainPlan } from './explainPlan.ts';

test('parses PostgreSQL JSON plan tree with nested nodes', () => {
  const plan = {
    raw_json: JSON.stringify([
      {
        Plan: {
          'Node Type': 'Sort',
          'Total Cost': 42.5,
          'Plan Rows': 10,
          'Actual Rows': 8,
          'Sort Key': ['created_at DESC'],
          Plans: [
            {
              'Node Type': 'Seq Scan',
              'Relation Name': 'users',
              'Plan Rows': 100,
              'Actual Rows': 8,
              'Total Cost': 15.0,
              Filter: '(active = true)',
            },
          ],
        },
      },
    ]),
    rows: [],
  };

  const parsed = parseExplainPlan(plan);
  assert.equal(parsed.dialectHint, 'postgres');
  assert.equal(parsed.nodes.length, 2);
  assert.equal(parsed.nodes[0].operation, 'Sort');
  assert.equal(parsed.nodes[0].depth, 0);
  assert.equal(parsed.nodes[0].isWarning, true);
  assert.equal(parsed.nodes[1].operation, 'Seq Scan');
  assert.equal(parsed.nodes[1].table, 'users');
  assert.equal(parsed.nodes[1].depth, 1);
  assert.equal(parsed.nodes[1].isDanger, true);
  assert.equal(parsed.nodes[1].actualRows, 8);
  assert.match(parsed.nodes[1].extra, /Filter: \(active = true\)/);
});

test('parses SQLite EXPLAIN QUERY PLAN rows accurately', () => {
  const plan = {
    raw_json: '',
    rows: [
      { id: 2, type: 'SCAN/SEARCH', table: '', key: '', rows: 0, extra: 'SCAN orders' },
      { id: 3, type: 'SCAN/SEARCH', table: '', key: '', rows: 0, extra: 'SEARCH users USING INDEX idx_users_id (id=?)' },
      { id: 4, type: 'SCAN/SEARCH', table: '', key: '', rows: 0, extra: 'USE TEMP B-TREE FOR ORDER BY' },
    ],
  };

  const parsed = parseExplainPlan(plan);
  assert.equal(parsed.dialectHint, 'sqlite');
  assert.equal(parsed.nodes.length, 3);
  assert.deepEqual(
    parsed.nodes.map((n) => [n.operation, n.table, n.key, n.isDanger, n.isWarning]),
    [
      ['SCAN', 'orders', '', true, false],
      ['SEARCH', 'users', 'idx_users_id', false, false],
      ['TEMP B-TREE', '(plan step)', '', false, true],
    ]
  );
});

test('parses MySQL tabular EXPLAIN rows', () => {
  const plan = {
    raw_json: '{"query_block":{"select_id":1}}',
    rows: [
      { id: 1, select_type: 'SIMPLE', table: 'orders', type: 'ALL', key: '', rows: 500, filtered: 10, extra: 'Using where; Using filesort' },
      { id: 1, select_type: 'SIMPLE', table: 'users', type: 'eq_ref', key: 'PRIMARY', rows: 1, filtered: 100, extra: '' },
    ],
  };

  const parsed = parseExplainPlan(plan);
  assert.equal(parsed.dialectHint, 'mysql');
  assert.equal(parsed.nodes.length, 2);
  assert.equal(parsed.nodes[0].isDanger, true);
  assert.equal(parsed.nodes[0].isWarning, true);
  assert.equal(parsed.nodes[1].isDanger, false);
  assert.equal(parsed.nodes[1].key, 'PRIMARY');
});
