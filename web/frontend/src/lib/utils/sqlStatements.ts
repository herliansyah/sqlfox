export interface StatementPos {
  start: number;
  end: number;
  text: string;
}

export type SqlDialect = 'mysql' | 'postgres' | 'sqlite' | 'postgresql' | 'generic';

export function quoteSqlIdentifier(name: string, driver?: string): string {
  const d = (driver || '').toLowerCase();
  if (d === 'postgres' || d === 'postgresql' || d === 'sqlite') {
    return `"${name.replace(/"/g, '""')}"`;
  }
  return `\`${name.replace(/`/g, '``')}\``;
}

export function formatSqlLiteral(val: unknown): string {
  if (val === null || val === undefined) return 'NULL';
  if (typeof val === 'number') return Number.isFinite(val) ? String(val) : 'NULL';
  if (typeof val === 'boolean') return val ? 'TRUE' : 'FALSE';
  if (typeof val === 'object') return `'${JSON.stringify(val).replace(/'/g, "''")}'`;
  return `'${String(val).replace(/'/g, "''")}'`;
}

function dialectRules(dialect: SqlDialect) {
  const isPg = dialect === 'postgres' || dialect === 'postgresql';
  return {
    hashComments: dialect === 'mysql' || dialect === 'generic',
    dollarQuotes: isPg || dialect === 'generic',
    backslashEscapes: dialect === 'mysql' || dialect === 'generic',
    bracketQuotes: dialect === 'sqlite',
    backtickQuotes: !isPg,
  };
}

function trimStatement(text: string, start: number, end: number): StatementPos {
  while (start < end && /\s/.test(text[start])) start++;
  while (end > start && /\s/.test(text[end - 1])) end--;
  return { start, end, text: text.slice(start, end) };
}

/** Split only on executable semicolons, preserving quoted bodies and comments. */
export function splitSqlStatements(text: string, dialect: SqlDialect = 'generic'): StatementPos[] {
  if (!text) return [];
  const rules = dialectRules(dialect);
  const statements: StatementPos[] = [];
  let quote = '';
  let escapeString = false;
  let lineComment = false;
  let blockComment = false;
  let dollarTag = '';
  let start = 0;

  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    const next = text[i + 1] || '';

    if (lineComment) {
      if (ch === '\n' || ch === '\r') lineComment = false;
      continue;
    }
    if (blockComment) {
      if (ch === '*' && next === '/') {
        blockComment = false;
        i++;
      }
      continue;
    }
    if (dollarTag) {
      if (ch === '$' && text.startsWith(dollarTag, i)) {
        i += dollarTag.length - 1;
        dollarTag = '';
      }
      continue;
    }
    if (quote) {
      if ((rules.backslashEscapes || escapeString) && (quote === "'" || quote === '"') && ch === '\\') {
        i++;
      } else if (ch === quote) {
        if (next === quote && quote !== ']') i++;
        else { quote = ''; escapeString = false; }
      }
      continue;
    }

    if ((ch === '-' && next === '-') || (ch === '#' && rules.hashComments)) {
      lineComment = true;
      if (ch === '-') i++;
    } else if (ch === '/' && next === '*') {
      blockComment = true;
      i++;
    } else if ((ch === 'e' || ch === 'E') && next === "'" && !/[a-zA-Z0-9_$]/.test(text[i - 1] || '')) {
      quote = "'";
      escapeString = true;
      i++;
    } else if (ch === "'" || ch === '"' || (ch === '`' && rules.backtickQuotes) || (ch === '[' && rules.bracketQuotes)) {
      quote = ch === '[' ? ']' : ch;
      escapeString = false;
    } else if (ch === '$' && rules.dollarQuotes) {
      const tag = text.slice(i).match(/^\$[a-zA-Z0-9_]*\$/)?.[0];
      if (tag) {
        dollarTag = tag;
        i += tag.length - 1;
      }
    } else if (ch === ';') {
      statements.push(trimStatement(text, start, i + 1));
      start = i + 1;
    }
  }

  if (start < text.length) statements.push(trimStatement(text, start, text.length));
  return statements;
}

/** Remove comments and literals for clause detection / empty-statement checks. */
export function maskSqlForAnalysis(text: string, dialect: SqlDialect = 'generic', keepIdentifiers = false): string {
  let result = '';
  const rules = dialectRules(dialect);
  let quote = '';
  let escapeString = false;
  let lineComment = false;
  let blockComment = false;
  let dollarTag = '';

  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    const next = text[i + 1] || '';
    const keep = (value = ' ') => { result += value === '\n' || value === '\r' ? value : ' '; };

    if (lineComment) {
      if (ch === '\n' || ch === '\r') {
        lineComment = false;
        result += ch;
      } else keep();
      continue;
    }
    if (blockComment) {
      if (ch === '*' && next === '/') {
        keep(); keep(); i++;
        blockComment = false;
      } else keep(ch);
      continue;
    }
    if (dollarTag) {
      if (ch === '$' && text.startsWith(dollarTag, i)) {
        for (let j = 0; j < dollarTag.length; j++) keep();
        i += dollarTag.length - 1;
        dollarTag = '';
      } else keep(ch);
      continue;
    }
    if (quote) {
      if (keepIdentifiers && (quote === '`' || quote === '"' || quote === ']')) {
        result += ch === '\n' || ch === '\r' ? ch : '_';
      } else {
        keep(ch);
      }
      if ((rules.backslashEscapes || escapeString) && (quote === "'" || quote === '"') && ch === '\\' && next) {
        if (keepIdentifiers && quote === '"') result += '_';
        else keep(next);
        i++;
      } else if (ch === quote) {
        if (next === quote && quote !== ']') {
          if (keepIdentifiers && (quote === '`' || quote === '"')) result += '_';
          else keep(next);
          i++;
        } else { quote = ''; escapeString = false; }
      }
      continue;
    }

    if ((ch === '-' && next === '-') || (ch === '#' && rules.hashComments)) {
      lineComment = true;
      keep(); if (ch === '-') { keep(); i++; }
    } else if (ch === '/' && next === '*') {
      blockComment = true;
      keep(); keep(); i++;
    } else if ((ch === 'e' || ch === 'E') && next === "'" && !/[a-zA-Z0-9_$]/.test(text[i - 1] || '')) {
      quote = "'";
      escapeString = true;
      keep(); keep(); i++;
    } else if (ch === "'" || ch === '"' || (ch === '`' && rules.backtickQuotes) || (ch === '[' && rules.bracketQuotes)) {
      quote = ch === '[' ? ']' : ch;
      escapeString = false;
      if (keepIdentifiers && (quote === '`' || quote === '"' || quote === ']')) result += '_';
      else keep();
    } else if (ch === '$' && rules.dollarQuotes) {
      const tag = text.slice(i).match(/^\$[a-zA-Z0-9_]*\$/)?.[0];
      if (tag) {
        dollarTag = tag;
        for (let j = 0; j < tag.length; j++) keep();
        i += tag.length - 1;
      } else result += ch;
    } else result += ch;
  }
  return result;
}

/** True when the cursor sits in executable code (not inside a literal or comment). */
export function isCodePosition(text: string, cursor: number, dialect: SqlDialect = 'generic'): boolean {
  const rules = dialectRules(dialect);
  let quote = '';
  let escapeString = false;
  let lineComment = false;
  let blockComment = false;
  let dollarTag = '';

  for (let i = 0; i < cursor && i < text.length; i++) {
    const ch = text[i];
    const next = text[i + 1] || '';

    if (lineComment) {
      if (ch === '\n' || ch === '\r') lineComment = false;
      continue;
    }
    if (blockComment) {
      if (ch === '*' && next === '/') { blockComment = false; i++; }
      continue;
    }
    if (dollarTag) {
      if (ch === '$' && text.startsWith(dollarTag, i)) { i += dollarTag.length - 1; dollarTag = ''; }
      continue;
    }
    if (quote) {
      if ((rules.backslashEscapes || escapeString) && (quote === "'" || quote === '"') && ch === '\\') { i++; continue; }
      if (ch === quote) {
        if (next === quote && quote !== ']') i++;
        else { quote = ''; escapeString = false; }
      }
      continue;
    }

    if ((ch === '-' && next === '-') || (ch === '#' && rules.hashComments)) {
      lineComment = true;
      if (ch === '-') i++;
    } else if (ch === '/' && next === '*') {
      blockComment = true;
      i++;
    } else if ((ch === 'e' || ch === 'E') && next === "'" && !/[a-zA-Z0-9_$]/.test(text[i - 1] || '')) {
      quote = "'";
      escapeString = true;
      i++;
    } else if (ch === "'" || ch === '"' || (ch === '`' && rules.backtickQuotes) || (ch === '[' && rules.bracketQuotes)) {
      quote = ch === '[' ? ']' : ch;
      escapeString = false;
    } else if (ch === '$' && rules.dollarQuotes) {
      const tag = text.slice(i).match(/^\$[a-zA-Z0-9_]*\$/)?.[0];
      if (tag) { dollarTag = tag; i += tag.length - 1; }
    }
  }

  return !lineComment && !blockComment && !dollarTag && !quote;
}

export function statementAtCursor(text: string, cursor: number, dialect: SqlDialect = 'generic'): StatementPos {
  const all = splitSqlStatements(text, dialect);
  const valid = all.filter((s) => maskSqlForAnalysis(s.text, dialect).replace(/;/g, '').trim().length > 0);
  if (valid.length === 0) return trimStatement(text, 0, text.length);
  if (valid.length === 1) return valid[0];

  const pos = Math.max(0, Math.min(cursor, text.length));
  return valid.find((s) => pos >= s.start && pos < s.end)
    || [...valid].reverse().find((s) => pos >= s.start)
    || valid[0];
}

export function executionTarget(text: string, from: number, to: number, dialect: SqlDialect = 'generic'): StatementPos {
  if (from !== to) {
    const selected = trimStatement(text, Math.min(from, to), Math.max(from, to));
    if (selected.text.trim()) return selected;
  }
  return statementAtCursor(text, Math.min(from, to), dialect);
}

/**
 * Extracts single backing table (plus optional explicit database qualifier) from a simple
 * `SELECT ... FROM [db.]table ...` query without JOIN/UNION so single-table ad-hoc queries
 * can support inline grid edits and PK/FK resolution just like legacy.
 */
export function extractSingleTableFromSQL(sql: string): { table: string; database?: string } | null {
  if (!sql) return null;
  const masked = maskSqlForAnalysis(sql, 'generic', false).trim();
  if (!/^(?:SELECT|WITH)\b/i.test(masked)) return null;
  if (/\b(?:JOIN|UNION|INTERSECT|EXCEPT)\b/i.test(masked)) return null;

  // Match FROM [db.]table on the original SQL using unmasked identifier pattern.
  const cleanSql = sql.replace(/--[^\r\n]*/g, ' ').replace(/\/\*[\s\S]*?\*\//g, ' ');
  const ident = '(?:`([^`]+)`|"([^"]+)"|\\[([^\\]]+)\\]|([a-zA-Z_][a-zA-Z0-9_$]*))';
  const fromMatch = new RegExp(`\\bFROM\\s+(?:${ident}\\s*\\.\\s*)?${ident}(?:\\s+(?:AS\\s+)?[a-zA-Z_][a-zA-Z0-9_$]*)?(?:\\s*(?:WHERE|ORDER|GROUP|HAVING|LIMIT|OFFSET|FOR|;|$))`, 'i').exec(cleanSql);
  if (!fromMatch) return null;

  const db = fromMatch[1] || fromMatch[2] || fromMatch[3] || fromMatch[4];
  const tbl = fromMatch[5] || fromMatch[6] || fromMatch[7] || fromMatch[8];
  if (!tbl) return null;
  // Ensure there isn't a comma join right after the matched table name
  const afterFrom = cleanSql.slice(fromMatch.index + 4).trim();
  if (/^[^\s,;]+(?:\s*\.\s*[^\s,;]+)?(?:\s+(?:AS\s+)?[a-zA-Z0-9_$]+)?\s*,/i.test(afterFrom)) {
    return null;
  }
  return db ? { table: tbl, database: db } : { table: tbl };
}

export function hasExplicitSQLLimit(sql: string): boolean {
  if (!sql) return false;
  const masked = maskSqlForAnalysis(sql, 'generic', false);
  // Strip parenthesized subqueries so inner LIMITs don't count as outer LIMIT
  let depth = 0;
  let top = '';
  for (let i = 0; i < masked.length; i++) {
    const ch = masked[i];
    if (ch === '(') depth++;
    else if (ch === ')') { if (depth > 0) depth--; }
    else if (depth === 0) top += ch;
  }
  return /\b(?:LIMIT\s+\d+|FETCH\s+(?:FIRST|NEXT)\b)/i.test(top);
}

/**
 * Wraps an arbitrary SELECT (including CTE `WITH ...`) for 5k batch streaming pagination.
 * MySQL forbids `WITH` inside a derived subquery, so leading CTE definitions are hoisted above.
 */
export function wrapPagedSQL(rawSql: string, limit: number, offset: number): string {
  const clean = rawSql.trim().replace(/;+\s*$/, '');
  if (/^\s*WITH\b/i.test(clean)) {
    let depth = 0;
    let inQuote = '';
    let splitIdx = -1;
    for (let i = 0; i < clean.length; i++) {
      const ch = clean[i];
      if (inQuote) {
        if (ch === '\\') { i++; continue; }
        if (ch === inQuote) inQuote = '';
        continue;
      }
      if (ch === "'" || ch === '"' || ch === '`') { inQuote = ch; continue; }
      if (ch === '(') depth++;
      else if (ch === ')') { if (depth > 0) depth--; }
      else if (depth === 0 && /^SELECT\b/i.test(clean.slice(i))) {
        splitIdx = i;
        break;
      }
    }
    if (splitIdx > 0) {
      const cteHeader = clean.slice(0, splitIdx).trim();
      const mainSelect = clean.slice(splitIdx).trim();
      return `${cteHeader}\nSELECT * FROM (\n${mainSelect}\n) AS _p LIMIT ${limit} OFFSET ${offset};`;
    }
  }
  return `SELECT * FROM (\n${clean}\n) AS _p LIMIT ${limit} OFFSET ${offset};`;
}

/** Strips MySQL `DEFINER=...` / `SQL SECURITY ...` clauses from CREATE VIEW/TRIGGER/ROUTINE/EVENT DDL. */
export function stripDefiner(ddl: string): string {
  return String(ddl || '')
    .replace(/\bDEFINER\s*=\s*(?:`[^`]*`|'[^']*'|"[^"]*"|[^\s@]+)(?:@(?:`[^`]*`|'[^']*'|"[^"]*"|[^\s]+))?\s+/gi, '')
    .trim();
}

/** Splits comma-separated items while respecting nested parentheses and quotes. */
export function splitTopLevelList(input: string): string[] {
  const res: string[] = [];
  let cur = '';
  let depth = 0;
  let q = '';
  for (let i = 0; i < input.length; i++) {
    const ch = input[i];
    if (q) {
      cur += ch;
      if (ch === '\\' && i + 1 < input.length) { cur += input[++i]; continue; }
      if (ch === q) q = '';
      continue;
    }
    if (ch === "'" || ch === '"' || ch === '`') { q = ch; cur += ch; continue; }
    if (ch === '(') depth++;
    else if (ch === ')' && depth > 0) depth--;
    else if (ch === ',' && depth === 0) {
      if (cur.trim()) res.push(cur.trim());
      cur = '';
      continue;
    }
    cur += ch;
  }
  if (cur.trim()) res.push(cur.trim());
  return res;
}
