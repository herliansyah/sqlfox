import { quoteSqlIdentifier, formatSqlLiteral } from '../../utils/sqlStatements';

export function formatCellValue(val: any): { display: string; type: string } {
  if (val === null || val === undefined) {
    return { display: 'NULL', type: 'null' };
  }
  if (typeof val === 'boolean') {
    return { display: val ? 'TRUE' : 'FALSE', type: 'boolean' };
  }
  if (typeof val === 'number') {
    return { display: val.toLocaleString(), type: 'number' };
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
