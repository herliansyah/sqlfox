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

import { compileFilterPills } from '../utils/filterCompiler';
export { compileFilterPills };
import { extractSqlParams, formatParamValue, bindSqlParamsInText } from './sqlParamBinding';
import {
  deleteConnectionAction,
  refreshDatabasesAction,
  pingActiveSessionAction,
  toggleReadOnlyModeAction,
  type ConnectionActionsContext,
} from './connectionActions';
import { resolveTabsStorageKey, persistTabsToStorage, loadTabsFromStorage } from './tabStorage';
import { parseActivityLogPayload, playAudioReminderChime } from './activityStream';
import {
  buildDropTableSQL,
  buildTruncateTableSQL,
  buildRenameTableSQL,
  buildDuplicateTableSQL,
  buildCreateDatabaseSQL,
  buildDropDatabaseSQL,
} from './ddlActions';
import {
  commitStagedEditsAction,
  insertRowAction,
  deleteRowsAction,
  type GridEditsContext,
} from './gridEdits';
import {
  setFilterModeAction,
  setFilterRawAction,
  toggleFilterBarAction,
  addFilterPillAction,
  updateFilterPillAction,
  removeFilterPillAction,
  applyTableFilterAction,
  clearTableFilterAction,
  handoffTableQueryToEditorAction,
  persistHiddenColumns as persistHiddenColumnsStorage,
  type TableFilterContext,
} from './tableFilters';
import {
  dropTableAction,
  truncateTableAction,
  renameTableAction,
  duplicateTableAction,
  createDatabaseAction,
  dropDatabaseAction,
  toggleEventStatusAction,
  openEventDDLTabAction,
  openRoutineCallerAction,
  type DdlOperationsContext,
} from './ddlOperations';


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

  // Tabs management — $state.raw avoids deep-proxying large query result row matrices
  tabs = $state.raw<EditorTab[]>([
    { id: 'tab-1', title: 'Query 1.sql', sql: 'SELECT 1;', isTableViewer: false },
  ]);
  activeTabId = $state<string>('tab-1');

  patchTab(tabId: string, patch: Partial<EditorTab>) {
    this.tabs = this.tabs.map((t) => (t.id === tabId ? { ...t, ...patch } : t));
  }

  /** Replace the tab entry with a shallow copy so $state.raw + $derived(activeTab) sees a new reference. */
  commitTab(tab: EditorTab) {
    this.tabs = this.tabs.map((t) => (t.id === tab.id ? { ...tab } : t));
  }

  // Staged Edits for Grid CRUD
  stagedEdits = $state<Record<string, StagedEdit>>({});
  gridRowUids = new WeakMap<object, number>();
  private nextRowUid = 1;
  private clientIdSeq = 0;

  private nextClientId(prefix: string): string {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
      return `${prefix}-${crypto.randomUUID()}`;
    }
    this.clientIdSeq += 1;
    return `${prefix}-${Date.now()}-${this.clientIdSeq}`;
  }

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
  appVersion = $state<string>('v0.20.1');
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
  activityLogs = $state.raw<ActivityLogEntry[]>([]);
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
  private connectionSwitchGen = 0;
  private databaseSwitchGen = 0;
  private tableLoadTokens = new Map<string, number>();
  private boundPageHideHandler: (() => void) | null = null;
  private boundBeforeUnloadHandler: ((e: BeforeUnloadEvent) => string | undefined) | null = null;
  private boundVisibilityHandler: (() => void) | null = null;

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
    api.onRemoteLocked = (val: boolean) => {
      this.isRemoteLocked = val;
      if (val) {
        if (this.activityEventSource) {
          this.activityEventSource.close();
          this.activityEventSource = null;
        }
        if (this.reminderTimer) {
          clearInterval(this.reminderTimer);
          this.reminderTimer = null;
        }
        this.tabs = this.tabs.map((t) => (t.isExecuting ? { ...t, isExecuting: false } : t));
      }
    };
    api.onReconnectNeeded = () => this.triggerAutoReconnect();
    api.onReadOnlySync = (val: boolean) => {
      this.isReadOnly = Boolean(val);
    };

    if (typeof window !== 'undefined') {
      // Persist active tabs synchronously on pagehide & beforeunload
      this.boundPageHideHandler = () => {
        this.saveTabsState();
      };
      window.addEventListener('pagehide', this.boundPageHideHandler);

      // ponytail: beforeunload warning guards against inadvertent data loss while queries are running or staged edits exist
      this.boundBeforeUnloadHandler = (e: BeforeUnloadEvent) => {
        this.saveTabsState();
        const hasRunningQuery = this.tabs.some((t) => t.isExecuting);
        const hasStagedEdits = Object.keys(this.stagedEdits).length > 0 || this.tabs.some((t) => (t.stagedDraftRows?.length || 0) > 0);
        if (hasRunningQuery || hasStagedEdits) {
          e.preventDefault();
          e.returnValue = 'You have unsaved staged edits or running database queries.';
          return e.returnValue;
        }
        return undefined;
      };
      window.addEventListener('beforeunload', this.boundBeforeUnloadHandler);

      // Idle auto-wakeup: when returning after >=5 minutes hidden, probe session health silently
      this.boundVisibilityHandler = () => {
        this.isPageVisible = !document.hidden;
        if (document.hidden) {
          this.lastHiddenAt = Date.now();
        } else if (this.lastHiddenAt > 0 && Date.now() - this.lastHiddenAt >= 5 * 60 * 1000) {
          this.lastHiddenAt = 0;
          void api.getAuthStatus().catch(() => {});
          void this.pingActiveSession();
        }
      };
      document.addEventListener('visibilitychange', this.boundVisibilityHandler);
    }
    this.init();
  }

  destroy() {
    if (this.reminderTimer) {
      clearInterval(this.reminderTimer);
      this.reminderTimer = null;
    }
    if (this.reconnectTimer) {
      clearInterval(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.activityReconnectTimer) {
      clearTimeout(this.activityReconnectTimer);
      this.activityReconnectTimer = null;
    }
    if (this.activityEventSource) {
      this.activityEventSource.close();
      this.activityEventSource = null;
    }
    for (const timer of this.timerIntervals.values()) {
      clearInterval(timer);
    }
    this.timerIntervals.clear();
    for (const ctrl of this.activeAbortControllers.values()) {
      ctrl.abort();
    }
    this.activeAbortControllers.clear();
    if (typeof window !== 'undefined') {
      if (this.boundPageHideHandler) window.removeEventListener('pagehide', this.boundPageHideHandler);
      if (this.boundBeforeUnloadHandler) window.removeEventListener('beforeunload', this.boundBeforeUnloadHandler);
      if (this.boundVisibilityHandler) document.removeEventListener('visibilitychange', this.boundVisibilityHandler);
      if (this.boundErrorHandler) window.removeEventListener('error', this.boundErrorHandler);
      if (this.boundRejectionHandler) window.removeEventListener('unhandledrejection', this.boundRejectionHandler);
      this.boundErrorHandler = null;
      this.boundRejectionHandler = null;
      this.clientTelemetryInstalled = false;
    }
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
      this.isAuditDrawerOpen = false;
      this.unreadActivityErrors = 0;
    }
  }

  toggleAuditDrawer(open?: boolean) {
    const next = open ?? !this.isAuditDrawerOpen;
    this.isAuditDrawerOpen = next;
    if (next) {
      this.isActivityDrawerOpen = false;
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
      // Same-origin EventSource automatically transmits the HttpOnly sqlfox_token cookie.
      const es = new EventSource('/api/logs/stream');
      this.activityEventSource = es;

      es.onopen = () => {
        this.activityStatus = this.isActivityPaused ? 'paused' : 'live';
      };

      es.onmessage = (e) => {
        if (this.isActivityPaused) return;
        const entry = parseActivityLogPayload(e.data);
        if (!entry) return;
        this.activityLogs = [...this.activityLogs.slice(-999), entry];
        if (!this.isActivityDrawerOpen && (entry.level === 'ERROR' || entry.level === 'WARN' || entry.level === 'WARNING')) {
          this.unreadActivityErrors++;
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

  private clientTelemetryInstalled = false;
  private boundErrorHandler: ((evt: ErrorEvent) => void) | null = null;
  private boundRejectionHandler: ((evt: PromiseRejectionEvent) => void) | null = null;
  private fkPeekToken = 0;

  installClientErrorTelemetry() {
    if (this.clientTelemetryInstalled || typeof window === 'undefined') return;
    this.clientTelemetryInstalled = true;

    this.boundErrorHandler = (evt: ErrorEvent) => {
      const msg = evt.message || 'Uncaught frontend error';
      const stack = evt.error?.stack || (evt.filename ? `${evt.filename}:${evt.lineno}:${evt.colno}` : '');
      api.reportClientLog({ level: 'error', module: 'frontend', message: msg, stack });
    };
    this.boundRejectionHandler = (evt: PromiseRejectionEvent) => {
      const reason = evt.reason;
      const msg = reason instanceof Error ? reason.message : String(reason || 'Unhandled promise rejection');
      const stack = reason instanceof Error ? reason.stack || '' : '';
      api.reportClientLog({ level: 'error', module: 'frontend', message: `UnhandledRejection: ${msg}`, stack });
    };

    window.addEventListener('error', this.boundErrorHandler);
    window.addEventListener('unhandledrejection', this.boundRejectionHandler);
  }

  async init() {
    try {
      const auth = await api.getAuthStatus();
      this.authStatus = auth;
      if (auth.version) {
        this.appVersion = auth.version;
      }
      if (!auth.is_remote) {
        try {
          const keyInfo = await api.getMasterKey();
          if (keyInfo) {
            this.authStatus = { ...auth, ...keyInfo };
          }
        } catch (_) {}
      }
      if (auth.is_remote && !auth.unlocked) {
        this.isRemoteLocked = true;
      } else {
        this.isRemoteLocked = false;
        if (typeof document !== 'undefined') {
          document.body.classList.remove('vault-locked');
        }
      }
      if (!this.isRemoteLocked) {
        this.installClientErrorTelemetry();
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
    if (this.isRemoteLocked || (typeof document !== 'undefined' && document.hidden)) return;
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
    playAudioReminderChime();
  }

  async unlockRemote(password: string) {
    const res = await api.unlockVault(password);
    if (res && res.success) {
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
        if (!auth.is_remote) {
          try { keyInfo = await api.getMasterKey(); } catch (_) {}
        }
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

  private connectionActionsContext(): ConnectionActionsContext {
    const self = this;
    return {
      get activeConnection() { return self.activeConnection; },
      set activeConnection(v) { self.activeConnection = v; },
      get activeSessionId() { return self.activeSessionId; },
      set activeSessionId(v) { self.activeSessionId = v; },
      get currentDatabase() { return self.currentDatabase; },
      set currentDatabase(v) { self.currentDatabase = v; },
      get availableDatabases() { return self.availableDatabases; },
      set availableDatabases(v) { self.availableDatabases = v; },
      get isLoadingCatalog() { return self.isLoadingCatalog; },
      set isLoadingCatalog(v) { self.isLoadingCatalog = v; },
      get isPingingServer() { return self.isPingingServer; },
      set isPingingServer(v) { self.isPingingServer = v; },
      get serverLatency() { return self.serverLatency; },
      set serverLatency(v) { self.serverLatency = v; },
      get serverMemoryMb() { return self.serverMemoryMb; },
      set serverMemoryMb(v) { self.serverMemoryMb = v; },
      get serverInfo() { return self.serverInfo; },
      set serverInfo(v) { self.serverInfo = v; },
      get isReadOnly() { return self.isReadOnly; },
      set isReadOnly(v) { self.isReadOnly = v; },
      get isProduction() { return self.isProduction; },
      setCatalog: (c) => self.setCatalog(c),
      clearCatalogCache: () => {
        self.catalog = null;
        self.catalogs = {};
        self.catalogRequests.clear();
      },
      loadConnections: () => self.loadConnections(),
      showToast: (msg, type) => self.showToast(msg, type),
      showNativeConfirm: (msg, title) => self.showNativeConfirm(msg, title),
    };
  }

  async deleteConnection(id: string) {
    await deleteConnectionAction(this.connectionActionsContext(), id);
  }

  async selectConnection(conn: ConnectionProfile) {
    const gen = ++this.connectionSwitchGen;
    this.isConnecting = true;
    this.connectionError = null;
    try {
      const previousConnectionId = this.activeConnection?.id;
      if (previousConnectionId && previousConnectionId !== conn.id) {
        // Persist the outgoing connection's tabs BEFORE changing activeConnection, then reset
        // tabs so a previous connection's SQL/results never leak into the new (potentially Production) connection.
        this.saveTabsState();
        this.tabs = [];
        this.activeTabId = '';
        this.catalog = null;
        this.catalogs = {};
        this.catalogRequests.clear();
        this.currentDatabase = '';
        this.stagedEdits = {};
      }
      this.activeConnection = conn;
      if (typeof sessionStorage !== 'undefined') sessionStorage.setItem('sqlfox_active_conn_id', conn.id);
      localStorage.setItem('sqlfox_last_conn_id', conn.id);
      const sess = await api.connectSession(conn.id);
      if (this.connectionSwitchGen !== gen) return;
      this.activeSessionId = sess.id || sess.connection_id || conn.id;
      if (sess.schema) {
        this.setCatalog(sess.schema);
      }

      let dbs = (sess.schema && sess.schema.databases) || [];
      if (!dbs || dbs.length === 0) {
        try {
          const fresh = await api.getSchema(this.activeSessionId, '');
          if (this.connectionSwitchGen !== gen) return;
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
      if (this.connectionSwitchGen !== gen) return;
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
        if (this.connectionSwitchGen !== gen) return;
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
      if (this.connectionSwitchGen !== gen) return;
      console.error('Failed to select connection:', err);
      this.connectionError = err.message || 'Failed to connect to database';
      throw err;
    } finally {
      if (this.connectionSwitchGen === gen) {
        this.isConnecting = false;
      }
    }
  }

  async refreshDatabases() {
    await refreshDatabasesAction(this.connectionActionsContext());
  }

  async pingActiveSession() {
    await pingActiveSessionAction(this.connectionActionsContext());
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
    await toggleReadOnlyModeAction(this.connectionActionsContext());
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
    const gen = ++this.databaseSwitchGen;
    this.isLoadingCatalog = true;
    const slowTimer = setTimeout(() => {
      if (this.databaseSwitchGen === gen) {
        this.showToast(`Inspecting database '${dbName}' schema (large catalog)...`, 'info');
      }
    }, 3000);
    try {
      const schema = await api.selectDatabase(this.activeSessionId, dbName);
      if (this.databaseSwitchGen !== gen) return;
      if (schema && (schema.tables || schema.databases)) {
        this.setCatalog(schema);
        this.currentDatabase = schema.database_name || dbName;
        if (schema.databases && schema.databases.length > 0) {
          this.availableDatabases = schema.databases;
        }
      } else {
        const fetched = await api.getSchema(this.activeSessionId, dbName);
        if (this.databaseSwitchGen !== gen) return;
        this.setCatalog(fetched);
        this.currentDatabase = fetched.database_name || dbName;
        if (fetched.databases && fetched.databases.length > 0) {
          this.availableDatabases = fetched.databases;
        }
      }
      this.showToast(`Switched to database ${dbName}`, 'info');
    } catch (err: any) {
      if (this.databaseSwitchGen !== gen) return;
      console.error('Failed to switch database:', err);
      this.showToast(`Failed to switch database: ${err.message}`, 'error');
    } finally {
      clearTimeout(slowTimer);
      if (this.databaseSwitchGen === gen) {
        this.isLoadingCatalog = false;
      }
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
    return resolveTabsStorageKey(connId || this.activeConnection?.id);
  }

  saveTabsState() {
    if (!this.activeConnection?.id) return;
    persistTabsToStorage(this.getTabsStorageKey(this.activeConnection.id), this.activeTabId, this.tabs);
  }

  restoreTabsState(connId: string): boolean {
    if (!connId) return false;
    const restored = loadTabsFromStorage(this.getTabsStorageKey(connId), this.activeConnection?.driver);
    if (!restored) return false;
    this.tabs = restored.tabs;
    this.activeTabId = restored.activeTabId;
    if (this.activeTab?.isCluster) this.processlistSubTab = 'cluster';
    else if (this.activeTab?.isVariables) this.processlistSubTab = 'variables';
    else if (this.activeTab?.isFlightRecorder) this.processlistSubTab = 'flight-recorder';
    if (this.activeTab?.isTableViewer && this.activeTab.tableName) {
      this.loadTableData(this.activeTab);
    }
    return true;
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
    const newId = this.nextClientId('tab');
    const newTab: EditorTab = {
      id: newId,
      title: finalTitle,
      sql,
      database: this.currentDatabase || '',
      isTableViewer: false,
    };
    this.tabs = [...this.tabs, newTab];
    this.activeTabId = newId;
    this.saveTabsState();
    return this.tabs[this.tabs.length - 1];
  }

  openTableDataTab(tableName: string, initialMode: TableMode = 'data') {
    const currentDb = this.currentDatabase || '';
    const existing = this.tabs.find((t) => t.isTableViewer && t.tableName === tableName && (!t.database || t.database === currentDb));
    if (existing) {
      this.patchTab(existing.id, { tableMode: initialMode });
      this.activeTabId = existing.id;
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
    this.patchTab(tab.id, { isExecuting: true });
    const startTime = performance.now();
    // Guard against out-of-order responses when the user pages/filters rapidly.
    const token = (this.tableLoadTokens.get(tab.id) || 0) + 1;
    this.tableLoadTokens.set(tab.id, token);
    try {
      const limit = tab.pageSize || 25;
      const offset = ((tab.currentPage || 1) - 1) * limit;
      const db = tab.database || this.currentDatabase || '';

      // Prefer structured filter pills for pill mode so the backend builds the WHERE clause safely.
      const structuredPills =
        tab.filterMode !== 'raw' && tab.filterPills?.length
          ? tab.filterPills
              .filter((p) => p.col && (p.op === 'IS NULL' || p.op === 'IS NOT NULL' || p.val !== ''))
              .map((p) => ({ column: p.col, operator: p.op, value: p.val }))
          : [];

      let filterClause = (tab.appliedFilter || '').trim();
      if (!filterClause && structuredPills.length === 0 && tab.filterPills && tab.filterPills.length > 0) {
        filterClause = compileFilterPills(tab.filterPills, this.activeConnection?.driver);
      }

      const res = await api.getTableRows(
        this.activeSessionId,
        tab.tableName,
        limit,
        offset,
        db,
        structuredPills.length > 0 ? undefined : filterClause || undefined,
        tab.sortCol || undefined,
        tab.sortDir || undefined,
        structuredPills.length > 0 ? structuredPills : undefined
      );
      if (this.tableLoadTokens.get(tab.id) !== token) return; // a newer load superseded this one
      if (res && res.duration_ms === undefined) {
        res.duration_ms = Math.round(performance.now() - startTime);
      }
      if (res && res.row_count === undefined && res.rows) {
        res.row_count = res.rows.length;
      }
      const liveTab = this.tabs.find((t) => t.id === tab.id);
      this.patchTab(tab.id, {
        resultRunId: ((liveTab?.resultRunId ?? tab.resultRunId) ?? 0) + 1,
        activeResult: res,
      });

      // Synchronize exact/effective row count back into the table catalog immutably
      if (res && typeof (res as any).total_rows === 'number') {
        const totalRows = (res as any).total_rows as number;
        this.updateCatalogTableRowCount(tab.tableName, db, totalRows, Boolean(res.is_exact));
      }
    } catch (err: any) {
      if (this.tableLoadTokens.get(tab.id) !== token) return;
      this.patchTab(tab.id, {
        activeResult: {
          columns: [],
          rows: [],
          duration_ms: Math.round(performance.now() - startTime),
          row_count: 0,
          error: err.message || 'Error loading table data',
        },
      });
    } finally {
      if (this.tableLoadTokens.get(tab.id) === token) {
        this.patchTab(tab.id, { isExecuting: false });
        this.saveTabsState();
      }
    }
  }

  private updateCatalogTableRowCount(tableName: string, db: string, totalRows: number, isExact: boolean) {
    const patchTables = (tables: TableMetadata[] | undefined) => {
      if (!tables) return tables;
      let changed = false;
      const next = tables.map((t) => {
        if (t.name.toLowerCase() !== tableName.toLowerCase()) return t;
        if (!isExact && t.row_count !== undefined && t.row_count !== 0) return t;
        changed = true;
        return {
          ...t,
          row_count: totalRows,
          rows_count: totalRows,
          ...(isExact ? { row_count_exact: true } : {}),
        };
      });
      return changed ? next : tables;
    };
    if (this.catalog) {
      const nextTables = patchTables(this.catalog.tables);
      if (nextTables && nextTables !== this.catalog.tables) {
        this.catalog = { ...this.catalog, tables: nextTables };
      }
    }
    const dbKey = db.toLowerCase();
    if (db && this.catalogs[dbKey]) {
      const nextTables = patchTables(this.catalogs[dbKey].tables);
      if (nextTables && nextTables !== this.catalogs[dbKey].tables) {
        this.catalogs = {
          ...this.catalogs,
          [dbKey]: { ...this.catalogs[dbKey], tables: nextTables },
        };
      }
    }
  }

  async refreshTableExactCount(tableName: string) {
    if (!this.activeSessionId || !tableName) return;
    const tab = this.tabs.find((t) => t.isTableViewer && t.tableName?.toLowerCase() === tableName.toLowerCase()) || this.activeTab;
    const db = tab?.database || this.currentDatabase || '';
    let filterClause = (tab?.appliedFilter || '').trim();
    if (!filterClause && tab?.filterPills && tab.filterPills.length > 0) {
      filterClause = compileFilterPills(tab.filterPills, this.activeConnection?.driver);
    }
    try {
      const res = await api.getTableCount(this.activeSessionId, tableName, db, filterClause || undefined);
      if (res && typeof res.count === 'number') {
        this.updateCatalogTableRowCount(tableName, db, res.count, true);
        if (tab && tab.activeResult) {
          tab.activeResult.total_rows = res.count;
          tab.activeResult.is_exact = true;
          this.tabs = [...this.tabs];
        }
        this.showToast(`Exact count: ${res.count.toLocaleString()} rows (${res.duration_ms} ms)`, 'info');
      }
    } catch (err: any) {
      this.showToast(`Failed to calculate exact row count: ${err.message || err}`, 'error');
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
    // Purge staged cell edits scoped to this tab so closed tabs never leak stale edits.
    let removedEdit = false;
    const nextEdits: Record<string, StagedEdit> = {};
    for (const [k, v] of Object.entries(this.stagedEdits)) {
      if (v.tabId === tab.id) {
        removedEdit = true;
      } else {
        nextEdits[k] = v;
      }
    }
    if (removedEdit) {
      this.stagedEdits = nextEdits;
    }
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
      Object.values(this.stagedEdits).some((e) => e.tabId === tab.id);

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

  private async rollbackTabsTransactions(tabs: EditorTab[]) {
    if (!this.activeSessionId) return;
    for (const t of tabs) {
      if (t.txActive) {
        try {
          await api.executeQuery(this.activeSessionId, 'ROLLBACK;', {
            confirmed: true,
            database: t.database || this.currentDatabase,
            tabId: t.id,
          });
        } catch { /* ignore */ }
      }
    }
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
    await this.rollbackTabsTransactions(others);
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
    await this.rollbackTabsTransactions(toClose);
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
    await this.rollbackTabsTransactions(this.tabs);
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

  private async promptAndBindSqlParams(sql: string): Promise<string | null> {
    const params = extractSqlParams(sql, this.activeConnection?.driver);
    if (params.length === 0) return sql;
    const values = new Map<string, string>();
    for (const p of params) {
      const val = await this.showNativePrompt(
        `Enter value for parameter ${p} (numbers, NULL/TRUE/FALSE unquoted; text auto-quoted):`,
        '',
        `Bind Parameter (${p})`
      );
      if (val === null) return null;
      values.set(p, formatParamValue(val));
    }
    return bindSqlParamsInText(sql, values);
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
      await this.selectDatabase(targetDb);
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

    const queryId = this.nextClientId(`q-${tab.id}`);
    const controller = new AbortController();
    this.activeAbortControllers.set(tab.id, controller);
    this.activeQueryIds.set(tab.id, queryId);

    tab.isExecuting = true;
    tab.isStreaming = true;
    tab.executionStartedAt = performance.now();
    tab.executionElapsedMs = 0;
    tab.resultRunId = (tab.resultRunId ?? 0) + 1;
    tab.statusText = 'Executing...';
    tab.streamBatchOffset = 0;
    tab.hasMoreStream = false;
    tab.baseStreamSQL = sql;
    tab.queryPage = 1;
    tab.manualTotalRows = null;
    tab.isCountingTotal = false;
    this.commitTab(tab);

    const timer = setInterval(() => {
      if (tab.executionStartedAt && !this.safetyChallenge.isOpen) {
        tab.executionElapsedMs = Math.round(performance.now() - tab.executionStartedAt);
        const sec = (tab.executionElapsedMs / 1000).toFixed(1);
        const rowCount = tab.activeResult?.rows?.length || 0;
        tab.statusText = rowCount > 0 ? `Streaming... (${rowCount.toLocaleString()} rows, ${sec}s)` : `Executing... (${sec}s)`;
        this.commitTab(tab);
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
      const effectiveLimit = (tab as any).queryLimit > 0 ? (tab as any).queryLimit : 1000;

      const flushChunkUI = () => {
        rafId = null;
        if (multiMode) {
          tab.multiResults = [...statementResults];
          const activeIdx = tab.activeResultIndex ?? 0;
          if (statementResults[activeIdx]) {
            tab.activeResult = { ...statementResults[activeIdx] };
          }
          this.commitTab(tab);
        } else if (singleResult) {
          tab.activeResult = { ...singleResult };
          this.commitTab(tab);
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
              if (meta.database && !tab.detectedTableDatabase) {
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
              this.commitTab(tab);
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
              this.commitTab(tab);
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
      this.commitTab(tab);
      this.saveTabsState();
    }
  }

  async pageStreamBatch(direction: 'next' | 'prev') {
    const tab = this.activeTab;
    if (!tab || !this.activeSessionId || tab.isTableViewer || tab.isExecuting) return;
    const currentOffset = tab.streamBatchOffset || 0;
    const batchSize = (tab as any).queryLimit > 0 ? (tab as any).queryLimit : 1000;
    const nextOffset = direction === 'next' ? currentOffset + batchSize : Math.max(0, currentOffset - batchSize);
    const rawBase = (tab.baseStreamSQL || tab.sql || '').trim();
    if (!rawBase) return;

    tab.streamBatchOffset = nextOffset;
    const targetSql = nextOffset === 0 ? rawBase : wrapPagedSQL(rawBase, batchSize + 1, nextOffset);
    tab.isExecuting = true;
    tab.statusText = `Loading rows ${nextOffset + 1}–${nextOffset + batchSize}...`;
    this.commitTab(tab);

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
      this.commitTab(tab);
    }
  }

  /**
   * Runs an explicit on-demand COUNT(*) for the current query tab without
   * triggering full scans on regular query executions.
   */
  async countQueryTotal(tab?: EditorTab) {
    const t = tab || this.activeTab;
    if (!t || !this.activeSessionId || t.isTableViewer || t.isCountingTotal) return;
    const rawBase = (t.baseStreamSQL || t.sql || '').trim();
    if (!rawBase) return;

    const clean = rawBase.replace(/;+\s*$/, '');
    const countSql = `SELECT COUNT(*) AS _total_count FROM (\n${clean}\n) AS _sf_count;`;

    t.isCountingTotal = true;
    this.commitTab(t);

    try {
      const res = await api.executeQuery(this.activeSessionId, countSql, {
        confirmed: true,
        database: t.database || this.currentDatabase || '',
        limit: 1,
        tabId: t.id,
      });
      if (res.rows && res.rows.length > 0 && res.rows[0].length > 0) {
        const val = Number(res.rows[0][0]);
        if (!isNaN(val)) {
          t.manualTotalRows = val;
          this.showToast(`Total records: ${val.toLocaleString()}`, 'info');
        }
      }
    } catch (err: any) {
      this.showToast(`Count failed: ${err.message}`, 'error');
    } finally {
      t.isCountingTotal = false;
      this.commitTab(t);
    }
  }

  private nativeDialogQueue: Array<() => void> = [];
  private safetyChallengeQueue: Array<() => void> = [];

  private enqueueNativeDialog(openFn: () => void) {
    if (!this.nativeDialog.isOpen) {
      openFn();
    } else {
      this.nativeDialogQueue.push(openFn);
    }
  }

  // HTML5 Native Dialog System
  showNativeAlert(message: string, title = 'SqlFox Notification'): Promise<boolean> {
    return new Promise((resolve) => {
      this.enqueueNativeDialog(() => {
        this.nativeDialog = {
          isOpen: true,
          type: 'alert',
          title,
          message,
          resolve,
        };
      });
    });
  }

  showNativeConfirm(message: string, title = 'Confirm Action'): Promise<boolean> {
    return new Promise((resolve) => {
      this.enqueueNativeDialog(() => {
        this.nativeDialog = {
          isOpen: true,
          type: 'confirm',
          title,
          message,
          resolve,
        };
      });
    });
  }

  showNativePrompt(message: string, defaultValue = '', title = 'Input'): Promise<string | null> {
    return new Promise((resolve) => {
      this.enqueueNativeDialog(() => {
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
    });
  }

  resolveNativeDialog(val: any) {
    const resolver = this.nativeDialog.resolve;
    this.nativeDialog.isOpen = false;
    this.nativeDialog.resolve = undefined;
    if (resolver) {
      resolver(val);
    }
    const next = this.nativeDialogQueue.shift();
    if (next) next();
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

    const token = ++this.fkPeekToken;
    const sessionId = this.activeSessionId;
    try {
      const currentDb = database || this.currentDatabase || '';
      const res = await api.peekForeignKey(sessionId, currentDb, refTable, refCol, val);
      if (token !== this.fkPeekToken || this.activeSessionId !== sessionId) return;
      if (res && res.row) {
        this.fkPeekState.data = res;
      } else {
        this.fkPeekState.error = 'No matching record found in parent table';
      }
    } catch (err: any) {
      if (token !== this.fkPeekToken) return;
      this.fkPeekState.error = err.message || 'Failed to peek foreign record';
    } finally {
      if (token === this.fkPeekToken) {
        this.fkPeekState.isLoading = false;
      }
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

    const startedAt = performance.now();
    this.patchTab(tab.id, {
      isExecuting: true,
      isExplaining: true,
      executionStartedAt: startedAt,
      executionElapsedMs: 0,
      statusText: 'Explaining...',
      ...(tab.explainSql !== sql ? { explainResult: null, explainSql: undefined } : {}),
    });
    const timer = setInterval(() => {
      const elapsed = Math.round(performance.now() - startedAt);
      this.patchTab(tab.id, {
        executionElapsedMs: elapsed,
        statusText: `Explaining... (${(elapsed / 1000).toFixed(1)}s)`,
      });
    }, 100);
    this.timerIntervals.set(tab.id, timer);

    try {
      const res = await api.explainQuery(sessionId, sql, database, controller.signal);
      if (this.activeSessionId !== sessionId) return;
      this.patchTab(tab.id, {
        explainResult: res,
        explainSql: sql,
        statusText: 'Ready',
      });
      if (openModal) {
        this.impactPreviewData = res;
        this.impactPreviewSql = sql;
        this.isImpactPreviewOpen = true;
      } else {
        this.showToast('Query plan generated', 'success');
      }
    } catch (err: any) {
      this.patchTab(tab.id, {
        statusText: err?.name === 'AbortError' ? 'Explain timed out' : 'Error',
      });
      this.showToast(
        err?.name === 'AbortError' ? 'Explain timed out after 30s' : `Explain failed: ${err.message}`,
        'error'
      );
    } finally {
      clearTimeout(timeoutId);
      clearInterval(timer);
      this.timerIntervals.delete(tab.id);
      this.activeAbortControllers.delete(tab.id);
      this.patchTab(tab.id, {
        isExecuting: false,
        isExplaining: false,
      });
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
    rowCriteria?: Record<string, any>,
    pkValues?: Record<string, any>
  ) {
    if (this.isReadOnly) {
      this.showToast('Read-only mode is active — cell edit blocked', 'warning');
      return;
    }
    const tabId = this.activeTab?.id || 'default';
    const rowObj = this.activeTab?.activeResult?.rows?.[rIdx];
    const uid = rowObj ? this.getRowUid(rowObj) : rIdx;
    const key = `${tabId}:${table}:${uid}:${cIdx}`;
    const existing = this.stagedEdits[key];
    this.stagedEdits[key] = {
      tabId,
      rIdx,
      cIdx,
      table,
      database: database || this.currentDatabase,
      colName,
      pkCol,
      pkVal,
      pkValues,
      rowCriteria,
      oldVal: existing ? existing.oldVal : oldVal,
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
    const activeTabId = this.activeTab?.id;
    const rows = this.activeTab?.activeResult?.rows;
    const remaining: Record<string, StagedEdit> = {};
    for (const [key, item] of Object.entries(this.stagedEdits)) {
      const belongsToActiveTab = !item.tabId || item.tabId === activeTabId;
      if (belongsToActiveTab) {
        if (Array.isArray(rows) && rows[item.rIdx] && item.cIdx < rows[item.rIdx].length) {
          rows[item.rIdx][item.cIdx] = item.oldVal;
        }
      } else {
        remaining[key] = item;
      }
    }
    if (this.activeTab?.stagedDraftRows?.length) {
      this.activeTab.stagedDraftRows = [];
    }
    this.stagedEdits = remaining;
    this.tabs = [...this.tabs];
    this.showToast('Discarded staged changes for active tab', 'info');
  }

  private gridEditsContext(): GridEditsContext {
    const self = this;
    return {
      get activeSessionId() { return self.activeSessionId; },
      get isReadOnly() { return self.isReadOnly; },
      get currentDatabase() { return self.currentDatabase; },
      get activeTab() { return self.activeTab; },
      get tabs() { return self.tabs; },
      get stagedEdits() { return self.stagedEdits; },
      set stagedEdits(v) { self.stagedEdits = v; },
      get gridEditMode() { return self.gridEditMode; },
      set gridEditMode(v) { self.gridEditMode = v; },
      get gridConfirmSave() { return self.gridConfirmSave; },
      set gridConfirmSave(v) { self.gridConfirmSave = v; },
      get auditRefreshToken() { return self.auditRefreshToken; },
      set auditRefreshToken(v) { self.auditRefreshToken = v; },
      get isInsertRowModalOpen() { return self.isInsertRowModalOpen; },
      set isInsertRowModalOpen(v) { self.isInsertRowModalOpen = v; },
      setTabs: (next) => { self.tabs = next; },
      setStagedEdits: (next) => { self.stagedEdits = next; },
      showToast: (msg, type) => self.showToast(msg, type),
      showNativeConfirm: (message, title) => self.showNativeConfirm(message, title),
      loadTableData: (tab) => self.loadTableData(tab),
      executeCurrentTab: () => self.executeCurrentTab(),
    };
  }

  async commitStagedEdits() {
    await commitStagedEditsAction(this.gridEditsContext());
  }

  async insertRow(data: Record<string, any>): Promise<boolean> {
    return await insertRowAction(this.gridEditsContext(), data);
  }

  async deleteRows(pkCol: string, pkValues: any[]) {
    await deleteRowsAction(this.gridEditsContext(), { pk_column: pkCol, pk_values: pkValues }, pkValues.length);
  }

  async deleteRowsByCriteria(rowCriterias: Record<string, any>[]) {
    await deleteRowsAction(this.gridEditsContext(), { row_criterias: rowCriterias }, rowCriterias.length);
  }

  async deleteRowsByCompositePK(pkRows: Record<string, any>[]) {
    await deleteRowsAction(this.gridEditsContext(), { pk_rows: pkRows }, pkRows.length);
  }

  private get modalOverlays(): Array<{ isOpen: () => boolean; close: () => void }> {
    const boolFlag = (key: keyof AppState): { isOpen: () => boolean; close: () => void } => ({
      isOpen: () => Boolean(this[key]),
      close: () => { (this as Record<string, any>)[key as string] = false; },
    });
    const modalObj = (key: 'ddlViewModal' | 'safetyInspectionModal' | 'jsonCellModal' | 'ddlReview' | 'fkPeekState') => ({
      isOpen: () => this[key].isOpen,
      close: () => { this[key].isOpen = false; },
    });

    return [
      boolFlag('isCommandPaletteOpen'),
      boolFlag('isSchemaSearchOpen'),
      boolFlag('isSmartSeedModalOpen'),
      boolFlag('isLockInspectorOpen'),
      modalObj('ddlViewModal'),
      modalObj('safetyInspectionModal'),
      boolFlag('isConnectionModalOpen'),
      boolFlag('isRecordInspectorOpen'),
      boolFlag('isInsertRowModalOpen'),
      boolFlag('isShortcutsModalOpen'),
      boolFlag('isCreateDbModalOpen'),
      boolFlag('isTableCreatorOpen'),
      boolFlag('isRenameTableModalOpen'),
      boolFlag('isDuplicateTableModalOpen'),
      boolFlag('isViewDesignerOpen'),
      boolFlag('isTriggerDesignerOpen'),
      boolFlag('isRoutineDesignerOpen'),
      boolFlag('isEventDesignerOpen'),
      boolFlag('isDumpModalOpen'),
      boolFlag('isStructureSyncModalOpen'),
      boolFlag('isRunSqlModalOpen'),
      {
        isOpen: () => this.isSnippetsModalOpen,
        close: () => {
          this.pendingSnippet = null;
          this.isSnippetsModalOpen = false;
        },
      },
      boolFlag('isCsvImportModalOpen'),
      boolFlag('isPrivilegeMatrixOpen'),
      boolFlag('isDbDiagnosticsOpen'),
      boolFlag('isAiSettingsModalOpen'),
      boolFlag('isNoteComposerModalOpen'),
      boolFlag('isUpdateModalOpen'),
      {
        isOpen: () => this.isMockDataModalOpen,
        close: () => this.closeMockDataModal(),
      },
      boolFlag('isSettingsModalOpen'),
      modalObj('jsonCellModal'),
      boolFlag('isDataTransferOpen'),
      boolFlag('isDataSyncOpen'),
      boolFlag('isRefreshAllOpen'),
      boolFlag('isImpactPreviewOpen'),
      modalObj('ddlReview'),
      modalObj('fkPeekState'),
    ];
  }

  get hasOpenModal(): boolean {
    if (this.isRemoteLocked || this.nativeDialog.isOpen || this.safetyChallenge.isOpen) {
      return true;
    }
    return this.modalOverlays.some((m) => m.isOpen());
  }

  closeTopOverlay(): boolean {
    for (const overlay of this.modalOverlays) {
      if (overlay.isOpen()) {
        overlay.close();
        return true;
      }
    }
    if (this.isAiDrawerOpen) { this.toggleAiDrawer(false); return true; }
    if (this.isNotesDrawerOpen) { this.isNotesDrawerOpen = false; return true; }
    if (this.isActivityDrawerOpen) { this.isActivityDrawerOpen = false; return true; }
    if (this.isAuditDrawerOpen) { this.isAuditDrawerOpen = false; return true; }
    return false;
  }

  // Filter Mode & Clause Management
  private tableFilterContext(): TableFilterContext {
    const self = this;
    return {
      get activeTab() { return self.activeTab; },
      get tabs() { return self.tabs; },
      get currentDatabase() { return self.currentDatabase; },
      get driver() { return self.activeConnection?.driver; },
      setTabs: (next) => { self.tabs = next; },
      saveTabsState: () => self.saveTabsState(),
      loadTableData: (tab) => self.loadTableData(tab),
      addTab: (title, sql) => self.addTab(title, sql),
    };
  }

  setFilterMode(mode: 'pills' | 'raw') {
    setFilterModeAction(this.tableFilterContext(), mode);
  }

  setFilterRaw(raw: string) {
    setFilterRawAction(this.tableFilterContext(), raw);
  }

  toggleFilterBar(tabId?: string) {
    toggleFilterBarAction(this.tableFilterContext(), tabId);
  }

  addFilterPill(col?: string, op: FilterPill['op'] = '=', val: string = '') {
    addFilterPillAction(this.tableFilterContext(), col, op, val);
  }

  updateFilterPill(index: number, patch: Partial<FilterPill>) {
    updateFilterPillAction(this.tableFilterContext(), index, patch);
  }

  removeFilterPill(index: number) {
    removeFilterPillAction(this.tableFilterContext(), index);
  }

  applyTableFilter() {
    applyTableFilterAction(this.tableFilterContext());
  }

  clearTableFilter() {
    clearTableFilterAction(this.tableFilterContext());
  }

  handoffTableQueryToEditor() {
    handoffTableQueryToEditorAction(this.tableFilterContext());
  }

  private persistHiddenColumns(tab: EditorTab) {
    persistHiddenColumnsStorage(tab, this.currentDatabase);
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
      const openFn = () => {
        this.safetyChallenge = {
          isOpen: true,
          sql,
          inspection,
          confirmed: false,
          challengeInput: '',
          resolve: (decision) => {
            this.safetyChallenge.isOpen = false;
            this.safetyChallenge.resolve = undefined;
            resolve(decision);
            const next = this.safetyChallengeQueue.shift();
            if (next) next();
          },
        };
      };
      if (!this.safetyChallenge.isOpen) {
        openFn();
      } else {
        this.safetyChallengeQueue.push(openFn);
      }
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
  private ddlOperationsContext(): DdlOperationsContext {
    const self = this;
    return {
      get activeSessionId() { return self.activeSessionId; },
      get isReadOnly() { return self.isReadOnly; },
      get currentDatabase() { return self.currentDatabase; },
      get availableDatabases() { return self.availableDatabases; },
      get driver() { return self.activeConnection?.driver; },
      get tabs() { return self.tabs; },
      showToast: (msg, type) => self.showToast(msg, type),
      promptSafetyChallenge: (sql, inspection) => self.promptSafetyChallenge(sql, inspection),
      openDDLReviewModal: (sql, onConfirm, title) => self.openDDLReviewModal(sql, onConfirm, title),
      executeDDL: (sql, database, connId) => self.executeDDL(sql, database, connId),
      closeTab: (tabId, force) => self.closeTab(tabId, force),
      loadTableData: (tab) => self.loadTableData(tab),
      refreshCatalog: (database) => self.refreshCatalog(database),
      refreshDatabases: () => self.refreshDatabases(),
      selectDatabase: (dbName) => self.selectDatabase(dbName),
      openTableDataTab: (tableName) => self.openTableDataTab(tableName),
      openDdlViewModal: (title, ddl) => self.openDdlViewModal(title, ddl),
      addTab: (title, sql) => self.addTab(title, sql),
    };
  }

  async dropTable(tableName: string, database?: string) {
    await dropTableAction(this.ddlOperationsContext(), tableName, database);
  }

  async truncateTable(tableName: string, database?: string): Promise<boolean> {
    return await truncateTableAction(this.ddlOperationsContext(), tableName, database);
  }

  async renameTable(oldName: string, newName: string, database?: string) {
    await renameTableAction(this.ddlOperationsContext(), oldName, newName, database);
  }

  async duplicateTable(sourceTable: string, targetTable: string, copyData = true, database?: string) {
    await duplicateTableAction(this.ddlOperationsContext(), sourceTable, targetTable, copyData, database);
  }

  async createDatabase(dbName: string, charset = '', collation = '') {
    await createDatabaseAction(this.ddlOperationsContext(), dbName, charset, collation);
  }

  async dropDatabase(dbName: string) {
    await dropDatabaseAction(this.ddlOperationsContext(), dbName);
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
    await toggleEventStatusAction(this.ddlOperationsContext(), event);
  }

  async openEventDDLTab(eventName: string) {
    await openEventDDLTabAction(this.ddlOperationsContext(), eventName);
  }

  async openRoutineCaller(routine: RoutineMetadata) {
    await openRoutineCallerAction(this.ddlOperationsContext(), routine);
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

    const effectiveDb = tab?.detectedTableDatabase || tab?.database || this.currentDatabase;
    const oldVal = result.rows[rowIdx][colIdx];
    if (newVal !== String(oldVal ?? '')) {
      const catalog = (effectiveDb && this.catalogs[effectiveDb.toLowerCase()]) || this.catalog;
      const table = catalog?.tables?.find((t) => t.name.toLowerCase() === tableName.toLowerCase());
      const getColName = (c: any) => (typeof c === 'string' ? c : c.name || '');
      const isColPK = (c: any) => {
        if (typeof c === 'object' && c.is_primary_key !== undefined) return Boolean(c.is_primary_key);
        const name = getColName(c);
        return Boolean(table?.columns?.find((cm) => cm.name.toLowerCase() === name.toLowerCase())?.is_primary_key);
      };

      const pkCols = result.columns.filter((c: any) => isColPK(c)).map((c: any) => getColName(c));
      let pkCol: string | undefined;
      let pkVal: any;
      let pkValues: Record<string, any> | undefined;

      if (pkCols.length === 1) {
        pkCol = pkCols[0];
        const pkIdx = result.columns.findIndex((c: any) => getColName(c) === pkCol);
        pkVal = pkIdx !== -1 ? result.rows[rowIdx][pkIdx] : undefined;
      } else if (pkCols.length > 1) {
        pkValues = {};
        for (const col of pkCols) {
          const idx = result.columns.findIndex((c: any) => getColName(c) === col);
          pkValues[col] = idx !== -1 ? result.rows[rowIdx][idx] : undefined;
        }
      }

      let rowCriteria: Record<string, any> | undefined;
      if (pkCols.length === 0) {
        rowCriteria = {};
        result.columns.forEach((c: any, idx: number) => {
          rowCriteria![getColName(c)] = result.rows[rowIdx][idx];
        });
      }

      this.stageEdit(tableName, rowIdx, colIdx, colName, pkCol, pkVal, oldVal, newVal, effectiveDb, rowCriteria, pkValues);
      result.rows[rowIdx][colIdx] = newVal;
      this.showToast(`Staged JSON update for '${colName}'`, 'info');
    }
  }
}

export const appState = new AppState();
