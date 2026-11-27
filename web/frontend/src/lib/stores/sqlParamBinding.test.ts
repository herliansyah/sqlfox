import { describe, it, expect } from 'vitest';
import { extractSqlParams, bindSqlParamsInText } from './sqlParamBinding';

describe('sqlParamBinding', () => {
  it('ignores dollar-quoted strings and partial-word tokens during bind', () => {
    const sql = "SELECT $$:id $tag$:name$tag$ foo:bar a@b val::int @@version :id, :name, $1";
    expect(extractSqlParams(sql, 'postgres')).toEqual([':id', ':name', '$1']);

    const values = new Map<string, string>([
      [':id', '42'],
      [':name', "'alice'"],
      [':bar', '999'],
      ['@b', '888'],
      ['$1', '7'],
    ]);
    const bound = bindSqlParamsInText(sql, values);
    expect(bound).toBe("SELECT $$:id $tag$:name$tag$ foo:bar a@b val::int @@version 42, 'alice', 7");
  });
});
