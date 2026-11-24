import type { FilterPill } from '../../types';

const STRICT_NUMBER_RE = /^-?(0|[1-9]\d*)(\.\d+)?$/;

function isStrictNumericLiteral(val: string): boolean {
  return STRICT_NUMBER_RE.test(val);
}

/**
 * Compiles structured UI filter pills into a driver-aware SQL WHERE predicate.
 * All literal values (including IN/NOT IN lists and BETWEEN bounds) are quoted
 * or strictly validated as numeric literals to prevent SQL injection.
 */
export function compileFilterPills(pills: FilterPill[], driver?: string): string {
  if (!Array.isArray(pills) || pills.length === 0) return '';
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const clauses: string[] = [];

  for (const p of pills) {
    if (!p || !p.col) continue;
    const cleanCol = p.col.replace(/^[`"\[]+|[`"\]]+$/g, '');
    const colEscaped = isPg
      ? `"${cleanCol.replace(/"/g, '""')}"`
      : `\`${cleanCol.replace(/`/g, '``')}\``;
    const op = (p.op || '=').toUpperCase();

    if (op === 'IS NULL' || op === 'IS NOT NULL') {
      clauses.push(`${colEscaped} ${op}`);
      continue;
    }

    const val = String(p.val !== undefined ? p.val : '').trim();
    if (val === '') continue;

    if (op === 'IN' || op === 'NOT IN') {
      const inner = val.startsWith('(') && val.endsWith(')') ? val.slice(1, -1) : val;
      const items = inner.split(',').map((s) => s.trim()).filter(Boolean);
      const formatted = items
        .map((item) => {
          if (isStrictNumericLiteral(item)) return item;
          const unquoted = item.replace(/^'(.*)'$/s, '$1');
          return `'${unquoted.replace(/'/g, "''")}'`;
        })
        .join(', ');
      if (formatted !== '') {
        clauses.push(`${colEscaped} ${op} (${formatted})`);
      }
    } else if (op === 'LIKE' || op === 'NOT LIKE') {
      const finalVal = !val.includes('%') && !val.includes('_') ? `%${val}%` : val;
      clauses.push(`${colEscaped} ${op} '${finalVal.replace(/'/g, "''")}'`);
    } else if (op === 'BETWEEN') {
      const parts = val.split(/\s+AND\s+/i).slice(0, 2);
      const fmt = (part: string) => {
        const pStr = part.trim().replace(/^'(.*)'$/s, '$1');
        return isStrictNumericLiteral(pStr) ? pStr : `'${pStr.replace(/'/g, "''")}'`;
      };
      if (parts.length === 2 && parts[0].trim() !== '' && parts[1].trim() !== '') {
        clauses.push(`${colEscaped} BETWEEN ${fmt(parts[0])} AND ${fmt(parts[1])}`);
      } else if (parts[0] && parts[0].trim() !== '') {
        clauses.push(`${colEscaped} >= ${fmt(parts[0])}`);
      }
    } else {
      if (isStrictNumericLiteral(val)) {
        clauses.push(`${colEscaped} ${op} ${val}`);
      } else {
        clauses.push(`${colEscaped} ${op} '${val.replace(/'/g, "''")}'`);
      }
    }
  }

  return clauses.join(' AND ');
}
