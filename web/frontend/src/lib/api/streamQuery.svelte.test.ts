import { describe, it, expect } from 'vitest';
import { streamDataTransfer, streamRunSqlScript } from './streamQuery';
import type { StreamContext } from './streamQuery';

function ndjsonCtx(lines: string[]): StreamContext {
  const enc = new TextEncoder();
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const l of lines) controller.enqueue(enc.encode(l + '\n'));
      controller.close();
    },
  });
  return {
    rawFetch: async () =>
      new Response(body, { status: 200, headers: { 'Content-Type': 'application/x-ndjson' } }),
  };
}

// Last line is a deliberately TRUNCATED terminal event (connection cut mid-line).
function truncatedTerminalCtx(prefixLines: string[], truncatedTerminal: string): StreamContext {
  const enc = new TextEncoder();
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      for (const l of prefixLines) controller.enqueue(enc.encode(l + '\n'));
      // no trailing newline: simulates buf-tail leftover after clean close
      controller.enqueue(enc.encode(truncatedTerminal));
      controller.close();
    },
  });
  return {
    rawFetch: async () =>
      new Response(body, { status: 200, headers: { 'Content-Type': 'application/x-ndjson' } }),
  };
}

describe('streamDataTransfer premature EOF', () => {
  it('throws on silent stream close before done event', async () => {
    const ctx = ndjsonCtx([
      JSON.stringify({ event: 'table_start', table: 'users' }),
      JSON.stringify({ event: 'progress', table: 'users', rows_copied: 5 }),
    ]);
    await expect(
      streamDataTransfer(ctx, { source_session_id: 's', target_session_id: 't', tables: ['users'] })
    ).rejects.toThrow(/premature EOF/);
  });

  it('returns result when done event received', async () => {
    const ctx = ndjsonCtx([
      JSON.stringify({ event: 'table_start', table: 'users' }),
      JSON.stringify({ event: 'done', total_tables: 1, total_rows: 42, has_errors: false }),
    ]);
    const res = await streamDataTransfer(ctx, { source_session_id: 's', target_session_id: 't', tables: ['users'] });
    expect(res.total_rows).toBe(42);
    expect(res.has_errors).toBe(false);
  });
  it('throws when terminal done line is truncated mid-JSON', async () => {
    const full = JSON.stringify({ event: 'done', total_tables: 1, total_rows: 42, has_errors: false });
    const ctx = truncatedTerminalCtx(
      [JSON.stringify({ event: 'table_start', table: 'users' })],
      full.slice(0, Math.floor(full.length / 2))
    );
    await expect(
      streamDataTransfer(ctx, { source_session_id: 's', target_session_id: 't', tables: ['users'] })
    ).rejects.toThrow(/premature EOF/);
  });
});

describe('streamRunSqlScript premature EOF', () => {
  it('throws on silent stream close before summary event', async () => {
    const ctx = ndjsonCtx([
      JSON.stringify({ statement_index: 1, total: 3, status: 'ok', sql_preview: 'SELECT 1' }),
      JSON.stringify({ statement_index: 2, total: 3, status: 'error', error: 'boom' }),
    ]);
    await expect(
      streamRunSqlScript(ctx, 'sess', new File([''], 'x.sql'), { stopOnError: false }, () => {})
    ).rejects.toThrow(/premature EOF/);
  });

  it('throws when response body is empty', async () => {
    const ctx: StreamContext = {
      rawFetch: async () => new Response(null, { status: 200 }),
    };
    await expect(
      streamRunSqlScript(ctx, 'sess', new File([''], 'x.sql'), { stopOnError: false }, () => {})
    ).rejects.toThrow(/body is empty/);
  });

  it('returns summary when terminal status received', async () => {
    const ctx = ndjsonCtx([
      JSON.stringify({ statement_index: 1, total: 1, status: 'ok', sql_preview: 'SELECT 1' }),
      JSON.stringify({ status: 'completed', total_statements: 1, executed_statements: 1, error_count: 0 }),
    ]);
    const res = await streamRunSqlScript(ctx, 'sess', new File([''], 'x.sql'), { stopOnError: false }, () => {});
    expect(res.status).toBe('completed');
    expect(res.executed).toBe(1);
    expect(res.errors).toBe(0);
  });
  it('throws when terminal summary line is truncated mid-JSON', async () => {
    const full = JSON.stringify({ status: 'completed', total_statements: 1, executed_statements: 1, error_count: 0 });
    const ctx = truncatedTerminalCtx(
      [JSON.stringify({ statement_index: 1, total: 1, status: 'ok', sql_preview: 'SELECT 1' })],
      full.slice(0, Math.floor(full.length / 2))
    );
    await expect(
      streamRunSqlScript(ctx, 'sess', new File([''], 'x.sql'), { stopOnError: false }, () => {})
    ).rejects.toThrow(/premature EOF/);
  });
});
