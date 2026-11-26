import assert from 'node:assert/strict';
import { createRequire } from 'node:module';

const require = createRequire(new URL('../web/frontend/package.json', import.meta.url));
const esbuild = require('esbuild');

const built = await esbuild.build({
  entryPoints: ['web/frontend/src/lib/utils/dataTypes.ts'],
  bundle: true,
  write: false,
  format: 'esm',
  platform: 'node',
});

const code = built.outputFiles[0].text;
const mod = await import(`data:text/javascript;base64,${Buffer.from(code).toString('base64')}`);

const {
  resolveColumnType,
  resolveColumnNullable,
  resolveEnumValues,
  validateCellValue,
  findTableMeta,
} = mod;

const catalog = {
  tables: [
    {
      name: 'orders',
      type: 'BASE TABLE',
      columns: [
        { name: 'id', type: 'bigint', is_nullable: false, is_primary_key: true },
        { name: 'status', type: "enum('pending','paid','cancelled')", is_nullable: false, is_primary_key: false },
        { name: 'pg_mood', type: 'mood_type', is_nullable: true, is_primary_key: false, enum_values: ['happy', 'sad'] },
        { name: 'order_date', type: 'date', is_nullable: false, is_primary_key: false },
        { name: 'note', type: 'varchar(5)', is_nullable: true, is_primary_key: false },
        { name: 'is_active', type: 'tinyint(1)', is_nullable: false, is_primary_key: false },
      ],
    },
  ],
};

const tableMeta = findTableMeta('public.orders', catalog);
assert.ok(tableMeta, 'should resolve schema-prefixed table name');

// Result column from SQL query has truncated DatabaseTypeName() and nullable=true fallback
const statusResultCol = { name: 'status', type: 'CHAR', nullable: true };
assert.equal(resolveColumnType(statusResultCol, tableMeta), "enum('pending','paid','cancelled')");
assert.equal(resolveColumnNullable(statusResultCol, tableMeta), false);
assert.deepEqual(resolveEnumValues(statusResultCol, tableMeta), ['pending', 'paid', 'cancelled']);

// Postgres enum_values
const pgMoodCol = { name: 'pg_mood', type: 'USER-DEFINED', nullable: true };
assert.deepEqual(resolveEnumValues(pgMoodCol, tableMeta), ['happy', 'sad']);
assert.equal(validateCellValue('happy', pgMoodCol, tableMeta), null);
assert.equal(validateCellValue(null, pgMoodCol, tableMeta), null);
assert.ok(validateCellValue('angry', pgMoodCol, tableMeta));

// Non-nullable enforcement
assert.ok(validateCellValue(null, statusResultCol, tableMeta), 'NOT NULL enum rejects null');
assert.ok(validateCellValue('shipped', statusResultCol, tableMeta), 'enum rejects invalid value');
assert.equal(validateCellValue('paid', statusResultCol, tableMeta), null);

// Date enforcement (format + calendar validity)
const dateCol = { name: 'order_date', type: 'DATE', nullable: false };
assert.equal(validateCellValue('2026-11-18', dateCol, tableMeta), null);
assert.ok(validateCellValue('2026-02-30', dateCol, tableMeta), 'rejects impossible calendar date');
assert.ok(validateCellValue('not-a-date', dateCol, tableMeta), 'rejects non-date text');
assert.ok(validateCellValue('', dateCol, tableMeta), 'rejects empty string for date');

// Max length & boolean
const noteCol = { name: 'note', type: 'VARCHAR', nullable: true };
assert.equal(validateCellValue('hello', noteCol, tableMeta), null);
assert.ok(validateCellValue('too long', noteCol, tableMeta), 'rejects exceeding varchar(5)');
assert.equal(validateCellValue(null, noteCol, tableMeta), null);

const activeCol = { name: 'is_active', type: 'TINYINT', nullable: false };
assert.equal(resolveColumnType(activeCol, tableMeta), 'tinyint(1)');
assert.ok(validateCellValue(null, activeCol, tableMeta), 'NOT NULL bool rejects null');
assert.equal(validateCellValue(true, activeCol, tableMeta), null);

console.log('✓ cell-constraints check passed');
