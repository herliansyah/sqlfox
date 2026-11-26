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

export interface ColumnWindow {
  startColIdx: number;
  endColIdx: number;
  leftSpacerWidth: number;
  rightSpacerWidth: number;
  totalWidth: number;
  colOffsets: number[];
}

/**
 * Compute the horizontally visible slice of columns and left/right spacer widths
 * for table-fixed column virtualization.
 */
export function computeVisibleColumnWindow(
  colNames: string[],
  columnWidths: Record<string, number>,
  scrollLeft: number,
  containerWidth: number,
  gutterWidth = 64,
  overscan = 4
): ColumnWindow {
  const count = colNames.length;
  const colOffsets = new Array<number>(count + 1);
  colOffsets[0] = 0;
  for (let i = 0; i < count; i++) {
    const w = columnWidths[colNames[i]] || 160;
    colOffsets[i + 1] = colOffsets[i] + w;
  }
  const totalColsWidth = colOffsets[count];
  if (count === 0) {
    return {
      startColIdx: 0,
      endColIdx: 0,
      leftSpacerWidth: 0,
      rightSpacerWidth: 0,
      totalWidth: gutterWidth,
      colOffsets,
    };
  }

  const viewLeft = Math.max(0, scrollLeft);
  const viewRight = viewLeft + Math.max(200, containerWidth - gutterWidth);

  let rawStart = 0;
  while (rawStart < count && colOffsets[rawStart + 1] <= viewLeft) {
    rawStart++;
  }
  let rawEnd = rawStart;
  while (rawEnd < count && colOffsets[rawEnd] < viewRight) {
    rawEnd++;
  }

  const startColIdx = Math.max(0, rawStart - overscan);
  const endColIdx = Math.min(count, rawEnd + overscan);
  const leftSpacerWidth = colOffsets[startColIdx];
  const rightSpacerWidth = Math.max(0, totalColsWidth - colOffsets[endColIdx]);

  return {
    startColIdx,
    endColIdx,
    leftSpacerWidth,
    rightSpacerWidth,
    totalWidth: gutterWidth + totalColsWidth,
    colOffsets,
  };
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
