/**
 * Pure SQL bind-parameter extraction and formatting helpers.
 */

export function extractSqlParams(sql: string, driverName?: string): string[] {
  // Strip strings, quoted identifiers, dollar quotes ($$...$$ and $tag$...$tag$), and comments.
  const masked = sql
    .replace(/'(?:''|\\.|[^'\\])*'/g, ' ')
    .replace(/"(?:""|\\.|[^"\\])*"/g, ' ')
    .replace(/`(?:``|[^`])*`/g, ' ')
    .replace(/(\$[A-Za-z_][A-Za-z0-9_]*\$|\$\$)[\s\S]*?\1/g, ' ')
    .replace(/--[^\n]*/g, ' ')
    .replace(/\/\*[\s\S]*?\*\//g, ' ');
  const drv = String(driverName || '').toLowerCase();
  const isMySQL = drv === 'mysql' || drv === 'mariadb';
  const found = new Set<string>();
  // Never capture MySQL user/session variables (@i, @row_num) as interactive bind parameters.
  const re = isMySQL
    ? /(?:^|[^:@A-Za-z0-9_])(:[A-Za-z_][A-Za-z0-9_]*|\$[1-9][0-9]*)\b/g
    : /(?:^|[^:@A-Za-z0-9_])(:[A-Za-z_][A-Za-z0-9_]*|@[A-Za-z_][A-Za-z0-9_]*|\$[1-9][0-9]*)\b/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(masked)) !== null) {
    found.add(m[1]);
  }
  return Array.from(found);
}

export function formatParamValue(raw: string): string {
  const trimmed = raw.trim();
  if (/^(null|true|false)$/i.test(trimmed)) return trimmed.toUpperCase();
  if (/^-?\d+(?:\.\d+)?$/.test(trimmed)) return trimmed;
  return `'${raw.replace(/'/g, "''")}'`;
}

export function bindSqlParamsInText(sql: string, values: Map<string, string>): string {
  return sql.replace(
    /'(?:''|\.|[^'\])*'|"(?:""|\.|[^"\])*"|`(?:``|[^`])*`|(\$[A-Za-z_][A-Za-z0-9_]*\$|\$\$)[\s\S]*?\1|--[^\n]*|\/\*[\s\S]*?\*\/|::[A-Za-z_][A-Za-z0-9_]*|@@[A-Za-z_][A-Za-z0-9_]*|(^|[^:@A-Za-z0-9_])(:[A-Za-z_][A-Za-z0-9_]*|@[A-Za-z_][A-Za-z0-9_]*|\$[1-9][0-9]*)\b/g,
    (full, _dq, prefix, param) => (param ? `${prefix}${values.get(param) ?? param}` : full)
  );
}
