import type { QuerySnippet, SchemaCatalog } from '../../types';
import { isCodePosition, maskSqlForAnalysis, splitSqlStatements } from './sqlStatements';

export const SQL_KEYWORDS = [
  'SELECT', 'FROM', 'WHERE', 'INSERT INTO', 'UPDATE', 'DELETE FROM',
  'JOIN', 'INNER JOIN', 'LEFT JOIN', 'LEFT OUTER JOIN', 'RIGHT JOIN', 'RIGHT OUTER JOIN',
  'FULL JOIN', 'FULL OUTER JOIN', 'CROSS JOIN', 'NATURAL JOIN', 'ON', 'USING',
  'GROUP BY', 'ORDER BY', 'HAVING', 'LIMIT', 'OFFSET', 'FETCH FIRST',
  'UNION ALL', 'UNION', 'INTERSECT', 'EXCEPT', 'DISTINCT', 'ALL', 'AS', 'AND', 'OR', 'NOT',
  'IN', 'BETWEEN', 'LIKE', 'ILIKE', 'IS NULL', 'IS NOT NULL', 'EXISTS', 'ANY', 'SOME',
  'CASE', 'WHEN', 'THEN', 'ELSE', 'END', 'CREATE TABLE', 'CREATE INDEX',
  'CREATE VIEW', 'CREATE OR REPLACE VIEW', 'CREATE DATABASE', 'CREATE SCHEMA',
  'ALTER TABLE', 'ALTER VIEW', 'ALTER DATABASE', 'DROP TABLE', 'DROP VIEW', 'DROP INDEX',
  'DROP DATABASE', 'DROP SCHEMA', 'TRUNCATE TABLE', 'RENAME TO', 'ADD COLUMN', 'DROP COLUMN',
  'PRIMARY KEY', 'FOREIGN KEY', 'REFERENCES', 'CHECK', 'UNIQUE', 'SET', 'VALUES',
  'RETURNING', 'DEFAULT', 'NULL', 'NOT NULL', 'AUTO_INCREMENT', 'GENERATED ALWAYS AS IDENTITY',
  'ASC', 'DESC', 'NULLS FIRST', 'NULLS LAST', 'CASCADE', 'RESTRICT', 'IF EXISTS',
  'IF NOT EXISTS', 'WITH', 'RECURSIVE', 'OVER', 'PARTITION BY', 'ROWS BETWEEN',
  'DATABASE', 'SCHEMA', 'TABLESPACE', 'SHOW TABLES', 'SHOW DATABASES', 'SHOW SCHEMAS',
  'DESCRIBE', 'EXPLAIN', 'EXPLAIN ANALYZE', 'ANALYZE', 'VACUUM', 'REINDEX',
  'BEGIN', 'COMMIT', 'ROLLBACK', 'SAVEPOINT', 'RELEASE SAVEPOINT', 'GRANT', 'REVOKE',
  'PRAGMA', 'ATTACH DATABASE', 'DETACH DATABASE', 'UPSERT', 'ON CONFLICT', 'ON DUPLICATE KEY UPDATE',
  'TRUE', 'FALSE', 'UNKNOWN',
];

export const SQL_FUNCTIONS = [
  'COUNT()', 'SUM()', 'AVG()', 'MIN()', 'MAX()', 'COALESCE()', 'NULLIF()', 'GREATEST()', 'LEAST()',
  'CONCAT()', 'CONCAT_WS()', 'GROUP_CONCAT()', 'STRING_AGG()', 'NOW()', 'CURRENT_TIMESTAMP()',
  'CURRENT_DATE()', 'CURRENT_TIME()', 'DATE()', 'TIME()', 'DATETIME()', 'DATE_FORMAT()', 'STRFTIME()',
  'DATE_TRUNC()', 'EXTRACT()', 'AGE()', 'TO_CHAR()', 'TO_DATE()', 'TO_TIMESTAMP()',
  'IFNULL()', 'IF()', 'IIF()', 'LOWER()', 'UPPER()', 'INITCAP()', 'SUBSTRING()', 'SUBSTR()',
  'LEFT()', 'RIGHT()', 'TRIM()', 'LTRIM()', 'RTRIM()', 'REPLACE()', 'REVERSE()', 'LPAD()', 'RPAD()',
  'ROUND()', 'CEIL()', 'CEILING()', 'FLOOR()', 'ABS()', 'MOD()', 'POWER()', 'SQRT()', 'RANDOM()',
  'LENGTH()', 'CHAR_LENGTH()', 'OCTET_LENGTH()', 'POSITION()', 'LOCATE()', 'INSTR()',
  'CAST()', 'CONVERT()', 'JSON_EXTRACT()', 'JSON_OBJECT()', 'JSON_ARRAY()', 'JSONB_BUILD_OBJECT()',
  'ROW_NUMBER()', 'RANK()', 'DENSE_RANK()', 'NTILE()', 'LAG()', 'LEAD()', 'FIRST_VALUE()', 'LAST_VALUE()',
];

export interface CompletionItem {
  label: string;
  kind: 'KW' | 'FN' | 'DB' | 'TBL' | 'COL' | 'SNP';
  detail?: string;
  apply?: string;
  snippet?: QuerySnippet;
  boost?: number;
}

export interface CompletionRequest {
  text: string;
  cursor: number;
  catalogs: Record<string, SchemaCatalog>;
  activeDatabase?: string;
  snippets?: QuerySnippet[];
  dialect?: 'mysql' | 'postgres' | 'sqlite';
  quoteIdentifier?: (identifier: string) => string;
  onDatabaseRequested?: (database: string) => void;
}

interface QualifiedReference { parts: string[]; trailingDot: boolean; start: number }
interface Relation { table: string; database?: string; alias?: string }

const identifier = '(?:"[^"]+"|`[^`]+`|\\[[^\\]]+\\]|[a-zA-Z0-9_$]+)';
const relationRegex = new RegExp(
  String.raw`\b(?:FROM|JOIN|UPDATE|INTO)\s+(${identifier}(?:\s*\.\s*${identifier}){0,2})(?:\s+(?:AS\s+)?(${identifier}))?`,
  'gi',
);
const reservedAliases = new Set([
  'ON', 'WHERE', 'JOIN', 'LEFT', 'RIGHT', 'INNER', 'OUTER', 'FULL', 'CROSS', 'NATURAL', 'USING',
  'GROUP', 'ORDER', 'HAVING', 'LIMIT', 'OFFSET', 'SET', 'VALUES', 'RETURNING', 'UNION', 'FOR',
  'WINDOW', 'FETCH', 'AS', 'SELECT', 'WHEN', 'THEN', 'ELSE', 'END',
]);

export function unquoteIdentifier(value: string): string {
  const trimmed = value.trim();
  if (trimmed.length >= 2) {
    const first = trimmed[0];
    const last = trimmed[trimmed.length - 1];
    if (first === '`' && last === '`') return trimmed.slice(1, -1).replace(/``/g, '`');
    if (first === '"' && last === '"') return trimmed.slice(1, -1).replace(/""/g, '"');
    if (first === '[' && last === ']') return trimmed.slice(1, -1);
  }
  return trimmed;
}

/** Parse an optionally quoted, whitespace-tolerant dotted identifier chain. */
export function parseQualifiedReference(text: string, cursor: number): QualifiedReference | null {
  const prefix = text.slice(0, Math.max(0, Math.min(cursor, text.length)));
  const chain = new RegExp(`(${identifier}(?:\\s*\\.\\s*(?:${identifier})?){1,2})$`).exec(prefix);
  if (!chain) return null;
  const raw = chain[1];
  const trailingDot = /\.\s*$/.test(raw);
  const parts = raw.match(new RegExp(identifier, 'g'))?.map(unquoteIdentifier) || [];
  if (!parts.length || (!trailingDot && parts.length < 2)) return null;
  return { parts, trailingDot, start: cursor - raw.length };
}

function relationsIn(statement: string, analyzedStatement: string): Relation[] {
  const relations: Relation[] = [];
  relationRegex.lastIndex = 0;
  let match: RegExpExecArray | null;
  while ((match = relationRegex.exec(statement))) {
    const keyword = match[0].match(/^[a-z]+/i)?.[0];
    if (keyword && !analyzedStatement.slice(match.index, match.index + match[0].length).toUpperCase().includes(keyword.toUpperCase())) continue;
    const parts = match[1].match(new RegExp(identifier, 'g'))?.map(unquoteIdentifier) || [];
    const table = parts.at(-1);
    if (!table) continue;
    const alias = match[2] ? unquoteIdentifier(match[2]) : undefined;
    relations.push({ table, database: parts.length > 1 ? parts.at(-2) : undefined, alias: alias && !reservedAliases.has(alias.toUpperCase()) ? alias : undefined });
  }
  return relations;
}

interface CatalogIndex {
  tables: NonNullable<SchemaCatalog['tables']>;
  byName: Map<string, NonNullable<SchemaCatalog['tables']>[number]>;
  /** All columns across all tables, sorted by lowercase label for prefix lookup. */
  columns: { label: string; lower: string; type: string; table: string }[];
}

const catalogIndexes = new WeakMap<SchemaCatalog, CatalogIndex>();

function getCatalogIndex(catalog: SchemaCatalog | undefined): CatalogIndex | undefined {
  if (!catalog) return undefined;
  const cached = catalogIndexes.get(catalog);
  if (cached) return cached;
  const tables = catalog.tables || [];
  const byName = new Map<string, (typeof tables)[number]>();
  const columns: CatalogIndex['columns'] = [];
  for (const table of tables) {
    byName.set(table.name.toLowerCase(), table);
    for (const column of (table.columns || []) as ({ name: string; type?: string } | string)[]) {
      const label = typeof column === 'string' ? column : column.name;
      const type = typeof column === 'string' ? '' : column.type || '';
      columns.push({ label, lower: label.toLowerCase(), type, table: table.name });
    }
  }
  columns.sort((a, b) => (a.lower < b.lower ? -1 : a.lower > b.lower ? 1 : 0));
  const index: CatalogIndex = { tables, byName, columns };
  catalogIndexes.set(catalog, index);
  return index;
}

function getTable(catalog: SchemaCatalog | undefined, name: string) {
  return getCatalogIndex(catalog)?.byName.get(name.toLowerCase());
}

/** Hard cap on catalog-derived candidates so typing stays O(1) in catalog size. */
const MAX_CATALOG_CANDIDATES = 250;
/** Legacy parity: the completion popup never shows more than 50 candidates. */
const MAX_CANDIDATES = 50;

interface CandidateBudget {
  remaining: number;
}

function columnItems(
  catalog: SchemaCatalog | undefined,
  tableName: string,
  quote: (s: string) => string,
  boost = 0,
  filter?: (label: string) => boolean,
  budget?: CandidateBudget,
): CompletionItem[] {
  const table = getTable(catalog, tableName);
  if (!table) return [];
  const items: CompletionItem[] = [];
  for (const column of (table.columns || []) as ({ name: string; type?: string } | string)[]) {
    const name = typeof column === 'string' ? column : column.name;
    if (filter && !filter(name)) continue;
    if (budget && budget.remaining <= 0) break;
    const type = typeof column === 'string' ? '' : column.type || '';
    items.push({ label: name, kind: 'COL', detail: type, apply: quote(name), boost });
    if (budget) budget.remaining--;
  }
  return items;
}

/** Columns across the whole catalog whose name contains the typed prefix. */
function catalogColumnItems(
  catalog: SchemaCatalog | undefined,
  quote: (s: string) => string,
  lower: string,
  boost: number,
  budget: CandidateBudget,
): CompletionItem[] {
  const index = getCatalogIndex(catalog);
  if (!index) return [];
  const items: CompletionItem[] = [];
  // Binary search the sorted column list for the first match, then scan forward.
  let low = 0;
  let high = index.columns.length;
  while (low < high) {
    const mid = (low + high) >> 1;
    if (index.columns[mid].lower < lower) low = mid + 1;
    else high = mid;
  }
  for (let i = low; i < index.columns.length; i++) {
    const column = index.columns[i];
    if (!column.lower.includes(lower)) {
      // Sorted order means prefix matches are contiguous; substring matches can appear later,
      // so keep scanning but only while the budget allows.
      if (budget.remaining <= 0) break;
      continue;
    }
    if (budget.remaining <= 0) break;
    items.push({ label: column.label, kind: 'COL', detail: column.type, apply: quote(column.label), boost });
    budget.remaining--;
  }
  return items;
}

function tableItems(
  catalog: SchemaCatalog | undefined,
  quote: (s: string) => string,
  filter?: (label: string) => boolean,
  budget?: CandidateBudget,
): CompletionItem[] {
  const items: CompletionItem[] = [];
  for (const table of catalog?.tables || []) {
    if (filter && !filter(table.name)) continue;
    if (budget && budget.remaining <= 0) break;
    items.push({
      label: table.name,
      kind: 'TBL',
      detail: table.type === 'VIEW' ? 'VIEW' : 'TABLE',
      apply: quote(table.name),
    });
    if (budget) budget.remaining--;
  }
  return items;
}

function tokenStart(text: string, cursor: number): number {
  let start = cursor;
  while (start > 0 && /[a-zA-Z0-9_$]/.test(text[start - 1])) start--;
  return start;
}

/** Pure, statement-scoped SQL completion engine shared by every Svelte editor. */
export function computeSqlCompletions(request: CompletionRequest): CompletionItem[] {
  const { text, cursor, catalogs } = request;
  if (!text || cursor < 0 || cursor > text.length) return [];
  const dialect = request.dialect || 'generic';
  if (!isCodePosition(text, cursor, dialect)) return [];
  const masked = maskSqlForAnalysis(text, dialect);
  // In a comment or quoted value, analysis retains whitespace but no typed word.
  const wordStart = tokenStart(text, cursor);
  if (cursor > wordStart && masked.slice(wordStart, cursor).trim() === '') return [];
  if (cursor === wordStart && cursor > 0 && !/[\s;()]/.test(masked[cursor - 1] || '')) {
    // Dot qualification is handled below; otherwise do not pop up inside literals/comments.
    if (masked[cursor - 1] !== '.') return [];
  }

  const activeDatabase = (request.activeDatabase || '').toLowerCase();
  const catalogKeys = Object.keys(catalogs);
  const activeCatalog = catalogs[activeDatabase] || (catalogKeys.length === 1 ? catalogs[catalogKeys[0]] : undefined);
  const quote = request.quoteIdentifier || ((identifier: string) => identifier);
  const statement = splitSqlStatements(text, dialect).find((s) => cursor >= s.start && cursor < s.end)
    || [...splitSqlStatements(text, dialect)].reverse().find((s) => cursor >= s.start)
    || { start: 0, end: text.length, text };
  const analyzedStatement = maskSqlForAnalysis(statement.text, dialect);
  const before = analyzedStatement.slice(0, Math.max(0, cursor - statement.start));
  const relations = relationsIn(statement.text, analyzedStatement);
  const relationCatalog = (database?: string) => {
    if (!database) return activeCatalog;
    const catalog = catalogs[database.toLowerCase()];
    if (!catalog) request.onDatabaseRequested?.(database);
    return catalog;
  };
  const qualified = parseQualifiedReference(text, cursor);

  if (qualified) {
    if (!masked.slice(qualified.start, cursor).trim()) return [];
    const { parts, trailingDot } = qualified;
    const dotCount = parts.length - (trailingDot ? 0 : 1);
    const prefix = trailingDot ? '' : (parts.at(-1) || '').toLowerCase();
    const matches = (label: string) => !prefix || label.toLowerCase().includes(prefix);
    const budget: CandidateBudget = { remaining: MAX_CATALOG_CANDIDATES };
    if (dotCount === 1) {
      const qualifier = parts[0];
      const catalog = catalogs[qualifier.toLowerCase()];
      if (catalog) return tableItems(catalog, quote, matches, budget);
      const relation = relations.find((item) => item.alias?.toLowerCase() === qualifier.toLowerCase() || (item.table.toLowerCase() === qualifier.toLowerCase() && !item.database));
      if (relation) return columnItems(relationCatalog(relation.database), relation.table, quote, 20, matches, budget);
      if (getTable(activeCatalog, qualifier)) return columnItems(activeCatalog, qualifier, quote, 20, matches, budget);
      request.onDatabaseRequested?.(qualifier);
      return [];
    }
    if (dotCount >= 2) {
      const database = trailingDot ? parts.at(-2)! : parts.at(-3)!;
      const table = trailingDot ? parts.at(-1)! : parts.at(-2)!;
      const catalog = relationCatalog(database);
      return catalog ? columnItems(catalog, table, quote, 20, matches, budget) : [];
    }
    return [];
  }

  const token = text.slice(wordStart, cursor);
  const lower = token.toLowerCase();
  const matches = (label: string) => !lower || label.toLowerCase().includes(lower);
  const unique = new Map<string, CompletionItem>();
  const add = (items: CompletionItem[]) => {
    for (const item of items) {
      const key = `${item.kind}:${item.label.toLowerCase()}`;
      if (!unique.has(key)) unique.set(key, item);
    }
  };

  const clause = before.trimStart().match(/^([a-z]+)/i)?.[1]?.toUpperCase() || '';
  const inFrom = /\b(FROM|JOIN|UPDATE|INTO)\s+[a-zA-Z0-9_$"`\[\].\s]*$/i.test(before);
  const tableVerb = new Set(['ALTER', 'DROP', 'TRUNCATE', 'DESCRIBE']).has(clause);
  const inDatabaseContext = /^(?:\s*)(?:USE|DROP\s+(?:DATABASE|SCHEMA)|CREATE\s+(?:DATABASE|SCHEMA)|ALTER\s+(?:DATABASE|SCHEMA))\s+[a-zA-Z0-9_$]*$/i.test(before);
  const keywordExclusions: Record<string, string[]> = {
    postgres: ['AUTO_INCREMENT', 'SHOW TABLES', 'SHOW DATABASES', 'SHOW SCHEMAS', 'ON DUPLICATE KEY UPDATE', 'UPSERT', 'PRAGMA', 'ATTACH DATABASE', 'DETACH DATABASE'],
    mysql: ['ILIKE', 'GENERATED ALWAYS AS IDENTITY', 'NULLS FIRST', 'NULLS LAST', 'FETCH FIRST'],
    sqlite: ['AUTO_INCREMENT', 'GENERATED ALWAYS AS IDENTITY', 'NULLS FIRST', 'NULLS LAST', 'SHOW TABLES', 'SHOW DATABASES', 'SHOW SCHEMAS', 'ON DUPLICATE KEY UPDATE', 'UPSERT', 'ILIKE'],
  };
  const keywordAdditions: Record<string, string[]> = {
    postgres: ['MATERIALIZED', 'LATERAL', 'WITH ORDINALITY', 'DO', 'CREATE EXTENSION', 'DROP EXTENSION', 'LISTEN', 'NOTIFY', 'UNLISTEN'],
    mysql: ['REPLACE INTO', 'SHOW COLUMNS', 'SHOW INDEX', 'LOCK TABLES', 'UNLOCK TABLES', 'DELIMITER'],
    sqlite: ['AUTOINCREMENT', 'ON CONFLICT DO NOTHING', 'ON CONFLICT DO UPDATE', 'STRICT'],
  };
  const excluded = new Set(keywordExclusions[dialect] || []);
  const keywords = [...SQL_KEYWORDS.filter((keyword) => !excluded.has(keyword)), ...(keywordAdditions[dialect] || [])];
  const functionExclusions: Record<string, string[]> = {
    postgres: ['IFNULL()', 'IF()', 'DATE_FORMAT()', 'GROUP_CONCAT()', 'STRFTIME()', 'INSTR()'],
    mysql: ['STRING_AGG()', 'DATE_TRUNC()', 'AGE()', 'TO_CHAR()', 'TO_DATE()', 'TO_TIMESTAMP()', 'JSONB_BUILD_OBJECT()', 'IIF()', 'INITCAP()'],
    sqlite: ['IF()', 'DATE_FORMAT()', 'GROUP_CONCAT()', 'STRING_AGG()', 'AGE()', 'TO_CHAR()', 'TO_DATE()', 'TO_TIMESTAMP()', 'JSONB_BUILD_OBJECT()', 'INITCAP()'],
  };
  const excludedFunctions = new Set(functionExclusions[dialect] || []);
  const functions = SQL_FUNCTIONS.filter((fn) => !excludedFunctions.has(fn));

  const databaseNames = new Map<string, string>();
  for (const [key, catalog] of Object.entries(catalogs)) {
    for (const name of catalog.databases || []) databaseNames.set(name.toLowerCase(), name);
    const name = catalog.database_name || key;
    if (name) databaseNames.set(name.toLowerCase(), name);
  }

  if (!inFrom && !tableVerb && !inDatabaseContext) {
    add(keywords.filter((label) => matches(label)).map((label) => ({ label, kind: 'KW', boost: 0 })));
    add(functions.filter((label) => matches(label)).map((label) => ({ label, kind: 'FN', detail: 'Function', boost: 4 })));
    const budget: CandidateBudget = { remaining: MAX_CATALOG_CANDIDATES };
    const seenTables = new Set<string>();
    for (const relation of relations) {
      const catalog = relationCatalog(relation.database);
      const key = `${relation.database || request.activeDatabase}:${relation.table}`.toLowerCase();
      if (!seenTables.has(key)) {
        add(columnItems(catalog, relation.table, quote, 18, matches, budget));
        seenTables.add(key);
      }
      if (relation.alias) unique.set(`COL:${relation.alias.toLowerCase()}`, { label: relation.alias, kind: 'COL', detail: `Alias → ${relation.table}`, boost: 19 });
    }
    // Unrelated tables contribute columns at a lower rank, but only while budget remains.
    // The whole-catalog scan uses the sorted index so a prefix never walks every column.
    if (relations.length === 0 && budget.remaining > 0) {
      if (lower) {
        add(catalogColumnItems(activeCatalog, quote, lower, 3, budget));
      } else {
        for (const table of activeCatalog?.tables || []) {
          if (budget.remaining <= 0) break;
          add(columnItems(activeCatalog, table.name, quote, 3, matches, budget));
        }
      }
    }
    for (const snippet of request.snippets || []) {
      if (matches(snippet.title)) {
        unique.set(`SNP:${snippet.id}`, { label: snippet.title, kind: 'SNP', detail: snippet.tags?.join(', ') || 'Template', snippet, boost: -4 });
      }
    }
  }
  if (inFrom || tableVerb) {
    const budget: CandidateBudget = { remaining: MAX_CATALOG_CANDIDATES };
    add(tableItems(activeCatalog, quote, matches, budget).map((item) => ({ ...item, boost: 25 })));
    for (const name of databaseNames.values()) {
      if (!matches(name)) continue;
      add([{ label: name, kind: 'DB', detail: 'Database', apply: quote(name), boost: 20 }]);
    }
  }
  if (inDatabaseContext) {
    return [...databaseNames.values()].filter((name) => !lower || name.toLowerCase().includes(lower))
      .map((label) => ({ label, kind: 'DB' as const, detail: 'Database', apply: quote(label), boost: 30 }));
  }

  const options = [...unique.values()];
  // ponytail: cap the popup payload like the legacy engine, but rank first so the most
  // relevant candidates survive the cut instead of whichever keyword happened to be first.
  if (options.length > MAX_CANDIDATES) {
    const rank = (item: CompletionItem) => (item.boost || 0)
      + (item.kind === 'COL' ? 12 : item.kind === 'TBL' ? 10 : item.kind === 'SNP' ? 6 : 0);
    options.sort((a, b) => rank(b) - rank(a) || a.label.localeCompare(b.label));
    return options.slice(0, MAX_CANDIDATES);
  }
  return options;
}

export function completionTokenStart(text: string, cursor: number): number {
  let start = cursor;
  while (start > 0 && /[a-zA-Z0-9_$]/.test(text[start - 1])) start--;
  // Multi-word keywords (LEFT JOIN, GROUP BY, ...) must replace the whole typed phrase.
  const phrase = text.slice(0, cursor).match(/\b([a-zA-Z]+(?:\s+[a-zA-Z]+)*)$/);
  if (phrase && phrase[1].split(/\s+/).length > 1) {
    const candidate = phrase[1].split(/\s+/)[0].toUpperCase();
    if (SQL_KEYWORDS.some((keyword) => keyword.startsWith(`${candidate} `))) {
      start = cursor - phrase[1].length;
    }
  }
  return start;
}
