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
  type: 'word' | 'string' | 'symbol';
  value: string;
}

export function formatSQL(sql: string): string {
  if (!sql || typeof sql !== 'string') return '';
  const trimmed = sql.trim();
  if (!trimmed) return '';

  const tokens: Token[] = [];
  let current = '';
  let inString = false;
  let stringChar = '';

  for (let i = 0; i < trimmed.length; i++) {
    const char = trimmed[i];
    if (inString) {
      current += char;
      if (char === stringChar && trimmed[i - 1] !== '\\') {
        inString = false;
        tokens.push({ type: 'string', value: current });
        current = '';
      }
    } else if (char === "'" || char === '"' || char === '`') {
      if (current.trim()) tokens.push({ type: 'word', value: current.trim() });
      current = char;
      inString = true;
      stringChar = char;
    } else if (char === '(' || char === ')' || char === ',' || char === ';') {
      if (current.trim()) tokens.push({ type: 'word', value: current.trim() });
      tokens.push({ type: 'symbol', value: char });
      current = '';
    } else if (/\s/.test(char)) {
      if (current.trim()) {
        tokens.push({ type: 'word', value: current.trim() });
        current = '';
      }
    } else {
      current += char;
    }
  }
  if (current.trim()) {
    tokens.push({ type: inString ? 'string' : 'word', value: current.trim() });
  }

  let result = '';
  let indentLevel = 0;
  const getIndent = () => '  '.repeat(indentLevel);

  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i];
    if (t.type === 'string') {
      result += t.value + ' ';
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
