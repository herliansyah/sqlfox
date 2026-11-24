import {
  HttpStatusError,
  normalizeExecResult,
  type ExecuteQueryOptions,
  type StreamEventHandlers,
} from './http';

export interface StreamContext {
  baseUrl: string;
  getAuthHeaders: (extra?: Record<string, string>, includeJsonContentType?: boolean) => Record<string, string>;
  handleNonOkResponse: (res: Response) => Promise<never>;
}

export async function streamExecuteQuery(
  ctx: StreamContext,
  sessionId: string,
  sql: string,
  opts: ExecuteQueryOptions,
  handlers: StreamEventHandlers
): Promise<void> {
  const headers = ctx.getAuthHeaders({}, true);
  let res: Response;
  try {
    res = await fetch(`${ctx.baseUrl}/session/${sessionId}/execute-stream`, {
      method: 'POST',
      headers,
      signal: opts.signal,
      body: JSON.stringify({
        sql,
        confirmed: Boolean(opts.confirmed),
        challenge_input: opts.challengeInput || '',
        database: opts.database,
        profile: Boolean(opts.profile),
        limit: opts.limit ?? 5000,
        tab_id: opts.tabId || '',
        query_id: opts.queryId || 'active',
        bypass_mutation_confirm: Boolean(opts.bypassMutationConfirm),
      }),
    });
  } catch (err: any) {
    if (err?.name === 'AbortError') throw err;
    if (typeof window !== 'undefined') (window as any).__sqlfox_trigger_reconnect?.();
    throw new Error('Cannot reach SqlFox daemon — is it still running?');
  }

  if (res.status === 422) {
    const data = await res.json();
    if (data?.inspection) {
      handlers.onBlocked?.(data.inspection);
      return;
    }
    throw new HttpStatusError(422, data?.error || 'Execution blocked by Safety Guard', data);
  }

  if (!res.ok) {
    await ctx.handleNonOkResponse(res);
  }

  if (!res.body) {
    throw new Error('Streaming response body unavailable');
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = '';
  let sawTerminalOrExec = false;

  const processLine = (line: string) => {
    const trimmed = line.trim();
    if (!trimmed) return;
    let evt: any;
    try {
      evt = JSON.parse(trimmed);
    } catch {
      return;
    }
    if (!evt || typeof evt !== 'object') return;

    if (evt.blocked && evt.inspection) {
      sawTerminalOrExec = true;
      handlers.onBlocked?.(evt.inspection);
      return;
    }
    switch (evt.type) {
      case 'meta':
        handlers.onMeta?.(evt);
        break;
      case 'chunk':
        handlers.onChunk?.(evt);
        break;
      case 'statement_done':
        handlers.onStatementDone?.(evt);
        break;
      case 'exec':
      case 'batch':
      case 'query':
        sawTerminalOrExec = true;
        handlers.onExec?.(normalizeExecResult(evt));
        break;
      case 'error':
        sawTerminalOrExec = true;
        handlers.onStatementError?.(evt);
        break;
      case 'done':
        sawTerminalOrExec = true;
        handlers.onDone?.(evt);
        break;
    }
  };

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      const lines = buf.split('\n');
      buf = lines.pop() || '';
      for (const line of lines) {
        processLine(line);
      }
    }
    if (buf.trim()) {
      processLine(buf);
    }
    if (!sawTerminalOrExec) {
      throw new Error('Query stream ended unexpectedly before completion (premature EOF).');
    }
  } finally {
    try {
      await reader.cancel();
    } catch {
      /* ignore */
    }
  }
}
