import { quoteSqlIdentifier, formatSqlLiteral } from '../../utils/sqlStatements';

export function formatCellValue(val: any): { display: string; type: string } {
  if (val === null || val === undefined) {
    return { display: 'NULL', type: 'null' };
  }
  if (typeof val === 'boolean') {
    return { display: val ? 'TRUE' : 'FALSE', type: 'boolean' };
  }
  if (typeof val === 'number') {
    // Preserve exact numeric precision (never use toLocaleString which truncates decimals to 3 digits and adds commas to IDs).
    return { display: String(val), type: 'number' };
  }
  if (typeof val === 'object') {
    try {
      return { display: JSON.stringify(val), type: 'json' };
    } catch {
      return { display: String(val), type: 'text' };
    }
  }
  return { display: String(val), type: 'text' };
}

export function getCellClass(type: string): string {
  switch (type) {
    case 'null':
      return 'text-rose-400/80 italic font-medium';
    case 'number':
      return 'text-amber-300 font-mono text-right';
    case 'boolean':
      return 'text-emerald-400 font-semibold';
    case 'json':
      return 'text-purple-300 font-mono';
    default:
      return 'text-slate-200 font-mono';
  }
}

export function formatRowCell(v: any): string {
  if (v === null || v === undefined) return '';
  return typeof v === 'object' ? JSON.stringify(v) : String(v);
}

export function formatSqlRowInsert(row: any[], cols: string[], tbl: string, driver?: string): string {
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const q = (s: string) => quoteSqlIdentifier(s, driver);
  const qCols = cols.map((c) => q(c)).join(', ');
  const vals = row
    .map((c) => (typeof c === 'boolean' ? (c ? (isPg ? 'TRUE' : '1') : (isPg ? 'FALSE' : '0')) : formatSqlLiteral(c)))
    .join(', ');
  return `INSERT INTO ${q(tbl)} (${qCols}) VALUES (${vals});`;
}

function sanitizeCSVCell(cell: any): string {
  if (cell === null || cell === undefined) return '""';
  const str = typeof cell === 'object' ? JSON.stringify(cell) : String(cell);
  // Neutralize spreadsheet formula injection (=, +, -, @, tab, CR) on non-numeric strings
  // and preserve leading-zero identifiers (ZIP codes, phone numbers) without formula syntax.
  if (typeof cell !== 'number' && (/^[=+\-@\t\r]/.test(str) || /^0\d+$/.test(str))) {
    return `"'${str.replace(/"/g, '""')}"`;
  }
  return `"${str.replace(/"/g, '""')}"`;
}

export function buildCSVExport(columnNames: string[], rows: any[][]): string {
  const headers = columnNames.map((name) => sanitizeCSVCell(name)).join(',');
  const body = rows.map((row) => row.map((cell) => sanitizeCSVCell(cell)).join(','));
  return '\uFEFF' + [headers, ...body].join('\n');
}

export function buildTSVExport(columnNames: string[], rows: any[][], colIndices: number[]): string {
  const cols = columnNames.join('\t');
  const lines = rows.map((row) =>
    colIndices
      .map((cIdx) => {
        const v = row[cIdx];
        if (v === null || v === undefined) return '';
        return typeof v === 'object' ? JSON.stringify(v) : String(v);
      })
      .join('\t')
  );
  return [cols, ...lines].join('\n');
}

export function buildMarkdownExport(columnNames: string[], rows: any[][], colIndices: number[]): string {
  const header = `| ${columnNames.join(' | ')} |`;
  const sep = `| ${columnNames.map(() => '---').join(' | ')} |`;
  const lines = rows.map((row) =>
    `| ${colIndices
      .map((cIdx) => {
        const v = row[cIdx];
        if (v === null || v === undefined) return '*NULL*';
        const s = typeof v === 'object' ? JSON.stringify(v) : String(v);
        return s.replace(/\|/g, '\\|').replace(/\n/g, ' ');
      })
      .join(' | ')} |`
  );
  return [header, sep, ...lines].join('\n');
}

/**
 * Evaluates a client-side BETWEEN filter expression ("low AND high" or "low, high").
 * Uses numeric comparison whenever the cell value and both bounds are finite numbers,
 * preventing lexicographic bugs such as "10" not being BETWEEN "2" AND "20".
 */
export function matchesBetweenFilter(cellVal: any, valStr: string): boolean {
  const parts = valStr
    .split(/\s+AND\s+|,/i)
    .map((s) => s.trim())
    .filter((s) => s !== '');
  if (parts.length < 2) return true;
  if (cellVal === null || cellVal === undefined) return false;
  const str = String(cellVal).trim();
  const loStr = parts[0];
  const hiStr = parts[1];
  const a = typeof cellVal === 'number' ? cellVal : str !== '' ? Number(str) : NaN;
  const lo = Number(loStr);
  const hi = Number(hiStr);
  if (Number.isFinite(a) && Number.isFinite(lo) && Number.isFinite(hi)) {
    return a >= lo && a <= hi;
  }
  return str >= loStr && str <= hiStr;
}

/**
 * Coerces an inline grid cell editor string back to the appropriate JS/JSON value type
 * based on column metadata and previous cell value, while preserving 64-bit BIGINT strings
 * that exceed Number.MAX_SAFE_INTEGER.
 */
export function coerceCellEditValue(rawStr: string, colType: string, oldVal: any): any {
  if (rawStr === '__NULL__') return null;
  const trimmed = rawStr.trim();
  const baseType = (colType || '').toLowerCase().replace(/\(.*\)/, '').trim();

  if (baseType === 'bool' || baseType === 'boolean') {
    const low = trimmed.toLowerCase();
    if (low === 'true' || low === '1' || low === 't' || low === 'yes') {
      return typeof oldVal === 'number' ? 1 : true;
    }
    if (low === 'false' || low === '0' || low === 'f' || low === 'no') {
      return typeof oldVal === 'number' ? 0 : false;
    }
    return rawStr;
  }

  if (
    /^(tinyint|smallint|mediumint|int|integer|bigint|int2|int4|int8|serial|bigserial|smallserial|year)$/.test(baseType)
  ) {
    if (/^-?\d+$/.test(trimmed)) {
      const num = Number(trimmed);
      if (Number.isSafeInteger(num)) {
        return num;
      }
      // Keep unsafe 64-bit integers as exact strings to prevent precision loss.
      return trimmed;
    }
    return rawStr;
  }

  if (/^(float|double|double precision|real|float4|float8)$/.test(baseType)) {
    if (trimmed !== '' && Number.isFinite(Number(trimmed))) {
      return Number(trimmed);
    }
    return rawStr;
  }

  if (/^(decimal|numeric|dec|fixed|money)$/.test(baseType)) {
    if (typeof oldVal === 'number' && trimmed !== '' && Number.isFinite(Number(trimmed))) {
      const num = Number(trimmed);
      // Only convert to JS number if exact decimal representation round-trips without precision loss.
      if (String(num) === trimmed) {
        return num;
      }
    }
    return rawStr;
  }

  // Fallback when column metadata type is unknown/empty: preserve previous primitive JS type.
  if (!baseType && typeof oldVal === 'boolean') {
    const low = trimmed.toLowerCase();
    if (low === 'true' || low === '1') return true;
    if (low === 'false' || low === '0') return false;
  }
  if (!baseType && typeof oldVal === 'number' && trimmed !== '') {
    const num = Number(trimmed);
    if (Number.isFinite(num) && (!/^-?\d+$/.test(trimmed) || Number.isSafeInteger(num))) {
      return num;
    }
  }

  return rawStr;
}

/**
 * Computes virtualized window indices and spacer heights while accounting for in-place
 * stagedDraftRows rendered above the data rows inside <tbody>.
 */
export function computeVirtualGridWindow(
  scrollTop: number,
  containerHeight: number,
  rowHeight: number,
  overscan: number,
  totalRows: number,
  draftRowCount: number
): { startIndex: number; endIndex: number; topSpacerHeight: number; bottomSpacerHeight: number } {
  const safeDrafts = Math.max(0, draftRowCount);
  const dataScrollTop = Math.max(0, scrollTop - safeDrafts * rowHeight);
  const startIndex = Math.max(0, Math.floor(dataScrollTop / rowHeight) - overscan);
  const endIndex = Math.min(totalRows, Math.ceil((dataScrollTop + containerHeight) / rowHeight) + overscan);
  const topSpacerHeight = startIndex * rowHeight;
  const bottomSpacerHeight = Math.max(0, (totalRows - endIndex) * rowHeight);
  return { startIndex, endIndex, topSpacerHeight, bottomSpacerHeight };
}

/**
 * Computes a stable pixel width for a grid column from header metadata and an initial row sample
 * so vertical virtual scrolling never causes horizontal column jitter.
 */
export function estimateColumnWidth(colName: string, colType: string, sampleValues: any[]): number {
  const headerChars = colName.length + (colType ? colType.length + 3 : 0) + 4;
  let maxChars = headerChars;
  const sampleLimit = Math.min(sampleValues.length, 50);
  for (let i = 0; i < sampleLimit; i++) {
    const v = sampleValues[i];
    if (v === null || v === undefined) continue;
    const len = typeof v === 'object' ? JSON.stringify(v).length : String(v).length;
    if (len > maxChars) maxChars = len;
  }
  // ~7.6px per monospace char + horizontal cell padding
  const px = Math.round(maxChars * 7.6 + 28);
  return Math.max(96, Math.min(360, px));
}
