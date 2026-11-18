import assert from 'node:assert/strict';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { createRequire } from 'node:module';
import { performance } from 'node:perf_hooks';

const require = createRequire(import.meta.url);
const { buildSync } = require('esbuild');
const outdir = fs.mkdtempSync(path.join(os.tmpdir(), 'sqlfox-codemirror-test-'));

try {
  buildSync({
    entryPoints: [
      'web/frontend/src/lib/utils/sqlStatements.ts',
      'web/frontend/src/lib/utils/sqlCompletion.ts',
      'web/frontend/src/lib/utils/snippets.ts',
    ],
    outdir,
    bundle: true,
    platform: 'node',
    format: 'cjs',
    logLevel: 'silent',
  });

  const { splitSqlStatements, statementAtCursor, executionTarget, isCodePosition } = require(path.join(outdir, 'sqlStatements.js'));
  const { computeSqlCompletions, parseQualifiedReference } = require(path.join(outdir, 'sqlCompletion.js'));
  const { parseSnippetVariables, substituteSnippetVariables } = require(path.join(outdir, 'snippets.js'));
  const labels = (items) => items.map((item) => item.label);
  const col = (name, type = 'text') => ({ name, type, is_nullable: true, is_primary_key: false });
  const table = (name, columns, type = 'BASE TABLE') => ({ name, type, columns });
  const catalogs = {
    app: {
      database_name: 'app', databases: ['app', 'analytics', 'SalesDB'],
      tables: [
        table('users', [col('id', 'int'), col('name'), col('email')]),
        table('orders', [col('id', 'int'), col('user_id', 'int'), col('total', 'decimal')]),
        table('active_users', [col('id', 'int')], 'VIEW'),
      ],
    },
    analytics: {
      database_name: 'analytics', databases: ['app', 'analytics', 'SalesDB'],
      tables: [table('users', [col('analytics_id', 'bigint'), col('region')]), table('events', [col('id', 'int'), col('payload', 'json')])],
    },
    salesdb: {
      database_name: 'SalesDB', databases: ['app', 'analytics', 'SalesDB'],
      tables: [table('Invoices', [col('InvoiceID', 'int'), col('Amount', 'decimal')])],
    },
  };
  const snippets = [{ id: 's1', title: 'Active Users', sql: 'SELECT * FROM users WHERE status = {{status:active}};', tags: ['users'] }];
  const complete = (text, cursor = text.length, extra = {}) => computeSqlCompletions({ text, cursor, catalogs, activeDatabase: 'app', snippets, ...extra });

  // Statement splitting / execution safety.
  const quoted = "SELECT 'semi;colon', \"another;value\", `tick;name`; # ignored ;\nSELECT 2;";
  assert.equal(splitSqlStatements(quoted).length, 2);
  assert.equal(statementAtCursor(quoted, quoted.length).text, '# ignored ;\nSELECT 2;');
  const dollar = "CREATE FUNCTION f() RETURNS void AS $$ BEGIN RAISE NOTICE 'one;two'; END $$ LANGUAGE plpgsql; SELECT 2;";
  assert.equal(splitSqlStatements(dollar).length, 2);
  assert.equal(statementAtCursor(dollar, dollar.length).text, 'SELECT 2;');
  const multi = 'SELECT 1;\nDELETE FROM logs;\nSELECT 3;';
  assert.equal(statementAtCursor(multi, multi.indexOf('DELETE') + 2).text, 'DELETE FROM logs;');
  assert.equal(executionTarget(multi, multi.indexOf('DELETE'), multi.indexOf('DELETE') + 16).text, 'DELETE FROM logs');
  assert.equal(executionTarget(multi, multi.indexOf('DELETE') + 6, multi.indexOf('DELETE') + 7).text, 'DELETE FROM logs;', 'whitespace selection falls back to statement');
  assert.equal(statementAtCursor('SELECT 1; -- trailing comment', 29).text, 'SELECT 1;', 'never execute the full script after its final semicolon');

  // Half-open ranges: the caret at the start of statement 2 must select statement 2.
  const adjacent = 'DELETE FROM logs WHERE id=1;SELECT 1;';
  assert.equal(statementAtCursor(adjacent, adjacent.indexOf('SELECT 1')).text, 'SELECT 1;');
  assert.equal(statementAtCursor(adjacent, adjacent.indexOf('DELETE') + 2).text, 'DELETE FROM logs WHERE id=1;');

  // Dialect-aware splitting: '#' is an operator in PostgreSQL, a comment in MySQL.
  const postgresHash = 'SELECT 1 # 1; DELETE FROM logs;';
  assert.equal(splitSqlStatements(postgresHash, 'postgres').length, 2);
  assert.equal(splitSqlStatements(postgresHash, 'mysql').length, 1);
  const mysqlDollar = 'SELECT 1 AS $tag$; DELETE FROM logs;';
  assert.equal(splitSqlStatements(mysqlDollar, 'mysql').length, 2);
  assert.equal(splitSqlStatements(mysqlDollar, 'postgres').length, 1);
  assert.equal(splitSqlStatements('SELECT 1; DELETE FROM logs;', 'sqlite').length, 2);

  // Code position detection must respect comments and literals.
  assert.equal(isCodePosition('SELECT 1 -- don\'t', 14, 'mysql'), false);
  assert.equal(isCodePosition("SELECT 'abc", 11, 'mysql'), false);
  assert.equal(isCodePosition('SELECT 1 FROM t', 16, 'mysql'), true);

  // Clause, table, alias and column completion.
  let result = complete('SELECT * FROM us');
  assert(labels(result).includes('users'));
  assert(!labels(result).includes('orders'));
  result = complete('SELECT u. FROM app.users u', 9);
  assert.deepEqual(new Set(labels(result)), new Set(['id', 'name', 'email']));
  assert(result.every((item) => item.kind === 'COL'));
  result = complete('SELECT u.i FROM app.users u', 10);
  assert(labels(result).includes('id'));
  assert(!labels(result).includes('total'));
  result = complete('SELECT  FROM app.users u', 7);
  assert(labels(result).includes('email'));
  assert(!labels(result).includes('total'), 'only referenced tables contribute unqualified columns');
  result = complete('SELECT COU');
  assert(labels(result).includes('COUNT()'));

  // Cross-DB catalogs must never leak the active database's objects.
  result = complete('SELECT analytics.us');
  assert.deepEqual(labels(result), ['users']);
  result = complete('SELECT analytics.users.');
  assert.deepEqual(new Set(labels(result)), new Set(['analytics_id', 'region']));
  result = complete('SELECT analytics.users.analytics');
  assert.deepEqual(labels(result), ['analytics_id']);
  result = complete('SELECT x. FROM analytics.users x', 9);
  assert.deepEqual(new Set(labels(result)), new Set(['analytics_id', 'region']));
  result = complete('SELECT * FROM Sales');
  const salesDb = result.find((item) => item.kind === 'DB' && item.label === 'SalesDB');
  assert(salesDb, 'database candidate preserves original casing');
  result = complete('SELECT SalesDB.Invoices.');
  assert.deepEqual(new Set(labels(result)), new Set(['InvoiceID', 'Amount']));
  result = complete('SELECT other.');
  assert.equal(result.length, 0, 'unknown DB must not return active-DB tables');

  // Lazy cross-DB loading is requested by exact qualifier only.
  let requested = '';
  complete('SELECT archive.', 'SELECT archive.'.length, { onDatabaseRequested: (db) => { requested = db; } });
  assert.equal(requested, 'archive');

  // Quoted identifiers and invalid completion contexts.
  result = complete('SELECT * FROM "analytics"."users".');
  assert.deepEqual(new Set(labels(result)), new Set(['analytics_id', 'region']));
  assert.equal(parseQualifiedReference('SELECT "users"', 14), null);
  for (const invalid of ["SELECT 'users.", 'SELECT "users.', '-- users.', '/* users. */']) {
    const cursor = invalid.indexOf('.') + 1;
    assert.equal(complete(invalid, cursor).length, 0, `no completion in ${invalid}`);
  }
  const fake = "SELECT 'FROM orders o'; SELECT  FROM app.users u";
  result = complete(fake, fake.length - ' FROM app.users u'.length + 7);
  assert(!labels(result).includes('total'), 'relations inside strings are ignored');

  // Snippet parser supports defaults and repeated variables.
  assert.deepEqual(parseSnippetVariables('SELECT {{status:active}}, {{status:active}}, {{limit:10}}'), [
    { name: 'status', defaultValue: 'active' }, { name: 'limit', defaultValue: '10' },
  ]);
  assert.equal(
    substituteSnippetVariables('SELECT {{status:active}}, {{status:active}}, {{limit:10}}', { status: 'paused', limit: '5' }),
    'SELECT paused, paused, 5',
  );
  result = complete('SELECT Active');
  assert(result.some((item) => item.kind === 'SNP' && item.label === 'Active Users'));

  // Large catalog stays scoped; generous ceiling avoids timing-sensitive flakes.
  const largeCatalog = {
    database_name: 'large', tables: Array.from({ length: 200 }, (_, tableIndex) =>
      table(`table_${tableIndex}`, Array.from({ length: 40 }, (_, columnIndex) => col(`column_${columnIndex}`)))),
  };
  const largeText = 'SELECT table_199.column_39 FROM large.table_199';
  const largeCursor = largeText.indexOf('column_39') + 'column_39'.length;
  const started = performance.now();
  for (let i = 0; i < 100; i++) {
    const items = computeSqlCompletions({ text: largeText, cursor: largeCursor, catalogs: { large: largeCatalog }, activeDatabase: 'large' });
    assert.deepEqual(labels(items), ['column_39']);
  }
  const elapsed = performance.now() - started;
  assert(elapsed < 2000, `100 large-catalog completions took ${elapsed.toFixed(1)}ms`);

  // Regression: completion must stay fast on huge catalogs with realistic (unique) names.
  // The engine used to build every candidate before filtering, which made typing lag.
  const hugeCatalog = {
    database_name: 'huge',
    databases: ['huge'],
    tables: Array.from({ length: 2000 }, (_, tableIndex) => ({
      name: `table_${tableIndex}`,
      type: 'BASE TABLE',
      columns: Array.from({ length: 40 }, (_, columnIndex) => ({ name: `table_${tableIndex}_column_${columnIndex}`, type: 'int' })),
    })),
  };
  const hugeStarted = performance.now();
  for (let i = 0; i < 20; i++) {
    computeSqlCompletions({ text: 'SELECT ', cursor: 7, catalogs: { huge: hugeCatalog }, activeDatabase: 'huge' });
    computeSqlCompletions({ text: 'SELECT cust', cursor: 11, catalogs: { huge: hugeCatalog }, activeDatabase: 'huge' });
  }
  const hugeElapsed = (performance.now() - hugeStarted) / 20;
  assert(hugeElapsed < 50, `huge-catalog completion took ${hugeElapsed.toFixed(1)}ms per keystroke`);

  // The popup payload must stay bounded (legacy parity: 50 options).
  const many = computeSqlCompletions({ text: 'SELECT ', cursor: 7, catalogs: { huge: hugeCatalog }, activeDatabase: 'huge' });
  assert(many.length <= 50, `completion must not exceed 50 candidates, got ${many.length}`);

  console.log(`✅ Svelte CodeMirror regression tests passed (large catalog: ${elapsed.toFixed(1)}ms/100 completions, huge catalog: ${hugeElapsed.toFixed(2)}ms/keystroke)`);
} finally {
  fs.rmSync(outdir, { recursive: true, force: true });
}
