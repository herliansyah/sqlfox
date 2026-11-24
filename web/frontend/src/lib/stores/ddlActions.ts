import { quoteSqlIdentifier } from '../utils/sqlStatements';

export function buildDropTableSQL(tableName: string, driver?: string): string {
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const qTable = quoteSqlIdentifier(tableName, driver);
  return isPg ? `DROP TABLE ${qTable} CASCADE;` : `DROP TABLE IF EXISTS ${qTable};`;
}

export function buildTruncateTableSQL(tableName: string, driver?: string): string {
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const isSqlite = driver === 'sqlite';
  const qTable = quoteSqlIdentifier(tableName, driver);
  if (isPg) return `TRUNCATE TABLE ${qTable} CASCADE;`;
  if (isSqlite) return `DELETE FROM ${qTable};`;
  return `TRUNCATE TABLE ${qTable};`;
}

export function buildRenameTableSQL(oldName: string, newName: string, driver?: string): string {
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const isSqlite = driver === 'sqlite';
  const qOld = quoteSqlIdentifier(oldName, driver);
  const qNew = quoteSqlIdentifier(newName, driver);
  return isPg || isSqlite
    ? `ALTER TABLE ${qOld} RENAME TO ${qNew};`
    : `RENAME TABLE ${qOld} TO ${qNew};`;
}

export function buildDuplicateTableSQL(
  sourceTable: string,
  targetTable: string,
  copyData: boolean,
  driver?: string
): string {
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const qSource = quoteSqlIdentifier(sourceTable, driver);
  const qTarget = quoteSqlIdentifier(targetTable, driver);
  if (isPg) {
    let sql = `CREATE TABLE ${qTarget} (LIKE ${qSource} INCLUDING ALL);`;
    if (copyData) sql += `\nINSERT INTO ${qTarget} SELECT * FROM ${qSource};`;
    return sql;
  }
  if (driver === 'sqlite') {
    return copyData
      ? `CREATE TABLE ${qTarget} AS SELECT * FROM ${qSource};`
      : `CREATE TABLE ${qTarget} AS SELECT * FROM ${qSource} WHERE 0 = 1;`;
  }
  let sql = `CREATE TABLE ${qTarget} LIKE ${qSource};`;
  if (copyData) sql += `\nINSERT INTO ${qTarget} SELECT * FROM ${qSource};`;
  return sql;
}

export function buildCreateDatabaseSQL(
  dbName: string,
  charset: string,
  collation: string,
  driver?: string
): string {
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const qDb = quoteSqlIdentifier(dbName, driver);
  let sql = `CREATE DATABASE ${qDb}`;
  if (!isPg && charset) {
    sql += ` CHARACTER SET ${charset}`;
    if (collation) sql += ` COLLATE ${collation}`;
  }
  return `${sql};`;
}

export function buildDropDatabaseSQL(dbName: string, driver?: string): string {
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const qDb = quoteSqlIdentifier(dbName, driver);
  return isPg ? `DROP DATABASE ${qDb};` : `DROP DATABASE IF EXISTS ${qDb};`;
}
