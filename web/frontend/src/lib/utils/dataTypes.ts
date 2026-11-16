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

export function formatEnumLength(values: string[]): string {
  return values
    .map((v) => `'${v.replace(/\\/g, '\\\\').replace(/'/g, "\\'")}'`)
    .join(',');
}
