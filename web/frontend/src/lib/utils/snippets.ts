export interface SnippetVariable {
  name: string;
  defaultValue: string;
}

export function parseSnippetVariables(sql: string): SnippetVariable[] {
  const variables: SnippetVariable[] = [];
  const seen = new Set<string>();
  for (const match of sql.matchAll(/\{\{([a-zA-Z0-9_]+)(?::([^}]*))?\}\}/g)) {
    if (!seen.has(match[1])) {
      seen.add(match[1]);
      variables.push({ name: match[1], defaultValue: match[2] ?? '' });
    }
  }
  return variables;
}

export function substituteSnippetVariables(sql: string, values: Record<string, string>): string {
  return sql.replace(/\{\{([a-zA-Z0-9_]+)(?::([^}]*))?\}\}/g, (template, name, fallback) => {
    if (Object.prototype.hasOwnProperty.call(values, name)) return values[name];
    return fallback ?? template;
  });
}
