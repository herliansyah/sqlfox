import type { ConnectionProfile, SchemaCatalog, Toast } from '../../types';
import { api } from '../api/client';

export interface ConnectionActionsContext {
  activeConnection: ConnectionProfile | null;
  activeSessionId: string | null;
  currentDatabase: string;
  availableDatabases: string[];
  isLoadingCatalog: boolean;
  isPingingServer: boolean;
  serverLatency: number | null;
  serverMemoryMb: number | null;
  serverInfo: any;
  isReadOnly: boolean;
  readonly isProduction: boolean;
  setCatalog(catalog: SchemaCatalog | null): void;
  clearCatalogCache(): void;
  loadConnections(): Promise<void>;
  showToast(msg: string, type?: Toast['type']): string;
  showNativeConfirm(message: string, title?: string): Promise<boolean>;
}

export async function deleteConnectionAction(ctx: ConnectionActionsContext, id: string): Promise<void> {
  try {
    await api.deleteConnection(id);
    ctx.showToast('Connection deleted', 'info');
    try {
      localStorage.removeItem(`sqlfox_tabs_${id}`);
    } catch { /* ignore */ }
    if (ctx.activeConnection?.id === id) {
      ctx.activeConnection = null;
      ctx.activeSessionId = null;
      ctx.clearCatalogCache();
      if (typeof sessionStorage !== 'undefined') sessionStorage.removeItem('sqlfox_active_conn_id');
    }
    await ctx.loadConnections();
  } catch (err: any) {
    ctx.showToast(`Failed to delete connection: ${err.message}`, 'error');
  }
}

export async function refreshDatabasesAction(ctx: ConnectionActionsContext): Promise<void> {
  if (!ctx.activeSessionId) return;
  ctx.isLoadingCatalog = true;
  try {
    const schema = await api.getSchema(ctx.activeSessionId, ctx.currentDatabase || '');
    if (schema.databases && schema.databases.length > 0) {
      ctx.availableDatabases = schema.databases;
    }
    if (schema.database_name) {
      ctx.currentDatabase = schema.database_name;
    }
    if (schema.tables) {
      ctx.setCatalog(schema);
    }
  } catch (err) {
    console.error('Failed to reload databases:', err);
  } finally {
    ctx.isLoadingCatalog = false;
  }
}

export async function pingActiveSessionAction(ctx: ConnectionActionsContext): Promise<void> {
  if (!ctx.activeSessionId) return;
  ctx.isPingingServer = true;
  try {
    const res = await api.pingSession(ctx.activeSessionId);
    if (res) {
      ctx.serverLatency = res.latency_ms;
      if (res.memory_mb != null) ctx.serverMemoryMb = res.memory_mb;
      if (res.server_info) ctx.serverInfo = res.server_info;
      if (typeof res.read_only === 'boolean') ctx.isReadOnly = res.read_only;
    }
  } catch (_) {
    ctx.serverLatency = -1;
  } finally {
    ctx.isPingingServer = false;
  }
}

export async function toggleReadOnlyModeAction(ctx: ConnectionActionsContext): Promise<void> {
  if (!ctx.activeSessionId) {
    ctx.isReadOnly = !ctx.isReadOnly;
    ctx.showToast(
      ctx.isReadOnly
        ? 'Read-only mode enabled — mutating statements will be blocked'
        : 'Read-only mode disabled — writes are now allowed',
      ctx.isReadOnly ? 'warning' : 'success'
    );
    return;
  }
  if (ctx.isReadOnly && ctx.isProduction) {
    const ok = await ctx.showNativeConfirm(
      `Disable Read-Only Protection on PRODUCTION connection "${ctx.activeConnection?.name}"?\nMutating queries (INSERT, UPDATE, DELETE, DROP, ALTER) will be allowed.`,
      'Production Write Confirmation'
    );
    if (!ok) return;
  }
  try {
    const res = await api.toggleReadOnly(ctx.activeSessionId);
    ctx.isReadOnly = Boolean(res.read_only);
    ctx.showToast(
      ctx.isReadOnly
        ? 'Read-only mode enabled — mutating statements will be blocked'
        : 'Read-only mode disabled — writes are now allowed',
      ctx.isReadOnly ? 'warning' : 'success'
    );
  } catch (err: any) {
    ctx.showToast(`Failed to toggle read-only mode: ${err.message}`, 'error');
  }
}
