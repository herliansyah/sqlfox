/**
 * Shared schema designer catalogs, MySQL/MariaDB charset/collation registry,
 * column type presets, parser, and validator (ported from legacy 03_schema.js & 06_table_designer.js).
 */

export const MYSQL_CHARSET_COLLATIONS: Record<string, string[]> = {
  utf8mb4: [
    'utf8mb4_unicode_ci',
    'utf8mb4_general_ci',
    'utf8mb4_0900_ai_ci',
    'utf8mb4_0900_as_cs',
    'utf8mb4_unicode_520_ci',
    'utf8mb4_bin',
    'utf8mb4_swedish_ci',
    'utf8mb4_spanish_ci',
    'utf8mb4_vietnamese_ci',
  ],
  utf8mb3: ['utf8mb3_unicode_ci', 'utf8mb3_general_ci', 'utf8mb3_bin'],
  utf8: ['utf8_unicode_ci', 'utf8_general_ci', 'utf8_bin'],
  latin1: ['latin1_swedish_ci', 'latin1_general_ci', 'latin1_general_cs', 'latin1_bin', 'latin1_danish_ci', 'latin1_german1_ci', 'latin1_spanish_ci'],
  ascii: ['ascii_general_ci', 'ascii_bin'],
  binary: ['binary'],
  ucs2: ['ucs2_general_ci', 'ucs2_unicode_ci', 'ucs2_bin'],
  utf16: ['utf16_general_ci', 'utf16_unicode_ci', 'utf16_bin'],
  utf16le: ['utf16le_general_ci', 'utf16le_bin'],
  utf32: ['utf32_general_ci', 'utf32_unicode_ci', 'utf32_bin'],
  latin2: ['latin2_general_ci', 'latin2_croatian_ci', 'latin2_hungarian_ci', 'latin2_bin'],
  latin5: ['latin5_turkish_ci', 'latin5_bin'],
  latin7: ['latin7_general_ci', 'latin7_estonian_cs', 'latin7_bin'],
  cp1250: ['cp1250_general_ci', 'cp1250_czech_cs', 'cp1250_croatian_ci', 'cp1250_polish_ci', 'cp1250_bin'],
  cp1251: ['cp1251_general_ci', 'cp1251_ukrainian_ci', 'cp1251_bulgarian_ci', 'cp1251_bin'],
  cp1256: ['cp1256_general_ci', 'cp1256_bin'],
  cp1257: ['cp1257_general_ci', 'cp1257_lithuanian_ci', 'cp1257_bin'],
  sjis: ['sjis_japanese_ci', 'sjis_bin'],
  euckr: ['euckr_korean_ci', 'euckr_bin'],
  gbk: ['gbk_chinese_ci', 'gbk_bin'],
  gb2312: ['gb2312_chinese_ci', 'gb2312_bin'],
  big5: ['big5_chinese_ci', 'big5_bin'],
};

export interface DataTypeGroup {
  group: string;
  types: string[];
}

export const STANDARD_DATA_TYPES: DataTypeGroup[] = [
  { group: 'Numeric', types: ['INT', 'BIGINT', 'SMALLINT', 'TINYINT', 'DECIMAL', 'NUMERIC', 'FLOAT', 'DOUBLE', 'REAL', 'BOOLEAN', 'SERIAL', 'BIGSERIAL'] },
  { group: 'String & Text', types: ['VARCHAR', 'CHAR', 'TEXT', 'MEDIUMTEXT', 'LONGTEXT', 'UUID'] },
  { group: 'Date & Time', types: ['DATETIME', 'TIMESTAMP', 'TIMESTAMPTZ', 'DATE', 'TIME', 'YEAR'] },
  { group: 'JSON & Structured', types: ['JSON', 'JSONB', 'ENUM', 'SET'] },
  { group: 'Binary', types: ['BLOB', 'LONGBLOB', 'BYTEA', 'VARBINARY', 'BINARY'] },
];

export const ALL_STANDARD_TYPES = STANDARD_DATA_TYPES.flatMap((g) => g.types);

const LENGTHLESS_TYPES = new Set([
  'TEXT', 'TINYTEXT', 'MEDIUMTEXT', 'LONGTEXT',
  'BLOB', 'TINYBLOB', 'MEDIUMBLOB', 'LONGBLOB', 'BYTEA',
  'JSON', 'JSONB', 'DATE', 'DATETIME', 'TIMESTAMP', 'TIMESTAMPTZ', 'TIME', 'YEAR',
  'BOOLEAN', 'BOOL', 'SERIAL', 'BIGSERIAL', 'UUID', 'REAL', 'DOUBLE', 'FLOAT',
]);

export function isLengthlessType(baseType: string): boolean {
  const clean = (baseType || '').trim().toUpperCase();
  return LENGTHLESS_TYPES.has(clean);
}

export function defaultLengthForType(baseType: string): string {
  const clean = (baseType || '').trim().toUpperCase();
  if (clean === 'VARCHAR') return '255';
  if (clean === 'CHAR') return '36';
  if (clean === 'DECIMAL' || clean === 'NUMERIC') return '10,2';
  if (clean === 'VARBINARY' || clean === 'BINARY') return '64';
  if (clean === 'ENUM' || clean === 'SET') return "'active','inactive'";
  return '';
}

export function parseColumnType(fullType: string): { baseType: string; length: string } {
  const raw = (fullType || '').trim();
  if (!raw) return { baseType: 'VARCHAR', length: '255' };
  const idx = raw.indexOf('(');
  if (idx === -1) {
    return { baseType: raw.toUpperCase(), length: '' };
  }
  const closeIdx = raw.lastIndexOf(')');
  const baseType = raw.slice(0, idx).trim().toUpperCase();
  const length = closeIdx > idx ? raw.slice(idx + 1, closeIdx).trim() : '';
  return { baseType, length };
}

export function formatColumnType(baseType: string, length: string): string {
  const cleanBase = (baseType || 'VARCHAR').trim().toUpperCase();
  const cleanLen = (length || '').trim();
  if (!cleanLen || isLengthlessType(cleanBase)) return cleanBase;
  return `${cleanBase}(${cleanLen})`;
}

export const DEFAULT_PRESETS: Array<{ label: string; value: string }> = [
  { label: 'None', value: '' },
  { label: 'NULL', value: 'NULL' },
  { label: 'CURRENT_TIMESTAMP', value: 'CURRENT_TIMESTAMP' },
  { label: '0', value: '0' },
  { label: "'' (Empty String)", value: "''" },
  { label: 'TRUE', value: 'TRUE' },
  { label: 'FALSE', value: 'FALSE' },
];

export interface ValidatableColumn {
  name: string;
  type: string;
  baseType?: string;
  length?: string;
  is_primary_key?: boolean;
  pk?: boolean;
}

export function validateTableColumns(cols: ValidatableColumn[], driver?: string): string | null {
  if (!cols || cols.length === 0) {
    return 'At least one column is required.';
  }
  const seen = new Set<string>();
  const isPg = driver === 'postgres' || driver === 'postgresql';
  for (let i = 0; i < cols.length; i++) {
    const col = cols[i];
    const name = (col.name || '').trim();
    if (!name) {
      return `Column #${i + 1} is missing a name.`;
    }
    const lower = name.toLowerCase();
    if (seen.has(lower)) {
      return `Duplicate column name '${name}'.`;
    }
    seen.add(lower);

    const parsed = col.baseType
      ? { baseType: col.baseType.toUpperCase(), length: col.length || '' }
      : parseColumnType(col.type || '');

    if ((parsed.baseType === 'VARCHAR' || parsed.baseType === 'CHAR' || parsed.baseType === 'VARBINARY') && !parsed.length.trim() && !isPg) {
      return `Column '${name}' (${parsed.baseType}) requires a length (e.g., 255).`;
    }
    if ((parsed.baseType === 'ENUM' || parsed.baseType === 'SET') && !parsed.length.trim()) {
      return `Column '${name}' (${parsed.baseType}) requires at least one quoted option (e.g., 'a','b').`;
    }
    if ((parsed.baseType === 'DECIMAL' || parsed.baseType === 'NUMERIC') && parsed.length.trim()) {
      if (!/^\d+(\s*,\s*\d+)?$/.test(parsed.length.trim())) {
        return `Column '${name}' (${parsed.baseType}) has invalid precision '${parsed.length}'. Expected format: 10,2.`;
      }
    }
  }
  return null;
}

export function parseEnumValues(typeStr: string): string[] {
  const m = String(typeStr || '').match(/^(?:enum|set)\s*\((.*)\)$/is);
  if (!m) return [];
  const inner = m[1];
  const values: string[] = [];
  const regex = /'((?:[^'\\]|\\.|'')*)'/g;
  let match: RegExpExecArray | null;
  while ((match = regex.exec(inner)) !== null) {
    values.push(match[1].replace(/''/g, "'").replace(/\\'/g, "'"));
  }
  return values;
}

import type { ColumnDefinition, SchemaCatalog, TableMetadata } from '../../types';

export function formatEnumLength(values: string[]): string {
  return values
    .map((v) => `'${v.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`)
    .join(',');
}

export function findTableMeta(
  tableName: string | null | undefined,
  catalog: SchemaCatalog | null | undefined
): TableMetadata | null {
  if (!tableName || !catalog?.tables) return null;
  const target = tableName.toLowerCase();
  const shortTarget = target.includes('.') ? target.split('.').pop()! : target;
  return (
    catalog.tables.find((t) => {
      const tn = t.name.toLowerCase();
      return tn === target || tn === shortTarget;
    }) || null
  );
}

export function getColDisplayName(col: any): string {
  if (!col) return '';
  return typeof col === 'string' ? col : col.name || '';
}

export function findColumnMeta(
  col: any,
  tableMeta: TableMetadata | null | undefined
): ColumnDefinition | undefined {
  const colName = getColDisplayName(col);
  if (!colName || !tableMeta?.columns) return undefined;
  const lower = colName.toLowerCase();
  return tableMeta.columns.find((c) => c.name.toLowerCase() === lower);
}

export function resolveColumnType(
  col: any,
  tableMeta: TableMetadata | null | undefined
): string {
  const meta = findColumnMeta(col, tableMeta);
  if (meta?.type) return meta.type;
  if (col && typeof col === 'object' && col.type) return String(col.type);
  return '';
}

export function resolveColumnNullable(
  col: any,
  tableMeta: TableMetadata | null | undefined
): boolean {
  const meta = findColumnMeta(col, tableMeta);
  if (meta && meta.is_nullable !== undefined) return Boolean(meta.is_nullable);
  if (col && typeof col === 'object') {
    if (col.is_nullable !== undefined) return Boolean(col.is_nullable);
    if (col.nullable !== undefined) return Boolean(col.nullable);
  }
  return true;
}

export function resolveEnumValues(
  col: any,
  tableMeta: TableMetadata | null | undefined
): string[] {
  const meta = findColumnMeta(col, tableMeta);
  if (meta?.enum_values && meta.enum_values.length > 0) {
    return meta.enum_values;
  }
  return parseEnumValues(resolveColumnType(col, tableMeta));
}

export interface ColumnTypeKind {
  isBool: boolean;
  isEnum: boolean;
  isSet: boolean;
  isDate: boolean;
  isTime: boolean;
  isDateTime: boolean;
  isInt: boolean;
  isUnsigned: boolean;
  isDecimal: boolean;
  isJson: boolean;
  isUuid: boolean;
  isString: boolean;
  maxLength: number | null;
}

export function classifyColumnType(colType: string, enumVals: string[] = []): ColumnTypeKind {
  const tLow = (colType || '').trim().toLowerCase();
  const isBool = tLow === 'bool' || tLow === 'boolean' || tLow === 'tinyint(1)' || tLow === 'bit(1)';
  const isSet = /^set\s*\(/i.test(tLow);
  const isEnum = enumVals.length > 0 && !isSet;
  const isDate = tLow === 'date';
  const isTime =
    tLow === 'time' ||
    /^time\(\d+\)$/.test(tLow) ||
    tLow === 'timetz' ||
    tLow.startsWith('time with') ||
    tLow.startsWith('time without');
  const isDateTime =
    tLow.startsWith('datetime') ||
    tLow.startsWith('timestamp') ||
    tLow === 'timestamptz';
  const isInt =
    !isBool &&
    /^(tinyint|smallint|mediumint|int|integer|bigint|serial|bigserial|smallserial|int2|int4|int8)\b/.test(tLow);
  const isUnsigned = tLow.includes('unsigned');
  const isDecimal = /^(decimal|numeric|float|double|real|float4|float8|double precision)\b/.test(tLow);
  const isJson = tLow.includes('json');
  const isUuid = tLow === 'uuid';
  const isString = /^(varchar|char|text|tinytext|mediumtext|longtext|character varying|character|nvarchar|nchar|bpchar)\b/.test(
    tLow
  );
  let maxLength: number | null = null;
  if (isString) {
    const m = tLow.match(/\((\d+)\)/);
    if (m) maxLength = Number(m[1]);
  }
  return {
    isBool,
    isEnum,
    isSet,
    isDate,
    isTime,
    isDateTime,
    isInt,
    isUnsigned,
    isDecimal,
    isJson,
    isUuid,
    isString,
    maxLength,
  };
}

function isValidCalendarDate(y: number, m: number, d: number): boolean {
  if (m < 1 || m > 12 || d < 1) return false;
  const maxDays = new Date(y, m, 0).getDate();
  return d <= maxDays;
}

function isValidClockTime(hh: number, mm: number, ss: number): boolean {
  return hh >= 0 && hh <= 23 && mm >= 0 && mm <= 59 && ss >= 0 && ss <= 59;
}

export function validateCellValue(
  val: any,
  col: any,
  tableMeta: TableMetadata | null | undefined
): string | null {
  const colName = getColDisplayName(col) || 'column';
  const nullable = resolveColumnNullable(col, tableMeta);

  if (val === null || val === undefined) {
    return nullable ? null : `Column '${colName}' cannot be NULL.`;
  }

  const colType = resolveColumnType(col, tableMeta);
  const enumVals = resolveEnumValues(col, tableMeta);
  const kind = classifyColumnType(colType, enumVals);
  const str = typeof val === 'object' ? JSON.stringify(val) : String(val);
  const trimmed = str.trim();

  if (kind.isEnum) {
    if (!enumVals.includes(str)) {
      return `Invalid value '${str}' for ${colName}. Allowed: ${enumVals.join(', ')}`;
    }
    return null;
  }

  if (kind.isSet) {
    if (str !== '') {
      const parts = str.split(',');
      const invalid = parts.find((p) => !enumVals.includes(p));
      if (invalid !== undefined) {
        return `Invalid SET option '${invalid}' for ${colName}. Allowed: ${enumVals.join(', ')}`;
      }
    }
    return null;
  }

  if (kind.isBool) {
    if (typeof val === 'boolean') return null;
    if (!['0', '1', 'true', 'false'].includes(trimmed.toLowerCase())) {
      return `Expected boolean (TRUE/FALSE) for '${colName}'.`;
    }
    return null;
  }

  if (kind.isDate) {
    const m = trimmed.match(/^(\d{4})-(\d{2})-(\d{2})$/);
    if (!m || !isValidCalendarDate(Number(m[1]), Number(m[2]), Number(m[3]))) {
      return `Expected valid date (YYYY-MM-DD) for '${colName}'.`;
    }
    return null;
  }

  if (kind.isTime) {
    const m = trimmed.match(/^(\d{2}):(\d{2})(?::(\d{2})(?:\.\d+)?)?$/);
    if (!m || !isValidClockTime(Number(m[1]), Number(m[2]), Number(m[3] ?? '0'))) {
      return `Expected valid time (HH:MM or HH:MM:SS) for '${colName}'.`;
    }
    return null;
  }

  if (kind.isDateTime) {
    const m = trimmed.match(
      /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})(?::(\d{2})(?:\.\d+)?)?(?:Z|[+-]\d{2}:?\d{2})?$/i
    );
    if (
      !m ||
      !isValidCalendarDate(Number(m[1]), Number(m[2]), Number(m[3])) ||
      !isValidClockTime(Number(m[4]), Number(m[5]), Number(m[6] ?? '0'))
    ) {
      return `Expected valid datetime (YYYY-MM-DD HH:MM:SS) for '${colName}'.`;
    }
    return null;
  }

  if (kind.isInt) {
    if (!/^-?\d+$/.test(trimmed)) {
      return `Expected integer for '${colName}' (${colType}).`;
    }
    if (kind.isUnsigned && trimmed.startsWith('-')) {
      return `Column '${colName}' (${colType}) cannot be negative.`;
    }
    return null;
  }

  if (kind.isDecimal) {
    if (trimmed === '' || !Number.isFinite(Number(trimmed))) {
      return `Expected numeric value for '${colName}' (${colType}).`;
    }
    return null;
  }

  if (kind.isJson) {
    try {
      JSON.parse(str);
    } catch {
      return `Invalid JSON syntax for '${colName}'.`;
    }
    return null;
  }

  if (kind.isUuid) {
    if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(trimmed)) {
      return `Expected valid UUID for '${colName}'.`;
    }
    return null;
  }

  if (kind.maxLength !== null && str.length > kind.maxLength) {
    return `Value for '${colName}' exceeds maximum length (${kind.maxLength}).`;
  }

  return null;
}
