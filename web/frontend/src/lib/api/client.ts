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
}

export const api = new ApiClient();
