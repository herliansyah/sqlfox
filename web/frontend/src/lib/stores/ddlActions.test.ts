import test from 'node:test';
import assert from 'node:assert/strict';
import {
  buildDropTableSQL,
  buildTruncateTableSQL,
  buildRenameTableSQL,
  buildDuplicateTableSQL,
  buildCreateDatabaseSQL,
} from './ddlActions.ts';

test('DDL builders quote identifiers per dialect and handle edge cases', () => {
  assert.equal(buildDropTableSQL('users', 'mysql'), 'DROP TABLE IF EXISTS `users`;');
  assert.equal(buildDropTableSQL('users', 'postgres'), 'DROP TABLE "users" CASCADE;');

  assert.equal(buildTruncateTableSQL('logs', 'sqlite'), 'DELETE FROM "logs";');
  assert.equal(buildTruncateTableSQL('logs', 'mysql'), 'TRUNCATE TABLE `logs`;');

  assert.equal(
    buildRenameTableSQL('old_tbl', 'new_tbl', 'postgres'),
    'ALTER TABLE "old_tbl" RENAME TO "new_tbl";'
  );
  assert.equal(
    buildRenameTableSQL('old_tbl', 'new_tbl', 'mysql'),
    'RENAME TABLE `old_tbl` TO `new_tbl`;'
  );

  assert.equal(
    buildDuplicateTableSQL('src', 'dst', true, 'sqlite'),
    'CREATE TABLE "dst" AS SELECT * FROM "src";'
  );
  assert.equal(
    buildDuplicateTableSQL('src', 'dst', false, 'mysql'),
    'CREATE TABLE `dst` LIKE `src`;'
  );

  assert.equal(
    buildCreateDatabaseSQL('analytics', 'utf8mb4', 'utf8mb4_unicode_ci', 'mysql'),
    'CREATE DATABASE `analytics` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;'
  );
});
