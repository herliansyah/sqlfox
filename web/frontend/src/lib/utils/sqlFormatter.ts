/**
 * Pure Client-Side Zero-Dependency SQL Formatter
 * Ported from SqlFox Legacy Engine (04_editor_tabs.js)
 */

const MAJOR_CLAUSES = [
  'SELECT',
  'FROM',
  'WHERE',
  'HAVING',
  'GROUP BY',
  'ORDER BY',
  'LIMIT',
  'OFFSET',
  'UNION ALL',
  'UNION',
  'INSERT INTO',
  'VALUES',
  'UPDATE',
  'SET',
  'DELETE FROM',
  'LEFT JOIN',
  'RIGHT JOIN',
  'INNER JOIN',
  'CROSS JOIN',
  'FULL JOIN',
  'JOIN',
  'ON',
  'CREATE TABLE',
  'ALTER TABLE',
  'DROP TABLE',
  'CREATE VIEW',
  'ALTER VIEW',
  'DROP VIEW',
  'CREATE TRIGGER',
  'DROP TRIGGER',
];

const STANDARD_SQL_KEYWORDS = [
  'SELECT', 'FROM', 'WHERE', 'AND', 'OR', 'NOT', 'IN', 'IS', 'NULL', 'LIKE',
  'AS', 'ON', 'JOIN', 'LEFT', 'RIGHT', 'INNER', 'OUTER', 'CROSS', 'GROUP',
  'BY', 'ORDER', 'ASC', 'DESC', 'LIMIT', 'OFFSET', 'HAVING', 'UNION', 'ALL',
  'INSERT', 'INTO', 'VALUES', 'UPDATE', 'SET', 'DELETE', 'CREATE', 'ALTER',
  'DROP', 'TABLE', 'VIEW', 'INDEX', 'TRIGGER', 'DATABASE', 'SCHEMA',
  'PRIMARY', 'KEY', 'FOREIGN', 'REFERENCES', 'CHECK', 'DEFAULT', 'UNIQUE',
  'AUTO_INCREMENT', 'CASCADE', 'RESTRICT', 'ENGINE', 'CHARSET', 'COLLATE',
  'COUNT', 'SUM', 'AVG', 'MIN', 'MAX', 'COALESCE', 'DISTINCT', 'CASE', 'WHEN',
  'THEN', 'ELSE', 'END', 'EXISTS', 'BETWEEN', 'TRUE', 'FALSE'
];

interface Token {
  type: 'word' | 'string' | 'comment' | 'symbol';
  value: string;
}

export function formatSQL(sql: string): string {
  if (!sql || typeof sql !== 'string') return '';
  const trimmed = sql.trim();
  if (!trimmed) return '';

  const tokens: Token[] = [];
  let current = '';

  const flushWord = () => {
    if (current.trim()) {
      tokens.push({ type: 'word', value: current.trim() });
    }
    current = '';
  };

  for (let i = 0; i < trimmed.length; i++) {
    const char = trimmed[i];
    const next = trimmed[i + 1] || '';

    // Line comments (-- or #)
    if ((char === '-' && next === '-') || char === '#') {
      flushWord();
      let end = trimmed.indexOf('\n', i);
      if (end === -1) end = trimmed.length;
      tokens.push({ type: 'comment', value: trimmed.slice(i, end).trimEnd() });
      i = end;
      continue;
    }

    // Block comments (/* ... */)
    if (char === '/' && next === '*') {
      flushWord();
      const closeIdx = trimmed.indexOf('*/', i + 2);
      const end = closeIdx === -1 ? trimmed.length : closeIdx + 2;
      tokens.push({ type: 'comment', value: trimmed.slice(i, end) });
      i = end - 1;
      continue;
    }

    // PostgreSQL dollar-quoted strings ($tag$...$tag$)
    if (char === '$') {
      const tag = trimmed.slice(i).match(/^\$[a-zA-Z0-9_]*\$/)?.[0];
      if (tag) {
        flushWord();
        const closeIdx = trimmed.indexOf(tag, i + tag.length);
        const end = closeIdx === -1 ? trimmed.length : closeIdx + tag.length;
        tokens.push({ type: 'string', value: trimmed.slice(i, end) });
        i = end - 1;
        continue;
      }
    }

    // Quoted strings and identifiers ('...', "...", `...`)
    if (char === "'" || char === '"' || char === '`') {
      flushWord();
      const quoteChar = char;
      let literal = quoteChar;
      i++;
      while (i < trimmed.length) {
        const c = trimmed[i];
        literal += c;
        if ((quoteChar === "'" || quoteChar === '"') && c === '\\' && i + 1 < trimmed.length) {
          i++;
          literal += trimmed[i];
        } else if (c === quoteChar) {
          if (trimmed[i + 1] === quoteChar) {
            i++;
            literal += trimmed[i];
          } else {
            break;
          }
        }
        i++;
      }
      tokens.push({ type: 'string', value: literal });
      continue;
    }

    if (char === '(' || char === ')' || char === ',' || char === ';') {
      flushWord();
      tokens.push({ type: 'symbol', value: char });
    } else if (/\s/.test(char)) {
      flushWord();
    } else {
      current += char;
    }
  }
  flushWord();

  let result = '';
  let indentLevel = 0;
  const getIndent = () => '  '.repeat(indentLevel);

  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i];
    if (t.type === 'string') {
      result += t.value + ' ';
      continue;
    }
    if (t.type === 'comment') {
      if (result.trim() && !result.endsWith('\n')) result += '\n';
      result += getIndent() + t.value + '\n';
      continue;
    }

    const upper = t.value.toUpperCase();
    let matchedClause = '';

    if (i + 1 < tokens.length && tokens[i + 1].type === 'word') {
      const pair = upper + ' ' + tokens[i + 1].value.toUpperCase();
      if (MAJOR_CLAUSES.includes(pair)) {
        matchedClause = pair;
        i++;
      }
    }
    if (!matchedClause && MAJOR_CLAUSES.includes(upper)) {
      matchedClause = upper;
    }

    if (matchedClause) {
      if (matchedClause === 'ON') {
        result += 'ON ';
      } else {
        if (result.trim() && !result.endsWith('\n')) result += '\n';
        result += getIndent() + matchedClause + ' ';
      }
    } else if (upper === 'AND' || upper === 'OR') {
      result += '\n' + getIndent() + '  ' + upper + ' ';
    } else if (t.value === '(') {
      result += ' (';
      indentLevel++;
    } else if (t.value === ')') {
      if (indentLevel > 0) indentLevel--;
      result = result.trimEnd() + ') ';
    } else if (t.value === ',') {
      result = result.trimEnd() + ',\n' + getIndent() + '  ';
    } else if (t.value === ';') {
      result = result.trimEnd() + ';\n\n';
      indentLevel = 0;
    } else {
      if (STANDARD_SQL_KEYWORDS.includes(upper)) {
        result += upper + ' ';
      } else {
        result += t.value + ' ';
      }
    }
  }

  return result.trim();
}
