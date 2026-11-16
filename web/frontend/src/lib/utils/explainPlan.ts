export interface NormalizedPlanNode {
  step: number;
  depth: number;
  table: string;
  operation: string;
  key: string;
  rows: number | null;
  actualRows: number | null;
  cost: number | null;
  extra: string;
  isDanger: boolean;
  isWarning: boolean;
}

export interface ParsedExplainPlan {
  nodes: NormalizedPlanNode[];
  rawJsonFormatted: string;
  rawRows: Record<string, any>[];
  dialectHint: 'postgres' | 'mysql' | 'sqlite' | 'unknown';
}

function formatRawJson(rawJson: unknown): string {
  if (typeof rawJson !== 'string' || !rawJson.trim()) return '';
  try {
    return JSON.stringify(JSON.parse(rawJson), null, 2);
  } catch {
    return rawJson;
  }
}

function parsePgNode(node: Record<string, any>, depth: number, out: NormalizedPlanNode[]) {
  const op = String(node['Node Type'] ?? 'Plan');
  const opUpper = op.toUpperCase();
  const table = String(
    node['Relation Name'] ||
      node['Alias'] ||
      node['CTE Name'] ||
      node['Function Name'] ||
      ''
  );
  const key = String(node['Index Name'] || '');
  const rows = typeof node['Plan Rows'] === 'number' ? node['Plan Rows'] : null;
  const actualRows = typeof node['Actual Rows'] === 'number' ? node['Actual Rows'] : null;
  const cost = typeof node['Total Cost'] === 'number' ? node['Total Cost'] : null;

  const details: string[] = [];
  if (node['Index Cond']) details.push(`Index Cond: ${node['Index Cond']}`);
  if (node['Filter']) details.push(`Filter: ${node['Filter']}`);
  if (node['Hash Cond']) details.push(`Hash Cond: ${node['Hash Cond']}`);
  if (node['Merge Cond']) details.push(`Merge Cond: ${node['Merge Cond']}`);
  if (node['Join Filter']) details.push(`Join Filter: ${node['Join Filter']}`);
  if (Array.isArray(node['Sort Key']) && node['Sort Key'].length > 0) {
    details.push(`Sort Key: ${node['Sort Key'].join(', ')}`);
  }

  out.push({
    step: out.length + 1,
    depth,
    table: table || '(operator)',
    operation: op,
    key,
    rows,
    actualRows,
    cost,
    extra: details.join(' | '),
    isDanger: opUpper === 'SEQ SCAN',
    isWarning: opUpper === 'SORT' || opUpper === 'BITMAP HEAP SCAN',
  });

  if (Array.isArray(node['Plans'])) {
    for (const child of node['Plans']) {
      if (child && typeof child === 'object') {
        const inner = (child as Record<string, any>)['Plan'] ?? child;
        if (inner && typeof inner === 'object') {
          parsePgNode(inner as Record<string, any>, depth + 1, out);
        }
      }
    }
  }
}

function tryParsePgJson(rawJson: string): NormalizedPlanNode[] | null {
  const trimmed = rawJson.trim();
  if (!trimmed.startsWith('[') && !trimmed.startsWith('{')) return null;
  try {
    const parsed = JSON.parse(trimmed);
    const items = Array.isArray(parsed) ? parsed : [parsed];
    const nodes: NormalizedPlanNode[] = [];
    for (const item of items) {
      if (!item || typeof item !== 'object') continue;
      const root = (item as Record<string, any>)['Plan'] ?? item;
      if (root && typeof root === 'object' && 'Node Type' in root) {
        parsePgNode(root as Record<string, any>, 0, nodes);
      }
    }
    return nodes.length > 0 ? nodes : null;
  } catch {
    return null;
  }
}

const SQLITE_SCAN_RE = /\bSCAN\s+(?:TABLE\s+)?([a-zA-Z0-9_."`]+)/i;
const SQLITE_SEARCH_RE = /\bSEARCH\s+(?:TABLE\s+)?([a-zA-Z0-9_."`]+)/i;
const SQLITE_INDEX_RE = /\bUSING\s+(?:COVERING\s+)?INDEX\s+([a-zA-Z0-9_."`]+)/i;

function parseSqliteDetailRow(r: Record<string, any>, idx: number): NormalizedPlanNode {
  const detail = String(r.extra ?? r.detail ?? '');
  const scanMatch = SQLITE_SCAN_RE.exec(detail);
  const searchMatch = SQLITE_SEARCH_RE.exec(detail);
  const indexMatch = SQLITE_INDEX_RE.exec(detail);

  let operation = 'PLAN';
  let table = '';
  let key = '';
  let isDanger = false;
  let isWarning = false;

  if (scanMatch) {
    operation = 'SCAN';
    table = scanMatch[1];
    isDanger = true;
  } else if (searchMatch) {
    operation = 'SEARCH';
    table = searchMatch[1];
    if (indexMatch) {
      key = indexMatch[1];
    } else if (/INTEGER PRIMARY KEY|ROWID/i.test(detail)) {
      key = 'PRIMARY KEY';
    }
  } else if (/USE TEMP B-TREE/i.test(detail)) {
    operation = 'TEMP B-TREE';
    isWarning = true;
  }

  return {
    step: idx + 1,
    depth: 0,
    table: table || '(plan step)',
    operation,
    key,
    rows: null,
    actualRows: null,
    cost: null,
    extra: detail,
    isDanger,
    isWarning,
  };
}

/**
 * Normalizes EXPLAIN output from MySQL/MariaDB, PostgreSQL (JSON tree or text fallback),
 * and SQLite (EXPLAIN QUERY PLAN) into a unified list of plan nodes.
 */
export function parseExplainPlan(planData: any): ParsedExplainPlan {
  if (!planData || typeof planData !== 'object') {
    return { nodes: [], rawJsonFormatted: '', rawRows: [], dialectHint: 'unknown' };
  }

  const rawRows: Record<string, any>[] = Array.isArray(planData.rows) ? planData.rows : [];
  const rawJsonFormatted = formatRawJson(planData.raw_json);

  // 1. PostgreSQL JSON plan tree (raw_json has [{"Plan": ...}] and rows is usually empty)
  if (typeof planData.raw_json === 'string' && planData.raw_json.trim()) {
    const pgNodes = tryParsePgJson(planData.raw_json);
    if (pgNodes) {
      return { nodes: pgNodes, rawJsonFormatted, rawRows, dialectHint: 'postgres' };
    }
  }

  // 2. Tabular rows (MySQL/MariaDB, SQLite EXPLAIN QUERY PLAN, or PG Tier-3 text fallback)
  if (rawRows.length > 0) {
    const first = rawRows[0];
    const isSqlite =
      first.type === 'SCAN/SEARCH' ||
      (!first.table && typeof first.extra === 'string' && /^(SCAN|SEARCH|USE TEMP B-TREE|CO-ROUTINE|COMPOUND|MULTI-INDEX|LIST|MERGE)\b/i.test(first.extra));

    if (isSqlite) {
      return {
        nodes: rawRows.map((r, idx) => parseSqliteDetailRow(r, idx)),
        rawJsonFormatted,
        rawRows,
        dialectHint: 'sqlite',
      };
    }

    // PostgreSQL Tier 3 text fallback: driver sets Table = line, Extra = line, Type = ""
    const isPgText = !first.type && !first.select_type && first.table && first.table === first.extra;
    if (isPgText) {
      const nodes: NormalizedPlanNode[] = rawRows.map((r, idx) => {
        const line = String(r.extra ?? r.table ?? '');
        const isSeqScan = /\bSeq Scan\b/i.test(line);
        const isSort = /\bSort\b/i.test(line);
        return {
          step: idx + 1,
          depth: 0,
          table: '',
          operation: isSeqScan ? 'Seq Scan' : isSort ? 'Sort' : 'Plan',
          key: '',
          rows: null,
          actualRows: null,
          cost: null,
          extra: line,
          isDanger: isSeqScan,
          isWarning: isSort,
        };
      });
      return { nodes, rawJsonFormatted, rawRows, dialectHint: 'postgres' };
    }

    // MySQL / MariaDB tabular EXPLAIN
    const nodes: NormalizedPlanNode[] = rawRows.map((r, idx) => {
      const table = String(r.table || r.TableName || '');
      const op = String(r.type || r.Operation || 'SCAN');
      const opUpper = op.toUpperCase();
      const key = String(r.key || r.Key || '');
      const extraStr = String(r.extra || '');
      const filteredStr = r.filtered && Number(r.filtered) > 0 && Number(r.filtered) < 100 ? `filtered: ${r.filtered}%` : '';
      const extra = [extraStr, filteredStr].filter(Boolean).join('; ');
      const rowsVal = r.rows ?? r.Rows;
      const rows = rowsVal != null && (Number(rowsVal) > 0 || !!table) ? Number(rowsVal) : null;

      const isSafeConst = !table || ['CONST', 'SYSTEM', 'EQ_REF', 'REF', 'NULL'].includes(opUpper);
      const isDanger = opUpper === 'ALL' || (!key && !isSafeConst);
      const isWarning = /using filesort|using temporary/i.test(extraStr) || opUpper === 'INDEX';

      return {
        step: idx + 1,
        depth: 0,
        table: table || '(subquery)',
        operation: op,
        key,
        rows,
        actualRows: null,
        cost: null,
        extra,
        isDanger,
        isWarning,
      };
    });

    return { nodes, rawJsonFormatted, rawRows, dialectHint: 'mysql' };
  }

  return { nodes: [], rawJsonFormatted, rawRows, dialectHint: 'unknown' };
}
