import type { TableMetadata } from '../../../types';
import { quoteSqlIdentifier } from '../../utils/sqlStatements';

export function escapeXml(unsafe: string): string {
  return (unsafe || '').replace(/[<>&'"]/g, (c) => {
    switch (c) {
      case '<':
        return '&lt;';
      case '>':
        return '&gt;';
      case '&':
        return '&amp;';
      case "'":
        return '&apos;';
      case '"':
        return '&quot;';
      default:
        return c;
    }
  });
}

export function safeMermaidId(name: string): string {
  const s = String(name || 'entity').replace(/[^a-zA-Z0-9_]/g, '_');
  return /^[a-zA-Z_]/.test(s) ? s : `t_${s}`;
}

export function buildMermaidErd(tables: TableMetadata[]): string {
  let out = 'erDiagram\n';
  tables.forEach((src) => {
    (src.foreign_keys || []).forEach((fk) => {
      const fkCol = fk.column || fk.column_name || '';
      const cleanLabel = `${fkCol}_to_${fk.referenced_column}`.replace(/["<>#]/g, '');
      out += `  ${safeMermaidId(src.name)} }o--|| ${safeMermaidId(fk.referenced_table)} : "${cleanLabel}"\n`;
    });
  });
  tables.forEach((t) => {
    const fkSet = new Set((t.foreign_keys || []).map((fk) => (fk.column || fk.column_name || '').toLowerCase()));
    out += `  ${safeMermaidId(t.name)} {\n`;
    (t.columns || []).forEach((c) => {
      const typeClean = (c.type || 'varchar').replace(/[^a-zA-Z0-9_]/g, '_');
      const isPK = Boolean(c.is_primary_key);
      const isFK = fkSet.has((c.name || '').toLowerCase());
      const keys = isPK && isFK ? 'PK,FK' : isPK ? 'PK' : isFK ? 'FK' : '';
      out += `    ${typeClean} ${safeMermaidId(c.name)}${keys ? ' ' + keys : ''}\n`;
    });
    out += '  }\n';
  });
  return out;
}

export function buildLeftJoinSQL(
  target: TableMetadata,
  allTables: TableMetadata[],
  driver: string
): { sql: string; joinCount: number } {
  const q = (id: string) => quoteSqlIdentifier(id, driver);
  const joins: string[] = [];
  let aliasIdx = 1;

  for (const fk of target.foreign_keys || []) {
    const col = fk.column || fk.column_name || '';
    const alias = `j${aliasIdx++}`;
    joins.push(
      `LEFT JOIN ${q(fk.referenced_table)} AS ${alias} ON t0.${q(col)} = ${alias}.${q(fk.referenced_column)}`
    );
  }

  for (const other of allTables) {
    if (other.name === target.name) continue;
    for (const fk of other.foreign_keys || []) {
      if ((fk.referenced_table || '').toLowerCase() === target.name.toLowerCase()) {
        const col = fk.column || fk.column_name || '';
        const alias = `j${aliasIdx++}`;
        joins.push(
          `LEFT JOIN ${q(other.name)} AS ${alias} ON t0.${q(fk.referenced_column)} = ${alias}.${q(col)}`
        );
      }
    }
  }

  const sql = [`SELECT t0.*`, `FROM ${q(target.name)} AS t0`, ...joins, `LIMIT 100;`].join('\n');
  return { sql, joinCount: joins.length };
}

export interface ErdSvgOptions {
  tables: TableMetadata[];
  positions: Record<string, { x: number; y: number }>;
  expandedTables: Record<string, boolean>;
  lines: Array<{ path: string }>;
  cardWidth: number;
  headerHeight: number;
  rowHeight: number;
  maxColPreview: number;
  getCardHeight: (t: TableMetadata) => number;
}

export function buildErdSvg(opts: ErdSvgOptions): string {
  const { tables, positions, expandedTables, lines, cardWidth, headerHeight, rowHeight, maxColPreview, getCardHeight } = opts;
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;

  tables.forEach((t) => {
    const p = positions[t.name] || { x: 0, y: 0 };
    const h = getCardHeight(t);
    minX = Math.min(minX, p.x);
    minY = Math.min(minY, p.y);
    maxX = Math.max(maxX, p.x + cardWidth);
    maxY = Math.max(maxY, p.y + h);
  });

  const pad = 40;
  const w = Math.max(800, maxX - minX + pad * 2);
  const h = Math.max(600, maxY - minY + pad * 2);

  let cardsSvg = '';
  tables.forEach((t) => {
    const p = positions[t.name] || { x: 0, y: 0 };
    const x = p.x - minX + pad;
    const y = p.y - minY + pad;
    const allCols = t.columns || [];
    const limit = expandedTables[t.name] ? allCols.length : maxColPreview;
    const cols = allCols.slice(0, limit);
    const hasMore = allCols.length > limit;
    const cardH = headerHeight + cols.length * rowHeight + (hasMore ? 24 : 8);

    cardsSvg += `<g transform="translate(${x},${y})">
        <rect width="${cardWidth}" height="${cardH}" rx="8" fill="#0f172a" stroke="#334155" stroke-width="1"/>
        <path d="M0,8 a8,8 0 0 1 8,-8 h${cardWidth - 16} a8,8 0 0 1 8,8 v30 h-${cardWidth} z" fill="#1e293b"/>
        <text x="12" y="23" fill="#38bdf8" font-family="'JetBrains Mono',monospace" font-size="12" font-weight="bold">${escapeXml(t.name)}</text>
        <text x="${cardWidth - 12}" y="23" text-anchor="end" fill="#64748b" font-family="'JetBrains Mono',monospace" font-size="10">${allCols.length}</text>`;

    cols.forEach((c, idx) => {
      const cy = headerHeight + idx * rowHeight;
      const isPK = c.is_primary_key;
      const isFK = (t.foreign_keys || []).some((fk) => (fk.column || fk.column_name) === c.name);
      const colColor = isPK ? '#34d399' : isFK ? '#fbbf24' : '#cbd5e1';
      const prefix = isPK ? '[PK] ' : isFK ? '[FK] ' : '';
      const typeStr = escapeXml((c.type || '').split('(')[0].toUpperCase());

      cardsSvg += `
          <line x1="0" y1="${cy}" x2="${cardWidth}" y2="${cy}" stroke="#1e293b" stroke-width="1"/>
          <text x="12" y="${cy + 16}" fill="${colColor}" font-family="'JetBrains Mono',monospace" font-size="10">${prefix}${escapeXml(c.name)}</text>
          <text x="${cardWidth - 12}" y="${cy + 16}" text-anchor="end" fill="#64748b" font-family="'JetBrains Mono',monospace" font-size="9">${typeStr}</text>`;
    });

    if (hasMore) {
      const fy = headerHeight + cols.length * rowHeight + 15;
      cardsSvg += `<text x="${cardWidth / 2}" y="${fy}" text-anchor="middle" fill="#38bdf8" font-family="'JetBrains Mono',monospace" font-size="9">+${allCols.length - cols.length} more columns</text>`;
    }

    cardsSvg += `</g>`;
  });

  const linesSvg = (lines || [])
    .map((line) => `<path d="${line.path}" stroke="#f59e0b" stroke-width="2" fill="none" opacity="0.85" marker-end="url(#erd-arrow-export)"/>`)
    .join('\n');

  return `<svg xmlns="http://www.w3.org/2000/svg" width="${w}" height="${h}" viewBox="0 0 ${w} ${h}">
      <rect width="${w}" height="${h}" fill="#020617"/>
      <defs>
        <marker id="erd-arrow-export" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
          <path d="M0,0 L0,6 L8,3 z" fill="#f59e0b" opacity="0.85"/>
        </marker>
      </defs>
      <g transform="translate(${-minX + pad},${-minY + pad})">
        ${linesSvg}
      </g>
      ${cardsSvg}
    </svg>`;
}
