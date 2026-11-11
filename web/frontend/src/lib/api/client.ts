import type {
  ConnectionProfile,
  SchemaCatalog,
  TableMetadata,
  QueryExecutionResult,
  InspectionResult,
  ProcesslistThread,
  TablePartition,
} from '../../types';

class ApiClient {
  private baseUrl = '/api';

  private async request<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
    const url = `${this.baseUrl}${endpoint}`;
    const token = typeof sessionStorage !== 'undefined' ? sessionStorage.getItem('sqlfox_remote_token') : null;
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      ...((options.headers as Record<string, string>) || {}),
    };

    if (token && !headers['Authorization']) {
      headers['Authorization'] = `Bearer ${token}`;
    }

    const res = await fetch(url, { ...options, headers });
    if (!res.ok) {
      if (res.status === 401 && typeof window !== 'undefined' && (window as any).__sqlfox_set_remote_locked) {
        (window as any).__sqlfox_set_remote_locked(true);
      }
      let errorMsg = `HTTP Error ${res.status}`;
      try {
        const data = await res.json();
        if (data && data.error) {
          errorMsg = data.error;
        } else if (data && data.reason) {
          errorMsg = data.reason;
        } else if (data && data.message) {
          errorMsg = data.message;
        }
      } catch {
        if (res.statusText) errorMsg = res.statusText;
      }
      throw new Error(errorMsg);
    }

    return res.json() as Promise<T>;
  }

  // Auth / Vault
  async getAuthStatus(): Promise<{ is_remote: boolean; unlocked: boolean; version?: string }> {
    return this.request('/auth/status');
  }

  async getMasterKey(): Promise<{ lan_enabled: boolean; master_password?: string; lan_ip?: string }> {
    return this.request('/auth/master-key');
  }

  async unlockVault(password: string): Promise<{ success: boolean; token?: string }> {
    return this.request('/auth/unlock', {
      method: 'POST',
      body: JSON.stringify({ password }),
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

  async testConnection(id: string): Promise<{ success: boolean; version?: string; latency_ms?: number }> {
    return this.request(`/connections/${id}/test`, {
      method: 'POST',
    });
  }

  async testTransientConnection(conn: Partial<ConnectionProfile>): Promise<{ success: boolean; version?: string; latency_ms?: number }> {
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
    });
  }

  async getSchema(connId: string, dbName = ''): Promise<SchemaCatalog> {
    const query = dbName ? `?db=${encodeURIComponent(dbName)}` : '';
    return this.request(`/session/${connId}/schema${query}`);
  }

  async selectDatabase(connId: string, database: string): Promise<SchemaCatalog> {
    return this.request(`/session/${connId}/select-db`, {
      method: 'POST',
      body: JSON.stringify({ database }),
    });
  }

  async getTableDDL(connId: string, tableName: string, db?: string): Promise<{ ddl: string }> {
    const qs = db ? `?db=${encodeURIComponent(db)}` : '';
    return this.request(`/session/${connId}/ddl/${encodeURIComponent(tableName)}${qs}`);
  }

  async getRoutineDDL(connId: string, routineName: string, type = 'PROCEDURE'): Promise<{ ddl: string }> {
    return this.request(`/session/${connId}/ddl/routine/${encodeURIComponent(routineName)}?type=${encodeURIComponent(type)}`);
  }

  async getFastCatalog(sessionId: string): Promise<Record<string, TableMetadata[]>> {
    return this.request(`/session/${sessionId}/fast-catalog`);
  }

  async getTableRows(
    sessionId: string,
    tableName: string,
    limit = 50,
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
    return this.request(url);
  }

  async executeQuery(
    sessionId: string,
    sql: string,
    confirmed = false
  ): Promise<QueryExecutionResult> {
    return this.request(`/session/${sessionId}/execute`, {
      method: 'POST',
      body: JSON.stringify({ sql, confirmed }),
    });
  }

  async cancelQuery(sessionId: string): Promise<{ success: boolean }> {
    return this.request(`/session/${sessionId}/cancel`, {
      method: 'POST',
    });
  }

  async inspectQuery(sessionId: string, sql: string): Promise<InspectionResult> {
    return this.request(`/session/${sessionId}/inspect`, {
      method: 'POST',
      body: JSON.stringify({ sql }),
    });
  }

  async explainQuery(sessionId: string, sql: string): Promise<any> {
    return this.request(`/session/${sessionId}/explain`, {
      method: 'POST',
      body: JSON.stringify({ sql }),
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
      row_criterias?: Record<string, any>[];
    }
  ): Promise<{ success: boolean; rows_affected: number }> {
    return this.request(`/session/${sessionId}/delete-rows`, {
      method: 'POST',
      body: JSON.stringify(payload),
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

  // Processlist & Server Management
  async getProcesslist(sessionId: string): Promise<{
    current_thread_id: number;
    threads: ProcesslistThread[];
    locks?: any[];
  }> {
    return this.request(`/session/${sessionId}/processlist`);
  }

  async killProcess(sessionId: string, threadId: number): Promise<{ success: boolean }> {
    return this.request(`/session/${sessionId}/kill`, {
      method: 'POST',
      body: JSON.stringify({ thread_id: threadId }),
    });
  }

  async batchKillProcesses(sessionId: string, threadIds: number[], mode: 'query' | 'connection' = 'connection'): Promise<{ killed_count?: number; errors?: string[] }> {
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

  async getDatabaseDiagnostics(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/databases`);
  }

  async getRedundantIndexes(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/redundant-indexes`);
  }

  async executeMaintenance(sessionId: string, db: string, table: string, operation: 'OPTIMIZE' | 'ANALYZE' | 'CHECK' | 'REPAIR'): Promise<any> {
    return this.request(`/session/${sessionId}/maintenance`, {
      method: 'POST',
      body: JSON.stringify({ database: db, table, operation }),
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

  async saveSnippet(snippet: { id?: string; title: string; description?: string; sql: string; tags?: string[] }): Promise<any> {
    return this.request('/snippets', {
      method: 'POST',
      body: JSON.stringify(snippet),
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

  // Session Read-Only Toggle
  async toggleReadOnly(sessionId: string): Promise<{ read_only: boolean }> {
    return this.request(`/session/${sessionId}/toggle-readonly`, {
      method: 'POST',
    });
  }

  // Operational Notes
  async listNotes(): Promise<any[]> {
    return this.request('/notes');
  }

  async createNote(note: {
    title: string;
    content: string;
    category?: string;
    table_name?: string;
    query_snapshot?: string;
    reminder_at?: string;
    is_pinned?: boolean;
  }): Promise<any> {
    return this.request('/notes', {
      method: 'POST',
      body: JSON.stringify(note),
    });
  }

  async getNote(id: string): Promise<any> {
    return this.request(`/notes/${id}`);
  }

  async updateNote(id: string, note: any): Promise<any> {
    return this.request(`/notes/${id}`, {
      method: 'PUT',
      body: JSON.stringify(note),
    });
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
    return this.request('/notes/reminders/due');
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
    options: { row_count: number; batch_size?: number; truncate_first?: boolean; locale?: string }
  ): Promise<any> {
    return this.request(`/session/${sessionId}/table/${encodeURIComponent(tableName)}/mock-data`, {
      method: 'POST',
      body: JSON.stringify(options),
    });
  }

  // System Update & Changelog
  async checkSystemUpdate(force = false): Promise<any> {
    return this.request(`/system/update/check${force ? '?force=true' : ''}`);
  }

  async applySystemUpdate(version?: string): Promise<any> {
    return this.request('/system/update/apply', {
      method: 'POST',
      body: JSON.stringify({ version }),
    });
  }

  async getSystemChangelog(): Promise<any> {
    return this.request('/system/changelog');
  }

  // AI Assistant
  async aiChat(
    prompt: string,
    context?: string,
    dialect = 'mysql'
  ): Promise<{ response: string; error?: string }> {
    return this.request('/ai/chat', {
      method: 'POST',
      body: JSON.stringify({ prompt, context, dialect }),
    });
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
  async transferData(payload: {
    source_connection_id: string;
    source_database?: string;
    source_table: string;
    target_connection_id: string;
    target_database?: string;
    target_table?: string;
    truncate_target?: boolean;
    batch_size?: number;
  }): Promise<any> {
    return this.request('/sync/data/transfer', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  // Row-Level Data Sync
  async compareDataSync(payload: {
    source_connection_id: string;
    source_database?: string;
    source_table: string;
    target_connection_id: string;
    target_database?: string;
    target_table: string;
    primary_key?: string;
  }): Promise<any> {
    return this.request('/sync/data/compare', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  async applyDataSync(payload: {
    target_connection_id: string;
    target_database?: string;
    target_table: string;
    sync_statements: string[];
  }): Promise<any> {
    return this.request('/sync/data/apply', {
      method: 'POST',
      body: JSON.stringify(payload),
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
    });
  }

  async applyStructure(payload: {
    target_session_id: string;
    target_db?: string;
    sqls: string[];
  }): Promise<{ success: boolean; applied_count: number }> {
    return this.request('/sync/structure/apply', {
      method: 'POST',
      body: JSON.stringify(payload),
    });
  }

  // Cloud Sync & Notes Bundle
  async testCloudSync(token: string, gistId?: string): Promise<{ success: boolean; message?: string }> {
    return this.request('/cloud-sync/test', {
      method: 'POST',
      body: JSON.stringify({ token, gist_id: gistId }),
    });
  }

  async runCloudSync(): Promise<{ success: boolean; message?: string }> {
    return this.request('/cloud-sync/run', {
      method: 'POST',
    });
  }

  async importNotesBundle(bundle: any): Promise<{ success: boolean; imported_count: number }> {
    return this.request('/notes/bundle/import', {
      method: 'POST',
      body: JSON.stringify(bundle),
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

    const token = typeof sessionStorage !== 'undefined' ? sessionStorage.getItem('sqlfox_remote_token') : null;
    const headers: Record<string, string> = {};
    if (token) headers['Authorization'] = `Bearer ${token}`;

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
      let errMsg = `HTTP ${res.status}`;
      try {
        const errObj = await res.json();
        errMsg = errObj.error || errObj.message || errMsg;
      } catch {
        const text = await res.text();
        if (text) errMsg = text;
      }
      throw new Error(errMsg);
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
          const item = JSON.parse(trimmed);
          if (item.total) total = item.total;
          if (item.status === 'ok') executed++;
          if (item.status === 'error') errors++;
          onProgress(item);
        } catch (_) {}
      }
    }

    if (streamBuf.trim()) {
      try {
        const item = JSON.parse(streamBuf.trim());
        if (item.total) total = item.total;
        if (item.status === 'ok') executed++;
        if (item.status === 'error') errors++;
        onProgress(item);
      } catch (_) {}
    }

    return {
      status: errors > 0 && options.stopOnError ? 'halted' : finalStatus,
      total,
      executed,
      errors,
    };
  }
}

export const api = new ApiClient();
