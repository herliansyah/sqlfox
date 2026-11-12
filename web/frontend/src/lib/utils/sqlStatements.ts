export interface StatementPos {
  start: number;
  end: number;
  text: string;
}

export type SqlDialect = 'mysql' | 'postgres' | 'sqlite' | 'generic';

function dialectRules(dialect: SqlDialect) {
  return {
    hashComments: dialect === 'mysql' || dialect === 'generic',
    dollarQuotes: dialect === 'postgres' || dialect === 'generic',
    backslashEscapes: dialect === 'mysql' || dialect === 'generic',
    bracketQuotes: dialect === 'sqlite' || dialect === 'generic',
    backtickQuotes: dialect !== 'postgres',
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
      if (rules.backslashEscapes && (quote === "'" || quote === '"') && ch === '\\') {
        i++;
      } else if (ch === quote) {
        if (next === quote && quote !== ']') i++;
        else quote = '';
      }
      continue;
    }

    if ((ch === '-' && next === '-') || (ch === '#' && rules.hashComments)) {
      lineComment = true;
      if (ch === '-') i++;
    } else if (ch === '/' && next === '*') {
      blockComment = true;
      i++;
    } else if (ch === "'" || ch === '"' || (ch === '`' && rules.backtickQuotes) || (ch === '[' && rules.bracketQuotes)) {
      quote = ch === '[' ? ']' : ch;
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
export function maskSqlForAnalysis(text: string, dialect: SqlDialect = 'generic'): string {
  let result = '';
  const rules = dialectRules(dialect);
  let quote = '';
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
      keep(ch);
      if (rules.backslashEscapes && (quote === "'" || quote === '"') && ch === '\\' && next) {
        keep(next); i++;
      } else if (ch === quote) {
        if (next === quote && quote !== ']') { keep(next); i++; }
        else quote = '';
      }
      continue;
    }

    if ((ch === '-' && next === '-') || (ch === '#' && rules.hashComments)) {
      lineComment = true;
      keep(); if (ch === '-') { keep(); i++; }
    } else if (ch === '/' && next === '*') {
      blockComment = true;
      keep(); keep(); i++;
    } else if (ch === "'" || ch === '"' || (ch === '`' && rules.backtickQuotes) || (ch === '[' && rules.bracketQuotes)) {
      quote = ch === '[' ? ']' : ch;
      keep();
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
      if (rules.backslashEscapes && (quote === "'" || quote === '"') && ch === '\\') { i++; continue; }
      if (ch === quote) {
        if (next === quote && quote !== ']') i++;
        else quote = '';
      }
      continue;
    }

    if ((ch === '-' && next === '-') || (ch === '#' && rules.hashComments)) {
      lineComment = true;
      if (ch === '-') i++;
    } else if (ch === '/' && next === '*') {
      blockComment = true;
      i++;
    } else if (ch === "'" || ch === '"' || (ch === '`' && rules.backtickQuotes) || (ch === '[' && rules.bracketQuotes)) {
      quote = ch === '[' ? ']' : ch;
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
