import type {
  ConnectionProfile,
  SchemaCatalog,
  EditorTab,
  InspectionResult,
  QueryExecutionResult,
  TableMetadata,
  TableMode,
  FilterPill,
  StagedEdit,
  Toast,
  TriggerMetadata,
  RoutineMetadata,
  EventMetadata,
  NativeDialogState,
  DdlReviewState,
  QuerySnippet,
  SessionAuditItem,
  ToastAction,
} from '../../types';
import { api } from '../api/client';
import { copyText } from '../utils/clipboard';
import { substituteSnippetVariables, parseSnippetVariables } from '../utils/snippets';
import {
  quoteSqlIdentifier,
  extractSingleTableFromSQL,
  hasExplicitSQLLimit,
  wrapPagedSQL,
} from '../utils/sqlStatements';

// ponytail: smart wildcard auto-wrap for LIKE/NOT LIKE filter
export function compileFilterPills(pills: FilterPill[], driver?: string): string {
  if (!Array.isArray(pills) || pills.length === 0) return '';
  const isPg = driver === 'postgres' || driver === 'postgresql';
  const clauses: string[] = [];
  for (const p of pills) {
    if (!p || !p.col) continue;
    const cleanCol = p.col.replace(/^[`"\[]+|[`"\]]+$/g, '');
    const colEscaped = isPg ? `"${cleanCol.replace(/"/g, '""')}"` : `\`${cleanCol.replace(/`/g, '``')}\``;
    const op = (p.op || '=').toUpperCase();
    if (op === 'IS NULL' || op === 'IS NOT NULL') {
      clauses.push(`${colEscaped} ${op}`);
      continue;
    }
    const val = String(p.val !== undefined ? p.val : '').trim();
    if (val === '') continue;
    if (op === 'IN' || op === 'NOT IN') {
      if (val.startsWith('(') && val.endsWith(')') && !val.includes(';')) {
        clauses.push(`${colEscaped} ${op} ${val}`);
      } else {
        const items = val.split(',').map((s) => s.trim()).filter(Boolean);
        const formatted = items
          .map((item) => {
            if (!isNaN(Number(item)) && item !== '') return item;
            return `'${item.replace(/'/g, "''")}'`;
          })
          .join(', ');
        clauses.push(`${colEscaped} ${op} (${formatted})`);
      }
    } else if (op === 'LIKE' || op === 'NOT LIKE') {
      let finalVal = val;
      if (!val.includes('%') && !val.includes('_')) {
        finalVal = `%${val}%`;
      }
      clauses.push(`${colEscaped} ${op} '${finalVal.replace(/'/g, "''")}'`);
    } else if (op === 'BETWEEN') {
      // Two literals only — quote any part that is not a plain number to block injection.
      const parts = val.split(/\s+AND\s+/i).slice(0, 2);
      const fmt = (part: string) => {
        const p = part.trim().replace(/^'(.*)'$/s, '$1');
        return /^-?\d+(\.\d+)?$/.test(p) ? p : `'${p.replace(/'/g, "''")}'`;
      };
      if (parts.length === 2) {
        clauses.push(`${colEscaped} BETWEEN ${fmt(parts[0])} AND ${fmt(parts[1])}`);
      } else {
        clauses.push(`${colEscaped} BETWEEN '${val.replace(/'/g, "''")}'`);
      }
    } else {
      const isNum = !isNaN(Number(val)) && val !== '' && !val.includes(' ');
      if (isNum) {
        clauses.push(`${colEscaped} ${op} ${val}`);
      } else {
        clauses.push(`${colEscaped} ${op} '${val.replace(/'/g, "''")}'`);
      }
    }
  }
  return clauses.join(' AND ');
}


export interface SafetyChallengeState {
  isOpen: boolean;
  sql: string;
  inspection: InspectionResult | null;
  confirmed: boolean;
  challengeInput?: string;
  resolve?: (result: { confirmed: boolean; challengeInput: string }) => void;
}

export interface ActivityLogEntry {
  id: string;
  time: string;
  timeMs?: string;
  level: string;
  module: string;
  message: string;
  attrs?: Record<string, any>;
}

export class AppState {
  connections = $state<ConnectionProfile[]>([]);
  activeConnection = $state<ConnectionProfile | null>(null);
  activeSessionId = $state<string | null>(null);
  currentDatabase = $state<string>('');
  availableDatabases = $state<string[]>([]);
  databaseTableCounts = $state<Record<string, number>>({});
  catalog = $state.raw<SchemaCatalog | null>(null);
  // ponytail: catalogs hold large schema graphs (thousands of tables/columns). Deep $state
  // proxies made every property read go through Svelte's proxy traps, which made typing in
  // the editor extremely slow. $state.raw keeps them plain objects; we replace the map
  // wholesale on write so reactivity still fires.
  catalogs = $state.raw<Record<string, SchemaCatalog>>({});
  snippets = $state<QuerySnippet[]>([]);
  snippetsLoaded = false;
  private catalogRequests = new Map<string, Promise<SchemaCatalog>>();
  pendingSnippet = $state<{ snippet: QuerySnippet; values: Record<string, string>; insert?: (sql: string) => void } | null>(null);
  private editorInserter: ((sql: string) => boolean) | null = null;
  private editorTargetProvider: (() => string | null) | null = null;
  isLoadingCatalog = $state<boolean>(false);
  isConnecting = $state<boolean>(false);
  connectionError = $state<string | null>(null);

  // Tabs management
  tabs = $state<EditorTab[]>([
    { id: 'tab-1', title: 'Query 1.sql', sql: 'SELECT 1;', isTableViewer: false },
  ]);
  activeTabId = $state<string>('tab-1');

  // Staged Edits for Grid CRUD
  stagedEdits = $state<Record<string, StagedEdit>>({});
  gridRowUids = new WeakMap<object, number>();
  private nextRowUid = 1;

  /**
   * Grid commit strategy. "staged" accumulates cell edits until the user hits
   * Apply; "auto" persists each edit as soon as the cell loses focus.
   */
  gridEditMode = $state<'staged' | 'auto'>(
    (typeof localStorage !== 'undefined' && localStorage.getItem('sqlfox_grid_edit_mode') === 'auto'
      ? 'auto'
      : 'staged') as 'staged' | 'auto'
  );
  gridConfirmSave = $state<boolean>(
    typeof localStorage !== 'undefined' && localStorage.getItem('sqlfox_grid_confirm_save') === 'true'
  );

  // Toasts
  toasts = $state<Toast[]>([]);

  // Safety Guard challenge modal
  safetyChallenge = $state<SafetyChallengeState>({
    isOpen: false,
    sql: '',
    inspection: null,
    confirmed: false,
  });

  // Global Modals State
  isConnectionModalOpen = $state<boolean>(false);
  connectionToEdit = $state<ConnectionProfile | null>(null);
  isCommandPaletteOpen = $state<boolean>(false);
  isShortcutsModalOpen = $state<boolean>(false);
  isCreateDbModalOpen = $state<boolean>(false);
  isTableCreatorOpen = $state<boolean>(false);
  isRenameTableModalOpen = $state<boolean>(false);
  tableToRename = $state<string>('');
  isDuplicateTableModalOpen = $state<boolean>(false);
  tableToDuplicate = $state<string>('');
  isViewDesignerOpen = $state<boolean>(false);
  viewToEdit = $state<{ name: string; query?: string } | null>(null);
  isTriggerDesignerOpen = $state<boolean>(false);
  triggerToEdit = $state<TriggerMetadata | null>(null);
  isRoutineDesignerOpen = $state<boolean>(false);
  routineToEdit = $state<RoutineMetadata | null>(null);
  isEventDesignerOpen = $state<boolean>(false);
  eventToEdit = $state<EventMetadata | null>(null);
  isDumpModalOpen = $state<boolean>(false);
  dumpTargetTable = $state<string>('');
  isStructureSyncModalOpen = $state<boolean>(false);
  isRunSqlModalOpen = $state<boolean>(false);
  isSnippetsModalOpen = $state<boolean>(false);
  isCsvImportModalOpen = $state<boolean>(false);
  isPrivilegeMatrixOpen = $state<boolean>(false);
  isDbDiagnosticsOpen = $state<boolean>(false);
  isLockInspectorOpen = $state<boolean>(false);
  isAiDrawerOpen = $state<boolean>(
    typeof localStorage !== 'undefined' && localStorage.getItem('sqlfox_copilot_visible') === 'true'
  );
  /** Prefill text handed to the AI Copilot drawer by "Send to AI" actions. */
  aiPromptDraft = $state<string>('');
  aiQuickRunRequest = $state<{ action: 'fix_query' | 'explain_query' | 'explain_plan' | 'optimize'; sql: string; error?: string; plan?: string; nonce: number } | null>(null);
  isAiSettingsModalOpen = $state<boolean>(false);
  isNotesDrawerOpen = $state<boolean>(false);
  isNoteComposerModalOpen = $state<boolean>(false);
  editingNote = $state<any>(null);
  dueNotesCount = $state<number>(0);
  notesRevision = $state<number>(0);
  private notifiedReminderIds = new Set<string>();
  private reminderTimer: any = null;
  isActivityDrawerOpen = $state<boolean>(false);
  isUpdateModalOpen = $state<boolean>(false);
  updateInfo = $state<any>(null);
  hasAvailableUpdate = $derived(Boolean(this.updateInfo?.has_update));
  isDaemonStopped = $state<boolean>(false);
  serverLatency = $state<number | null>(null);
  serverMemoryMb = $state<number | null>(null);
  serverInfo = $state<{ version?: string; uptime_sec?: number; active_conns?: number } | null>(null);
  isPingingServer = $state<boolean>(false);
  appVersion = $state<string>('v0.18.0');
  isMockDataModalOpen = $state<boolean>(false);
  mockDataTableName = $state<string>('');
  isSettingsModalOpen = $state<boolean>(false);
  isSchemaSearchOpen = $state<boolean>(false);
  isSmartSeedModalOpen = $state<boolean>(false);
  ddlViewModal = $state<{ isOpen: boolean; title: string; ddl: string }>({ isOpen: false, title: '', ddl: '' });
  safetyInspectionModal = $state<{ isOpen: boolean; sql: string; inspection: InspectionResult | null }>({ isOpen: false, sql: '', inspection: null });
  isReadOnly = $state<boolean>(false);
  enableQueryProfiling = $state<boolean>(false);
  isRecordInspectorOpen = $state<boolean>(false);
  recordInspectorRowIndex = $state<number>(0);
  isInsertRowModalOpen = $state<boolean>(false);
  jsonCellModal = $state<{
    isOpen: boolean;
    tableName?: string;
    rowIdx?: number;
    colIdx?: number;
    colName: string;
    value: string;
    isReadOnly: boolean;
  }>({
    isOpen: false,
    colName: '',
    value: '',
    isReadOnly: true,
  });

  // Native HTML5 dialog state
  nativeDialog = $state<NativeDialogState>({
    isOpen: false,
    type: 'alert',
    title: '',
    message: '',
    defaultValue: '',
    inputValue: '',
  });

  // Pre-Execution DDL Review modal state
  ddlReview = $state<DdlReviewState>({
    isOpen: false,
    sql: '',
    title: 'Pre-Execution DDL Review',
  });

  // Global Network Error Boundary & Health Watcher
  isReconnecting = $state<boolean>(false);
  reconnectCountdown = $state<number>(0);
  reconnectAttempts = $state<number>(0);
  private reconnectTimer: any = null;

  // Schema Explorer Prefix Grouping (ADR 0058)
  prefixGroupingEnabled = $state<boolean>(false);

  // Additional Modals & Telemetry
  isRefreshAllOpen = $state<boolean>(false);
  isImpactPreviewOpen = $state<boolean>(false);
  impactPreviewData = $state<any>(null);
  impactPreviewSql = $state<string>('');
  isDataTransferOpen = $state<boolean>(false);
  isDataSyncOpen = $state<boolean>(false);
  isAuditDrawerOpen = $state<boolean>(false);
  auditRefreshToken = $state<number>(0);
  // Which DBA sub-view to show next; ProcesslistView applies it on change.
  processlistSubTab = $state<string>('threads');
  auditLogs = $state<SessionAuditItem[]>([]);
  // DBA live telemetry summaries for Header badges & Processlist sync
  dbaBlockersCount = $state<number>(0);
  dbaSlowQueriesCount = $state<number>(0);
  dbaClusterStatus = $state<'ok' | 'warn' | 'critical' | null>(null);
  dbaClusterLabel = $state<string>('');
  flightRecordingActive = $state<boolean>(false);
  flightRecordingRemainingSec = $state<number>(0);

  // Always-on Live Activity Console SSE state
  activityLogs = $state<ActivityLogEntry[]>([]);
  activityStatus = $state<'live' | 'reconnecting' | 'paused'>('live');
  isActivityPaused = $state<boolean>(false);
  unreadActivityErrors = $state<number>(0);
  private activityEventSource: EventSource | null = null;
  private activityReconnectTimer: any = null;

  // Schema cache timestamps per DB
  catalogTimestamps = $state<Record<string, string>>({});
  isPageVisible = $state<boolean>(true);
  private lastHiddenAt = 0;
  private activeAbortControllers = new Map<string, AbortController>();
  private activeQueryIds = new Map<string, string>();
  private timerIntervals = new Map<string, any>();

  // FK Peek Popover state
  fkPeekState = $state<{
    isOpen: boolean;
    x: number;
    y: number;
    table: string;
    col: string;
    refTable: string;
    refCol: string;
    val: any;
    data: any | null;
    isLoading: boolean;
    error: string | null;
  }>({
    isOpen: false,
    x: 0,
    y: 0,
    table: '',
    col: '',
    refTable: '',
    refCol: '',
    val: null,
    data: null,
    isLoading: false,
    error: null,
  });

  // UI state
  sidebarCollapsed = $state<boolean>(
    typeof localStorage !== 'undefined' && localStorage.getItem('sqlfox_left_sidebar_visible') === 'false'
  );
  activeSidebarTab = $state<'schema' | 'connections' | 'history'>('schema');
  schemaFilter = $state<string>('');
  collapsedSections = $state<Record<string, boolean>>((() => {
    const defaults = { tables: false, views: false, procedures: false, functions: false, routines: false, triggers: false, events: false };
    if (typeof localStorage === 'undefined') return defaults;
    try {
      const saved = JSON.parse(localStorage.getItem('sqlfox_schema_collapse') || '{}');
      return { ...defaults, ...saved };
    } catch {
      return defaults;
    }
  })());

  // Remote Access Guard lock state
  isRemoteLocked = $state<boolean>(
    typeof document !== 'undefined' && document.body.classList.contains('vault-locked')
  );
  authStatus = $state<any>(null);

  // Derived properties
  activeTab = $derived(this.tabs.find((t) => t.id === this.activeTabId) || this.tabs[0]);
  isTransactionActive = $derived(Boolean(this.activeTab?.txActive));
  isProduction = $derived(this.activeConnection?.environment === 'Production');
  isStaging = $derived(this.activeConnection?.environment === 'Staging');
  stagedCount = $derived(
    Object.keys(this.stagedEdits).length + (this.activeTab?.stagedDraftRows?.length || 0)
  );

  filteredTables = $derived.by(() => {
    if (!this.catalog?.tables) return [];
    const query = this.schemaFilter.toLowerCase().trim();
    const tables = this.catalog.tables.filter((t) => t.type !== 'VIEW');
    if (!query) return tables;
    return tables.filter((t) => t.name.toLowerCase().includes(query));
  });

  filteredViews = $derived.by(() => {
    if (!this.catalog?.tables) return [];
    const query = this.schemaFilter.toLowerCase().trim();
    const views = this.catalog.tables.filter((t) => t.type === 'VIEW');
    if (!query) return views;
    return views.filter((t) => t.name.toLowerCase().includes(query));
  });

  filteredRoutines = $derived.by(() => {
    if (!this.catalog?.routines) return [];
    const query = this.schemaFilter.toLowerCase().trim();
    if (!query) return this.catalog.routines;
    return this.catalog.routines.filter((r) => r.name.toLowerCase().includes(query));
  });

  filteredProcedures = $derived(
    this.filteredRoutines.filter((r) => (r.type || 'PROCEDURE').toUpperCase() === 'PROCEDURE')
  );

  filteredFunctions = $derived(
    this.filteredRoutines.filter((r) => (r.type || '').toUpperCase() === 'FUNCTION')
  );

  filteredTriggers = $derived.by(() => {
    if (!this.catalog?.triggers) return [];
    const query = this.schemaFilter.toLowerCase().trim();
    if (!query) return this.catalog.triggers;
    return this.catalog.triggers.filter((t) => t.name.toLowerCase().includes(query));
  });

  filteredEvents = $derived.by(() => {
    if (!this.catalog?.events) return [];
    const query = this.schemaFilter.toLowerCase().trim();
    if (!query) return this.catalog.events;
    return this.catalog.events.filter((ev) => ev.name.toLowerCase().includes(query));
  });

  totalObjectsCount = $derived.by(() => {
    const tablesCount = this.catalog?.tables?.length || 0;
    const routinesCount = this.catalog?.routines?.length || 0;
    const triggersCount = this.catalog?.triggers?.length || 0;
    const eventsCount = this.catalog?.events?.length || 0;
    return tablesCount + routinesCount + triggersCount + eventsCount;
  });

  constructor() {
    if (typeof window !== 'undefined') {
      (window as any).__sqlfox_set_remote_locked = (val: boolean) => {
        this.isRemoteLocked = val;
      };
      (window as any).__sqlfox_trigger_reconnect = () => this.triggerAutoReconnect();
      (window as any).__sqlfox_sync_readonly = (val: boolean) => {
        this.isReadOnly = Boolean(val);
      };

      // Route raw window.alert() through styled SqlFox NativeDialogModal
      window.alert = (msg?: any) => {
        void this.showNativeAlert(String(msg ?? ''));
      };

      // Persist active tabs synchronously on pagehide & beforeunload
      window.addEventListener('pagehide', () => {
        this.saveTabsState();
      });

      // ponytail: beforeunload warning guards against inadvertent data loss while queries are running or staged edits exist
      window.addEventListener('beforeunload', (e) => {
        this.saveTabsState();
        const hasRunningQuery = this.tabs.some((t) => t.isExecuting);
        const hasStagedEdits = Object.keys(this.stagedEdits).length > 0 || this.tabs.some((t) => (t.stagedDraftRows?.length || 0) > 0);
        if (hasRunningQuery || hasStagedEdits) {
          e.preventDefault();
          e.returnValue = 'You have unsaved staged edits or running database queries.';
          return e.returnValue;
        }
      });

      // Idle auto-wakeup: when returning after >=5 minutes hidden, probe session health silently
      document.addEventListener('visibilitychange', () => {
        this.isPageVisible = !document.hidden;
        if (document.hidden) {
          this.lastHiddenAt = Date.now();
        } else if (this.lastHiddenAt > 0 && Date.now() - this.lastHiddenAt >= 5 * 60 * 1000) {
          this.lastHiddenAt = 0;
          void api.getAuthStatus().catch(() => {});
          void this.pingActiveSession();
        }
      });
    }
    this.init();
  }

  // Toast System
  showToast(
    message: string,
    type: 'info' | 'success' | 'warning' | 'error' = 'info',
    options: { durationMs?: number; actions?: ToastAction[] } = {}
  ) {
    const id = `toast-${Date.now()}-${Math.random()}`;
    const durationMs = options.durationMs !== undefined ? options.durationMs : 4000;
    this.toasts.push({ id, message, type, durationMs, actions: options.actions });
    if (durationMs > 0) {
      setTimeout(() => {
        this.removeToast(id);
      }, durationMs);
    }
    return id;
  }

  removeToast(id: string) {
    const idx = this.toasts.findIndex((t) => t.id === id);
    if (idx !== -1) {
      this.toasts.splice(idx, 1);
    }
  }

  toggleSidebarCollapsed(collapsed?: boolean) {
    this.sidebarCollapsed = collapsed !== undefined ? collapsed : !this.sidebarCollapsed;
    try {
      localStorage.setItem('sqlfox_left_sidebar_visible', String(!this.sidebarCollapsed));
    } catch { /* ignore */ }
  }

  toggleSidebar(collapsed?: boolean) {
    this.toggleSidebarCollapsed(collapsed);
  }

  toggleAiDrawer(open?: boolean) {
    this.isAiDrawerOpen = open !== undefined ? open : !this.isAiDrawerOpen;
    if (this.isAiDrawerOpen) {
      this.isNotesDrawerOpen = false;
    }
    try {
      localStorage.setItem('sqlfox_copilot_visible', String(this.isAiDrawerOpen));
    } catch { /* ignore */ }
  }

  toggleNotesDrawer(open?: boolean) {
    this.isNotesDrawerOpen = open !== undefined ? open : !this.isNotesDrawerOpen;
    if (this.isNotesDrawerOpen) {
      this.isAiDrawerOpen = false;
      try {
        localStorage.setItem('sqlfox_copilot_visible', 'false');
      } catch { /* ignore */ }
    }
  }

  toggleActivityDrawer() {
    this.isActivityDrawerOpen = !this.isActivityDrawerOpen;
    if (this.isActivityDrawerOpen) {
      this.unreadActivityErrors = 0;
    }
  }

  initActivityStream() {
    if (typeof window === 'undefined' || typeof EventSource === 'undefined' || this.isRemoteLocked) return;
    if (this.activityEventSource) return;

    const connect = () => {
      if (this.activityEventSource) {
        this.activityEventSource.close();
        this.activityEventSource = null;
      }
      const token = sessionStorage.getItem('sqlfox_remote_token');
      const url = token ? `/api/logs/stream?token=${encodeURIComponent(token)}` : '/api/logs/stream';
      const es = new EventSource(url);
      this.activityEventSource = es;

      es.onopen = () => {
        this.activityStatus = this.isActivityPaused ? 'paused' : 'live';
      };

      es.onmessage = (e) => {
        if (this.isActivityPaused) return;
        try {
          const data = JSON.parse(e.data);
          const now = data.time ? new Date(data.time) : new Date();
          const hh = String(now.getHours()).padStart(2, '0');
          const mm = String(now.getMinutes()).padStart(2, '0');
          const ss = String(now.getSeconds()).padStart(2, '0');
          const ms = String(now.getMilliseconds()).padStart(3, '0');
          const level = String(data.level || 'INFO').toUpperCase();
          const entry: ActivityLogEntry = {
            id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
            time: `${hh}:${mm}:${ss}`,
            timeMs: `${hh}:${mm}:${ss}.${ms}`,
            level,
            module: String(data.module || 'core'),
            message: String(data.msg || data.message || e.data),
            attrs: data.attrs || undefined,
          };
          this.activityLogs = [...this.activityLogs.slice(-999), entry];
          if (!this.isActivityDrawerOpen && (level === 'ERROR' || level === 'WARN' || level === 'WARNING')) {
            this.unreadActivityErrors++;
          }
        } catch {
          /* ignore unparseable log line */
        }
      };

      es.onerror = () => {
        if (this.activityEventSource === es) {
          es.close();
          this.activityEventSource = null;
        }
        this.activityStatus = 'reconnecting';
        if (this.activityReconnectTimer) clearTimeout(this.activityReconnectTimer);
        this.activityReconnectTimer = setTimeout(() => {
          if (!this.isRemoteLocked) connect();
        }, 3000);
      };
    };

    connect();
  }

  async checkForUpdatesQuietly(force = false) {
    if (this.isRemoteLocked) return;
    try {
      const info = await api.checkSystemUpdate(force);
      if (info && typeof info === 'object') {
        this.updateInfo = info;
      }
    } catch {
      /* silent background check */
    }
  }

  async init() {
    try {
      const auth = await api.getAuthStatus();
      this.authStatus = auth;
      if (auth.version) {
        this.appVersion = auth.version;
      }
      try {
        const keyInfo = await api.getMasterKey();
        if (keyInfo) {
          this.authStatus = { ...auth, ...keyInfo };
        }
      } catch (_) {}
      if (auth.is_remote && !auth.unlocked) {
        this.isRemoteLocked = true;
      } else {
        this.isRemoteLocked = false;
        if (typeof document !== 'undefined') {
          document.body.classList.remove('vault-locked');
        }
      }
      if (!this.isRemoteLocked) {
        this.initActivityStream();
        await this.loadConnections();
        void this.loadSnippets();
        this.startNotesReminderPoller();
        setTimeout(() => void this.checkForUpdatesQuietly(), 2000);
      }
    } catch (err) {
      console.warn('Initial connection load deferred:', err);
    } finally {
      if (typeof document !== 'undefined') {
        const serverShell = document.getElementById('remote-lock-modal');
        if (serverShell) serverShell.classList.add('hidden');
      }
    }
  }

  startNotesReminderPoller() {
    if (this.reminderTimer) clearInterval(this.reminderTimer);
    this.reminderTimer = setInterval(() => this.checkDueNotesReminders(), 30000);
    setTimeout(() => this.checkDueNotesReminders(), 3000);
  }

  async checkDueNotesReminders() {
    if (this.isRemoteLocked) return;
    try {
      const dueNotes = await api.getDueReminders();
      if (!Array.isArray(dueNotes)) return;
      this.dueNotesCount = dueNotes.length;

      dueNotes.forEach((note: any) => {
        if (!this.notifiedReminderIds.has(note.id)) {
          this.notifiedReminderIds.add(note.id);
          this.playReminderChime();
          const toastId = this.showToast(
            `Reminder Due: ${note.title || 'Untitled Note'}`,
            'warning',
            {
              durationMs: 15000,
              actions: [
                {
                  label: 'Snooze 10m',
                  variant: 'secondary',
                  onClick: async () => {
                    await api.snoozeNote(note.id, 10);
                    this.notifiedReminderIds.delete(note.id);
                    this.notesRevision++;
                    this.removeToast(toastId);
                    void this.checkDueNotesReminders();
                    this.showToast('Reminder snoozed 10 minutes', 'info');
                  },
                },
                {
                  label: 'Open Note',
                  variant: 'primary',
                  onClick: () => {
                    this.editingNote = note;
                    this.isNoteComposerModalOpen = true;
                    this.removeToast(toastId);
                  },
                },
                {
                  label: 'Done',
                  variant: 'secondary',
                  onClick: async () => {
                    await api.resolveNote(note.id);
                    this.notesRevision++;
                    this.removeToast(toastId);
                    void this.checkDueNotesReminders();
                    this.showToast('Note resolved', 'success');
                  },
                },
              ],
            }
          );
        }
      });
    } catch (_) {
      // quiet background poller
    }
  }

  playReminderChime() {
    try {
      if (typeof window === 'undefined') return;
      const AudioCtx = window.AudioContext || (window as any).webkitAudioContext;
      if (!AudioCtx) return;
      const ctx = new AudioCtx();
      const osc = ctx.createOscillator();
      const gain = ctx.createGain();
      osc.type = 'sine';
      osc.frequency.setValueAtTime(587.33, ctx.currentTime);
      osc.frequency.setValueAtTime(880, ctx.currentTime + 0.12);
      gain.gain.setValueAtTime(0.18, ctx.currentTime);
      gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.7);
      osc.connect(gain);
      gain.connect(ctx.destination);
      osc.start();
      osc.stop(ctx.currentTime + 0.7);
      osc.onended = () => void ctx.close();
    } catch (_) {}
  }

  async unlockRemote(password: string) {
    const res = await api.unlockVault(password);
    if (res && res.token) {
      sessionStorage.setItem('sqlfox_remote_token', res.token);
      document.cookie = 'sqlfox_token=' + encodeURIComponent(res.token) + '; path=/; SameSite=Lax';
      this.isRemoteLocked = false;
      if (typeof document !== 'undefined') {
        document.body.classList.remove('vault-locked');
        const serverShell = document.getElementById('remote-lock-modal');
        if (serverShell) serverShell.classList.add('hidden');
      }
      try {
        const auth = await api.getAuthStatus();
        if (auth.version) this.appVersion = auth.version;
        let keyInfo = null;
        try { keyInfo = await api.getMasterKey(); } catch (_) {}
        this.authStatus = { ...auth, ...(keyInfo || {}) };
      } catch (_) {}
      this.initActivityStream();
      await this.loadConnections();
      void this.loadSnippets();
      this.startNotesReminderPoller();
      setTimeout(() => void this.checkForUpdatesQuietly(), 2000);
      return true;
    }
    return false;
  }

  // Connections CRUD
  async loadConnections() {
    const list = await api.listConnections();
    this.connections = list || [];
    if (!this.activeConnection && this.connections.length > 0) {
      const tabSavedId = typeof sessionStorage !== 'undefined' ? sessionStorage.getItem('sqlfox_active_conn_id') : null;
      const savedId = tabSavedId || localStorage.getItem('sqlfox_last_conn_id');
      const found = this.connections.find((c) => c.id === savedId) || this.connections[0];
      await this.selectConnection(found);
    }
  }

  openNewConnectionModal() {
    this.connectionToEdit = null;
    this.isConnectionModalOpen = true;
  }

  openEditConnectionModal(conn: ConnectionProfile) {
    this.connectionToEdit = conn;
    this.isConnectionModalOpen = true;
  }

  async saveConnection(conn: Partial<ConnectionProfile>) {
    try {
      const saved = await api.saveConnection(conn);
      this.showToast(`Connection "${saved.name}" saved`, 'success');
      await this.loadConnections();
      this.isConnectionModalOpen = false;
      await this.selectConnection(saved);
      return saved;
    } catch (err: any) {
      this.showToast(`Failed to save connection: ${err.message}`, 'error');
      throw err;
    }
  }

  openViewDesigner(view: { name: string; query?: string } | null = null) {
    this.viewToEdit = view;
    this.isViewDesignerOpen = true;
  }

  openTriggerDesigner(trigger: TriggerMetadata | null = null) {
    this.triggerToEdit = trigger;
    this.isTriggerDesignerOpen = true;
  }

  openRoutineDesigner(routine: RoutineMetadata | null = null) {
    this.routineToEdit = routine;
    this.isRoutineDesignerOpen = true;
  }

  openEventDesigner(event: EventMetadata | null = null) {
    this.eventToEdit = event;
    this.isEventDesignerOpen = true;
  }

  openDumpModal(tableName = '') {
    this.dumpTargetTable = tableName;
    this.isDumpModalOpen = true;
  }

  openStructureSyncModal() {
    this.isStructureSyncModalOpen = true;
  }

  openRunSqlModal() {
    this.isRunSqlModalOpen = true;
  }

  openSnippetsModal() {
    this.isSnippetsModalOpen = true;
  }

  openCsvImportModal() {
    this.isCsvImportModalOpen = true;
  }

  openPrivilegeMatrixModal() {
    this.isPrivilegeMatrixOpen = true;
  }

  openDbDiagnosticsModal() {
    this.isDbDiagnosticsOpen = true;
  }

  openMockDataModal(tableName = '') {
    this.mockDataTableName = tableName;
    this.isMockDataModalOpen = true;
  }

  closeMockDataModal() {
    this.isMockDataModalOpen = false;
    this.mockDataTableName = '';
  }

  openDdlViewModal(title: string, ddl: string) {
    this.ddlViewModal = { isOpen: true, title, ddl };
  }

  /** Opens the AI Copilot drawer and hands it a ready-to-review prompt. */
  sendToAi(prompt: string, note = 'Context sent to AI Copilot') {
    if (!prompt || !prompt.trim()) return;
    this.aiPromptDraft = prompt.trim();
    this.toggleAiDrawer(true);
    if (note) this.showToast(note, 'success');
  }

  fixQueryWithAI(sql: string, errorMessage: string) {
    if (!sql && !errorMessage) return;
    this.aiQuickRunRequest = {
      action: 'fix_query',
      sql,
      error: errorMessage,
      nonce: Date.now(),
    };
    this.toggleAiDrawer(true);
  }

  /** Drawer consumes the draft once, so it never re-injects stale text. */
  consumeAiPromptDraft(): string {
    const draft = this.aiPromptDraft;
    this.aiPromptDraft = '';
    return draft;
  }

  async deleteConnection(id: string) {
    try {
      await api.deleteConnection(id);
      this.showToast('Connection deleted', 'info');
      if (this.activeConnection?.id === id) {
        this.activeConnection = null;
        this.activeSessionId = null;
        this.catalog = null;
        this.catalogs = {};
        this.catalogRequests.clear();
        if (typeof sessionStorage !== 'undefined') sessionStorage.removeItem('sqlfox_active_conn_id');
      }
      await this.loadConnections();
    } catch (err: any) {
      this.showToast(`Failed to delete connection: ${err.message}`, 'error');
    }
  }

  async selectConnection(conn: ConnectionProfile) {
    this.isConnecting = true;
    this.connectionError = null;
    try {
      const previousConnectionId = this.activeConnection?.id;
      this.activeConnection = conn;
      if (previousConnectionId !== conn.id) {
        this.catalog = null;
        this.catalogs = {};
        this.catalogRequests.clear();
        this.currentDatabase = '';
        this.stagedEdits = {};
      }
      if (typeof sessionStorage !== 'undefined') sessionStorage.setItem('sqlfox_active_conn_id', conn.id);
      localStorage.setItem('sqlfox_last_conn_id', conn.id);
      const sess = await api.connectSession(conn.id);
      this.activeSessionId = sess.id || sess.connection_id || conn.id;
      if (sess.schema) {
        this.setCatalog(sess.schema);
      }

      let dbs = (sess.schema && sess.schema.databases) || [];
      if (!dbs || dbs.length === 0) {
        try {
          const fresh = await api.getSchema(this.activeSessionId, '');
          if (fresh) {
            dbs = fresh.databases || [];
            if (fresh.database_name && !conn.database && !conn.database_name) {
              this.currentDatabase = fresh.database_name;
            }
            this.setCatalog(fresh);
          }
        } catch (fetchErr) {
          console.warn('Fallback schema fetch deferred:', fetchErr);
        }
      }
      this.availableDatabases = dbs || [];

      let targetDb = conn.database || conn.database_name || this.currentDatabase || (sess.schema && sess.schema.database_name) || '';
      if (!targetDb && this.availableDatabases.length > 0) {
        const userDb = this.availableDatabases.find(
          (d) => !['mysql', 'information_schema', 'performance_schema', 'sys'].includes(d)
        );
        targetDb = userDb || this.availableDatabases[0];
      }
      this.currentDatabase = targetDb;

      if (!this.catalog?.tables || this.catalog.tables.length === 0) {
        await this.refreshCatalog();
      }

      this.loadFastCatalogBackground(this.activeSessionId);

      // Restore persisted tabs for this connection
      const restored = this.restoreTabsState(conn.id);
      if (!restored && this.tabs.length === 0) {
        this.addTab('Query 1.sql', '');
      }

      if (sess.server_info) {
        this.serverInfo = sess.server_info;
      }
      this.pingActiveSession().catch(() => {});
    } catch (err: any) {
      console.error('Failed to select connection:', err);
      this.connectionError = err.message || 'Failed to connect to database';
      throw err;
    } finally {
      this.isConnecting = false;
    }
  }

  async refreshDatabases() {
    if (!this.activeSessionId) return;
    this.isLoadingCatalog = true;
    try {
      const schema = await api.getSchema(this.activeSessionId, this.currentDatabase || '');
      if (schema.databases && schema.databases.length > 0) {
        this.availableDatabases = schema.databases;
      }
      if (schema.database_name) {
        this.currentDatabase = schema.database_name;
      }
      if (schema.tables) {
        this.setCatalog(schema);
      }
    } catch (err) {
      console.error('Failed to reload databases:', err);
    } finally {
      this.isLoadingCatalog = false;
    }
  }

  async pingActiveSession() {
    if (!this.activeSessionId) return;
    this.isPingingServer = true;
    try {
      const res = await api.pingSession(this.activeSessionId);
      if (res) {
        this.serverLatency = res.latency_ms;
        if (res.memory_mb != null) this.serverMemoryMb = res.memory_mb;
        if (res.server_info) this.serverInfo = res.server_info;
        if (typeof res.read_only === 'boolean') this.isReadOnly = res.read_only;
      }
    } catch (_) {
      this.serverLatency = -1;
    } finally {
      this.isPingingServer = false;
    }
  }

  private openNoteDraft(
    title: string,
    content: string,
    opts?: { category?: string; userTags?: string[] }
  ) {
    const tab = this.activeTab;
    const tableName = tab?.tableName || tab?.detectedTableName || '';
    const dbName = tab?.database || this.currentDatabase || 'db';
    const objectTags = tableName && this.activeConnection?.id
      ? [`db://${this.activeConnection.id}/${dbName}/${tableName}`]
      : [];
    this.editingNote = {
      title: title.trim() || 'Query Snapshot',
      content,
      side_note: '',
      status: 'inbox',
      category: opts?.category || 'Optimization',
      user_tags: opts?.userTags || ['#query'],
      object_tags: objectTags,
      table_name: tableName,
      is_pinned: false,
    };
    this.isNoteComposerModalOpen = true;
  }

  saveQueryToNote(sqlText?: string, titleHint?: string) {
    const tab = this.activeTab;
    const sql = (sqlText ?? tab?.sql ?? '').trim();
    if (!sql) {
      this.showToast('Worksheet query is empty. Enter or run a SQL statement first.', 'warning');
      return;
    }
    const cleanTitle = (titleHint || tab?.title || 'Query Snapshot').replace(/\.sql$/i, '').trim() || 'Query Snapshot';
    this.openNoteDraft(cleanTitle, `### Query Snapshot\n\n\`\`\`sql\n${sql}\n\`\`\`\n`);
  }

  saveMarkdownToNote(
    markdownText: string,
    titleHint?: string,
    opts?: { category?: string; userTags?: string[] }
  ) {
    const content = (markdownText ?? '').trim();
    if (!content) {
      this.showToast('Note content is empty.', 'warning');
      return;
    }
    const cleanTitle = (titleHint || 'AI Copilot Note').replace(/\.md$/i, '').trim() || 'AI Copilot Note';
    this.openNoteDraft(cleanTitle, content, opts);
  }

  // --- System lifecycle actions (surfaced from the command palette & header) ---

  async toggleReadOnlyMode() {
    if (!this.activeSessionId) {
      this.isReadOnly = !this.isReadOnly;
      this.showToast(
        this.isReadOnly
          ? 'Read-only mode enabled — mutating statements will be blocked'
          : 'Read-only mode disabled — writes are now allowed',
        this.isReadOnly ? 'warning' : 'success'
      );
      return;
    }
    if (this.isReadOnly && this.isProduction) {
      const ok = await this.showNativeConfirm(
        `Disable Read-Only Protection on PRODUCTION connection "${this.activeConnection?.name}"?\nMutating queries (INSERT, UPDATE, DELETE, DROP, ALTER) will be allowed.`,
        'Production Write Confirmation'
      );
      if (!ok) return;
    }
    try {
      const res = await api.toggleReadOnly(this.activeSessionId);
      this.isReadOnly = Boolean(res.read_only);
      this.showToast(
        this.isReadOnly
          ? 'Read-only mode enabled — mutating statements will be blocked'
          : 'Read-only mode disabled — writes are now allowed',
        this.isReadOnly ? 'warning' : 'success'
      );
    } catch (err: any) {
      this.showToast(`Failed to toggle read-only mode: ${err.message}`, 'error');
    }
  }

  async openInSystemBrowser() {
    const url = typeof window !== 'undefined' ? window.location.href : '';
    try {
      await api.openInSystemBrowser(url);
      this.showToast('Opening in your default web browser…', 'info');
      return;
    } catch {
      /* Remote sessions and headless daemons cannot open a host browser; fall through. */
    }
    const win = typeof window !== 'undefined' ? window.open(url, '_blank', 'noopener,noreferrer') : null;
    this.showToast(
      win ? 'Opened in a new browser tab' : 'Popup was blocked — allow popups or copy the URL instead',
      win ? 'info' : 'warning'
    );
  }

  async copyAppUrl() {
    const url = typeof window !== 'undefined' ? window.location.href : '';
    try {
      await copyText(url);
      this.showToast('SqlFox URL copied to clipboard', 'success');
    } catch {
      this.showToast(`Copy failed. URL: ${url}`, 'warning');
    }
  }

  async shutdownDaemon() {
    const ok = await this.showNativeConfirm(
      'Shut down the SqlFox daemon? Active background jobs and database connections will be closed.',
      'Shut Down SqlFox Daemon'
    );
    if (!ok) return;
    try {
      await api.shutdownDaemon();
    } catch {
      /* expected when socket closes during graceful shutdown */
    }
    this.isDaemonStopped = true;
    this.showToast('SqlFox daemon stopped gracefully', 'info');
  }

  async loadFastCatalogBackground(sessionId: string) {
    try {
      const fastCat = await api.getFastCatalog(sessionId);
      if (fastCat && typeof fastCat === 'object' && this.activeSessionId === sessionId) {
        const counts: Record<string, number> = {};
        const additions: Record<string, SchemaCatalog> = {};
        for (const [db, tbls] of Object.entries(fastCat)) {
          counts[db] = Array.isArray(tbls) ? tbls.length : 0;
          if (!this.catalogs[db.toLowerCase()]) {
            additions[db.toLowerCase()] = {
              database_name: db,
              databases: this.availableDatabases,
              tables: Array.isArray(tbls) ? tbls : [],
            };
          }
        }
        if (Object.keys(additions).length) this.catalogs = { ...this.catalogs, ...additions };
        this.databaseTableCounts = counts;
      }
    } catch (err) {
      console.warn('Fast catalog background load failed:', err);
    }
  }

  async loadSnippets(force = false): Promise<QuerySnippet[]> {
    if (this.snippetsLoaded && !force) return this.snippets;
    try {
      const response = await api.listSnippets();
      this.snippets = (Array.isArray(response) ? response : []).map((snippet: any) => ({
        id: snippet.id,
        title: snippet.title,
        description: snippet.description,
        sql: snippet.sql_content ?? snippet.sql ?? '',
        tags: Array.isArray(snippet.tags)
          ? snippet.tags
          : String(snippet.tags || '').split(',').map((tag: string) => tag.trim()).filter(Boolean),
      })).filter((snippet: QuerySnippet) => snippet.title && snippet.sql);
      this.snippetsLoaded = true;
    } catch (err) {
      console.warn('Failed to load SQL snippets:', err);
    }
    return this.snippets;
  }

  setEditorInserter(inserter: ((sql: string) => boolean) | null) {
    this.editorInserter = inserter;
  }

  setEditorTargetProvider(provider: (() => string | null) | null) {
    this.editorTargetProvider = provider;
  }

  /** SQL that the editor would execute: selection, else statement at the caret. */
  getEditorTargetSql(): string | null {
    return this.editorTargetProvider?.() ?? null;
  }

  invokeSnippet(snippet: QuerySnippet) {
    this.requestSnippetParameters(snippet);
  }

  requestSnippetParameters(snippet: QuerySnippet, insert?: (sql: string) => void) {
    const variables = parseSnippetVariables(snippet.sql);
    if (variables.length === 0) {
      if (insert) insert(snippet.sql);
      else this.insertSnippetSQL(snippet.sql, snippet.title);
      return;
    }
    this.pendingSnippet = {
      snippet,
      values: Object.fromEntries(variables.map((variable) => [variable.name, variable.defaultValue])),
      insert,
    };
    this.isSnippetsModalOpen = true;
  }

  setSnippetValue(name: string, value: string) {
    if (this.pendingSnippet) this.pendingSnippet.values[name] = value;
  }

  confirmPendingSnippet() {
    const pending = this.pendingSnippet;
    if (!pending) return;
    this.pendingSnippet = null;
    const sql = substituteSnippetVariables(pending.snippet.sql, pending.values);
    if (pending.insert) {
      this.isSnippetsModalOpen = false;
      pending.insert(sql);
      return;
    }
    this.insertSnippetSQL(sql, pending.snippet.title);
  }

  insertSnippetSQL(sql: string, title = 'Query') {
    this.isSnippetsModalOpen = false;
    const inserted = this.editorInserter?.(sql) || false;
    if (!inserted) {
      const tab = this.activeTab;
      if (tab && !tab.isTableViewer && !tab.isProcesslist && !tab.isErdCanvas) {
        tab.sql += sql;
      } else {
        this.addTab(`${title.replace(/[^a-zA-Z0-9_-]/g, '_')}.sql`, sql);
      }
    }
    this.saveTabsState();
  }

  cacheCatalog(catalog: SchemaCatalog | null | undefined, fallbackDatabase = '') {
    const database = catalog?.database_name || fallbackDatabase;
    if (catalog && database) {
      this.catalogs = { ...this.catalogs, [database.toLowerCase()]: catalog };
      const now = new Date();
      const stamp = `${String(now.getHours()).padStart(2, '0')}:${String(now.getMinutes()).padStart(2, '0')}`;
      this.catalogTimestamps = { ...this.catalogTimestamps, [database.toLowerCase()]: stamp };
    }
    return catalog;
  }

  setCatalog(catalog: SchemaCatalog | null) {
    this.catalog = catalog;
    if (catalog) this.cacheCatalog(catalog);
  }

  getSchemaCatalog(sessionId: string, database: string): Promise<SchemaCatalog> {
    const key = `${sessionId}:${database.toLowerCase()}`;
    const existing = this.catalogRequests.get(key);
    if (existing) return existing;
    const request = api.getSchema(sessionId, database).then((catalog) => {
      if (this.activeSessionId === sessionId) this.cacheCatalog(catalog, database);
      return catalog;
    }).finally(() => this.catalogRequests.delete(key));
    this.catalogRequests.set(key, request);
    return request;
  }

  async ensureEditorCatalog(database: string): Promise<void> {
    const sessionId = this.activeSessionId;
    if (!sessionId || !database.trim() || this.catalogs[database.toLowerCase()]) return;
    await this.getSchemaCatalog(sessionId, database);
  }

  async selectDatabase(dbName: string) {
    if (!this.activeSessionId || this.currentDatabase === dbName) return;
    this.isLoadingCatalog = true;
    const slowTimer = setTimeout(() => {
      this.showToast(`Inspecting database '${dbName}' schema (large catalog)...`, 'info');
    }, 3000);
    try {
      const schema = await api.selectDatabase(this.activeSessionId, dbName);
      if (schema && (schema.tables || schema.databases)) {
        this.setCatalog(schema);
        this.currentDatabase = schema.database_name || dbName;
        if (schema.databases && schema.databases.length > 0) {
          this.availableDatabases = schema.databases;
        }
      } else {
        const fetched = await api.getSchema(this.activeSessionId, dbName);
        this.setCatalog(fetched);
        this.currentDatabase = fetched.database_name || dbName;
        if (fetched.databases && fetched.databases.length > 0) {
          this.availableDatabases = fetched.databases;
        }
      }
      this.showToast(`Switched to database ${dbName}`, 'info');
    } catch (err: any) {
      console.error('Failed to switch database:', err);
      this.showToast(`Failed to switch database: ${err.message}`, 'error');
    } finally {
      clearTimeout(slowTimer);
      this.isLoadingCatalog = false;
    }
  }

  /** Reload the schema catalog, optionally scoped to a connection + database. */
  async loadCatalog(connId?: string, database?: string) {
    const sessionId = connId || this.activeSessionId;
    if (!sessionId) return;
    const db = (database || this.currentDatabase || '').trim();
    const schema = await api.getSchema(sessionId, db);
    if (sessionId === this.activeSessionId) this.setCatalog(schema);
    else this.cacheCatalog(schema, db);
    return schema;
  }

  async refreshSingleDatabase(dbName: string) {
    if (!this.activeSessionId || !dbName) return;
    this.isLoadingCatalog = true;
    try {
      const schema =
        dbName === this.currentDatabase
          ? await api.refreshSchema(this.activeSessionId)
          : await api.getSchema(this.activeSessionId, dbName);
      this.cacheCatalog(schema, dbName);
      if (dbName === this.currentDatabase) {
        this.setCatalog(schema);
        if (schema.databases && schema.databases.length > 0) {
          this.availableDatabases = schema.databases;
        }
      }
      this.showToast(`Refreshed catalog for '${dbName}'`, 'success');
    } catch (err: any) {
      this.showToast(`Failed to refresh '${dbName}': ${err.message}`, 'error');
    } finally {
      this.isLoadingCatalog = false;
    }
  }

  async refreshCatalog(database?: string) {
    if (!this.activeSessionId) return;
    const targetDb = (database ?? this.currentDatabase ?? '').trim();
    this.isLoadingCatalog = true;
    try {
      const schema = await api.refreshSchema(this.activeSessionId, targetDb);
      this.cacheCatalog(schema, targetDb);
      if (!targetDb || targetDb.toLowerCase() === (this.currentDatabase || '').toLowerCase()) {
        this.setCatalog(schema);
        if (schema.database_name) {
          this.currentDatabase = schema.database_name;
        }
        if (schema.databases && schema.databases.length > 0) {
          this.availableDatabases = schema.databases;
        }
      }
      this.showToast('Schema refreshed', 'info');
    } catch (err: any) {
      console.error('Failed to load schema catalog:', err);
      this.showToast(`Failed to refresh schema: ${err.message}`, 'error');
    } finally {
      this.isLoadingCatalog = false;
    }
  }

  invalidateTableViewers(database?: string, tableName?: string) {
    const dbNorm = (database || this.currentDatabase || '').trim().toLowerCase();
    const tblNorm = (tableName || '').trim().toLowerCase();
    this.tabs = this.tabs.map((t) => {
      if (!t.isTableViewer || t.isExecuting) return t;
      const tabDb = (t.database || this.currentDatabase || '').trim().toLowerCase();
      if (dbNorm && tabDb && tabDb !== dbNorm) return t;
      if (tblNorm && (t.tableName || '').trim().toLowerCase() !== tblNorm) return t;
      return { ...t, activeResult: null };
    });
  }

  toggleSection(section: string) {
    this.collapsedSections[section] = !this.collapsedSections[section];
    try {
      localStorage.setItem('sqlfox_schema_collapse', JSON.stringify(this.collapsedSections));
    } catch { /* ignore */ }
  }

  // Tabs Management & Session Persistence (ADR 0055 & CONTEXT.md)
  getTabsStorageKey(connId?: string): string {
    const id = connId || this.activeConnection?.id || (typeof localStorage !== 'undefined' ? localStorage.getItem('sqlfox_last_conn_id') : '');
    return id ? `sqlfox_tabs_${id}` : 'sqlfox_tabs_default';
  }

  saveTabsState() {
    if (typeof window === 'undefined' || !this.activeConnection?.id) return;
    const key = this.getTabsStorageKey(this.activeConnection.id);
    const serializable = {
      activeTabId: this.activeTabId,
      tabs: this.tabs.map((t) => ({
        id: t.id,
        title: t.title,
        sql: t.sql,
        database: t.database,
        isTableViewer: Boolean(t.isTableViewer),
        tableName: t.tableName,
        tableMode: t.tableMode,
        pageSize: t.pageSize || 25,
        currentPage: t.currentPage || 1,
        sortCol: t.sortCol || null,
        sortDir: t.sortDir || null,
        filterPills: t.filterPills || [],
        filterMode: t.filterMode || 'pills',
        filterRaw: t.filterRaw || '',
        appliedFilter: t.appliedFilter || '',
        queryPage: t.queryPage || 1,
        queryPageSize: t.queryPageSize || 100,
        showFilterBar: Boolean(t.showFilterBar),
        hiddenColumns: t.hiddenColumns || [],
        isProcesslist: Boolean(t.isProcesslist),
        isCluster: Boolean(t.isCluster),
        isFlightRecorder: Boolean(t.isFlightRecorder),
        isVariables: Boolean(t.isVariables),
        isErdCanvas: Boolean(t.isErdCanvas),
        isNote: Boolean(t.isNote),
        noteMode: t.noteMode || 'edit',
      })),
    };
    try {
      localStorage.setItem(key, JSON.stringify(serializable));
    } catch (e) {
      console.warn('Failed to save tabs state:', e);
    }
  }

  restoreTabsState(connId: string): boolean {
    if (typeof window === 'undefined' || !connId) return false;
    const key = this.getTabsStorageKey(connId);
    const raw = localStorage.getItem(key);
    if (!raw) return false;
    try {
      const parsed = JSON.parse(raw);
      if (parsed && Array.isArray(parsed.tabs) && parsed.tabs.length > 0) {
        const isSqlite = (this.activeConnection?.driver || '').toLowerCase() === 'sqlite';
        const filteredTabs = isSqlite
          ? parsed.tabs.filter((t: EditorTab) => !t.isProcesslist && !t.isCluster && !t.isVariables)
          : parsed.tabs;
        this.tabs = filteredTabs.length > 0
          ? filteredTabs
          : [{ id: `tab-${Date.now()}`, title: 'Query 1.sql', sql: '', isTableViewer: false }];
        this.activeTabId = parsed.activeTabId || parsed.tabs[0].id;
        if (!this.tabs.some((t) => t.id === this.activeTabId)) {
          this.activeTabId = this.tabs[0].id;
        }
        if (this.activeTab?.isCluster) this.processlistSubTab = 'cluster';
        else if (this.activeTab?.isVariables) this.processlistSubTab = 'variables';
        else if (this.activeTab?.isFlightRecorder) this.processlistSubTab = 'flight-recorder';
        if (this.activeTab?.isTableViewer && this.activeTab.tableName) {
          this.loadTableData(this.activeTab);
        }
        return true;
      }
    } catch (e) {
      console.warn('Failed to restore tabs state:', e);
    }
    return false;
  }

  getNextQueryTabTitle(): string {
    let maxNum = 0;
    for (const t of this.tabs) {
      const m = (t.title || '').match(/^Query\s+(\d+)\.sql$/i);
      if (m) {
        const n = parseInt(m[1], 10);
        if (n > maxNum) maxNum = n;
      }
    }
    return `Query ${maxNum + 1}.sql`;
  }

  addTab(title?: string, sql = '') {
    const finalTitle = (!title || title === 'New Query.sql') ? this.getNextQueryTabTitle() : title;
    const newId = `tab-${Date.now()}`;
    const newTab: EditorTab = {
      id: newId,
      title: finalTitle,
      sql,
      database: this.currentDatabase || '',
      isTableViewer: false,
    };
    this.tabs.push(newTab);
    this.activeTabId = newId;
    this.saveTabsState();
    // Return the proxied element, not the raw literal, so later mutations notify.
    return this.tabs[this.tabs.length - 1];
  }

  openTableDataTab(tableName: string, initialMode: TableMode = 'data') {
    const currentDb = this.currentDatabase || '';
    const existing = this.tabs.find((t) => t.isTableViewer && t.tableName === tableName && (!t.database || t.database === currentDb));
    if (existing) {
      existing.tableMode = initialMode;
      this.activeTabId = existing.id;
      this.tabs = [...this.tabs];
      if (initialMode === 'data' && (!existing.activeResult || !existing.activeResult.rows || existing.activeResult.rows.length === 0)) {
        this.loadTableData(existing);
      }
      this.saveTabsState();
      return;
    }
    const newId = `tab-tbl-${currentDb ? currentDb + '-' : ''}${tableName}-${Date.now()}`;
    const driver = this.activeConnection?.driver;
    const dbPrefix = currentDb ? `${quoteSqlIdentifier(currentDb, driver)}.` : '';
    let restoredHidden: string[] = [];
    if (typeof sessionStorage !== 'undefined') {
      try {
        const rawHidden = sessionStorage.getItem(`sqlfox-hidden-cols:${currentDb}:${tableName}`);
        if (rawHidden) restoredHidden = JSON.parse(rawHidden);
      } catch { /* ignore */ }
    }
    const newTab: EditorTab = {
      id: newId,
      title: tableName,
      sql: `SELECT * FROM ${dbPrefix}${quoteSqlIdentifier(tableName, driver)} LIMIT 25;`,
      isTableViewer: true,
      tableName,
      database: currentDb,
      tableMode: initialMode,
      pageSize: 25,
      currentPage: 1,
      filterPills: [],
      filterMode: 'pills',
      filterRaw: '',
      appliedFilter: '',
      showFilterBar: false,
      hiddenColumns: Array.isArray(restoredHidden) ? restoredHidden : [],
      isExecuting: initialMode === 'data',
      activeResult: null,
    };
    this.tabs.push(newTab);
    this.activeTabId = newId;
    this.tabs = [...this.tabs];
    this.saveTabsState();
    if (initialMode === 'data') {
      const liveTab = this.tabs.find((t) => t.id === newId) || newTab;
      this.loadTableData(liveTab);
    }
  }

  openTableStructureTab(tableName: string) {
    this.openTableDataTab(tableName, 'structure');
  }

  openProcesslistTab(subTab = 'threads') {
    this.processlistSubTab = subTab;
    const existing = this.tabs.find((t) => t.isProcesslist);
    if (existing) {
      this.activeTabId = existing.id;
      return;
    }
    const newId = `tab-pl-${Date.now()}`;
    const newTab: EditorTab = {
      id: newId,
      title: 'Processlist',
      sql: '',
      isProcesslist: true,
      isTableViewer: false,
    };
    this.tabs.push(newTab);
    this.activeTabId = newId;
    this.tabs = [...this.tabs];
    this.saveTabsState();
  }

  openErdTab() {
    const targetDb = this.currentDatabase || '';
    const existing = this.tabs.find((t) => t.isErdCanvas);
    if (existing) {
      if (existing.database !== targetDb) {
        existing.database = targetDb;
        this.tabs = [...this.tabs];
        this.saveTabsState();
      }
      this.activeTabId = existing.id;
      return;
    }
    const newId = `tab-erd-${Date.now()}`;
    const newTab: EditorTab = {
      id: newId,
      title: 'ERD Canvas',
      sql: '',
      isErdCanvas: true,
      isTableViewer: false,
      database: targetDb,
    };
    this.tabs.push(newTab);
    this.activeTabId = newId;
    this.tabs = [...this.tabs];
    this.saveTabsState();
  }

  setTableMode(tabId: string, mode: TableMode) {
    const tab = this.tabs.find((t) => t.id === tabId);
    if (tab) {
      tab.tableMode = mode;
      this.tabs = [...this.tabs];
      this.saveTabsState();
      if (mode === 'data' && (!tab.activeResult || !tab.activeResult.rows?.length)) {
        this.loadTableData(tab);
      }
    }
  }

  async loadTableData(targetTab?: EditorTab) {
    const tabId = targetTab?.id || this.activeTabId;
    const tab = this.tabs.find((t) => t.id === tabId) || targetTab || this.activeTab;
    if (!this.activeSessionId || !tab || !tab.tableName) return;
    tab.isExecuting = true;
    this.tabs = [...this.tabs];
    const startTime = performance.now();
    // Guard against out-of-order responses when the user pages/filters rapidly.
    const token = ((tab as any)._loadToken = ((tab as any)._loadToken || 0) + 1);
    try {
      const limit = tab.pageSize || 25;
      const offset = ((tab.currentPage || 1) - 1) * limit;
      const db = tab.database || this.currentDatabase || '';

      // Determine active filter clause (appliedFilter or compiled filterPills)
      let filterClause = (tab.appliedFilter || '').trim();
      if (!filterClause && tab.filterPills && tab.filterPills.length > 0) {
        filterClause = compileFilterPills(tab.filterPills, this.activeConnection?.driver);
      }

      const res = await api.getTableRows(
        this.activeSessionId,
        tab.tableName,
        limit,
        offset,
        db,
        filterClause || undefined,
        tab.sortCol || undefined,
        tab.sortDir || undefined
      );
      if ((tab as any)._loadToken !== token) return; // a newer load superseded this one
      if (res && res.duration_ms === undefined) {
        res.duration_ms = Math.round(performance.now() - startTime);
      }
      if (res && res.row_count === undefined && res.rows) {
        res.row_count = res.rows.length;
      }
      tab.activeResult = res;
    } catch (err: any) {
      if ((tab as any)._loadToken !== token) return;
      tab.activeResult = {
        columns: [],
        rows: [],
        duration_ms: Math.round(performance.now() - startTime),
        row_count: 0,
        error: err.message || 'Error loading table data',
      };
    } finally {
      if ((tab as any)._loadToken === token) {
        tab.isExecuting = false;
        this.tabs = [...this.tabs];
        this.saveTabsState();
      }
    }
  }

  async openTableDDLTab(tableName: string) {
    const existing = this.tabs.find((t) => t.title === `${tableName}.ddl.sql`);
    if (existing) {
      this.activeTabId = existing.id;
      return;
    }
    const newTab = this.addTab(`${tableName}.ddl.sql`, `-- Fetching CREATE TABLE for ${tableName}...`);
    if (!this.activeSessionId) return;
    try {
      const res = await api.getTableDDL(this.activeSessionId, tableName);
      if (res && res.ddl) newTab.sql = res.ddl;
    } catch (err: any) {
      newTab.sql = `-- Error fetching DDL: ${err.message || err}`;
    }
    this.saveTabsState();
  }

  async openRoutineTab(routineName: string, routineType: 'PROCEDURE' | 'FUNCTION' = 'PROCEDURE') {
    const existing = this.tabs.find((t) => t.title === `${routineName}.sql`);
    if (existing) {
      this.activeTabId = existing.id;
      return;
    }
    const newTab = this.addTab(`${routineName}.sql`, `-- Fetching definition for ${routineType} ${routineName}...`);
    if (!this.activeSessionId) return;
    try {
      const res = await api.getRoutineDDL(this.activeSessionId, routineName, routineType);
      if (res && res.ddl) newTab.sql = res.ddl;
    } catch (err: any) {
      newTab.sql = `-- Error fetching routine definition: ${err.message || err}`;
    }
    this.saveTabsState();
  }

  openTriggerTab(trigger: TriggerMetadata) {
    const title = `${trigger.name}.sql`;
    const existing = this.tabs.find((t) => t.title === title);
    if (existing) {
      this.activeTabId = existing.id;
      return;
    }
    // Triggers expose no DDL endpoint; the catalog already carries the body.
    const driver = this.activeConnection?.driver?.toLowerCase() || '';
    const body = trigger.statement.trim().replace(/;\s*$/, '');
    const create = [
      `CREATE TRIGGER \`${trigger.name}\` ${trigger.timing} ${trigger.event} ON \`${trigger.table}\` FOR EACH ROW`,
      `${body};`,
    ];
    const ddl = [
      `-- Trigger ${trigger.name} on ${trigger.table} (${trigger.timing} ${trigger.event})`,
      ...(driver.includes('mysql') || driver.includes('maria')
        ? ['DELIMITER ;;', ...create, ';;', 'DELIMITER ;']
        : create),
    ].join('\n');
    this.addTab(title, ddl);
    this.saveTabsState();
  }

  private cleanupTabResources(tab: EditorTab) {
    const controller = this.activeAbortControllers.get(tab.id);
    if (controller) {
      controller.abort();
      this.activeAbortControllers.delete(tab.id);
    }
    const timer = this.timerIntervals.get(tab.id);
    if (timer) {
      clearInterval(timer);
      this.timerIntervals.delete(tab.id);
    }
    this.activeQueryIds.delete(tab.id);
  }

  private isTabDirty(tab: EditorTab): boolean {
    if (tab.isTableViewer || tab.isProcesslist || tab.isCluster || tab.isFlightRecorder || tab.isVariables || tab.isErdCanvas) {
      return false;
    }
    const trimmed = (tab.sql || '').trim();
    if (!trimmed) return false;
    if (trimmed === 'SELECT 1;' && !tab.activeResult) return false;
    return true;
  }

  duplicateTab(tabId: string) {
    const tab = this.tabs.find((t) => t.id === tabId);
    if (!tab) return;
    if (tab.isProcesslist || tab.isErdCanvas || tab.isCluster || tab.isFlightRecorder || tab.isVariables) {
      this.showToast(`"${tab.title}" is a singleton workspace tab and cannot be duplicated.`, 'info');
      return;
    }
    const baseTitle = tab.title.replace(/^Copy(?: \(\d+\))? of /, '');
    let copyNum = 1;
    let nextTitle = `Copy of ${baseTitle}`;
    while (this.tabs.some((t) => t.title === nextTitle)) {
      copyNum++;
      nextTitle = `Copy (${copyNum}) of ${baseTitle}`;
    }
    const newTab = this.addTab(nextTitle, tab.sql);
    newTab.database = tab.database;
    newTab.isNote = tab.isNote;
    newTab.noteMode = tab.noteMode;
    newTab.isTableViewer = tab.isTableViewer;
    newTab.tableName = tab.tableName;
    newTab.tableMode = tab.tableMode;
    newTab.filterMode = tab.filterMode;
    newTab.filterPills = tab.filterPills ? JSON.parse(JSON.stringify(tab.filterPills)) : undefined;
    newTab.filterRaw = tab.filterRaw;
    newTab.appliedFilter = tab.appliedFilter;
    newTab.sortCol = tab.sortCol;
    newTab.sortDir = tab.sortDir;
    if (newTab.isTableViewer && newTab.tableName) {
      void this.loadTableData(newTab);
    }
    this.tabs = [...this.tabs];
    this.saveTabsState();
  }

  async closeTab(tabId: string, force = false) {
    const idx = this.tabs.findIndex((t) => t.id === tabId);
    if (idx === -1) return;
    const tab = this.tabs[idx];

    const hasDrafts =
      (tab.stagedDraftRows?.length || 0) > 0 ||
      Boolean(tab.tableName && Object.values(this.stagedEdits).some((e) => e.table === tab.tableName));

    if (!force && (tab.isExecuting || tab.txActive || this.isTabDirty(tab) || hasDrafts)) {
      const reason = tab.txActive
        ? `Tab "${tab.title}" has an ACTIVE UNCOMMITTED TRANSACTION. Closing it will ROLLBACK uncommitted changes. Continue?`
        : tab.isExecuting
        ? `Tab "${tab.title}" has a query currently executing. Cancel execution and close?`
        : hasDrafts
        ? `Tab "${tab.title}" has unsaved staged table edits. Discard changes and close?`
        : `Tab "${tab.title}" has unsaved ${tab.isNote ? 'note' : 'SQL'} content. Close anyway?`;
      const ok = await this.showNativeConfirm(reason, 'Close Tab');
      if (!ok) return;
      if (tab.txActive && this.activeSessionId) {
        try {
          await api.executeQuery(this.activeSessionId, 'ROLLBACK;', { confirmed: true, database: tab.database || this.currentDatabase, tabId: tab.id });
        } catch { /* ignore */ }
      }
    }

    this.cleanupTabResources(tab);

    if (this.tabs.length <= 1) {
      this.tabs = [
        {
          id: `tab-${Date.now()}`,
          title: 'Query 1.sql',
          sql: '',
          database: this.currentDatabase || '',
          isTableViewer: false,
        },
      ];
      this.activeTabId = this.tabs[0].id;
      this.saveTabsState();
      return;
    }
    this.tabs.splice(idx, 1);
    if (this.activeTabId === tabId) {
      this.activeTabId = this.tabs[Math.max(0, idx - 1)].id;
    }
    this.tabs = [...this.tabs];
    this.saveTabsState();
  }

  async closeOtherTabs(tabId: string) {
    const others = this.tabs.filter((t) => t.id !== tabId);
    const hasRisk = others.some((t) => t.txActive || t.isExecuting || this.isTabDirty(t) || (t.stagedDraftRows?.length || 0) > 0);
    if (hasRisk) {
      const ok = await this.showNativeConfirm(
        'One or more other tabs have unsaved content, active transactions, or running queries. Close them anyway?',
        'Close Other Tabs'
      );
      if (!ok) return;
    }
    for (const t of others) this.cleanupTabResources(t);
    this.tabs = this.tabs.filter((t) => t.id === tabId);
    this.activeTabId = tabId;
    this.saveTabsState();
  }

  async closeTabsToRight(tabId: string) {
    const idx = this.tabs.findIndex((t) => t.id === tabId);
    if (idx === -1 || idx === this.tabs.length - 1) return;
    const toClose = this.tabs.slice(idx + 1);
    if (toClose.some((t) => t.txActive || t.isExecuting)) {
      const ok = await this.showNativeConfirm(
        'Tabs to the right have active transactions or running queries. Close them anyway?',
        'Close Tabs to the Right'
      );
      if (!ok) return;
    }
    for (const t of toClose) this.cleanupTabResources(t);
    this.tabs = this.tabs.slice(0, idx + 1);
    if (!this.tabs.some((t) => t.id === this.activeTabId)) {
      this.activeTabId = tabId;
    }
    this.saveTabsState();
  }

  async closeAllTabs() {
    if (this.tabs.some((t) => t.txActive || t.isExecuting)) {
      const ok = await this.showNativeConfirm(
        'Active transactions or running queries will be cancelled/rolled back. Close all tabs?',
        'Close All Tabs'
      );
      if (!ok) return;
    }
    for (const t of this.tabs) this.cleanupTabResources(t);
    this.tabs = [
      {
        id: `tab-${Date.now()}`,
        title: 'Query 1.sql',
        sql: '',
        database: this.currentDatabase || '',
        isTableViewer: false,
      },
    ];
    this.activeTabId = this.tabs[0].id;
    this.saveTabsState();
  }

  renameTab(tabId: string, nextTitle: string) {
    const tab = this.tabs.find((t) => t.id === tabId);
    const title = nextTitle.trim();
    if (!tab || !title) return;
    if (this.tabs.some((t) => t.id !== tabId && t.title === title)) {
      this.showToast(`A tab named "${title}" already exists`, 'warning');
      return;
    }
    tab.title = title;
    this.tabs = [...this.tabs];
    this.saveTabsState();
  }

  // Execution
  async executeTransaction(command: 'START TRANSACTION' | 'COMMIT' | 'ROLLBACK') {
    const tab = this.activeTab;
    if (!this.activeSessionId || !tab) return;
    try {
      const isSqlite = this.activeConnection?.driver === 'sqlite';
      const sqlCmd = isSqlite && command === 'START TRANSACTION' ? 'BEGIN TRANSACTION' : command;
      const res = await api.executeQuery(this.activeSessionId, `${sqlCmd};`, {
        confirmed: false,
        database: tab.database || this.currentDatabase,
        tabId: tab.id,
      });
      if (res.error) {
        this.showToast(`${command} failed: ${res.error}`, 'error');
        return;
      }
      tab.txActive = typeof res.tx_active === 'boolean' ? res.tx_active : command === 'START TRANSACTION';
      this.tabs = [...this.tabs];
      this.saveTabsState();
      this.showToast(
        command === 'START TRANSACTION'
          ? 'Transaction started (Uncommitted)'
          : command === 'COMMIT'
          ? 'Transaction committed successfully'
          : 'Transaction rolled back',
        command === 'ROLLBACK' ? 'warning' : 'success'
      );
      if (command === 'COMMIT') await this.refreshCatalog();
    } catch (err: any) {
      this.showToast(`${command} failed: ${err.message}`, 'error');
    }
  }

  private checkClientDdlTargetMismatch(sql: string, tabDb: string): InspectionResult | null {
    const clean = sql.replace(/--[^\r\n]*/g, ' ').replace(/\/\*[\s\S]*?\*\//g, ' ').trim();
    const ddlMatch = clean.match(/^(?:CREATE|ALTER)\s+(?:OR\s+REPLACE\s+)?(?:TABLE|VIEW|INDEX)\s+(?:IF\s+(?:NOT\s+)?EXISTS\s+)?(?:[`"]([^`"]+)[`"]|([a-zA-Z0-9_$]+))\s*\.\s*(?:[`"]([^`"]+)[`"]|([a-zA-Z0-9_$]+))/i);
    if (!ddlMatch) return null;
    const explicitDb = (ddlMatch[1] || ddlMatch[2] || '').trim();
    const objName = (ddlMatch[3] || ddlMatch[4] || '').trim();
    if (!explicitDb || !tabDb || explicitDb.toLowerCase() === tabDb.toLowerCase()) return null;
    return {
      risk_level: 'HIGH',
      is_destructive: false,
      requires_affirmation: !this.isProduction,
      requires_challenge: this.isProduction,
      schema_scope_mismatch: true,
      target_database: explicitDb,
      target_table: objName,
      operation: 'DDL',
      reason: `Schema Scope Mismatch: Statement targets '${explicitDb}.${objName}', whereas this tab is scoped to '${tabDb}'.`,
      estimated_rows: null,
    };
  }

  private extractSqlParams(sql: string): string[] {
    // Strip strings, quoted identifiers, dollar quotes, and comments so literals like '10:30' or ::int are ignored.
    const masked = sql
      .replace(/'(?:''|\\.|[^'\\])*'/g, ' ')
      .replace(/"(?:""|\\.|[^"\\])*"/g, ' ')
      .replace(/`(?:``|[^`])*`/g, ' ')
      .replace(/\$[A-Za-z_]*\$[\s\S]*?\$[A-Za-z_]*\$/g, ' ')
      .replace(/--[^\n]*/g, ' ')
      .replace(/\/\*[\s\S]*?\*\//g, ' ');
    const found = new Set<string>();
    const re = /(?:^|[^:@A-Za-z0-9_])(:[A-Za-z_][A-Za-z0-9_]*|@[A-Za-z_][A-Za-z0-9_]*|\$[1-9][0-9]*)\b/g;
    let m: RegExpExecArray | null;
    while ((m = re.exec(masked)) !== null) {
      found.add(m[1]);
    }
    return Array.from(found);
  }

  private formatParamValue(raw: string): string {
    const trimmed = raw.trim();
    if (/^(null|true|false)$/i.test(trimmed)) return trimmed.toUpperCase();
    if (/^-?\d+(?:\.\d+)?$/.test(trimmed)) return trimmed;
    return `'${raw.replace(/'/g, "''")}'`;
  }

  private async promptAndBindSqlParams(sql: string): Promise<string | null> {
    const params = this.extractSqlParams(sql);
    if (params.length === 0) return sql;
    const values = new Map<string, string>();
    for (const p of params) {
      const val = await this.showNativePrompt(
        `Enter value for parameter ${p} (numbers, NULL/TRUE/FALSE unquoted; text auto-quoted):`,
        '',
        `Bind Parameter (${p})`
      );
      if (val === null) return null;
      values.set(p, this.formatParamValue(val));
    }
    // Replace outside strings/comments using a single-pass tokenizer
    return sql.replace(
      /('(?:''|\\.|[^'\\])*'|"(?:""|\\.|[^"\\])*"|`(?:``|[^`])*`|--[^\n]*|\/\*[\s\S]*?\*\/|::[A-Za-z_][A-Za-z0-9_]*|@@[A-Za-z_][A-Za-z0-9_]*|(?::[A-Za-z_][A-Za-z0-9_]*|@[A-Za-z_][A-Za-z0-9_]*|\$[1-9][0-9]*)\b)/g,
      (tok) => values.get(tok) ?? tok
    );
  }

  async executeCurrentTab(sqlOverride?: string) {
    const tab = this.activeTab;
    if (!tab || !this.activeSessionId || tab.isExecuting) return;

    if (tab.isTableViewer && sqlOverride === undefined) {
      await this.loadTableData(tab);
      return;
    }

    let sql = (sqlOverride ?? tab.sql ?? '').trim();
    if (!sql) return;

    const boundSql = await this.promptAndBindSqlParams(sql);
    if (boundSql === null) return;
    sql = boundSql;

    // Sync USE <database> statements bidirectionally with active tab DB scope
    const useMatch = sql.match(/^(?:\/\*[\s\S]*?\*\/\s*|--[^\n]*\n\s*)*USE\s+[`"]?([^`";\s]+)[`"]?\s*;?\s*$/i);
    if (useMatch && useMatch[1]) {
      const targetDb = useMatch[1];
      tab.database = targetDb;
      this.tabs = [...this.tabs];
      this.saveTabsState();
      void this.selectDatabase(targetDb);
    }

    let database = tab.database || this.currentDatabase || '';
    const sessionId = this.activeSessionId;

    // Client-side DDL target database mismatch pre-check
    const clientMismatch = this.checkClientDdlTargetMismatch(sql, database);
    let confirmed = false;
    let challengeInput = '';
    if (clientMismatch) {
      const res = await this.promptSafetyChallenge(sql, clientMismatch);
      if (!res.confirmed) return;
      confirmed = true;
      challengeInput = res.challengeInput;
    }

    const queryId = `q-${tab.id}-${Date.now()}`;
    const controller = new AbortController();
    this.activeAbortControllers.set(tab.id, controller);
    this.activeQueryIds.set(tab.id, queryId);

    tab.isExecuting = true;
    tab.isStreaming = true;
    tab.executionStartedAt = performance.now();
    tab.executionElapsedMs = 0;
    tab.statusText = 'Executing...';
    tab.streamBatchOffset = 0;
    tab.hasMoreStream = false;
    tab.baseStreamSQL = sql;
    tab.queryPage = 1;
    this.tabs = [...this.tabs];

    const timer = setInterval(() => {
      if (tab.executionStartedAt && !this.safetyChallenge.isOpen) {
        tab.executionElapsedMs = Math.round(performance.now() - tab.executionStartedAt);
        const sec = (tab.executionElapsedMs / 1000).toFixed(1);
        const rowCount = tab.activeResult?.rows?.length || 0;
        tab.statusText = rowCount > 0 ? `Streaming... (${rowCount.toLocaleString()} rows, ${sec}s)` : `Executing... (${sec}s)`;
      }
    }, 100);
    this.timerIntervals.set(tab.id, timer);

    const singleMeta = extractSingleTableFromSQL(sql);
    tab.detectedTableName = singleMeta?.table;
    tab.detectedTableDatabase = singleMeta?.database || database;
    if (singleMeta?.database && !this.catalogs[singleMeta.database.toLowerCase()]) {
      void this.ensureEditorCatalog(singleMeta.database);
    }

    const MAX_CLIENT_ROWS = 25000;

    const runAttempt = async (isConfirmed: boolean, challengeStr: string): Promise<'completed' | 'blocked' | 'cancelled'> => {
      let blockedInspection: InspectionResult | null = null;
      const statementResults: QueryExecutionResult[] = [];
      let singleResult: QueryExecutionResult | null = null;
      let multiMode = false;
      let rafId: number | null = null;
      const effectiveLimit = (tab as any).queryLimit > 0 ? (tab as any).queryLimit : 5000;

      const flushChunkUI = () => {
        rafId = null;
        if (multiMode) {
          tab.multiResults = [...statementResults];
          const activeIdx = tab.activeResultIndex ?? 0;
          if (statementResults[activeIdx]) {
            tab.activeResult = { ...statementResults[activeIdx] };
          }
        } else if (singleResult) {
          tab.activeResult = { ...singleResult };
        }
      };

      const scheduleChunkFlush = () => {
        if (rafId !== null) return;
        if (typeof requestAnimationFrame === 'function') {
          rafId = requestAnimationFrame(flushChunkUI);
        } else {
          flushChunkUI();
        }
      };

      const cancelScheduledFlush = () => {
        if (rafId !== null && typeof cancelAnimationFrame === 'function') {
          cancelAnimationFrame(rafId);
          rafId = null;
        }
      };

      const ensureStmtResult = (idx: number): QueryExecutionResult => {
        if (!statementResults[idx]) {
          statementResults[idx] = {
            columns: [],
            rows: [],
            duration_ms: 0,
            row_count: 0,
          };
        }
        return statementResults[idx];
      };

      try {
        await api.executeQueryStream(
          sessionId,
          sql,
          {
            confirmed: isConfirmed,
            challengeInput: challengeStr,
            database,
            profile: this.enableQueryProfiling,
            limit: effectiveLimit,
            tabId: tab.id,
            queryId,
            signal: controller.signal,
          },
          {
            onBlocked: (inspection) => {
              blockedInspection = inspection;
            },
            onMeta: (meta) => {
              if (!this.tabs.some((t) => t.id === tab.id)) {
                controller.abort();
                return;
              }
              if (meta.target_table && !tab.detectedTableName) {
                tab.detectedTableName = meta.target_table;
              }
              if (meta.database) {
                tab.detectedTableDatabase = meta.database;
              }
              if (typeof meta.tx_active === 'boolean') tab.txActive = meta.tx_active;

              if (meta.result_index !== undefined) {
                multiMode = true;
                const r = ensureStmtResult(meta.result_index);
                r.columns = meta.columns || [];
                r.sql = meta.sql;
                r.type = 'query';
                tab.multiResults = [...statementResults];
                if (tab.activeResultIndex === undefined || meta.result_index === 0) {
                  tab.activeResultIndex = 0;
                  tab.activeResult = r;
                }
              } else {
                singleResult = {
                  columns: meta.columns || [],
                  rows: [],
                  duration_ms: 0,
                  row_count: 0,
                  type: 'query',
                  tx_active: meta.tx_active,
                };
                tab.multiResults = undefined;
                tab.activeResultIndex = undefined;
                tab.activeResult = singleResult;
              }
              this.tabs = [...this.tabs];
            },
            onChunk: (chunk) => {
              if (!this.tabs.some((t) => t.id === tab.id)) {
                controller.abort();
                return;
              }
              const incoming = Array.isArray(chunk.rows) ? chunk.rows : [];
              if (chunk.result_index !== undefined || multiMode) {
                const idx = chunk.result_index ?? 0;
                const r = ensureStmtResult(idx);
                if (r.rows.length < MAX_CLIENT_ROWS) {
                  const room = MAX_CLIENT_ROWS - r.rows.length;
                  r.rows.push(...incoming.slice(0, room));
                  r.row_count = r.rows.length;
                }
                scheduleChunkFlush();
              } else if (singleResult) {
                if (singleResult.rows.length < MAX_CLIENT_ROWS) {
                  const room = MAX_CLIENT_ROWS - singleResult.rows.length;
                  singleResult.rows.push(...incoming.slice(0, room));
                  singleResult.row_count = singleResult.rows.length;
                }
                scheduleChunkFlush();
              }
            },
            onStatementDone: (info) => {
              cancelScheduledFlush();
              if (info.result_index !== undefined) {
                const r = ensureStmtResult(info.result_index);
                r.execution_ms = info.execution_ms;
                r.duration_ms = info.execution_ms ?? r.duration_ms;
                r.row_count = info.row_count ?? r.rows.length;
                r.sql = info.sql || r.sql;
                if ((info.row_count ?? r.rows.length) >= effectiveLimit) r.capped_at = effectiveLimit;
                tab.multiResults = [...statementResults];
                if ((tab.activeResultIndex ?? 0) === info.result_index) {
                  tab.activeResult = { ...r };
                }
              }
            },
            onExec: (execRes) => {
              if (typeof execRes.tx_active === 'boolean') tab.txActive = execRes.tx_active;
              if (execRes.result_index !== undefined) {
                multiMode = true;
                statementResults[execRes.result_index] = {
                  ...execRes,
                  type: execRes.type || 'exec',
                  columns: execRes.columns || [],
                  rows: execRes.rows || [],
                };
                tab.multiResults = [...statementResults];
                if (tab.activeResultIndex === undefined || execRes.result_index === 0) {
                  tab.activeResultIndex = 0;
                  tab.activeResult = statementResults[0];
                }
              } else if (Array.isArray(execRes.results) && execRes.results.length > 1) {
                tab.multiResults = execRes.results;
                tab.activeResultIndex = 0;
                tab.activeResult = execRes.results[0];
              } else {
                tab.multiResults = undefined;
                tab.activeResultIndex = undefined;
                tab.activeResult = execRes;
              }
              if (execRes.backup_file) {
                this.showToast(`Pre-DDL snapshot saved: ${execRes.backup_file}`, 'info');
              }
              if (execRes.backup_warning) {
                this.showToast(`Backup notice: ${execRes.backup_warning}`, 'warning');
              }
            },
            onStatementError: (errEvt) => {
              const errMsg = errEvt.error || 'Execution error';
              if (errEvt.result_index !== undefined && multiMode) {
                const r = ensureStmtResult(errEvt.result_index);
                r.error = errMsg;
                r.sql = errEvt.sql || r.sql;
                tab.multiResults = [...statementResults];
                if ((tab.activeResultIndex ?? 0) === errEvt.result_index) {
                  tab.activeResult = { ...r };
                }
              } else {
                tab.activeResult = {
                  columns: [],
                  rows: [],
                  duration_ms: Math.round(performance.now() - (tab.executionStartedAt || performance.now())),
                  row_count: 0,
                  error: errMsg,
                  statement: errEvt.sql || sql,
                };
              }
            },
            onDone: (done) => {
              cancelScheduledFlush();
              if (typeof done.tx_active === 'boolean') tab.txActive = done.tx_active;
              const elapsed = done.execution_ms ?? Math.round(performance.now() - (tab.executionStartedAt || performance.now()));
              if (singleResult) {
                singleResult.duration_ms = elapsed;
                singleResult.execution_ms = elapsed;
                singleResult.row_count = done.row_count ?? singleResult.rows.length;
                singleResult.profile = done.profile;
                singleResult.tx_active = done.tx_active;
                if (done.has_more && !hasExplicitSQLLimit(sql)) {
                  tab.hasMoreStream = true;
                  singleResult.capped_at = effectiveLimit;
                }
                tab.activeResult = { ...singleResult };
              } else if (multiMode && statementResults.length > 0) {
                tab.multiResults = statementResults.map((r) => ({
                  ...r,
                  duration_ms: r.duration_ms || elapsed,
                  profile: done.profile,
                }));
                const activeIdx = tab.activeResultIndex ?? 0;
                tab.activeResult = tab.multiResults[activeIdx] || tab.multiResults[0];
              }
            },
          }
        );

        if (blockedInspection) {
          return 'blocked';
        }
        return 'completed';
      } catch (err: any) {
        if (err?.name === 'AbortError') {
          return 'cancelled';
        }
        throw err;
      }
    };

    try {
      let status = await runAttempt(confirmed, challengeInput);
      while (status === 'blocked') {
        // Re-check inspection via safetyChallenge when server returns 422
        let inspectionPayload: InspectionResult | null = null;
        try {
          inspectionPayload = await api.inspectQuery(sessionId, sql, database);
        } catch {
          inspectionPayload = { risk_level: 'HIGH', is_destructive: true, requires_challenge: true, reason: 'Destructive statement requires confirmation.' };
        }
        const decision = await this.promptSafetyChallenge(sql, inspectionPayload);
        if (!decision.confirmed) {
          tab.statusText = 'Cancelled by Safety Guard';
          return;
        }
        status = await runAttempt(true, decision.challengeInput);
      }

      if (status === 'cancelled') {
        tab.statusText = 'Cancelled';
        return;
      }

      const finalRes = tab.activeResult;
      const elapsedMs = finalRes?.duration_ms || Math.round(performance.now() - (tab.executionStartedAt || performance.now()));
      tab.statusText = finalRes?.error ? 'Error' : 'Ready';

      // Refresh catalog on DDL mutations (strip leading comments and check multi-statement batches)
      const strippedSql = sql.replace(/--[^\n]*\n|\/\*[\s\S]*?\*\//g, ' ');
      if (!finalRes?.error && /(?:^|;)\s*(?:CREATE|ALTER|DROP|RENAME|TRUNCATE)\b/i.test(strippedSql)) {
        this.invalidateTableViewers(database);
        void this.refreshCatalog(database);
      }

      // Notify if user switched to another tab while query ran in background
      if (this.activeTabId !== tab.id) {
        if (finalRes?.error) {
          this.showToast(`Query failed in "${tab.title}": ${finalRes.error}`, 'error');
        } else {
          this.showToast(`Query finished in "${tab.title}" (${elapsedMs}ms)`, 'success');
        }
      }

      this.auditLogs.unshift({
        id: 'audit-' + Date.now() + '-' + Math.random().toString(36).substring(2, 6),
        timestamp: new Date().toLocaleTimeString(),
        sql,
        duration_ms: elapsedMs,
        row_count: finalRes?.row_count ?? 0,
        affected_rows: finalRes?.affected_rows ?? finalRes?.rows_affected,
        source: 'editor_stream',
        error: finalRes?.error,
        database,
      });
      if (this.auditLogs.length > 200) this.auditLogs.pop();
      this.auditRefreshToken++;
    } catch (err: any) {
      const errMsg = err.message || 'Execution error';
      const elapsedMs = Math.round(performance.now() - (tab.executionStartedAt || performance.now()));
      tab.statusText = 'Error';
      tab.activeResult = {
        columns: [],
        rows: [],
        duration_ms: elapsedMs,
        row_count: 0,
        error: errMsg,
        statement: sql,
      };
      if (this.activeTabId !== tab.id) {
        this.showToast(`Query failed in "${tab.title}": ${errMsg}`, 'error');
      }
      this.auditLogs.unshift({
        id: 'audit-' + Date.now() + '-' + Math.random().toString(36).substring(2, 6),
        timestamp: new Date().toLocaleTimeString(),
        sql,
        duration_ms: elapsedMs,
        row_count: 0,
        source: 'editor_stream',
        error: errMsg,
        database,
      });
      this.auditRefreshToken++;
    } finally {
      clearInterval(timer);
      this.timerIntervals.delete(tab.id);
      this.activeAbortControllers.delete(tab.id);
      this.activeQueryIds.delete(tab.id);
      tab.isExecuting = false;
      tab.isStreaming = false;
      this.tabs = [...this.tabs];
      this.saveTabsState();
    }
  }

  async pageStreamBatch(direction: 'next' | 'prev') {
    const tab = this.activeTab;
    if (!tab || !this.activeSessionId || tab.isTableViewer || tab.isExecuting) return;
    const currentOffset = tab.streamBatchOffset || 0;
    const batchSize = 5000;
    const nextOffset = direction === 'next' ? currentOffset + batchSize : Math.max(0, currentOffset - batchSize);
    const rawBase = (tab.baseStreamSQL || tab.sql || '').trim();
    if (!rawBase) return;

    tab.streamBatchOffset = nextOffset;
    const targetSql = nextOffset === 0 ? rawBase : wrapPagedSQL(rawBase, batchSize + 1, nextOffset);
    tab.isExecuting = true;
    tab.statusText = `Loading rows ${nextOffset + 1}–${nextOffset + batchSize}...`;
    this.tabs = [...this.tabs];

    try {
      const res = await api.executeQuery(this.activeSessionId, targetSql, {
        confirmed: true,
        database: tab.database || this.currentDatabase || '',
        limit: batchSize + 1,
        tabId: tab.id,
      });
      const rows = res.rows || [];
      const hasMore = rows.length > batchSize;
      tab.hasMoreStream = hasMore;
      tab.activeResult = {
        ...res,
        rows: hasMore ? rows.slice(0, batchSize) : rows,
        row_count: Math.min(rows.length, batchSize),
      };
      tab.statusText = 'Ready';
    } catch (err: any) {
      tab.streamBatchOffset = currentOffset;
      tab.hasMoreStream = true;
      tab.statusText = 'Error';
      this.showToast(`Batch paging failed: ${err.message}`, 'error');
    } finally {
      tab.isExecuting = false;
      this.tabs = [...this.tabs];
    }
  }

  // HTML5 Native Dialog System
  showNativeAlert(message: string, title = 'SqlFox Notification'): Promise<boolean> {
    return new Promise((resolve) => {
      this.nativeDialog = {
        isOpen: true,
        type: 'alert',
        title,
        message,
        resolve,
      };
    });
  }

  showNativeConfirm(message: string, title = 'Confirm Action'): Promise<boolean> {
    return new Promise((resolve) => {
      this.nativeDialog = {
        isOpen: true,
        type: 'confirm',
        title,
        message,
        resolve,
      };
    });
  }

  showNativePrompt(message: string, defaultValue = '', title = 'Input'): Promise<string | null> {
    return new Promise((resolve) => {
      this.nativeDialog = {
        isOpen: true,
        type: 'prompt',
        title,
        message,
        defaultValue,
        inputValue: defaultValue,
        resolve,
      };
    });
  }

  resolveNativeDialog(val: any) {
    if (this.nativeDialog.resolve) {
      this.nativeDialog.resolve(val);
    }
    this.nativeDialog.isOpen = false;
  }

  // Pre-Execution DDL Review Modal
  openDDLReviewModal(
    sql: string,
    onConfirm?: () => Promise<void> | void,
    title = 'Pre-Execution DDL Review',
    meta: { confirmLabel?: string; tableName?: string; database?: string } = {}
  ) {
    this.ddlReview = {
      isOpen: true,
      sql,
      title,
      confirmLabel: meta.confirmLabel,
      tableName: meta.tableName,
      database: meta.database,
      onConfirm,
    };
  }

  closeDDLReviewModal() {
    this.ddlReview.isOpen = false;
  }

  // Network Error Boundary & Health Watcher
  isConnectionRestored = $state<boolean>(false);

  triggerAutoReconnect() {
    if (this.isReconnecting) return;
    this.isConnectionRestored = false;
    this.isReconnecting = true;
    this.reconnectAttempts = 0;

    const scheduleNext = () => {
      this.reconnectAttempts++;
      const delaySec = Math.min(Math.pow(2, this.reconnectAttempts - 1), 8);
      this.reconnectCountdown = delaySec;

      if (this.reconnectTimer) clearInterval(this.reconnectTimer);
      this.reconnectTimer = setInterval(() => {
        this.reconnectCountdown--;
        if (this.reconnectCountdown <= 0) {
          clearInterval(this.reconnectTimer);
          this.attemptReconnect(scheduleNext);
        }
      }, 1000);
    };

    scheduleNext();
  }

  async attemptReconnect(onFail: () => void) {
    try {
      const res = await fetch('/api/auth/status', { method: 'GET' });
      if (res.ok) {
        this.hideReconnectBanner();
        if (this.activeConnection) {
          try {
            await this.selectConnection(this.activeConnection);
            await this.pingActiveSession();
          } catch (_) {}
        } else {
          await this.loadConnections();
        }
        return;
      }
    } catch (_) {}
    onFail();
  }

  hideReconnectBanner() {
    if (this.reconnectTimer) clearInterval(this.reconnectTimer);
    this.isReconnecting = false;
    this.reconnectAttempts = 0;
    this.reconnectCountdown = 0;
    this.isConnectionRestored = true;
    setTimeout(() => {
      this.isConnectionRestored = false;
    }, 1500);
  }

  // Foreign Key Peek
  async openFkPeek(event: MouseEvent, table: string, col: string, refTable: string, refCol: string, val: any, database?: string) {
    if (!this.activeSessionId || !refTable || !refCol) return;
    const posX = Math.min(window.innerWidth - 420, Math.max(10, event.clientX - 100));
    const posY = Math.min(window.innerHeight - 300, Math.max(10, event.clientY + 15));

    this.fkPeekState = {
      isOpen: true,
      x: posX,
      y: posY,
      table,
      col,
      refTable,
      refCol,
      val,
      data: null,
      isLoading: true,
      error: null,
    };

    try {
      const currentDb = database || this.currentDatabase || '';
      const res = await api.peekForeignKey(this.activeSessionId, currentDb, refTable, refCol, val);
      if (res && res.row) {
        this.fkPeekState.data = res;
      } else {
        this.fkPeekState.error = 'No matching record found in parent table';
      }
    } catch (err: any) {
      this.fkPeekState.error = err.message || 'Failed to peek foreign record';
    } finally {
      this.fkPeekState.isLoading = false;
    }
  }

  closeFkPeek() {
    this.fkPeekState.isOpen = false;
  }

  // Export Table SQL Dump
  exportTableDump(tableName?: string) {
    if (!this.activeSessionId) return;
    const tbl = tableName || this.activeTab?.tableName;
    if (!tbl) {
      this.openDumpModal();
      return;
    }
    const dbName = this.activeTab?.database || this.currentDatabase || '';
    let url = `/api/session/${this.activeSessionId}/export-dump?schema=true&data=true&table=${encodeURIComponent(tbl)}`;
    if (dbName) url += `&db=${encodeURIComponent(dbName)}`;
    window.location.href = url;
  }

  async cancelCurrentTab() {
    const tab = this.activeTab;
    if (tab) {
      const qid = this.activeQueryIds.get(tab.id) || 'active';
      this.cleanupTabResources(tab);
      tab.isExecuting = false;
      tab.isExplaining = false;
      tab.isStreaming = false;
      tab.statusText = 'Cancelled';
      this.tabs = [...this.tabs];
      if (this.activeSessionId) {
        try {
          await api.cancelQuery(this.activeSessionId, qid);
        } catch {
          /* server may already have closed query */
        }
      }
      this.showToast('Query execution cancelled', 'info');
      return;
    }
    if (!this.activeSessionId) return;
    try {
      await api.cancelQuery(this.activeSessionId);
      this.showToast('Query execution cancelled', 'info');
    } catch (err: any) {
      this.showToast(`Failed to cancel query: ${err.message}`, 'error');
    }
  }

  async inspectCurrentTab(sqlOverride?: string) {
    const tab = this.activeTab;
    const sessionId = this.activeSessionId;
    const sql = (sqlOverride ?? this.getEditorTargetSql() ?? tab?.sql ?? '').trim();
    if (!sessionId || !sql) {
      this.showToast('Enter SQL to inspect first.', 'warning');
      return;
    }
    try {
      const inspection = await api.inspectQuery(sessionId, sql, tab?.database || this.currentDatabase);
      this.safetyInspectionModal = {
        isOpen: true,
        sql,
        inspection,
      };
    } catch (err: any) {
      this.showToast(`Safety inspection failed: ${err.message}`, 'error');
    }
  }

  async explainCurrentTab(sqlOverride?: string, openModal = true) {
    const tab = this.activeTab;
    const sessionId = this.activeSessionId;
    const sql = sqlOverride ?? tab?.sql ?? '';
    const database = tab?.database || this.currentDatabase;
    if (!tab || !sessionId || !sql.trim() || tab.isExecuting) return;

    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 30000);
    this.activeAbortControllers.set(tab.id, controller);

    tab.isExecuting = true;
    tab.isExplaining = true;
    tab.executionStartedAt = performance.now();
    tab.executionElapsedMs = 0;
    tab.statusText = 'Explaining...';
    const timer = setInterval(() => {
      if (tab.executionStartedAt) {
        tab.executionElapsedMs = Math.round(performance.now() - tab.executionStartedAt);
        tab.statusText = `Explaining... (${(tab.executionElapsedMs / 1000).toFixed(1)}s)`;
      }
    }, 100);
    this.timerIntervals.set(tab.id, timer);
    this.tabs = [...this.tabs];

    if (tab.explainSql !== sql) {
      tab.explainResult = null;
      tab.explainSql = undefined;
    }
    try {
      const res = await api.explainQuery(sessionId, sql, database, controller.signal);
      if (this.activeSessionId !== sessionId) return;
      tab.explainResult = res;
      tab.explainSql = sql;
      tab.statusText = 'Ready';
      if (openModal) {
        this.impactPreviewData = res;
        this.impactPreviewSql = sql;
        this.isImpactPreviewOpen = true;
      } else {
        this.showToast('Query plan generated', 'success');
      }
    } catch (err: any) {
      tab.statusText = err?.name === 'AbortError' ? 'Explain timed out' : 'Error';
      this.showToast(
        err?.name === 'AbortError' ? 'Explain timed out after 30s' : `Explain failed: ${err.message}`,
        'error'
      );
    } finally {
      clearTimeout(timeoutId);
      clearInterval(timer);
      this.timerIntervals.delete(tab.id);
      this.activeAbortControllers.delete(tab.id);
      tab.isExecuting = false;
      tab.isExplaining = false;
      this.tabs = [...this.tabs];
    }
  }

  getRowUid(row: any): number {
    if (!row || typeof row !== 'object') return -1;
    let uid = this.gridRowUids.get(row);
    if (!uid) {
      uid = this.nextRowUid++;
      this.gridRowUids.set(row, uid);
    }
    return uid;
  }

  // Grid Staged Edits & CRUD
  stageEdit(
    table: string,
    rIdx: number,
    cIdx: number,
    colName: string,
    pkCol: string | undefined,
    pkVal: any,
    oldVal: any,
    newVal: any,
    database?: string,
    rowCriteria?: Record<string, any>
  ) {
    if (this.isReadOnly) {
      this.showToast('Read-only mode is active — cell edit blocked', 'warning');
      return;
    }
    const rowObj = this.activeTab?.activeResult?.rows?.[rIdx];
    const uid = rowObj ? this.getRowUid(rowObj) : rIdx;
    const key = `${table}:${uid}:${cIdx}`;
    this.stagedEdits[key] = {
      rIdx,
      cIdx,
      table,
      database: database || this.currentDatabase,
      colName,
      pkCol,
      pkVal,
      rowCriteria,
      oldVal,
      newVal,
    };
    // Auto mode persists immediately (optionally after confirmation); staged mode waits for Apply.
    if (this.gridEditMode === 'auto') {
      void this.autoCommitSingleEdit(key);
    }
  }

  private async autoCommitSingleEdit(key: string) {
    const edit = this.stagedEdits[key];
    if (!edit) return;
    if (this.gridConfirmSave) {
      const ok = await this.showNativeConfirm(
        `Save change to "${edit.table}.${edit.colName}"?`,
        'Confirm Auto-Save'
      );
      if (!ok) {
        const row = this.activeTab?.activeResult?.rows?.[edit.rIdx];
        if (row) row[edit.cIdx] = edit.oldVal;
        delete this.stagedEdits[key];
        this.tabs = [...this.tabs];
        return;
      }
    }
    await this.commitStagedEdits();
  }

  setGridEditMode(mode: 'staged' | 'auto') {
    this.gridEditMode = mode;
    try {
      localStorage.setItem('sqlfox_grid_edit_mode', mode);
    } catch {
      /* storage unavailable */
    }
    this.showToast(
      mode === 'auto'
        ? 'Auto-commit enabled — cell edits save on blur'
        : 'Staged mode enabled — review before applying',
      'info'
    );
  }

  setGridConfirmSave(val: boolean) {
    this.gridConfirmSave = val;
    try {
      localStorage.setItem('sqlfox_grid_confirm_save', String(val));
    } catch { /* ignore */ }
  }

  discardStagedEdits() {
    const rows = this.activeTab?.activeResult?.rows;
    if (Array.isArray(rows)) {
      for (const item of Object.values(this.stagedEdits)) {
        if (rows[item.rIdx] && item.cIdx < rows[item.rIdx].length) {
          rows[item.rIdx][item.cIdx] = item.oldVal;
        }
      }
    }
    if (this.activeTab?.stagedDraftRows?.length) {
      this.activeTab.stagedDraftRows = [];
    }
    this.stagedEdits = {};
    this.tabs = [...this.tabs];
    this.showToast('Discarded all staged changes', 'info');
  }

  async commitStagedEdits() {
    if (!this.activeSessionId) return;
    if (this.isReadOnly) {
      this.showToast('Read-only mode is active — writes blocked', 'warning');
      return;
    }
    const committedEntries = Object.entries(this.stagedEdits);
    const tab = this.activeTab;
    const draftRows = tab?.stagedDraftRows || [];
    if (committedEntries.length === 0 && draftRows.length === 0) return;

    const slowWarning = committedEntries.length + draftRows.length >= 20
      ? setTimeout(() => this.showToast('Applying batch changes...', 'info'), 1500)
      : null;

    const groups = new Map<string, { table: string; database?: string; entries: Array<[string, StagedEdit]> }>();
    for (const entry of committedEntries) {
      const item = entry[1];
      const targetDb = item.database || this.currentDatabase;
      const key = `${targetDb || ''}::${item.table}`;
      let group = groups.get(key);
      if (!group) {
        group = { table: item.table, database: targetDb, entries: [] };
        groups.set(key, group);
      }
      group.entries.push(entry);
    }

    try {
      let totalAffected = 0;
      for (const group of groups.values()) {
        const edits = group.entries.map(([, item]) => ({
          database: item.database || group.database,
          table: item.table,
          column: item.colName,
          value: item.newVal,
          pk_column: item.pkCol,
          pk_value: item.pkVal,
          row_criteria: item.rowCriteria,
        }));
        const res = await api.cellEdit(this.activeSessionId, {
          database: group.database,
          table: group.table,
          edits,
        });
        totalAffected += res.rows_affected ?? group.entries.length;
        for (const [k] of group.entries) {
          delete this.stagedEdits[k];
        }
      }

      const draftTable = tab?.tableName || tab?.detectedTableName;
      if (draftRows.length > 0 && draftTable) {
        const remaining: Record<string, any>[] = [];
        for (let i = 0; i < draftRows.length; i++) {
          const d = draftRows[i];
          const cleanPayload: Record<string, any> = {};
          for (const [k, v] of Object.entries(d)) {
            if (!k.startsWith('__') && v !== undefined) {
              cleanPayload[k] = v;
            }
          }
          try {
            await api.insertRow(this.activeSessionId, {
              database: tab?.database || tab?.detectedTableDatabase || this.currentDatabase,
              table: draftTable,
              data: cleanPayload,
            });
            totalAffected++;
          } catch (draftErr: any) {
            remaining.push(d);
            this.showToast(`Draft row insert failed: ${draftErr.message}`, 'error');
          }
        }
        if (tab) tab.stagedDraftRows = remaining;
      }

      this.showToast(`Saved ${totalAffected} change(s) successfully`, 'success');
      this.auditRefreshToken++;

      // Refresh data
      if (this.activeTab?.isTableViewer) {
        await this.loadTableData(this.activeTab);
      } else {
        await this.executeCurrentTab();
      }
    } catch (err: any) {
      this.showToast(`Failed to commit edits: ${err.message}`, 'error');
    } finally {
      if (slowWarning) clearTimeout(slowWarning);
    }
  }

  async insertRow(data: Record<string, any>) {
    const tab = this.activeTab;
    const targetTable = tab?.tableName || tab?.detectedTableName;
    if (!this.activeSessionId || !targetTable) return;
    if (this.isReadOnly) {
      this.showToast('Read-only mode is active — insert blocked', 'warning');
      return;
    }

    try {
      await api.insertRow(this.activeSessionId, {
        database: tab?.database || tab?.detectedTableDatabase || this.currentDatabase,
        table: targetTable,
        data,
      });
      this.showToast(`Row inserted into ${targetTable}`, 'success');
      this.isInsertRowModalOpen = false;
      this.auditRefreshToken++;
      if (tab?.isTableViewer) await this.loadTableData(tab);
      else await this.executeCurrentTab();
    } catch (err: any) {
      this.showToast(`Failed to insert row: ${err.message}`, 'error');
    }
  }

  async deleteRows(pkCol: string, pkValues: any[]) {
    const tab = this.activeTab;
    const targetTable = tab?.tableName || tab?.detectedTableName;
    if (!this.activeSessionId || !targetTable) return;
    if (this.isReadOnly) {
      this.showToast('Read-only mode is active — delete blocked', 'warning');
      return;
    }

    try {
      await api.deleteRows(this.activeSessionId, {
        database: tab?.database || tab?.detectedTableDatabase || this.currentDatabase,
        table: targetTable,
        pk_column: pkCol,
        pk_values: pkValues,
      });
      this.showToast(`Deleted ${pkValues.length} row(s)`, 'success');
      this.auditRefreshToken++;
      if (tab?.isTableViewer) await this.loadTableData(tab);
      else await this.executeCurrentTab();
    } catch (err: any) {
      this.showToast(`Failed to delete row(s): ${err.message}`, 'error');
    }
  }

  async deleteRowsByCriteria(rowCriterias: Record<string, any>[]) {
    const tab = this.activeTab;
    const targetTable = tab?.tableName || tab?.detectedTableName;
    if (!this.activeSessionId || !targetTable || !rowCriterias.length) return;
    if (this.isReadOnly) {
      this.showToast('Read-only mode is active — delete blocked', 'warning');
      return;
    }

    try {
      await api.deleteRows(this.activeSessionId, {
        database: tab?.database || tab?.detectedTableDatabase || this.currentDatabase,
        table: targetTable,
        row_criterias: rowCriterias,
      });
      this.showToast(`Deleted ${rowCriterias.length} row(s)`, 'success');
      this.auditRefreshToken++;
      if (tab?.isTableViewer) await this.loadTableData(tab);
      else await this.executeCurrentTab();
    } catch (err: any) {
      this.showToast(`Failed to delete row(s): ${err.message}`, 'error');
    }
  }

  // Filter Mode & Clause Management
  setFilterMode(mode: 'pills' | 'raw') {
    const tab = this.activeTab;
    if (!tab) return;
    tab.filterMode = mode;
    this.tabs = [...this.tabs];
    this.saveTabsState();
  }

  setFilterRaw(raw: string) {
    const tab = this.activeTab;
    if (!tab) return;
    tab.filterRaw = raw;
    this.tabs = [...this.tabs];
  }

  toggleFilterBar(tabId?: string) {
    const tab = tabId ? this.tabs.find((t) => t.id === tabId) : this.activeTab;
    if (!tab) return;
    tab.showFilterBar = !tab.showFilterBar;
    this.tabs = [...this.tabs];
    this.saveTabsState();
  }

  addFilterPill(col?: string, op: FilterPill['op'] = '=', val: string = '') {
    const tab = this.activeTab;
    if (!tab) return;
    if (!tab.filterPills) tab.filterPills = [];
    const defaultCol =
      col ||
      (tab.activeResult?.columns?.[0]
        ? typeof tab.activeResult.columns[0] === 'string'
          ? tab.activeResult.columns[0]
          : tab.activeResult.columns[0].name
        : 'id');
    tab.filterPills.push({ col: defaultCol, op, val });
    tab.filterMode = 'pills';
    tab.showFilterBar = true;
    if (val !== '' || op === 'IS NULL' || op === 'IS NOT NULL') {
      tab.appliedFilter = compileFilterPills(tab.filterPills, this.activeConnection?.driver);
      if (tab.isTableViewer) {
        tab.currentPage = 1;
        this.loadTableData(tab);
      }
    } else {
      this.tabs = [...this.tabs];
    }
    this.saveTabsState();
  }

  updateFilterPill(index: number, patch: Partial<FilterPill>) {
    const tab = this.activeTab;
    if (!tab || !tab.filterPills || !tab.filterPills[index]) return;
    Object.assign(tab.filterPills[index], patch);
    this.tabs = [...this.tabs];
  }

  removeFilterPill(index: number) {
    const tab = this.activeTab;
    if (!tab || !tab.filterPills) return;
    tab.filterPills.splice(index, 1);
    tab.appliedFilter = compileFilterPills(tab.filterPills, this.activeConnection?.driver);
    if (tab.isTableViewer) {
      tab.currentPage = 1;
      this.loadTableData(tab);
    }
    this.saveTabsState();
  }

  applyTableFilter() {
    const tab = this.activeTab;
    if (!tab || !tab.isTableViewer) return;
    tab.currentPage = 1;
    if (tab.filterMode === 'raw') {
      let rawVal = (tab.filterRaw || '').trim();
      rawVal = rawVal.replace(/^where\s+/i, '').trim();
      tab.filterRaw = rawVal;
      tab.appliedFilter = rawVal;
    } else {
      tab.appliedFilter = compileFilterPills(tab.filterPills || [], this.activeConnection?.driver);
    }
    this.loadTableData(tab);
    this.saveTabsState();
  }

  clearTableFilter() {
    const tab = this.activeTab;
    if (!tab) return;
    tab.appliedFilter = '';
    tab.filterRaw = '';
    tab.filterPills = [];
    tab.currentPage = 1;
    if (tab.isTableViewer) {
      this.loadTableData(tab);
    }
    this.saveTabsState();
  }

  handoffTableQueryToEditor() {
    const tab = this.activeTab;
    if (!tab || !tab.isTableViewer || !tab.tableName) return;
    const driver = this.activeConnection?.driver;
    const targetTable = quoteSqlIdentifier(tab.tableName, driver);
    let sql = `SELECT * FROM ${targetTable}`;
    const filter = (tab.appliedFilter || '').trim();
    if (filter) {
      sql += `\nWHERE ${filter}`;
    }
    if (tab.sortCol) {
      const escSortCol = quoteSqlIdentifier(tab.sortCol, driver);
      sql += `\nORDER BY ${escSortCol} ${tab.sortDir || 'ASC'}`;
    }
    sql += `\nLIMIT 50;`;
    this.addTab(`query_${tab.tableName}.sql`, sql);
  }

  private persistHiddenColumns(tab: EditorTab) {
    if (typeof sessionStorage === 'undefined' || !tab.tableName) return;
    const db = tab.database || this.currentDatabase || '';
    try {
      sessionStorage.setItem(`sqlfox-hidden-cols:${db}:${tab.tableName}`, JSON.stringify(tab.hiddenColumns || []));
    } catch { /* ignore */ }
  }

  toggleColumnHidden(colName: string) {
    const tab = this.activeTab;
    if (!tab) return;
    if (!tab.hiddenColumns) tab.hiddenColumns = [];
    const idx = tab.hiddenColumns.indexOf(colName);
    if (idx !== -1) {
      tab.hiddenColumns.splice(idx, 1);
    } else {
      tab.hiddenColumns.push(colName);
    }
    this.tabs = [...this.tabs];
    this.persistHiddenColumns(tab);
    this.saveTabsState();
  }

  showAllColumns() {
    const tab = this.activeTab;
    if (!tab) return;
    tab.hiddenColumns = [];
    this.tabs = [...this.tabs];
    this.persistHiddenColumns(tab);
    this.saveTabsState();
  }

  hideAllColumns(columns: string[], pkCol?: string) {
    const tab = this.activeTab;
    if (!tab) return;
    tab.hiddenColumns = columns.filter((c) => c !== pkCol);
    this.tabs = [...this.tabs];
    this.persistHiddenColumns(tab);
    this.saveTabsState();
  }

  // Safety Challenge
  promptSafetyChallenge(sql: string, inspection: InspectionResult): Promise<{ confirmed: boolean; challengeInput: string }> {
    return new Promise((resolve) => {
      this.safetyChallenge = {
        isOpen: true,
        sql,
        inspection,
        confirmed: false,
        challengeInput: '',
        resolve,
      };
    });
  }

  /**
   * Execute a DDL statement with Safety Guard challenge handling and error checking.
   * Returns false if the user cancelled the Safety Guard challenge.
   */
  async executeDDL(sql: string, database?: string, connId?: string): Promise<boolean> {
    const sessionId = connId || this.activeSessionId;
    if (!sessionId) throw new Error('No active database session');
    let res = await api.executeQuery(sessionId, sql, { confirmed: true, database });
    if (res.blocked && res.inspection) {
      const decision = await this.promptSafetyChallenge(sql, res.inspection);
      if (!decision.confirmed) {
        this.showToast('Cancelled by Safety Guard', 'info');
        return false;
      }
      res = await api.executeQuery(sessionId, sql, {
        confirmed: true,
        challengeInput: decision.challengeInput,
        database,
      });
    }
    if (res.blocked) {
      throw new Error(res.inspection?.reason || 'Execution blocked by Safety Guard (challenge confirmation required).');
    }
    if (res.error) {
      throw new Error(res.error);
    }
    return true;
  }

  confirmSafetyChallenge(challengeInput = '') {
    if (this.safetyChallenge.resolve) {
      this.safetyChallenge.resolve({ confirmed: true, challengeInput });
    }
    this.safetyChallenge.isOpen = false;
  }

  cancelSafetyChallenge() {
    if (this.safetyChallenge.resolve) {
      this.safetyChallenge.resolve({ confirmed: false, challengeInput: '' });
    }
    this.safetyChallenge.isOpen = false;
  }

  // Schema & Table Manipulation Operations
  async dropTable(tableName: string, database?: string) {
    if (!this.activeSessionId) return;
    if (this.isReadOnly) {
      this.showToast('Read-only mode is active — DROP TABLE blocked', 'warning');
      return;
    }
    const targetDb = database || this.tabs.find((t) => t.tableName?.toLowerCase() === tableName.toLowerCase())?.database || this.currentDatabase || '';
    const driver = this.activeConnection?.driver;
    const isPg = driver === 'postgres' || driver === 'postgresql';
    const qTable = quoteSqlIdentifier(tableName, driver);
    const sql = isPg ? `DROP TABLE ${qTable} CASCADE;` : `DROP TABLE IF EXISTS ${qTable};`;
    const decision = await this.promptSafetyChallenge(sql, {
      risk_level: 'CRITICAL',
      requires_challenge: true,
      target_table: tableName,
      target_database: targetDb,
      operation: 'DROP TABLE',
      reason: `Destructive operation: DROP TABLE will permanently erase table '${tableName}' and all its data.`,
      estimated_rows: null,
    });
    if (!decision.confirmed) return;

    try {
      const res = await api.executeQuery(this.activeSessionId, sql, {
        confirmed: true,
        challengeInput: decision.challengeInput || tableName,
        database: targetDb,
      });
      if (res.error) {
        this.showToast(`Failed to drop table: ${res.error}`, 'error');
        return;
      }
      this.showToast(`Table '${tableName}' dropped successfully`, 'success');
      const openTab = this.tabs.find((t) => t.tableName?.toLowerCase() === tableName.toLowerCase());
      if (openTab) {
        await this.closeTab(openTab.id, true);
      }
      await this.refreshCatalog();
    } catch (err: any) {
      this.showToast(`Failed to drop table: ${err.message}`, 'error');
    }
  }

  async truncateTable(tableName: string, database?: string) {
    if (!this.activeSessionId) return;
    if (this.isReadOnly) {
      this.showToast('Read-only mode is active — TRUNCATE TABLE blocked', 'warning');
      return;
    }
    const targetDb = database || this.tabs.find((t) => t.tableName?.toLowerCase() === tableName.toLowerCase())?.database || this.currentDatabase || '';
    const driver = this.activeConnection?.driver;
    const isPg = driver === 'postgres' || driver === 'postgresql';
    const isSqlite = driver === 'sqlite';
    const qTable = quoteSqlIdentifier(tableName, driver);
    const sql = isPg
      ? `TRUNCATE TABLE ${qTable} CASCADE;`
      : isSqlite
      ? `DELETE FROM ${qTable};`
      : `TRUNCATE TABLE ${qTable};`;
    const decision = await this.promptSafetyChallenge(sql, {
      risk_level: 'HIGH',
      requires_challenge: true,
      target_table: tableName,
      target_database: targetDb,
      operation: 'TRUNCATE',
      reason: `Destructive mutation: TRUNCATE TABLE will delete all records in table '${tableName}'.`,
      estimated_rows: null,
    });
    if (!decision.confirmed) return;

    try {
      const res = await api.executeQuery(this.activeSessionId, sql, {
        confirmed: true,
        challengeInput: decision.challengeInput || tableName,
        database: targetDb,
      });
      if (res.error) {
        this.showToast(`Failed to truncate table: ${res.error}`, 'error');
        return;
      }
      this.showToast(`Table '${tableName}' truncated`, 'success');
      const openTab = this.tabs.find((t) => t.tableName?.toLowerCase() === tableName.toLowerCase());
      if (openTab) {
        await this.loadTableData(openTab);
      }
      await this.refreshCatalog();
    } catch (err: any) {
      this.showToast(`Failed to truncate table: ${err.message}`, 'error');
    }
  }

  async renameTable(oldName: string, newName: string, database?: string) {
    if (!this.activeSessionId) return;
    const targetDb = database || this.tabs.find((t) => t.tableName?.toLowerCase() === oldName.toLowerCase())?.database || this.currentDatabase || '';
    const driver = this.activeConnection?.driver;
    const isPg = driver === 'postgres' || driver === 'postgresql';
    const isSqlite = driver === 'sqlite';
    const qOld = quoteSqlIdentifier(oldName, driver);
    const qNew = quoteSqlIdentifier(newName, driver);
    const sql = isPg || isSqlite
      ? `ALTER TABLE ${qOld} RENAME TO ${qNew};`
      : `RENAME TABLE ${qOld} TO ${qNew};`;

    this.openDDLReviewModal(sql, async () => {
      if (!(await this.executeDDL(sql, targetDb))) return;
      this.showToast(`Table renamed to '${newName}'`, 'success');
      const openTab = this.tabs.find((t) => t.tableName?.toLowerCase() === oldName.toLowerCase());
      if (openTab) {
        openTab.tableName = newName;
        openTab.title = newName;
      }
      await this.refreshCatalog(targetDb);
    }, `Rename Table: ${oldName} → ${newName}`);
  }

  async duplicateTable(sourceTable: string, targetTable: string, copyData = true, database?: string) {
    if (!this.activeSessionId) return;
    const targetDb = database || this.tabs.find((t) => t.tableName?.toLowerCase() === sourceTable.toLowerCase())?.database || this.currentDatabase || '';
    const driver = this.activeConnection?.driver;
    const isPg = driver === 'postgres' || driver === 'postgresql';
    const qSource = quoteSqlIdentifier(sourceTable, driver);
    const qTarget = quoteSqlIdentifier(targetTable, driver);
    let sql = '';
    if (isPg) {
      sql = `CREATE TABLE ${qTarget} (LIKE ${qSource} INCLUDING ALL);`;
      if (copyData) sql += `\nINSERT INTO ${qTarget} SELECT * FROM ${qSource};`;
    } else if (driver === 'sqlite') {
      sql = copyData
        ? `CREATE TABLE ${qTarget} AS SELECT * FROM ${qSource};`
        : `CREATE TABLE ${qTarget} AS SELECT * FROM ${qSource} WHERE 0 = 1;`;
    } else {
      sql = `CREATE TABLE ${qTarget} LIKE ${qSource};`;
      if (copyData) sql += `\nINSERT INTO ${qTarget} SELECT * FROM ${qSource};`;
    }

    this.openDDLReviewModal(sql, async () => {
      if (!(await this.executeDDL(sql, targetDb))) return;
      this.showToast(`Table '${sourceTable}' duplicated to '${targetTable}'`, 'success');
      await this.refreshCatalog(targetDb);
      this.openTableDataTab(targetTable);
    }, `Duplicate Table: ${sourceTable} → ${targetTable}`);
  }

  async createDatabase(dbName: string, charset = '', collation = '') {
    if (!this.activeSessionId) return;
    const driver = this.activeConnection?.driver;
    const isPg = driver === 'postgres' || driver === 'postgresql';
    const qDb = quoteSqlIdentifier(dbName, driver);
    let sql = `CREATE DATABASE ${qDb}`;
    if (!isPg && charset) {
      sql += ` CHARACTER SET ${charset}`;
      if (collation) sql += ` COLLATE ${collation}`;
    }
    sql += ';';

    this.openDDLReviewModal(sql, async () => {
      if (!(await this.executeDDL(sql))) return;
      this.showToast(`Database '${dbName}' created`, 'success');
      await this.refreshDatabases();
      await this.selectDatabase(dbName);
    }, `Create Database: ${dbName}`);
  }

  async dropDatabase(dbName: string) {
    if (!this.activeSessionId) return;
    const driver = this.activeConnection?.driver;
    const isPg = driver === 'postgres' || driver === 'postgresql';
    const qDb = quoteSqlIdentifier(dbName, driver);
    const sql = isPg ? `DROP DATABASE ${qDb};` : `DROP DATABASE IF EXISTS ${qDb};`;
    const decision = await this.promptSafetyChallenge(sql, {
      risk_level: 'CRITICAL',
      requires_challenge: true,
      target_table: dbName,
      target_database: dbName,
      operation: 'DROP DATABASE',
      reason: `Destructive operation: DROP DATABASE will permanently erase database '${dbName}' and all tables inside it.`,
      estimated_rows: null,
    });
    if (!decision.confirmed) return;

    try {
      const res = await api.executeQuery(this.activeSessionId, sql, {
        confirmed: true,
        challengeInput: decision.challengeInput || dbName,
      });
      if (res.error) throw new Error(res.error);
      this.showToast(`Database '${dbName}' dropped`, 'success');
      await this.refreshDatabases();
      if (this.currentDatabase === dbName && this.availableDatabases.length > 0) {
        await this.selectDatabase(this.availableDatabases[0]);
      }
    } catch (err: any) {
      this.showToast(`Failed to drop database: ${err.message}`, 'error');
    }
  }

  async dropView(viewName: string) {
    if (!this.activeSessionId) return;
    const driver = this.activeConnection?.driver;
    const qView = quoteSqlIdentifier(viewName, driver);
    const sql = `DROP VIEW IF EXISTS ${qView};`;
    this.openDDLReviewModal(sql, async () => {
      if (!(await this.executeDDL(sql, this.currentDatabase))) return;
      this.showToast(`View '${viewName}' dropped`, 'success');
      await this.refreshCatalog();
    }, `Drop View: ${viewName}`);
  }

  async dropRoutine(name: string, routineType: 'PROCEDURE' | 'FUNCTION' = 'PROCEDURE') {
    if (!this.activeSessionId) return;
    const driver = this.activeConnection?.driver;
    const qName = quoteSqlIdentifier(name, driver);
    const sql = `DROP ${routineType} IF EXISTS ${qName};`;
    this.openDDLReviewModal(sql, async () => {
      if (!(await this.executeDDL(sql, this.currentDatabase))) return;
      this.showToast(`${routineType} '${name}' dropped`, 'success');
      await this.refreshCatalog();
    }, `Drop ${routineType}: ${name}`);
  }

  async dropTrigger(name: string, tableName?: string) {
    if (!this.activeSessionId) return;
    const driver = this.activeConnection?.driver;
    const isPg = driver === 'postgres' || driver === 'postgresql';
    const qName = quoteSqlIdentifier(name, driver);
    const sql = isPg && tableName
      ? `DROP TRIGGER IF EXISTS ${qName} ON ${quoteSqlIdentifier(tableName, driver)};`
      : `DROP TRIGGER IF EXISTS ${qName};`;
    this.openDDLReviewModal(sql, async () => {
      if (!(await this.executeDDL(sql, this.currentDatabase))) return;
      this.showToast(`Trigger '${name}' dropped`, 'success');
      await this.refreshCatalog();
    }, `Drop Trigger: ${name}`);
  }

  async dropEvent(name: string) {
    if (!this.activeSessionId) return;
    const qName = quoteSqlIdentifier(name, this.activeConnection?.driver);
    const sql = `DROP EVENT IF EXISTS ${qName};`;
    this.openDDLReviewModal(sql, async () => {
      if (!(await this.executeDDL(sql, this.currentDatabase))) return;
      this.showToast(`Event '${name}' dropped`, 'success');
      await this.refreshCatalog();
    }, `Drop Event: ${name}`);
  }

  async toggleEventStatus(event: EventMetadata) {
    if (!this.activeSessionId) return;
    const currentlyEnabled = (event.status || '').toUpperCase() === 'ENABLED';
    const nextEnable = !currentlyEnabled;
    try {
      await api.toggleEvent(this.activeSessionId, event.name, nextEnable);
      this.showToast(`Event '${event.name}' ${nextEnable ? 'ENABLED' : 'DISABLED'}`, 'success');
      await this.refreshCatalog();
    } catch (err: any) {
      this.showToast(`Failed to toggle event: ${err.message}`, 'error');
    }
  }

  async openEventDDLTab(eventName: string) {
    if (!this.activeSessionId) return;
    try {
      const res = await api.getEventDDL(this.activeSessionId, eventName);
      this.openDdlViewModal(`EVENT: ${eventName}`, res?.ddl || `-- No DDL returned for ${eventName}`);
    } catch (err: any) {
      this.showToast(`Failed to load event DDL: ${err.message}`, 'error');
    }
  }

  async openRoutineCaller(routine: RoutineMetadata) {
    const rType = (routine.type || 'PROCEDURE').toUpperCase();
    let ddl = routine.definition || '';
    if (!ddl && this.activeSessionId) {
      try {
        const res = await api.getRoutineDDL(this.activeSessionId, routine.name, rType);
        ddl = res?.ddl || '';
      } catch { /* fallback to empty params */ }
    }
    const headerMatch = ddl.match(/(?:PROCEDURE|FUNCTION)\s+[`"a-zA-Z0-9_.]+\s*\(([\s\S]*?)\)/i);
    const paramListRaw = headerMatch ? headerMatch[1].trim() : '';
    const args: string[] = [];
    const postSelects: string[] = [];
    if (paramListRaw) {
      for (const rawParam of paramListRaw.split(',')) {
        const p = rawParam.trim();
        if (!p) continue;
        const m = p.match(/^(?:(IN|OUT|INOUT)\s+)?[`"]?([a-zA-Z0-9_$]+)[`"]?\s+(.+)$/i);
        const dir = (m?.[1] || 'IN').toUpperCase();
        const pName = m?.[2] || 'arg';
        const pType = m?.[3] || 'VARCHAR';
        if (dir === 'OUT' || dir === 'INOUT') {
          args.push(`@${pName}`);
          postSelects.push(`@${pName} AS ${pName}`);
        } else {
          const sample = /int|num|dec|float|double|bit|bool/i.test(pType) ? '0' : `'value'`;
          args.push(`${sample} /* ${pName}: ${pType} */`);
        }
      }
    }
    const qName = quoteSqlIdentifier(routine.name, this.activeConnection?.driver);
    let callSql =
      rType === 'FUNCTION'
        ? `SELECT ${qName}(${args.join(', ')}) AS result;`
        : `CALL ${qName}(${args.join(', ')});`;
    if (postSelects.length > 0 && rType !== 'FUNCTION') {
      callSql += `\nSELECT ${postSelects.join(', ')};`;
    }
    this.addTab(`Call ${routine.name}.sql`, callSql);
  }

  async createTableDirect(ddl: string, tableName: string) {
    if (!this.activeSessionId) return;
    try {
      if (!(await this.executeDDL(ddl, this.currentDatabase || ''))) return;
      this.showToast(`Table '${tableName}' created successfully`, 'success');
      await this.refreshCatalog();
      this.openTableDataTab(tableName);
    } catch (err: any) {
      this.showToast(`Failed to create table: ${err.message}`, 'error');
      throw err;
    }
  }

  openJsonCellModal(
    tableName: string | undefined,
    rowIdx: number | undefined,
    colIdx: number | undefined,
    colName: string,
    value: any,
    isReadOnly = false
  ) {
    let strVal = '';
    if (value !== null && value !== undefined) {
      if (typeof value === 'object') {
        try {
          strVal = JSON.stringify(value, null, 2);
        } catch {
          strVal = String(value);
        }
      } else {
        strVal = String(value);
      }
    }
    this.jsonCellModal = {
      isOpen: true,
      tableName,
      rowIdx,
      colIdx,
      colName,
      value: strVal,
      isReadOnly: Boolean(isReadOnly || this.isReadOnly),
    };
  }

  saveJsonCellValue(newVal: string) {
    if (this.isReadOnly) {
      this.showToast('Read-only mode is active — JSON update blocked', 'warning');
      return;
    }
    const { tableName, rowIdx, colIdx, colName } = this.jsonCellModal;
    if (!tableName || rowIdx === undefined || colIdx === undefined) return;

    const tab = this.activeTab;
    const result = tab?.activeResult;
    if (!result?.rows || !result.rows[rowIdx] || !result.columns) return;

    const oldVal = result.rows[rowIdx][colIdx];
    if (newVal !== String(oldVal ?? '')) {
      const catalog = (tab?.database && this.catalogs[tab.database.toLowerCase()]) || this.catalog;
      const table = catalog?.tables?.find((t) => t.name.toLowerCase() === tableName.toLowerCase());
      let pkCol = '';
      const pkColObj = result.columns.find((c: any) => typeof c === 'object' && c.is_primary_key);
      if (pkColObj) {
        pkCol = typeof pkColObj === 'string' ? pkColObj : pkColObj.name;
      } else if (table?.columns) {
        const pkMeta = table.columns.find((c) => c.is_primary_key);
        if (pkMeta) pkCol = pkMeta.name;
      }
      const pkIdx = pkCol ? result.columns.findIndex((c: any) => (typeof c === 'string' ? c : c.name) === pkCol) : -1;
      const pkVal = pkIdx !== -1 ? result.rows[rowIdx][pkIdx] : undefined;

      let rowCriteria: Record<string, any> | undefined;
      if (!pkCol) {
        rowCriteria = {};
        result.columns.forEach((c: any, idx: number) => {
          const colKey = typeof c === 'string' ? c : c.name;
          rowCriteria![colKey] = result.rows[rowIdx][idx];
        });
      }

      this.stageEdit(tableName, rowIdx, colIdx, colName, pkCol || undefined, pkVal, oldVal, newVal, tab?.database, rowCriteria);
      result.rows[rowIdx][colIdx] = newVal;
      this.showToast(`Staged JSON update for '${colName}'`, 'info');
    }
  }
}

export const appState = new AppState();
