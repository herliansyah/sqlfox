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

  // Sessions
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

  async getTableDDL(connId: string, tableName: string): Promise<{ ddl: string }> {
    return this.request(`/session/${connId}/ddl/${encodeURIComponent(tableName)}`);
  }

  async getRoutineDDL(connId: string, routineName: string, type = 'PROCEDURE'): Promise<{ ddl: string }> {
    return this.request(`/session/${connId}/ddl/routine/${encodeURIComponent(routineName)}?type=${encodeURIComponent(type)}`);
  }

  async getFastCatalog(sessionId: string): Promise<Record<string, TableMetadata[]>> {
    return this.request(`/session/${sessionId}/fast-catalog`);
  }

  async getTableRows(sessionId: string, tableName: string, limit = 100, offset = 0): Promise<QueryExecutionResult> {
    return this.request(`/session/${sessionId}/table/${encodeURIComponent(tableName)}/rows?limit=${limit}&offset=${offset}`);
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

  async getTableDiagnostics(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/tables`);
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

  async listUsers(sessionId: string): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/users`);
  }

  async getUserGrants(sessionId: string, user: string, host: string): Promise<any> {
    return this.request(`/session/${sessionId}/diagnostics/grants?user=${encodeURIComponent(user)}&host=${encodeURIComponent(host)}`);
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
}

export const api = new ApiClient();
