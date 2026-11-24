import type {
  ConnectionProfile,
  SchemaCatalog,
  TableMetadata,
  QueryExecutionResult,
  InspectionResult,
  ProcesslistThread,
  TablePartition,
  QuerySnippet,
  OperationalNote,
} from '../../types';
import {
  ApiHttpBase,
  HttpStatusError,
  loadAiConfig,
  normalizeExecResult,
  type AIProfile,
  type AiConfig,
  type ExecuteQueryOptions,
  type StreamEventHandlers,
} from './http';
import { streamExecuteQuery } from './streamQuery';

export type { OperationalNote, AIProfile, AiConfig, ExecuteQueryOptions, StreamEventHandlers };
export { HttpStatusError, loadAiConfig };

export async function syncOrMigrateLegacyAiConfig(): Promise<void> {
  if (typeof window === 'undefined') return;
  const legacyRaw = localStorage.getItem('sqlfox_ai_config');
  if (!legacyRaw) return;
  try {
    const legacy = JSON.parse(legacyRaw);
    if (legacy && (legacy.apiKey || legacy.modelName || legacy.provider)) {
      const profiles = await api.listAiProfiles();
      const active = profiles.find((p) => p.is_active) || profiles[0];
      if (active) {
        await api.updateAiProfile(active.id, {
          provider: legacy.provider || active.provider,
          api_key: legacy.apiKey || undefined,
          base_url: legacy.baseUrl || active.base_url,
          model_name: legacy.modelName || active.model_name,
          temperature: typeof legacy.temperature === 'number' ? legacy.temperature : active.temperature,
          language: legacy.language || active.language,
          response_style: legacy.responseStyle || active.response_style,
          custom_prompt: legacy.customPrompt !== undefined ? legacy.customPrompt : active.custom_prompt,
          include_schema: legacy.includeSchema !== undefined ? legacy.includeSchema : active.include_schema,
          mask_identifiers: legacy.maskIdentifiers !== undefined ? legacy.maskIdentifiers : active.mask_identifiers,
        });
      }
    }
    localStorage.removeItem('sqlfox_ai_config');
  } catch (e) {
    console.warn('Failed to migrate legacy AI config:', e);
  }
}

class ApiClient extends ApiHttpBase {

  // Auth / Vault
  async getAuthStatus(): Promise<{ is_remote: boolean; unlocked: boolean; version?: string; lan_enabled?: boolean }> {
    return this.request('/auth/status');
  }

  async getMasterKey(): Promise<{ lan_enabled: boolean; master_password?: string; lan_ip?: string; port?: number | string }> {
    return this.request('/auth/master-key');
  }

  async unlockVault(password: string): Promise<{ success: boolean; token?: string }> {
    return this.request('/auth/unlock', {
      method: 'POST',
      body: JSON.stringify({ password }),
    });
  }

  async exportVault(exportPassphrase: string): Promise<Blob> {
    const headers = this.getAuthHeaders({}, true);
    const res = await fetch(`${this.baseUrl}/vault/export`, {
      method: 'POST',
      headers,
      body: JSON.stringify({ export_passphrase: exportPassphrase }),
    });
    if (!res.ok) await this.handleNonOkResponse(res);
    return res.blob();
  }

  async importVault(exportPassphrase: string, archive: unknown): Promise<{ imported_count: number; message?: string }> {
    return this.request('/vault/import', {
      method: 'POST',
      body: JSON.stringify({ export_passphrase: exportPassphrase, archive }),
    });
  }

  // Connections
  async listConnections(): Promise<ConnectionProfile[]> {
    return this.request('/connections');
  }

  async saveConnection(conn: Partial<ConnectionProfile>): Promise<ConnectionProfile> {
    return this.request('/connections', {
      method: 'POST',
      body: JSON.stringify(conn),
    });
  }

  async deleteConnection(id: string): Promise<{ success: boolean }> {
    return this.request(`/connections/${id}`, {
      method: 'DELETE',
    });
  }

  async testConnection(id: string): Promise<{ success: boolean; version?: string; latency_ms?: number; error?: string; server?: any }> {
    return this.request(`/connections/${id}/test`, {
      method: 'POST',
    });
  }

  async testTransientConnection(conn: Partial<ConnectionProfile>): Promise<{ success: boolean; version?: string; latency_ms?: number; error?: string; server?: any }> {
    return this.request('/connections/test-transient/test', {
      method: 'POST',
      body: JSON.stringify(conn),
    });
  }

  // Sessions
  async pingSession(sessionId: string): Promise<{
    latency_ms: number;
    memory_mb?: number;
    server_info?: { version?: string; uptime_sec?: number; active_conns?: number };
    read_only?: boolean;
  }> {
    return this.request(`/session/${sessionId}/ping`, { method: 'POST' });
  }

  async connectSession(connId: string): Promise<{
    id: string;
    connection_id: string;
    current_database?: string;
    schema?: SchemaCatalog;
    server_info?: any;
  }> {
    return this.request(`/session/connect/${connId}`, {
      method: 'POST',
      timeoutMs: 30000,
    });
  }

  async getSchema(connId: string, dbName = ''): Promise<SchemaCatalog> {
    const query = dbName ? `?db=${encodeURIComponent(dbName)}` : '';
    return this.request(`/session/${connId}/schema${query}`, { timeoutMs: 30000 });
  }

  async refreshSchema(connId: string, dbName = ''): Promise<SchemaCatalog> {
    const query = dbName ? `?db=${encodeURIComponent(dbName)}` : '';
    return this.request(`/session/${connId}/refresh-schema${query}`, {
      method: 'POST',
      timeoutMs: 30000,
    });
  }

  async selectDatabase(connId: string, database: string): Promise<SchemaCatalog> {
    return this.request(`/session/${connId}/select-db`, {
      method: 'POST',
      body: JSON.stringify({ database }),
      timeoutMs: 30000,
    });
  }

  async getTableDDL(connId: string, tableName: string, db?: string): Promise<{ ddl: string }> {
    const qs = db ? `?db=${encodeURIComponent(db)}` : '';
    return this.request(`/session/${connId}/ddl/${encodeURIComponent(tableName)}${qs}`);
  }

  async getRoutineDDL(connId: string, routineName: string, type = 'PROCEDURE'): Promise<{ ddl: string }> {
    return this.request(`/session/${connId}/ddl/routine/${encodeURIComponent(routineName)}?type=${encodeURIComponent(type)}`);
  }

  async getEventDDL(connId: string, eventName: string): Promise<{ name: string; ddl: string }> {
    return this.request(`/session/${connId}/ddl/event/${encodeURIComponent(eventName)}`);
  }

  async toggleEvent(connId: string, eventName: string, enable: boolean): Promise<{ success: boolean }> {
    return this.request(`/session/${connId}/event/${encodeURIComponent(eventName)}/toggle`, {
      method: 'POST',
      body: JSON.stringify({ enable }),
    });
  }

  async preflightDDL(connId: string, tableName: string, db = '', alterClause = ''): Promise<{
    table: string;
    database: string;
    active_locks: any[];
    relationships: any[];
    osc_ghost: string;
    osc_pt: string;
  }> {
    const params = new URLSearchParams();
    if (db) params.set('db', db);
    if (alterClause) params.set('alter', alterClause);
    const qs = params.toString() ? `?${params.toString()}` : '';
    return this.request(`/session/${connId}/preflight-ddl/${encodeURIComponent(tableName)}${qs}`);
  }

  async getFastCatalog(sessionId: string): Promise<Record<string, TableMetadata[]>> {
    return this.request(`/session/${sessionId}/fast-catalog`, { timeoutMs: 30000 });
  }

  async getTableRows(
    sessionId: string,
    tableName: string,
    limit = 25,
    offset = 0,
    db?: string,
    filter?: string,
    sortCol?: string,
    sortDir?: string
  ): Promise<QueryExecutionResult> {
    let url = `/session/${sessionId}/table/${encodeURIComponent(tableName)}/rows?limit=${limit}&offset=${offset}`;
    if (db) url += `&db=${encodeURIComponent(db)}`;
    if (filter) url += `&filter=${encodeURIComponent(filter)}`;
    if (sortCol) url += `&sort_col=${encodeURIComponent(sortCol)}&sort_dir=${encodeURIComponent(sortDir || 'ASC')}`;
    const res = await this.request<QueryExecutionResult>(url, { timeoutMs: 30000 });
    return this.normalizeExecResult(res);
  }

  async getTableCount(
    sessionId: string,
    tableName: string,
    db?: string,
    filter?: string
  ): Promise<{ table: string; count: number; duration_ms: number }> {
    let url = `/session/${sessionId}/table/${encodeURIComponent(tableName)}/count`;
    const params = new URLSearchParams();
    if (db) params.set('db', db);
    if (filter) params.set('filter', filter);
    const qs = params.toString();
    if (qs) url += `?${qs}`;
    return this.request(url, { timeoutMs: 30000 });
  }

  private normalizeExecResult(res: QueryExecutionResult): QueryExecutionResult {
    return normalizeExecResult(res);
  }

  async executeQuery(
    sessionId: string,
    sql: string,
    confirmedOrOpts: boolean | ExecuteQueryOptions = false,
    databaseArg?: string
  ): Promise<QueryExecutionResult> {
    const opts: ExecuteQueryOptions =
      typeof confirmedOrOpts === 'object'
        ? confirmedOrOpts
        : { confirmed: confirmedOrOpts, database: databaseArg };

    const headers = this.getAuthHeaders({}, true);
    let res: Response;
    try {
      res = await fetch(`${this.baseUrl}/session/${sessionId}/execute`, {
        method: 'POST',
        headers,
        signal: opts.signal,
        body: JSON.stringify({
          sql,
          confirmed: Boolean(opts.confirmed),
          challenge_input: opts.challengeInput || '',
          database: opts.database,
          profile: Boolean(opts.profile),
          limit: opts.limit,
          tab_id: opts.tabId,
          query_id: opts.queryId,
          bypass_mutation_confirm: Boolean(opts.bypassMutationConfirm),
        }),
      });
    } catch (err: any) {
      if (err?.name === 'AbortError') throw err;
      if (typeof window !== 'undefined') (window as any).__sqlfox_trigger_reconnect?.();
      throw new Error(`Cannot reach SqlFox daemon — is it still running?`);
    }

    if (res.status === 422) {
      const data = await res.json();
      return this.normalizeExecResult({
        columns: [],
        rows: [],
        duration_ms: 0,
        row_count: 0,
        blocked: true,
        is_destructive_blocked: true,
        inspection: data?.inspection,
      });
    }
    if (!res.ok) {
      await this.handleNonOkResponse(res);
    }
    const json = await res.json();
    return this.normalizeExecResult(json);
  }

  async executeQueryStream(
    sessionId: string,
    sql: string,
    opts: ExecuteQueryOptions,
    handlers: StreamEventHandlers
  ): Promise<void> {
    return streamExecuteQuery(
      {
        baseUrl: this.baseUrl,
        getAuthHeaders: (extra, includeJson) => this.getAuthHeaders(extra, includeJson),
        handleNonOkResponse: (res) => this.handleNonOkResponse(res),
      },
      sessionId,
      sql,
      opts,
      handlers
    );
  }

  async cancelQuery(sessionId: string, queryId = 'active'): Promise<{ success?: boolean; status?: string }> {
    return this.request(`/session/${sessionId}/cancel`, {
      method: 'POST',
      body: JSON.stringify({ query_id: queryId }),
      timeoutMs: 5000,
    });
  }

  async inspectQuery(sessionId: string, sql: string, database?: string): Promise<InspectionResult> {
    return this.request(`/session/${sessionId}/inspect`, {
      method: 'POST',
      body: JSON.stringify({ sql, database }),
    });
  }

  async explainQuery(sessionId: string, sql: string, database?: string, signal?: AbortSignal): Promise<any> {
    return this.request(`/session/${sessionId}/explain`, {
      method: 'POST',
      body: JSON.stringify({ sql, database }),
      signal,
      timeoutMs: 30000,
    });
  }

  async adviseQuery(sessionId: string, sql: string, database?: string): Promise<any> {
    return this.request(`/session/${sessionId}/advise`, {
      method: 'POST',
      body: JSON.stringify({ sql, database }),
      timeoutMs: 30000,
    });
  }

  // Grid CRUD Mutations
  async cellEdit(
    sessionId: string,
    payload: {
      database?: string;
      table: string;
      edits: Array<{
        database?: string;
        table: string;
        column: string;
        value: any;
        pk_column?: string;
        pk_value?: any;
        pk_values?: Record<string, any>;
        row_criteria?: Record<string, any>;
      }>;
    }
  ): Promise<{ success: boolean; rows_affected: number; applied_count: number }> {
    return this.request(`/session/${sessionId}/cell-edit`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async insertRow(
    sessionId: string,
    payload: {
      database?: string;
      table: string;
      data: Record<string, any>;
    }
  ): Promise<{ success: boolean; rows_affected: number }> {
    return this.request(`/session/${sessionId}/insert-row`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async deleteRows(
    sessionId: string,
    payload: {
      database?: string;
      table: string;
      pk_column?: string;
      pk_values?: any[];
      pk_rows?: Record<string, any>[];
      row_criterias?: Record<string, any>[];
    }
  ): Promise<{ success: boolean; rows_affected: number }> {
    return this.request(`/session/${sessionId}/delete-rows`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  // CSV Server-Side Streaming Import
  async importCsvFormData(
    sessionId: string,
    tableName: string,
    file: File,
    options: {
      database?: string;
      delimiter?: string;
      emptyAsNull?: boolean;
      skipErrors?: boolean;
      mapping?: Record<string, string>;
    }
  ): Promise<{ inserted: number; failed?: number; errors?: string[]; message?: string }> {
    const formData = new FormData();
    formData.append('file', file);
    formData.append('delimiter', options.delimiter || 'auto');
    formData.append('empty_as_null', String(options.emptyAsNull !== false));
    formData.append('skip_errors', String(Boolean(options.skipErrors)));
    if (options.mapping) {
      formData.append('mapping', JSON.stringify(options.mapping));
    }
    const qs = options.database ? `?db=${encodeURIComponent(options.database)}` : '';
    return this.request(`/session/${sessionId}/table/${encodeURIComponent(tableName)}/import-csv${qs}`, {
      method: 'POST',
      body: formData,
      timeoutMs: null,
    });
  }

  // Table Partitions & Diagnostics
  async getTablePartitions(sessionId: string, tableName: string, db = ''): Promise<TablePartition[]> {
    const q = db ? `?db=${encodeURIComponent(db)}` : '';
    return this.request(`/session/${sessionId}/partitions/${encodeURIComponent(tableName)}${q}`);
  }

  async getTableDiagnostics(sessionId: string, database = '', table = ''): Promise<any> {
    const params = new URLSearchParams();
    if (database) params.set('database', database);
    if (table) params.set('table', table);
    const qs = params.toString() ? `?${params.toString()}` : '';
    return this.request(`/session/${sessionId}/diagnostics/tables${qs}`);
  }

  async getDatabaseDiagnostics(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/databases`);
  }

  async getRedundantIndexes(sessionId: string, database = '', table = ''): Promise<any> {
    const params = new URLSearchParams();
    if (database) params.set('database', database);
    if (table) params.set('table', table);
    const qs = params.toString() ? `?${params.toString()}` : '';
    return this.request(`/session/${sessionId}/diagnostics/redundant-indexes${qs}`);
  }

  async executeMaintenance(
    sessionId: string,
    db: string,
    tables: string | string[],
    action: 'OPTIMIZE' | 'ANALYZE' | 'CHECK' | 'VACUUM' | 'REPAIR'
  ): Promise<any> {
    const tableList = Array.isArray(tables) ? tables : [tables];
    return this.request(`/session/${sessionId}/maintenance`, {
      method: 'POST',
      body: JSON.stringify({ database: db, tables: tableList, action }),
      timeoutMs: 60000,
    });
  }

  // Processlist & Server Management
  async getProcesslist(sessionId: string): Promise<{
    current_thread_id: number;
    threads: ProcesslistThread[];
    locks?: any[];
    questions?: number;
  }> {
    return this.request(`/session/${sessionId}/processlist`);
  }

  async killProcess(sessionId: string, threadId: number, mode: 'query' | 'connection' = 'query'): Promise<{ success: boolean }> {
    return this.request(`/session/${sessionId}/kill`, {
      method: 'POST',
      body: JSON.stringify({ thread_id: threadId, mode }),
    });
  }

  async batchKillProcesses(
    sessionId: string,
    threadIds: number[],
    mode: 'query' | 'connection' = 'connection'
  ): Promise<{ total: number; success_count: number; failed_count: number; mode: string; results?: Array<{ thread_id: number; success: boolean; error?: string }> }> {
    return this.request(`/session/${sessionId}/batch-kill`, {
      method: 'POST',
      body: JSON.stringify({ thread_ids: threadIds, mode }),
    });
  }

  async getDeadlock(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/deadlock`);
  }

  async getStatementDigest(sessionId: string, limit = 30): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/statement-digest?limit=${limit}`);
  }

  async resetStatementDigest(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/statement-digest/reset`, {
      method: 'POST',
    });
  }

  async getServerVariables(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/variables`);
  }

  async getClusterHealth(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/cluster`);
  }

  // Audit Logs
  async getAuditLogs(
    sessionId: string,
    params: { limit?: number; offset?: number; search?: string; source?: string } = {}
  ): Promise<{ logs: any[]; total: number; limit: number; offset: number }> {
    const qs = new URLSearchParams();
    if (params.limit) qs.set('limit', String(params.limit));
    if (params.offset !== undefined) qs.set('offset', String(params.offset));
    if (params.search) qs.set('search', params.search);
    if (params.source) qs.set('source', params.source);
    const q = qs.toString() ? `?${qs.toString()}` : '';
    return this.request(`/session/${sessionId}/audit-logs${q}`);
  }

  async clearAuditLogs(sessionId: string): Promise<{ success: boolean }> {
    return this.request(`/session/${sessionId}/audit-logs`, {
      method: 'DELETE',
    });
  }

  // Performance Flight Recorder
  async startFlightRecorder(sessionId: string, intervalMs = 1000): Promise<{ recording_id: string; started_at: string }> {
    return this.request(`/session/${sessionId}/flight-recorder/start`, {
      method: 'POST',
      body: JSON.stringify({ interval_ms: intervalMs }),
    });
  }

  async stopFlightRecorder(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/flight-recorder/stop`, {
      method: 'POST',
    });
  }

  async getFlightRecorderStatus(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/flight-recorder/status`);
  }

  async listFlightRecorderSessions(sessionId: string): Promise<any[]> {
    return this.request(`/session/${sessionId}/flight-recorder/sessions`);
  }

  async getFlightRecorderSession(sessionId: string, recId: string): Promise<any> {
    return this.request(`/session/${sessionId}/flight-recorder/sessions/${recId}`);
  }

  async deleteFlightRecorderSession(sessionId: string, recId: string): Promise<any> {
    return this.request(`/session/${sessionId}/flight-recorder/sessions/${recId}`, {
      method: 'DELETE',
    });
  }

  // Database Users & Privilege Matrix
  async listUsers(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/users`);
  }

  async getUserGrants(sessionId: string, user: string, host: string): Promise<any> {
    return this.request(`/session/${sessionId}/users/grants?user=${encodeURIComponent(user)}&host=${encodeURIComponent(host)}`);
  }

  async saveUser(sessionId: string, payload: { user: string; host: string; password?: string; is_new?: boolean }): Promise<any> {
    return this.request(`/session/${sessionId}/save-user`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async deleteUser(sessionId: string, payload: { user: string; host: string }): Promise<any> {
    const q = `?user=${encodeURIComponent(payload.user)}&host=${encodeURIComponent(payload.host)}`;
    return this.request(`/session/${sessionId}/delete-user${q}`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async applyGrants(sessionId: string, payload: {
    user: string;
    host: string;
    scope: string;
    database?: string;
    privileges: string[];
    with_grant?: boolean;
  }): Promise<any> {
    return this.request(`/session/${sessionId}/apply-grants`, {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  // Snippets
  async listSnippets(): Promise<any[]> {
    return this.request('/snippets');
  }

  async saveSnippet(snippet: Partial<QuerySnippet> & { title: string; sql: string }): Promise<QuerySnippet> {
    return this.request('/snippets', {
      method: 'POST',
      body: JSON.stringify({
        id: snippet.id,
        title: snippet.title,
        description: snippet.description,
        sql_content: snippet.sql,
        tags: (snippet.tags || []).join(', '),
      }),
    });
  }

  async deleteSnippet(id: string): Promise<any> {
    return this.request(`/snippets/${id}`, {
      method: 'DELETE',
    });
  }

  // Cloud Sync Settings
  async getCloudSyncConfig(): Promise<any> {
    return this.request('/cloud-sync/config');
  }

  async saveCloudSyncConfig(config: any): Promise<any> {
    return this.request('/cloud-sync/config', {
      method: 'POST',
      body: JSON.stringify(config),
    });
  }

  async getCloudSyncStatus(): Promise<any> {
    return this.request('/cloud-sync/status');
  }

  // Session Read-Only Toggle
  async toggleReadOnly(sessionId: string): Promise<{ read_only: boolean }> {
    return this.request(`/session/${sessionId}/toggle-readonly`, {
      method: 'POST',
    });
  }

  // Operational Notes — normalize between UI conveniences and backend OperationalNote fields
  // (`status`, `side_note`, `object_tags`, `user_tags`), preserving all fields losslessly.
  private normalizeNote(note: any): OperationalNote {
    if (!note || typeof note !== 'object') return note;
    const userTags: string[] = Array.isArray(note.user_tags) ? note.user_tags : [];
    const objectTags: string[] = Array.isArray(note.object_tags) ? note.object_tags : [];
    const category = note.category ?? userTags.find((t) => t.startsWith('#'))?.slice(1) ?? '';
    let tableName = note.table_name ?? '';
    if (!tableName) {
      const legacyTag = objectTags.find((t) => t.startsWith('db://'));
      if (legacyTag) {
        if (legacyTag.startsWith('db://table/')) {
          tableName = legacyTag.slice('db://table/'.length);
        } else {
          const parts = legacyTag.slice('db://'.length).split('/');
          tableName = parts[parts.length - 1] || '';
        }
      }
    }
    const status = note.status || (note.is_resolved ? 'resolved' : 'inbox');
    // Extract fenced SQL block if content includes one (for quick Open Query convenience)
    const sqlMatch = typeof note.content === 'string' ? note.content.match(/```sql\s*\n([\s\S]*?)```/i) : null;
    return {
      ...note,
      status,
      is_resolved: status === 'resolved',
      side_note: note.side_note ?? '',
      category,
      table_name: tableName,
      query_snapshot: note.query_snapshot ?? (sqlMatch ? sqlMatch[1].trim() : ''),
      user_tags: userTags,
      object_tags: objectTags,
    };
  }

  private denormalizeNote(note: any): any {
    const out: any = {
      title: note.title ?? 'Untitled Note',
      content: note.content ?? '',
      side_note: note.side_note ?? '',
      status: note.status || (note.is_resolved ? 'resolved' : 'inbox'),
      is_pinned: Boolean(note.is_pinned),
    };
    if (note.captured_at) out.captured_at = note.captured_at;
    if (note.reminder_at) out.reminder_at = note.reminder_at;

    const userTags: string[] = Array.isArray(note.user_tags) ? [...note.user_tags] : [];
    const category = (note.category || '').trim();
    if (category) {
      const tag = category.startsWith('#') ? category : `#${category}`;
      if (!userTags.includes(tag)) userTags.push(tag);
    }
    out.user_tags = userTags;

    const objectTags: string[] = Array.isArray(note.object_tags) ? [...note.object_tags] : [];
    const tableName = (note.table_name || '').trim();
    if (tableName && !objectTags.some((t) => t === `db://table/${tableName}` || t.endsWith(`/${tableName}`))) {
      objectTags.push(`db://table/${tableName}`);
    }
    out.object_tags = objectTags;

    if (note.id) out.id = note.id;
    return out;
  }

  async listNotes(filter?: { status?: string; object_tag?: string; user_tag?: string; q?: string; is_pinned?: boolean }): Promise<OperationalNote[]> {
    const params = new URLSearchParams();
    if (filter?.status) params.set('status', filter.status);
    if (filter?.object_tag) params.set('object_tag', filter.object_tag);
    if (filter?.user_tag) params.set('user_tag', filter.user_tag);
    if (filter?.q) params.set('q', filter.q);
    if (filter?.is_pinned !== undefined) params.set('is_pinned', String(filter.is_pinned));
    const qs = params.toString() ? `?${params.toString()}` : '';
    const notes = await this.request<any[]>(`/notes${qs}`);
    return (Array.isArray(notes) ? notes : []).map((n) => this.normalizeNote(n));
  }

  async createNote(note: {
    title: string;
    content: string;
    side_note?: string;
    status?: string;
    category?: string;
    table_name?: string;
    object_tags?: string[];
    user_tags?: string[];
    reminder_at?: string | null;
    is_pinned?: boolean;
  }): Promise<OperationalNote> {
    const res = await this.request('/notes', {
      method: 'POST',
      body: JSON.stringify(this.denormalizeNote(note)),
    });
    return this.normalizeNote(res);
  }

  async getNote(id: string): Promise<OperationalNote> {
    return this.normalizeNote(await this.request(`/notes/${id}`));
  }

  async updateNote(id: string, note: any): Promise<OperationalNote> {
    const res = await this.request(`/notes/${id}`, {
      method: 'PUT',
      body: JSON.stringify(this.denormalizeNote({ ...note, id })),
    });
    return this.normalizeNote(res);
  }

  async deleteNote(id: string): Promise<any> {
    return this.request(`/notes/${id}`, {
      method: 'DELETE',
    });
  }

  async snoozeNote(id: string, minutes: number): Promise<any> {
    return this.request(`/notes/${id}/snooze`, {
      method: 'POST',
      body: JSON.stringify({ minutes }),
    });
  }

  async resolveNote(id: string): Promise<any> {
    return this.request(`/notes/${id}/resolve`, {
      method: 'POST',
    });
  }

  async getDueReminders(): Promise<any[]> {
    const res = await this.request<any[]>('/notes/reminders/due');
    return (Array.isArray(res) ? res : []).map((n) => this.normalizeNote(n));
  }

  async getTableNote(sessionId: string, table: string): Promise<{ content: string; updated_at?: string }> {
    return this.request(`/session/${sessionId}/notes/${encodeURIComponent(table)}`);
  }

  async saveTableNote(sessionId: string, table: string, content: string): Promise<any> {
    return this.request(`/session/${sessionId}/notes/${encodeURIComponent(table)}`, {
      method: 'POST',
      body: JSON.stringify({ content }),
    });
  }

  // Smart Mock Data Generator
  async generateMockData(
    sessionId: string,
    tableName: string,
    options: { rows: number; database?: string }
  ): Promise<any> {
    return this.request(`/session/${sessionId}/table/${encodeURIComponent(tableName)}/mock-data`, {
      method: 'POST',
      body: JSON.stringify(options),
      timeoutMs: 60000,
    });
  }

  // Smart Seed SQL Export URL
  exportSmartSeedUrl(sessionId: string, options: { limit?: number; includeDDL?: boolean; db?: string } = {}): string {
    const params = new URLSearchParams();
    params.set('limit', String(options.limit || 20));
    params.set('ddl', String(options.includeDDL !== false));
    if (options.db) params.set('db', options.db);
    return `/api/session/${sessionId}/export-smart-seed?${params.toString()}`;
  }

  // System Update & Changelog
  async checkSystemUpdate(force = false): Promise<any> {
    return this.request(`/system/update/check${force ? '?force=true' : ''}`, { timeoutMs: 20000 });
  }

  async applySystemUpdate(assetUrl: string, version?: string): Promise<any> {
    return this.request('/system/update/apply', {
      method: 'POST',
      body: JSON.stringify({ asset_url: assetUrl, version }),
      timeoutMs: null,
    });
  }

  async getSystemChangelog(): Promise<any> {
    return this.request('/system/changelog');
  }

  // System Lifecycle
  async openInSystemBrowser(url: string): Promise<{ ok?: boolean; status?: string }> {
    return this.request('/system/open-browser', {
      method: 'POST',
      body: JSON.stringify({ url }),
    });
  }

  async shutdownDaemon(): Promise<{ ok?: boolean; status?: string }> {
    return this.request('/system/shutdown', { method: 'POST', timeoutMs: 5000 });
  }

  // AI Assistant
  async aiChat(
    promptOrOpts: string | {
      action?: 'chat' | 'explain_query' | 'explain_plan' | 'optimize' | 'nl_to_sql' | 'fix_query';
      profile_id?: string;
      prompt: string;
      query?: string;
      plan?: string;
      error_message?: string;
      connection_id?: string;
      config?: AiConfig;
    },
    queryArg = '',
    _dialectArg = 'mysql',
    configArg: AiConfig = loadAiConfig()
  ): Promise<{ content: string; model?: string }> {
    const opts =
      typeof promptOrOpts === 'string'
        ? { action: 'chat' as const, prompt: promptOrOpts, query: queryArg, config: configArg }
        : promptOrOpts;
    const cfg = opts.config || loadAiConfig();

    return this.request('/ai/chat', {
      method: 'POST',
      timeoutMs: 65000,
      body: JSON.stringify({
        action: opts.action || 'chat',
        profile_id: opts.profile_id || '',
        prompt: opts.prompt,
        query: opts.query || '',
        plan: opts.plan || '',
        error_message: opts.error_message || '',
        connection_id: opts.connection_id || '',
        provider: cfg.provider,
        api_key: cfg.apiKey,
        model: cfg.modelName,
        endpoint: cfg.baseUrl,
        mask_identifiers: cfg.maskIdentifiers,
        language: cfg.language,
        response_style: cfg.responseStyle,
        custom_prompt: cfg.customPrompt,
        include_schema: cfg.includeSchema,
        temperature: cfg.temperature,
      }),
    });
  }

  async listAiProfiles(): Promise<AIProfile[]> {
    return this.request('/ai/profiles');
  }

  async getAiProfile(id: string): Promise<AIProfile> {
    return this.request(`/ai/profiles/${encodeURIComponent(id)}`);
  }

  async createAiProfile(profile: Partial<AIProfile>): Promise<AIProfile> {
    return this.request('/ai/profiles', {
      method: 'POST',
      body: JSON.stringify(profile),
    });
  }

  async updateAiProfile(id: string, profile: Partial<AIProfile>): Promise<AIProfile> {
    return this.request(`/ai/profiles/${encodeURIComponent(id)}`, {
      method: 'PUT',
      body: JSON.stringify(profile),
    });
  }

  async deleteAiProfile(id: string): Promise<{ status: string }> {
    return this.request(`/ai/profiles/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
  }

  async setActiveAiProfile(id: string): Promise<{ status: string; active_id: string }> {
    return this.request(`/ai/profiles/${encodeURIComponent(id)}/active`, {
      method: 'POST',
    });
  }

  async checkOllama(endpoint = ''): Promise<{ available: boolean }> {
    const qs = endpoint ? `?endpoint=${encodeURIComponent(endpoint)}` : '';
    return this.request(`/ai/ollama-check${qs}`, { timeoutMs: 5000 });
  }

  // Active Threads & Safety
  async getActiveThreads(sessionId: string): Promise<{ active_threads: number }> {
    return this.request(`/session/${sessionId}/active-threads`);
  }

  // Foreign Key Peek
  async peekForeignKey(sessionId: string, database: string, table: string, column: string, value: any): Promise<any> {
    const q = `database=${encodeURIComponent(database)}&table=${encodeURIComponent(table)}&column=${encodeURIComponent(column)}&value=${encodeURIComponent(String(value ?? ''))}`;
    return this.request(`/session/${sessionId}/peek?${q}`);
  }

  // Cross-Connection Data Transfer
  async transferData(
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
    onProgress?: (evt: any) => void
  ): Promise<any> {
    const headers = this.getAuthHeaders({}, true);
    let res: Response;
    try {
      res = await fetch('/api/sync/data/transfer', {
        method: 'POST',
        headers,
        body: JSON.stringify(payload),
      });
    } catch (err) {
      if (typeof window !== 'undefined') (window as any).__sqlfox_trigger_reconnect?.();
      throw new Error(`Cannot reach SqlFox daemon: ${err instanceof Error ? err.message : String(err)}`);
    }
    if (!res.ok) {
      await this.handleNonOkResponse(res);
    }
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
        } catch { /* ignore malformed */ }
      }
    }
    if (buf.trim()) {
      try { handleEvt(JSON.parse(buf.trim())); } catch { /* ignore */ }
    }
    if (tableErrors.length > 0 && !result.error) {
      result.has_errors = true;
      result.error = tableErrors.join('; ');
    }
    return result;
  }

  // Row-Level Data Sync
  async compareDataSync(payload: {
    source_session_id: string;
    source_db?: string;
    target_session_id: string;
    target_db?: string;
    table: string;
    delete_missing?: boolean;
  }): Promise<any> {
    return this.request('/sync/data/compare', {
      method: 'POST',
      body: JSON.stringify(payload),
      timeoutMs: 60000,
    });
  }

  async applyDataSync(payload: {
    target_session_id: string;
    target_db?: string;
    table: string;
    columns: string[];
    operations: Array<{ type: string; pk: any; old_data?: any[]; data?: any[] }>;
    confirmed?: boolean;
    challenge_input?: string;
  }): Promise<any> {
    return this.request('/sync/data/apply', {
      method: 'POST',
      body: JSON.stringify(payload),
      timeoutMs: 60000,
    });
  }

  // Structure Synchronization
  async compareStructure(payload: {
    source_session_id: string;
    source_db?: string;
    target_session_id: string;
    target_db?: string;
    include_drops?: boolean;
  }): Promise<{ diffs: any[] }> {
    return this.request('/sync/structure/compare', {
      method: 'POST',
      body: JSON.stringify(payload),
      timeoutMs: 60000,
    });
  }

  async applyStructure(payload: {
    target_session_id: string;
    target_db?: string;
    statements: string[];
    confirmed?: boolean;
    challenge_input?: string;
  }): Promise<{ success: boolean; executed: number; errors?: string[]; blocked?: boolean; requires_confirm?: boolean; message?: string }> {
    return this.request('/sync/structure/apply', {
      method: 'POST',
      body: JSON.stringify(payload),
      timeoutMs: 60000,
    });
  }

  // Cloud Sync & Notes Bundle
  async testCloudSync(payload: { github_token?: string; github_repo: string; github_branch?: string }): Promise<{ success: boolean; message?: string }> {
    return this.request('/cloud-sync/test', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async runCloudSync(): Promise<{ success: boolean; message?: string }> {
    return this.request('/cloud-sync/run', {
      method: 'POST',
      timeoutMs: 60000,
    });
  }

  async importNotesBundle(bundle: File): Promise<{ status?: string; imported_count: number; message?: string }> {
    const formData = new FormData();
    formData.append('bundle', bundle);
    return this.request('/notes/bundle/import', {
      method: 'POST',
      body: formData,
      timeoutMs: 60000,
    });
  }

  // Large SQL Script Streaming Runner
  async runSqlScript(
    sessionId: string,
    file: File,
    options: { stopOnError?: boolean; database?: string; confirm?: boolean },
    onProgress: (event: any) => void
  ): Promise<{ status: string; total: number; executed: number; errors: number }> {
    const formData = new FormData();
    formData.append('file', file);

    const headers = this.getAuthHeaders({}, false);
    const params = new URLSearchParams();
    if (options.stopOnError !== undefined) params.set('stop_on_error', String(options.stopOnError));
    if (options.database) params.set('db', options.database);
    if (options.confirm) params.set('confirm', 'true');

    const res = await fetch(`/api/session/${sessionId}/run-sql-script?${params.toString()}`, {
      method: 'POST',
      headers,
      body: formData,
    });

    if (!res.ok) {
      await this.handleNonOkResponse(res);
    }

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
        } catch { /* ignore */ }
      }
    }

    if (streamBuf.trim()) {
      try {
        processItem(JSON.parse(streamBuf.trim()));
      } catch { /* ignore */ }
    }

    return {
      status: errors > 0 && options.stopOnError ? 'halted' : finalStatus,
      total,
      executed,
      errors,
    };
  }

  // Diagnostic Logs (Persistent Rotating Log File & Client Telemetry)
  async getLogFile(tail = 500): Promise<{ path: string; size_bytes: number; rotated_count: number; lines: string[] }> {
    return this.request(`/logs/file?tail=${encodeURIComponent(String(tail))}`);
  }

  reportClientLog(payload: { level: string; module: string; message: string; stack?: string }) {
    try {
      const headers = this.getAuthHeaders({}, true);
      void fetch('/api/logs/client', {
        method: 'POST',
        headers,
        body: JSON.stringify(payload),
        keepalive: true,
      }).catch(() => {});
    } catch { /* ignore telemetry errors */ }
  }
}

export const api = new ApiClient();
