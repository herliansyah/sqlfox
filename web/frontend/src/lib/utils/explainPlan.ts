export interface NormalizedPlanNode {
  step: number;
  depth: number;
  table: string;
  alias?: string;
  selectType?: string;
  operation: string;
  key: string;
  possibleKeys?: string;
  rows: number | null;
  actualRows: number | null;
  filtered: number | null;
  cost: number | null;
  readCost?: string;
  filterExpr?: string;
  extra: string;
  isDanger: boolean;
  isWarning: boolean;
  isOptimal?: boolean;
  hasFilesort?: boolean;
  hasTemporary?: boolean;
  isCoveringIndex?: boolean;
  hasDiskSpill?: boolean;
  actualTimeMs?: number | null;
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
  const alias = node['Alias'] && node['Relation Name'] && node['Alias'] !== node['Relation Name'] ? String(node['Alias']) : undefined;
  const key = String(node['Index Name'] || '');
  const rows = typeof node['Plan Rows'] === 'number' ? node['Plan Rows'] : null;
  const actualRows = typeof node['Actual Rows'] === 'number' ? node['Actual Rows'] : null;
  const cost = typeof node['Total Cost'] === 'number' ? node['Total Cost'] : null;
  const actualTimeMs = typeof node['Actual Total Time'] === 'number' ? node['Actual Total Time'] : null;
  const hasDiskSpill =
    (typeof node['Temp Read Blocks'] === 'number' && node['Temp Read Blocks'] > 0) ||
    (typeof node['Temp Written Blocks'] === 'number' && node['Temp Written Blocks'] > 0) ||
    /external|disk/i.test(String(node['Sort Method'] || ''));

  const filterExprParts: string[] = [];
  if (node['Index Cond']) filterExprParts.push(`Index Cond: ${node['Index Cond']}`);
  if (node['Filter']) filterExprParts.push(`Filter: ${node['Filter']}`);
  if (node['Hash Cond']) filterExprParts.push(`Hash Cond: ${node['Hash Cond']}`);
  if (node['Merge Cond']) filterExprParts.push(`Merge Cond: ${node['Merge Cond']}`);
  if (node['Join Filter']) filterExprParts.push(`Join Filter: ${node['Join Filter']}`);
  if (Array.isArray(node['Sort Key']) && node['Sort Key'].length > 0) {
    filterExprParts.push(`Sort Key: ${node['Sort Key'].join(', ')}`);
  }

  const isDanger = opUpper === 'SEQ SCAN';
  const isWarning = opUpper === 'SORT' || opUpper === 'BITMAP HEAP SCAN' || hasDiskSpill;
  const isOptimal = opUpper.includes('INDEX') && !isDanger && !isWarning;
  const isCoveringIndex = opUpper === 'INDEX ONLY SCAN';

  out.push({
    step: out.length + 1,
    depth,
    table: table || '(operator)',
    alias,
    operation: op,
    key,
    rows,
    actualRows,
    filtered: null,
    cost,
    readCost: cost !== null ? cost.toFixed(2) : undefined,
    filterExpr: filterExprParts.join(' | ') || undefined,
    extra: filterExprParts.join(' | '),
    isDanger,
    isWarning,
    isOptimal,
    hasFilesort: opUpper === 'SORT' && /external/i.test(String(node['Sort Method'] || '')),
    hasTemporary: hasDiskSpill,
    isCoveringIndex,
    hasDiskSpill,
    actualTimeMs,
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

function collectMySQLTables(node: any, out: Array<{ tbl: Record<string, any>; depth: number; filesort?: boolean; temp?: boolean }>, depth = 0, ctxFlags: { filesort?: boolean; temp?: boolean } = {}) {
  if (!node || typeof node !== 'object') return;
  const nextFlags = {
    filesort: ctxFlags.filesort || Boolean(node['using_filesort']),
    temp: ctxFlags.temp || Boolean(node['using_temporary_table']),
  };
  if (node['table'] && typeof node['table'] === 'object') {
    out.push({ tbl: node['table'], depth, ...nextFlags });
  } else if (node['table_name']) {
    out.push({ tbl: node, depth, ...nextFlags });
  }
  if (Array.isArray(node['nested_loop'])) {
    for (const child of node['nested_loop']) collectMySQLTables(child, out, depth, nextFlags);
  }
  for (const wrapperKey of ['ordering_operation', 'grouping_operation', 'duplicates_removal']) {
    if (node[wrapperKey]) collectMySQLTables(node[wrapperKey], out, depth, nextFlags);
  }
  if (node['materialized_from_subquery']?.['query_block']) {
    collectMySQLTables(node['materialized_from_subquery']['query_block'], out, depth + 1, {});
  }
  if (Array.isArray(node['attached_subqueries'])) {
    for (const sq of node['attached_subqueries']) {
      if (sq?.['query_block']) collectMySQLTables(sq['query_block'], out, depth + 1, {});
    }
  }
  if (Array.isArray(node['union_result']?.['query_specifications'])) {
    for (const qs of node['union_result']['query_specifications']) {
      if (qs?.['query_block']) collectMySQLTables(qs['query_block'], out, depth + 1, {});
    }
  }
}

function tryParseMySQLJson(rawJson: string): NormalizedPlanNode[] | null {
  const trimmed = rawJson.trim();
  if (!trimmed.startsWith('{')) return null;
  try {
    const parsed = JSON.parse(trimmed);
    const qb = parsed?.['query_block'];
    if (!qb || typeof qb !== 'object') return null;
    const collected: Array<{ tbl: Record<string, any>; depth: number; filesort?: boolean; temp?: boolean }> = [];
    collectMySQLTables(qb, collected, 0);
    if (collected.length === 0) return null;

    return collected.map(({ tbl, depth, filesort, temp }, idx) => {
      const accessType = String(tbl['access_type'] || 'SCAN').toUpperCase();
      const hasFilesort = Boolean(tbl['using_filesort'] || filesort);
      const hasTemporary = Boolean(tbl['using_temporary_table'] || temp);
      const isCoveringIndex = Boolean(tbl['using_index']);
      const isDanger = accessType === 'ALL';
      const isWarning = accessType === 'INDEX' || hasFilesort || hasTemporary;
      const isOptimal = ['REF', 'EQ_REF', 'CONST', 'SYSTEM', 'RANGE', 'FULLTEXT'].includes(accessType);
      const rowsVal = tbl['rows_examined_per_scan'] ?? tbl['rows'] ?? null;
      const rows = rowsVal != null ? Number(rowsVal) : null;
      const filteredRaw = tbl['filtered'];
      const filtered = filteredRaw != null && !Number.isNaN(Number(filteredRaw)) ? Number(filteredRaw) : null;
      const readCostNum = tbl['cost_info']?.read_cost != null ? Number(tbl['cost_info'].read_cost) : null;
      const extraParts: string[] = [];
      if (tbl['attached_condition']) extraParts.push(String(tbl['attached_condition']));
      if (hasFilesort) extraParts.push('Using filesort');
      if (hasTemporary) extraParts.push('Using temporary');
      if (isCoveringIndex) extraParts.push('Using index');
      return {
        step: idx + 1,
        depth,
        table: String(tbl['table_name'] || '(subquery)'),
        alias: tbl['alias'] && tbl['alias'] !== tbl['table_name'] ? String(tbl['alias']) : undefined,
        selectType: depth > 0 ? 'SUBQUERY' : 'SIMPLE',
        operation: accessType,
        key: String(tbl['key'] || ''),
        possibleKeys: Array.isArray(tbl['possible_keys']) ? tbl['possible_keys'].join(', ') : undefined,
        rows,
        actualRows: tbl['rows_produced_per_join'] != null ? Number(tbl['rows_produced_per_join']) : null,
        filtered,
        cost: readCostNum !== null && !Number.isNaN(readCostNum) ? readCostNum : null,
        readCost: tbl['cost_info']?.read_cost != null ? String(tbl['cost_info'].read_cost) : undefined,
        filterExpr: tbl['attached_condition'] ? String(tbl['attached_condition']) : undefined,
        extra: extraParts.join('; '),
        isDanger,
        isWarning,
        isOptimal,
        hasFilesort,
        hasTemporary,
        isCoveringIndex,
      };
    });
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
    filtered: null,
    cost: null,
    extra: detail,
    isDanger,
    isWarning,
    isOptimal: operation === 'SEARCH' && !!key,
    isCoveringIndex: /COVERING\s+INDEX/i.test(detail),
    hasTemporary: /TEMP B-TREE/i.test(detail),
  };
}

/**
 * Normalizes EXPLAIN output from MySQL/MariaDB (tabular & EXPLAIN FORMAT=JSON),
 * PostgreSQL (JSON tree or text fallback), and SQLite (EXPLAIN QUERY PLAN).
 */
export function parseExplainPlan(planData: any): ParsedExplainPlan {
  if (!planData || typeof planData !== 'object') {
    return { nodes: [], rawJsonFormatted: '', rawRows: [], dialectHint: 'unknown' };
  }

  const rawRows: Record<string, any>[] = Array.isArray(planData.rows) ? planData.rows : [];
  const rawJsonFormatted = formatRawJson(planData.raw_json);

  // 1. PostgreSQL JSON plan tree
  if (typeof planData.raw_json === 'string' && planData.raw_json.trim()) {
    const pgNodes = tryParsePgJson(planData.raw_json);
    if (pgNodes) {
      return { nodes: pgNodes, rawJsonFormatted, rawRows, dialectHint: 'postgres' };
    }
  }

  // 2. MySQL EXPLAIN FORMAT=JSON (prefer JSON when tabular rows are absent, or enrich tabular rows)
  const mysqlJsonNodes = typeof planData.raw_json === 'string' && planData.raw_json.trim()
    ? tryParseMySQLJson(planData.raw_json)
    : null;

  // 3. Tabular rows (MySQL/MariaDB, SQLite EXPLAIN QUERY PLAN, or PG Tier-3 text fallback)
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
          filtered: null,
          cost: null,
          extra: line,
          isDanger: isSeqScan,
          isWarning: isSort,
        };
      });
      return { nodes, rawJsonFormatted, rawRows, dialectHint: 'postgres' };
    }

    // MySQL / MariaDB tabular EXPLAIN (cross-enriched with MySQL JSON readCost/filterExpr when available)
    const nodes: NormalizedPlanNode[] = rawRows.map((r, idx) => {
      const table = String(r.table || r.TableName || '');
      const op = String(r.type || r.Operation || 'SCAN');
      const opUpper = op.toUpperCase();
      const key = String(r.key || r.Key || '');
      const extraStr = String(r.extra || '');
      const filteredNum = r.filtered != null && !Number.isNaN(Number(r.filtered)) ? Number(r.filtered) : null;
      const filteredStr = filteredNum != null && filteredNum > 0 && filteredNum < 100 ? `filtered: ${filteredNum}%` : '';
      const extra = [extraStr, filteredStr].filter(Boolean).join('; ');
      const rowsVal = r.rows ?? r.Rows;
      const rows = rowsVal != null && (Number(rowsVal) > 0 || !!table) ? Number(rowsVal) : null;
      const matchedJson = mysqlJsonNodes?.find((m) => m.table.toLowerCase() === table.toLowerCase());

      const isSafeConst = !table || ['CONST', 'SYSTEM', 'EQ_REF', 'REF', 'NULL'].includes(opUpper);
      const isDanger = opUpper === 'ALL' || (!key && !isSafeConst);
      const hasFilesort = /using filesort/i.test(extraStr) || Boolean(matchedJson?.hasFilesort);
      const hasTemporary = /using temporary/i.test(extraStr) || Boolean(matchedJson?.hasTemporary);
      const isCoveringIndex = /using index\b/i.test(extraStr) && !/using index condition/i.test(extraStr);
      const isWarning = hasFilesort || hasTemporary || opUpper === 'INDEX';
      const isOptimal = !isDanger && !isWarning && !!key;

      return {
        step: idx + 1,
        depth: 0,
        table: table || '(subquery)',
        selectType: r.select_type ? String(r.select_type) : undefined,
        operation: op,
        key,
        possibleKeys: r.possible_keys ? String(r.possible_keys) : matchedJson?.possibleKeys,
        rows,
        actualRows: null,
        filtered: filteredNum ?? matchedJson?.filtered ?? null,
        cost: matchedJson?.cost ?? null,
        readCost: matchedJson?.readCost,
        filterExpr: matchedJson?.filterExpr,
        extra,
        isDanger,
        isWarning,
        isOptimal,
        hasFilesort,
        hasTemporary,
        isCoveringIndex,
      };
    });

    return { nodes, rawJsonFormatted, rawRows, dialectHint: 'mysql' };
  }

  if (mysqlJsonNodes) {
    return { nodes: mysqlJsonNodes, rawJsonFormatted, rawRows, dialectHint: 'mysql' };
  }

  return { nodes: [], rawJsonFormatted, rawRows, dialectHint: 'unknown' };
}
