import type { QueryExecutionResult, InspectionResult } from '../../types';

export interface AIProfile {
  id: string;
  name: string;
  provider: 'openai' | 'anthropic' | 'gemini' | 'ollama' | 'custom';
  api_key?: string;
  encrypted_api_key?: string;
  has_api_key?: boolean;
  base_url: string;
  model_name: string;
  temperature: number;
  language: 'auto' | 'en' | 'id';
  response_style: 'normal' | 'concise' | 'detailed';
  custom_prompt: string;
  include_schema: boolean;
  mask_identifiers: boolean;
  is_active: boolean;
  created_at?: string;
  updated_at?: string;
}

export interface AiConfig {
  provider: string;
  apiKey: string;
  baseUrl: string;
  modelName: string;
  temperature: number;
  language: 'auto' | 'en' | 'id';
  responseStyle: 'normal' | 'concise' | 'detailed';
  customPrompt: string;
  includeSchema: boolean;
  maskIdentifiers: boolean;
}

export function loadAiConfig(): AiConfig {
  const defaults: AiConfig = {
    provider: 'openai',
    apiKey: '',
    baseUrl: 'https://api.openai.com/v1',
    modelName: 'gpt-4o-mini',
    temperature: 0.3,
    language: 'auto',
    responseStyle: 'normal',
    customPrompt: '',
    includeSchema: true,
    maskIdentifiers: true,
  };
  try {
    const saved = JSON.parse(localStorage.getItem('sqlfox_ai_config') || '{}');
    return {
      ...defaults,
      ...saved,
      baseUrl: saved.baseUrl || saved.endpoint || defaults.baseUrl,
      modelName: saved.modelName || saved.model || defaults.modelName,
      apiKey: saved.apiKey ?? saved.api_key ?? defaults.apiKey,
      language: saved.language || defaults.language,
      responseStyle: saved.responseStyle || saved.response_style || defaults.responseStyle,
      customPrompt: saved.customPrompt ?? saved.custom_prompt ?? defaults.customPrompt,
      includeSchema: saved.includeSchema ?? saved.include_schema ?? defaults.includeSchema,
      maskIdentifiers: saved.maskIdentifiers ?? saved.mask_identifiers ?? defaults.maskIdentifiers,
      temperature: typeof saved.temperature === 'number' ? saved.temperature : defaults.temperature,
    };
  } catch {
    return defaults;
  }
}

export class HttpStatusError extends Error {
  status: number;
  data: any;
  constructor(status: number, message: string, data?: any) {
    super(message);
    this.status = status;
    this.data = data;
  }
}

export interface ExecuteQueryOptions {
  confirmed?: boolean;
  challengeInput?: string;
  database?: string;
  profile?: boolean;
  limit?: number;
  tabId?: string;
  queryId?: string;
  bypassMutationConfirm?: boolean;
  signal?: AbortSignal;
}

export interface StreamEventHandlers {
  onBlocked?: (inspection: InspectionResult) => void;
  onMeta?: (meta: {
    result_index?: number;
    columns: any[];
    tx_active?: boolean;
    target_table?: string;
    database?: string;
    sql?: string;
  }) => void;
  onChunk?: (chunk: { result_index?: number; rows: any[][] }) => void;
  onStatementDone?: (info: { result_index?: number; sql?: string; row_count?: number; execution_ms?: number }) => void;
  onExec?: (execRes: QueryExecutionResult & { result_index?: number }) => void;
  onStatementError?: (err: { result_index?: number; sql?: string; error: string }) => void;
  onDone?: (done: {
    row_count?: number;
    execution_ms?: number;
    has_more?: boolean;
    tx_active?: boolean;
    profile?: any;
    total_results?: number;
  }) => void;
}

export function normalizeExecResult(res: QueryExecutionResult): QueryExecutionResult {
  if (!res || typeof res !== 'object') return res;
  const duration = res.duration_ms ?? res.execution_ms ?? 0;
  const affected = res.affected_rows ?? res.rows_affected;
  const out: QueryExecutionResult = {
    ...res,
    duration_ms: duration,
    execution_ms: res.execution_ms ?? duration,
    affected_rows: affected,
    rows_affected: res.rows_affected ?? affected,
    is_destructive_blocked: Boolean(res.is_destructive_blocked || res.blocked),
    total_rows: (res as any).total_rows,
    is_exact: (res as any).is_exact,
  };
  if (Array.isArray(out.results)) {
    out.results = out.results.map((r) => normalizeExecResult(r));
  }
  return out;
}

export class ApiHttpBase {
  protected baseUrl = '/api';

  protected getAuthHeaders(extra: Record<string, string> = {}, includeJsonContentType = true): Record<string, string> {
    const headers: Record<string, string> = {};
    if (includeJsonContentType) {
      headers['Content-Type'] = 'application/json';
    }
    Object.assign(headers, extra);
    return headers;
  }

  protected async handleNonOkResponse(res: Response): Promise<never> {
    if (res.status === 401 && typeof window !== 'undefined' && (window as any).__sqlfox_set_remote_locked) {
      (window as any).__sqlfox_set_remote_locked(true);
    }
    let errorMsg = `HTTP Error ${res.status}`;
    let parsedData: any = null;
    try {
      parsedData = await res.json();
      if (parsedData?.error) errorMsg = parsedData.error;
      else if (parsedData?.reason) errorMsg = parsedData.reason;
      else if (parsedData?.message) errorMsg = parsedData.message;
    } catch {
      if (res.statusText) errorMsg = res.statusText;
    }
    if (res.status === 403 && /read[- ]?only/i.test(errorMsg) && typeof window !== 'undefined') {
      (window as any).__sqlfox_sync_readonly?.(true);
    }
    throw new HttpStatusError(res.status, errorMsg, parsedData);
  }

  protected async request<T>(
    endpoint: string,
    options: RequestInit & { timeoutMs?: number | null } = {}
  ): Promise<T> {
    const url = endpoint.startsWith('/api') ? endpoint : `${this.baseUrl}${endpoint}`;
    const isFormData = typeof FormData !== 'undefined' && options.body instanceof FormData;
    const headers = this.getAuthHeaders((options.headers as Record<string, string>) || {}, !isFormData);

    const { timeoutMs = 15000, ...fetchOpts } = options;
    let signal = fetchOpts.signal;
    if (!signal && timeoutMs && timeoutMs > 0 && typeof AbortSignal !== 'undefined' && 'timeout' in AbortSignal) {
      signal = AbortSignal.timeout(timeoutMs);
    }

    let res: Response;
    try {
      res = await fetch(url, { ...fetchOpts, headers, signal });
    } catch (err: any) {
      if (err?.name === 'AbortError' || err?.name === 'TimeoutError') {
        throw err;
      }
      if (typeof window !== 'undefined') (window as any).__sqlfox_trigger_reconnect?.();
      throw new Error(`Cannot reach SqlFox daemon at ${this.baseUrl.replace(/\/api$/, '')} — is it still running?`);
    }
    if (!res.ok) {
      await this.handleNonOkResponse(res);
    }

    return res.json() as Promise<T>;
  }
}
