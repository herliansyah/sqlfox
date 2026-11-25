import {
  HttpStatusError,
  normalizeExecResult,
  type ExecuteQueryOptions,
  type StreamEventHandlers,
} from './http';

export interface StreamContext {
  rawFetch: (
    endpoint: string,
    options?: RequestInit & { timeoutMs?: number | null; allowStatuses?: number[] }
  ) => Promise<Response>;
}

export async function streamExecuteQuery(
  ctx: StreamContext,
  sessionId: string,
  sql: string,
  opts: ExecuteQueryOptions,
  handlers: StreamEventHandlers
): Promise<void> {
  const res = await ctx.rawFetch(`/session/${sessionId}/execute-stream`, {
    method: 'POST',
    signal: opts.signal,
    timeoutMs: null,
    allowStatuses: [422],
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

  if (res.status === 422) {
    const data = await res.json();
    if (data?.inspection) {
      handlers.onBlocked?.(data.inspection);
      return;
    }
    throw new HttpStatusError(422, data?.error || 'Execution blocked by Safety Guard', data);
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

export async function streamDataTransfer(
  ctx: StreamContext,
  payload: {
    source_session_id: string;
    source_db?: string;
    target_session_id: string;
    target_db?: string;
    tables: string[];
    create_tables?: boolean;
    truncate_target?: boolean;
    batch_size?: number;
  },
  onProgress?: (evt: any) => void,
  signal?: AbortSignal
): Promise<any> {
  const res = await ctx.rawFetch('/sync/data/transfer', {
    method: 'POST',
    timeoutMs: null,
    signal,
    body: JSON.stringify(payload),
  });
  if (!res.body) {
    throw new Error('Streaming response body is empty');
  }
  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buf = '';
  let result: any = { status: 'completed', total_rows: 0, has_errors: false, errors: [] as string[] };
  const tableErrors: string[] = [];

  const handleEvt = (evt: any) => {
    onProgress?.(evt);
    if (evt.event === 'table_error' && evt.error) {
      tableErrors.push(`${evt.table || 'table'}: ${evt.error}`);
    }
    if (evt.event === 'done') {
      result = { ...evt, errors: tableErrors };
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
        const trimmed = line.trim();
        if (!trimmed) continue;
        try {
          handleEvt(JSON.parse(trimmed));
        } catch {
          /* ignore malformed */
        }
      }
    }
    if (buf.trim()) {
      try {
        handleEvt(JSON.parse(buf.trim()));
      } catch {
        /* ignore */
      }
    }
  } finally {
    try {
      await reader.cancel();
    } catch {
      /* ignore */
    }
  }
  if (tableErrors.length > 0 && !result.error) {
    result.has_errors = true;
    result.error = tableErrors.join('; ');
  }
  return result;
}

export async function streamRunSqlScript(
  ctx: StreamContext,
  sessionId: string,
  file: File,
  options: { stopOnError?: boolean; database?: string; confirm?: boolean; challengeInput?: string; signal?: AbortSignal },
  onProgress: (event: any) => void
): Promise<{ status: string; total: number; executed: number; errors: number }> {
  const formData = new FormData();
  formData.append('file', file);

  const params = new URLSearchParams();
  if (options.stopOnError !== undefined) params.set('stop_on_error', String(options.stopOnError));
  if (options.database) params.set('db', options.database);
  if (options.confirm) params.set('confirm', 'true');
  if (options.challengeInput) params.set('challenge_input', options.challengeInput);

  const res = await ctx.rawFetch(`/session/${encodeURIComponent(sessionId)}/run-sql-script?${params.toString()}`, {
    method: 'POST',
    timeoutMs: null,
    signal: options.signal,
    body: formData,
  });

  if (!res.body) {
    return { status: 'completed', total: 0, executed: 0, errors: 0 };
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let streamBuf = '';
  let total = 0;
  let executed = 0;
  let errors = 0;
  let finalStatus = 'completed';

  const processItem = (item: any) => {
    if (item.total) total = item.total;
    if (item.status === 'ok') executed++;
    if (item.status === 'error') errors++;
    if (item.executed_statements !== undefined) executed = item.executed_statements;
    if (item.error_count !== undefined) errors = item.error_count;
    if (item.status === 'halted' || item.status === 'completed') finalStatus = item.status;
    onProgress(item);
  };

  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      streamBuf += decoder.decode(value, { stream: true });
      const lines = streamBuf.split('\n');
      streamBuf = lines.pop() || '';

      for (const line of lines) {
        const trimmed = line.trim();
        if (!trimmed) continue;
        try {
          processItem(JSON.parse(trimmed));
        } catch {
          /* ignore */
        }
      }
    }

    if (streamBuf.trim()) {
      try {
        processItem(JSON.parse(streamBuf.trim()));
      } catch {
        /* ignore */
      }
    }
  } finally {
    try {
      await reader.cancel();
    } catch {
      /* ignore */
    }
  }

  return {
    status: errors > 0 && options.stopOnError ? 'halted' : finalStatus,
    total,
    executed,
    errors,
  };
}
