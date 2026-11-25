import type { FilterPill } from '../../../types';

export interface ColumnWidthEntry {
  colName: string;
  colType: string;
  origIdx: number;
}

export function computeColumnWidths(
  entries: ColumnWidthEntry[],
  rows: any[][]
): Record<string, number> {
  const widths: Record<string, number> = {};
  const sampleCount = Math.min(rows.length, 40);
  for (const entry of entries) {
    let maxChars = entry.colName.length + (entry.colType ? entry.colType.length + 3 : 0);
    for (let i = 0; i < sampleCount; i++) {
      const v = rows[i]?.[entry.origIdx];
      if (v !== null && v !== undefined) {
        const len = typeof v === 'string' ? v.length : String(v).length;
        if (len > maxChars) maxChars = len;
      }
    }
    widths[entry.colName] = Math.min(340, Math.max(110, maxChars * 8 + 32));
  }
  return widths;
}

export function matchesFilterPill(cellVal: any, pill: FilterPill): boolean {
  if (pill.op === 'IS NULL') return cellVal === null || cellVal === undefined;
  if (pill.op === 'IS NOT NULL') return cellVal !== null && cellVal !== undefined;
  if (String(pill.val ?? '').trim() === '') return true;
  if (cellVal === null || cellVal === undefined) return false;
  const str = typeof cellVal === 'object' ? JSON.stringify(cellVal) : String(cellVal);
  const valStr = String(pill.val ?? '');
  switch (pill.op) {
    case '=':
      return str === valStr;
    case '!=':
      return str !== valStr;
    case 'LIKE': {
      const needle = valStr.replace(/%/g, '').toLowerCase();
      return str.toLowerCase().includes(needle);
    }
    case 'NOT LIKE': {
      const needle = valStr.replace(/%/g, '').toLowerCase();
      return !str.toLowerCase().includes(needle);
    }
    case '>':
    case '<':
    case '>=':
    case '<=': {
      const a = Number(cellVal);
      const b = Number(valStr);
      const cmp = Number.isFinite(a) && Number.isFinite(b) ? a - b : str.localeCompare(valStr);
      if (pill.op === '>') return cmp > 0;
      if (pill.op === '<') return cmp < 0;
      if (pill.op === '>=') return cmp >= 0;
      return cmp <= 0;
    }
    case 'IN': {
      const parts = valStr.split(',').map((s) => s.trim().replace(/^['"]|['"]$/g, ''));
      return parts.includes(str);
    }
    case 'NOT IN': {
      const parts = valStr.split(',').map((s) => s.trim().replace(/^['"]|['"]$/g, ''));
      return !parts.includes(str);
    }
    case 'BETWEEN': {
      const parts = valStr.split(/\s+AND\s+|,/i).map((s) => s.trim());
      if (parts.length < 2) return true;
      const num = Number(str);
      const low = Number(parts[0]);
      const high = Number(parts[1]);
      if (
        str !== '' &&
        parts[0] !== '' &&
        parts[1] !== '' &&
        Number.isFinite(num) &&
        Number.isFinite(low) &&
        Number.isFinite(high)
      ) {
        return num >= low && num <= high;
      }
      return str >= parts[0] && str <= parts[1];
    }
    default:
      return true;
  }
}
