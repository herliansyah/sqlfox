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
} from '../../types';
import { api } from '../api/client';
import { copyText } from '../utils/clipboard';
import { substituteSnippetVariables, parseSnippetVariables } from '../utils/snippets';

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
  resolve?: (value: boolean) => void;
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

  /**
   * Grid commit strategy. "staged" accumulates cell edits until the user hits
   * Apply; "auto" persists each edit as soon as the cell loses focus.
   */
  gridEditMode = $state<'staged' | 'auto'>(
    (typeof localStorage !== 'undefined' && localStorage.getItem('sqlfox_grid_edit_mode') === 'auto'
      ? 'auto'
      : 'staged') as 'staged' | 'auto'
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
  isAiDrawerOpen = $state<boolean>(false);
  /** Prefill text handed to the AI Copilot drawer by "Send to AI" actions. */
  aiPromptDraft = $state<string>('');
  isAiSettingsModalOpen = $state<boolean>(false);
  isNotesDrawerOpen = $state<boolean>(false);
  isNoteComposerModalOpen = $state<boolean>(false);
  editingNote = $state<any>(null);
  dueNotesCount = $state<number>(0);
  private notifiedReminderIds = new Set<string>();
  private reminderTimer: any = null;
  isActivityDrawerOpen = $state<boolean>(false);
  isUpdateModalOpen = $state<boolean>(false);
  serverLatency = $state<number | null>(null);
  serverMemoryMb = $state<number | null>(null);
  serverInfo = $state<{ version?: string; uptime_sec?: number; active_conns?: number } | null>(null);
  isPingingServer = $state<boolean>(false);
  appVersion = $state<string>('v0.18.0');
  isMockDataModalOpen = $state<boolean>(false);
  mockDataTableName = $state<string>('');
  isSettingsModalOpen = $state<boolean>(false);
  isReadOnly = $state<boolean>(false);
  isTransactionActive = $state<boolean>(false);
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
  isDataTransferOpen = $state<boolean>(false);
  isDataSyncOpen = $state<boolean>(false);
  isAuditDrawerOpen = $state<boolean>(false);
  // Which DBA sub-view to show next; ProcesslistView applies it on change.
  processlistSubTab = $state<string>('threads');
  auditLogs = $state<SessionAuditItem[]>([]);

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
  sidebarCollapsed = $state<boolean>(false);
  sidebarViewMode = $state<'objects' | 'databases'>('objects');
  activeSidebarTab = $state<'schema' | 'connections' | 'history'>('schema');
  schemaFilter = $state<string>('');
  collapsedSections = $state<Record<string, boolean>>({
    tables: false,
    views: false,
    routines: false,
    triggers: false,
  });

  // Remote Access Guard lock state
  isRemoteLocked = $state<boolean>(false);
  authStatus = $state<any>(null);

  // Derived properties
  activeTab = $derived(this.tabs.find((t) => t.id === this.activeTabId) || this.tabs[0]);
  isProduction = $derived(this.activeConnection?.environment === 'Production');
  isStaging = $derived(this.activeConnection?.environment === 'Staging');
  stagedCount = $derived(Object.keys(this.stagedEdits).length);

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

  filteredTriggers = $derived.by(() => {
    if (!this.catalog?.triggers) return [];
    const query = this.schemaFilter.toLowerCase().trim();
    if (!query) return this.catalog.triggers;
    return this.catalog.triggers.filter((t) => t.name.toLowerCase().includes(query));
  });

  filteredDatabases = $derived.by(() => {
    const list = this.availableDatabases || [];
    const query = this.schemaFilter.toLowerCase().trim();
    if (!query) return list;
    return list.filter((db) => db.toLowerCase().includes(query));
  });

  totalObjectsCount = $derived.by(() => {
    const tablesCount = this.catalog?.tables?.length || 0;
    const routinesCount = this.catalog?.routines?.length || 0;
    const triggersCount = this.catalog?.triggers?.length || 0;
    return tablesCount + routinesCount + triggersCount;
  });

  constructor() {
    if (typeof window !== 'undefined') {
      (window as any).__sqlfox_set_remote_locked = (val: boolean) => {
        this.isRemoteLocked = val;
      };
      (window as any).__sqlfox_trigger_reconnect = () => this.triggerAutoReconnect();

      // ponytail: beforeunload warning guards against inadvertent data loss while queries are running or staged edits exist
      window.addEventListener('beforeunload', (e) => {
        const hasRunningQuery = this.tabs.some((t) => t.isExecuting);
        const hasStagedEdits = Object.keys(this.stagedEdits).length > 0;
        if (hasRunningQuery || hasStagedEdits) {
          e.preventDefault();
          e.returnValue = 'You have unsaved staged edits or running database queries.';
          return e.returnValue;
        }
      });
    }
    this.init();
  }

  // Toast System
  showToast(message: string, type: 'info' | 'success' | 'warning' | 'error' = 'info') {
    const id = `toast-${Date.now()}-${Math.random()}`;
    this.toasts.push({ id, message, type });
    setTimeout(() => {
      this.removeToast(id);
    }, 4000);
  }

  removeToast(id: string) {
    const idx = this.toasts.findIndex((t) => t.id === id);
    if (idx !== -1) {
      this.toasts.splice(idx, 1);
    }
  }

  async init() {
    try {
      const auth = await api.getAuthStatus();
      this.authStatus = auth;
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
      if (typeof document !== 'undefined') {
        const serverShell = document.getElementById('remote-lock-modal');
        if (serverShell) serverShell.classList.add('hidden');
      }
      if (!this.isRemoteLocked) {
        await this.loadConnections();
        void this.loadSnippets();
        this.startNotesReminderPoller();
      }
    } catch (err) {
      console.warn('Initial connection load deferred:', err);
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
          this.showToast(`Reminder Due: ${note.title || 'Untitled Note'}`, 'warning');
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
        let keyInfo = null;
        try { keyInfo = await api.getMasterKey(); } catch (_) {}
        this.authStatus = { ...auth, ...(keyInfo || {}) };
      } catch (_) {}
      await this.loadConnections();
      this.startNotesReminderPoller();
      return true;
    }
    return false;
  }

  // Connections CRUD
  async loadConnections() {
    const list = await api.listConnections();
    this.connections = list || [];
    if (!this.activeConnection && this.connections.length > 0) {
      const savedId = localStorage.getItem('sqlfox_last_conn_id');
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
      if (!this.activeConnection || this.activeConnection.id === saved.id) {
        await this.selectConnection(saved);
      }
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

  /** Opens the AI Copilot drawer and hands it a ready-to-review prompt. */
  sendToAi(prompt: string, note = 'Context sent to AI Copilot') {
    if (!prompt || !prompt.trim()) return;
    this.aiPromptDraft = prompt.trim();
    this.isAiDrawerOpen = true;
    if (note) this.showToast(note, 'success');
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
      }
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

  saveQueryToNote(sqlText?: string, titleHint?: string) {
    const tab = this.activeTab;
    const sql = (sqlText ?? tab?.sql ?? '').trim();
    if (!sql) {
      this.showToast('Worksheet query is empty. Enter or run a SQL statement first.', 'warning');
      return;
    }
    const cleanTitle = (titleHint || tab?.title || 'Query Snapshot').replace(/\.sql$/i, '').trim() || 'Query Snapshot';
    this.editingNote = {
      title: cleanTitle,
      content: `### Query Snapshot\n\n\`\`\`sql\n${sql}\n\`\`\`\n`,
      category: 'Analysis',
      table_name: tab?.tableName || '',
      query_snapshot: sql,
      is_pinned: false,
    };
    this.isNoteComposerModalOpen = true;
  }

  // --- System lifecycle actions (surfaced from the command palette) ---

  toggleReadOnlyMode() {
    this.isReadOnly = !this.isReadOnly;
    this.showToast(
      this.isReadOnly
        ? 'Read-only mode enabled — mutating statements will be blocked'
        : 'Read-only mode disabled — writes are now allowed',
      this.isReadOnly ? 'warning' : 'success'
    );
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
    if (!confirm('Shut down the SqlFox daemon? Active background jobs and database connections will be closed.')) {
      return;
    }
    try {
      await api.shutdownDaemon();
      this.showToast('Daemon shutting down gracefully…', 'info');
    } catch {
      // A graceful shutdown closes the socket, so a network failure here usually
      // means the daemon already stopped — report it as success rather than an error.
      this.showToast('Server disconnected — the daemon has stopped', 'info');
    }
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
    if (catalog && database) this.catalogs = { ...this.catalogs, [database.toLowerCase()]: catalog };
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

  async refreshCatalog() {
    if (!this.activeSessionId) return;
    this.isLoadingCatalog = true;
    try {
      const schema = await api.getSchema(this.activeSessionId, this.currentDatabase);
      this.setCatalog(schema);
      if (schema.database_name) {
        this.currentDatabase = schema.database_name;
      }
      if (schema.databases && schema.databases.length > 0) {
        this.availableDatabases = schema.databases;
      }
      this.showToast('Schema refreshed', 'info');
    } catch (err: any) {
      console.error('Failed to load schema catalog:', err);
      this.showToast(`Failed to refresh schema: ${err.message}`, 'error');
    } finally {
      this.isLoadingCatalog = false;
    }
  }

  toggleSection(section: string) {
    this.collapsedSections[section] = !this.collapsedSections[section];
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
        pageSize: t.pageSize || 50,
        currentPage: t.currentPage || 1,
        filterPills: t.filterPills || [],
        filterMode: t.filterMode || 'pills',
        filterRaw: t.filterRaw || '',
        appliedFilter: t.appliedFilter || '',
        showFilterBar: Boolean(t.showFilterBar),
        hiddenColumns: t.hiddenColumns || [],
        isProcesslist: Boolean(t.isProcesslist),
        isErdCanvas: Boolean(t.isErdCanvas),
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
        this.tabs = parsed.tabs;
        this.activeTabId = parsed.activeTabId || parsed.tabs[0].id;
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
    const dbPrefix = currentDb ? `\`${currentDb}\`.` : '';
    const newTab: EditorTab = {
      id: newId,
      title: tableName,
      sql: `SELECT * FROM ${dbPrefix}\`${tableName}\` LIMIT 50;`,
      isTableViewer: true,
      tableName,
      database: currentDb,
      tableMode: initialMode,
      pageSize: 50,
      currentPage: 1,
      filterPills: [],
      filterMode: 'pills',
      filterRaw: '',
      appliedFilter: '',
      showFilterBar: false,
      hiddenColumns: [],
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
    const existing = this.tabs.find((t) => t.isErdCanvas);
    if (existing) {
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

  /**
   * Runs a one-off `SELECT COUNT(*)` for the active table viewer, honouring the
   * current filter. Pagination totals use the catalog's estimated row_count, so
   * this is the "accurate count" escape hatch when the estimate drifts.
   */
  async requestExactRowCount(targetTab?: EditorTab) {
    const tabId = targetTab?.id || this.activeTabId;
    const tab = this.tabs.find((t) => t.id === tabId) || targetTab || this.activeTab;
    if (!this.activeSessionId || !tab?.isTableViewer || !tab.tableName) return;
    if (tab.isCountingExact) return;

    // Quote per dialect so identifiers with reserved words or mixed case work.
    const driver = (this.activeConnection?.driver || '').toLowerCase();
    const quoted =
      driver === 'mysql' || driver === 'mariadb'
        ? '`' + tab.tableName.replace(/`/g, '``') + '`'
        : '"' + tab.tableName.replace(/"/g, '""') + '"';

    let filterClause = (tab.appliedFilter || '').trim();
    if (!filterClause && tab.filterPills && tab.filterPills.length > 0) {
      filterClause = compileFilterPills(tab.filterPills, this.activeConnection?.driver);
    }
    const sql = `SELECT COUNT(*) AS total FROM ${quoted}${filterClause ? ` WHERE (${filterClause})` : ''}`;

    tab.isCountingExact = true;
    this.tabs = [...this.tabs];
    try {
      const res = await api.executeQuery(
        this.activeSessionId,
        sql,
        true,
        tab.database || this.currentDatabase || ''
      );
      const raw = res?.rows?.[0]?.[0];
      const count = Number(raw);
      if (Number.isFinite(count)) {
        tab.exactRowCount = count;
        this.showToast(`Exact row count: ${count.toLocaleString()}`, 'success');
      } else {
        this.showToast('Could not determine an exact row count', 'warning');
      }
    } catch (err: any) {
      this.showToast(`Row count failed: ${err?.message ?? err}`, 'error');
    } finally {
      tab.isCountingExact = false;
      this.tabs = [...this.tabs];
    }
  }

  async loadTableData(targetTab?: EditorTab) {
    const tabId = targetTab?.id || this.activeTabId;
    const tab = this.tabs.find((t) => t.id === tabId) || targetTab || this.activeTab;
    if (!this.activeSessionId || !tab || !tab.tableName) return;
    tab.isExecuting = true;
    this.tabs = [...this.tabs];
    const startTime = performance.now();
    try {
      const limit = tab.pageSize || 50;
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
      if (res && res.duration_ms === undefined) {
        res.duration_ms = Math.round(performance.now() - startTime);
      }
      if (res && res.row_count === undefined && res.rows) {
        res.row_count = res.rows.length;
      }
      tab.activeResult = res;
    } catch (err: any) {
      tab.activeResult = {
        columns: [],
        rows: [],
        duration_ms: Math.round(performance.now() - startTime),
        row_count: 0,
        error: err.message || 'Error loading table data',
      };
    } finally {
      tab.isExecuting = false;
      this.tabs = [...this.tabs];
      this.saveTabsState();
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

  duplicateTab(tabId: string) {
    const tab = this.tabs.find((t) => t.id === tabId);
    if (!tab) return;
    const baseTitle = tab.title.replace(/^Copy(?: \(\d+\))? of /, '');
    let copyNum = 1;
    let nextTitle = `Copy of ${baseTitle}`;
    while (this.tabs.some((t) => t.title === nextTitle)) {
      copyNum++;
      nextTitle = `Copy (${copyNum}) of ${baseTitle}`;
    }
    const newTab = this.addTab(nextTitle, tab.sql);
    newTab.isTableViewer = tab.isTableViewer;
    newTab.tableName = tab.tableName;
    newTab.tableMode = tab.tableMode;
    newTab.database = tab.database;
    this.saveTabsState();
  }

  closeTab(tabId: string) {
    const idx = this.tabs.findIndex((t) => t.id === tabId);
    if (idx === -1) return;
    if (this.tabs.length <= 1) {
      // Closing last tab resets workspace to fresh Query 1.sql
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

  closeOtherTabs(tabId: string) {
    this.tabs = this.tabs.filter((t) => t.id === tabId);
    this.activeTabId = tabId;
    this.saveTabsState();
  }

  closeTabsToRight(tabId: string) {
    const idx = this.tabs.findIndex((t) => t.id === tabId);
    if (idx === -1 || idx === this.tabs.length - 1) return;
    this.tabs = this.tabs.slice(0, idx + 1);
    // Keep the anchor tab active; the previously active tab may have been closed.
    this.activeTabId = tabId;
    this.saveTabsState();
  }

  closeAllTabs() {
    // Delegate to closeTab's reset semantics so the workspace never ends up empty.
    const anchor = this.tabs[0];
    if (!anchor) return;
    this.closeOtherTabs(anchor.id);
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
    if (!this.activeSessionId) return;
    try {
      const res = await api.executeQuery(this.activeSessionId, `${command};`, false, this.activeTab?.database || this.currentDatabase);
      if (res.error) {
        this.showToast(`${command} failed: ${res.error}`, 'error');
        return;
      }
      this.isTransactionActive = command === 'START TRANSACTION';
      this.showToast(command === 'START TRANSACTION' ? 'Transaction started (Uncommitted)' : command === 'COMMIT' ? 'Transaction committed successfully' : 'Transaction rolled back', command === 'ROLLBACK' ? 'warning' : 'success');
      if (command === 'COMMIT') await this.refreshCatalog();
    } catch (err: any) {
      this.showToast(`${command} failed: ${err.message}`, 'error');
    }
  }

  async executeCurrentTab(sqlOverride?: string) {
    const tab = this.activeTab;
    if (!tab || !this.activeSessionId || tab.isExecuting) return;

    if (tab.isTableViewer && sqlOverride === undefined) {
      await this.loadTableData(tab);
      return;
    }

    const sql = sqlOverride ?? tab.sql;
    const database = tab.database || this.currentDatabase;
    if (!sql.trim()) return;
    tab.isExecuting = true;
    this.tabs = [...this.tabs];
    const startTime = performance.now();
    try {
      const res = await api.executeQuery(this.activeSessionId, sql, false, database);
      let finalRes = res;
      if (res.is_destructive_blocked && res.inspection) {
        const confirmed = await this.promptSafetyChallenge(sql, res.inspection);
        if (confirmed) {
          finalRes = await api.executeQuery(this.activeSessionId, sql, true, database);
        } else {
          return;
        }
      }

      tab.activeResult = finalRes;
      if (finalRes.results && finalRes.results.length > 1) {
        tab.multiResults = finalRes.results;
        tab.activeResultIndex = 0;
      } else {
        tab.multiResults = undefined;
        tab.activeResultIndex = undefined;
      }

      // Record to audit log
      this.auditLogs.unshift({
        id: 'audit-' + Date.now() + '-' + Math.random().toString(36).substring(2, 6),
        timestamp: new Date().toLocaleTimeString(),
        sql,
        duration_ms: finalRes.duration_ms || Math.round(performance.now() - startTime),
        row_count: finalRes.row_count,
        affected_rows: finalRes.affected_rows,
        database,
      });
      if (this.auditLogs.length > 200) {
        this.auditLogs.pop();
      }
    } catch (err: any) {
      const errMsg = err.message || 'Execution error';
      tab.activeResult = {
        columns: [],
        rows: [],
        duration_ms: Math.round(performance.now() - startTime),
        row_count: 0,
        error: errMsg,
      };
      this.auditLogs.unshift({
        id: 'audit-' + Date.now() + '-' + Math.random().toString(36).substring(2, 6),
        timestamp: new Date().toLocaleTimeString(),
        sql: tab.sql,
        duration_ms: Math.round(performance.now() - startTime),
        row_count: 0,
        error: errMsg,
        database,
      });
    } finally {
      tab.isExecuting = false;
      this.tabs = [...this.tabs];
    }
  }

  async executeStatementAtCursor(stmtSql: string) {
    return this.executeCurrentTab(stmtSql);
  }

  async pageStreamBatch(direction: 'next' | 'prev') {
    const tab = this.activeTab;
    if (!tab || !this.activeSessionId || tab.isTableViewer) return;
    const currentOffset = tab.streamBatchOffset || 0;
    const batchSize = 5000;
    let nextOffset = direction === 'next' ? currentOffset + batchSize : Math.max(0, currentOffset - batchSize);
    tab.streamBatchOffset = nextOffset;

    const baseSql = tab.sql.trim().replace(/;+\s*$/, '');
    const pagedSql = `SELECT * FROM (\n${baseSql}\n) AS _p LIMIT ${batchSize + 1} OFFSET ${nextOffset};`;
    tab.isExecuting = true;
    try {
      const res = await api.executeQuery(this.activeSessionId, pagedSql, true, tab.database || this.currentDatabase || '');
      const rows = res.rows || [];
      const hasMore = rows.length > batchSize;
      tab.hasMoreStream = hasMore;
      tab.activeResult = {
        ...res,
        rows: hasMore ? rows.slice(0, batchSize) : rows,
        row_count: Math.min(rows.length, batchSize),
      };
    } catch (err: any) {
      tab.streamBatchOffset = currentOffset;
      tab.hasMoreStream = true;
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
  openDDLReviewModal(sql: string, onConfirm?: () => Promise<void> | void, title = 'Pre-Execution DDL Review') {
    this.ddlReview = {
      isOpen: true,
      sql,
      title,
      onConfirm,
    };
  }

  closeDDLReviewModal() {
    this.ddlReview.isOpen = false;
  }

  // Network Error Boundary & Health Watcher
  triggerAutoReconnect() {
    if (this.isReconnecting) return;
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
        this.showToast('Reconnected to daemon', 'success');
        if (this.activeConnection) {
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
    const dbName = this.currentDatabase || '';
    let url = `/api/session/${this.activeSessionId}/export-dump?schema=true&data=true&table=${encodeURIComponent(tbl)}`;
    if (dbName) url += `&db=${encodeURIComponent(dbName)}`;
    window.location.href = url;
  }

  async cancelCurrentTab() {
    if (!this.activeSessionId) return;
    try {
      await api.cancelQuery(this.activeSessionId);
      this.showToast('Query execution cancelled', 'info');
    } catch (err: any) {
      this.showToast(`Failed to cancel query: ${err.message}`, 'error');
    }
  }

  async explainCurrentTab(sqlOverride?: string) {
    const tab = this.activeTab;
    const sql = sqlOverride ?? tab?.sql ?? '';
    const database = tab?.database || this.currentDatabase;
    if (!tab || !this.activeSessionId || !sql.trim() || tab.isExecuting) return;
    tab.isExecuting = true;
    try {
      const res = await api.explainQuery(this.activeSessionId, sql, database);
      tab.explainResult = res;
      this.showToast('Query plan generated', 'success');
    } catch (err: any) {
      this.showToast(`Explain failed: ${err.message}`, 'error');
    } finally {
      tab.isExecuting = false;
      this.tabs = [...this.tabs];
    }
  }

  async openImpactPreview() {
    const tab = this.activeTab;
    if (!tab || !this.activeSessionId) return;
    if (!tab.explainResult && tab.sql) {
      await this.explainCurrentTab();
    }
    this.impactPreviewData = tab.explainResult;
    this.isImpactPreviewOpen = true;
  }

  // Grid Staged Edits & CRUD
  stageEdit(table: string, rIdx: number, cIdx: number, colName: string, pkCol: string | undefined, pkVal: any, oldVal: any, newVal: any, database?: string) {
    const key = `${table}:${rIdx}:${cIdx}`;
    this.stagedEdits[key] = {
      rIdx,
      cIdx,
      table,
      database: database || this.currentDatabase,
      colName,
      pkCol,
      pkVal,
      oldVal,
      newVal,
    };
    // Auto mode persists immediately; staged mode waits for the Apply action.
    if (this.gridEditMode === 'auto') {
      void this.commitStagedEdits();
    }
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

  discardStagedEdits() {
    this.stagedEdits = {};
    this.showToast('Discarded all staged changes', 'info');
  }

  async commitStagedEdits() {
    if (!this.activeSessionId) return;
    const items = Object.values(this.stagedEdits);
    if (items.length === 0) return;

    try {
      const tableName = items[0].table;
      const edits = items.map((item) => ({
        database: item.database || this.currentDatabase,
        table: item.table,
        column: item.colName,
        value: item.newVal,
        pk_column: item.pkCol,
        pk_value: item.pkVal,
      }));

      const res = await api.cellEdit(this.activeSessionId, {
        database: this.currentDatabase,
        table: tableName,
        edits,
      });

      this.showToast(`Saved ${res.rows_affected ?? items.length} change(s) successfully`, 'success');
      this.stagedEdits = {};

      // Refresh data
      if (this.activeTab?.isTableViewer) {
        await this.loadTableData(this.activeTab);
      } else {
        await this.executeCurrentTab();
      }
    } catch (err: any) {
      this.showToast(`Failed to commit edits: ${err.message}`, 'error');
    }
  }

  async insertRow(data: Record<string, any>) {
    const tab = this.activeTab;
    if (!this.activeSessionId || !tab?.tableName) return;

    try {
      await api.insertRow(this.activeSessionId, {
        database: this.currentDatabase,
        table: tab.tableName,
        data,
      });
      this.showToast(`Row inserted into ${tab.tableName}`, 'success');
      this.isInsertRowModalOpen = false;
      await this.loadTableData(tab);
    } catch (err: any) {
      this.showToast(`Failed to insert row: ${err.message}`, 'error');
    }
  }

  async deleteRows(pkCol: string, pkValues: any[]) {
    const tab = this.activeTab;
    if (!this.activeSessionId || !tab?.tableName) return;

    try {
      await api.deleteRows(this.activeSessionId, {
        database: this.currentDatabase,
        table: tab.tableName,
        pk_column: pkCol,
        pk_values: pkValues,
      });
      this.showToast(`Deleted ${pkValues.length} row(s)`, 'success');
      await this.loadTableData(tab);
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

  setFilterBarVisible(visible: boolean, tabId?: string) {
    const tab = tabId ? this.tabs.find((t) => t.id === tabId) : this.activeTab;
    if (!tab) return;
    tab.showFilterBar = visible;
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

  clearFilterPills() {
    this.clearTableFilter();
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
    const targetTable = '`' + tab.tableName.replace(/`/g, '``') + '`';
    let sql = `SELECT * FROM ${targetTable}`;
    const filter = (tab.appliedFilter || '').trim();
    if (filter) {
      sql += `\nWHERE ${filter}`;
    }
    if (tab.sortCol) {
      const escSortCol = '`' + tab.sortCol.replace(/`/g, '``') + '`';
      sql += `\nORDER BY ${escSortCol} ${tab.sortDir || 'ASC'}`;
    }
    sql += `\nLIMIT 50;`;
    this.addTab(`query_${tab.tableName}.sql`, sql);
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
  }

  showAllColumns() {
    const tab = this.activeTab;
    if (!tab) return;
    tab.hiddenColumns = [];
  }

  hideAllColumns(columns: string[], pkCol?: string) {
    const tab = this.activeTab;
    if (!tab) return;
    tab.hiddenColumns = columns.filter((c) => c !== pkCol);
  }

  // Safety Challenge
  private promptSafetyChallenge(sql: string, inspection: InspectionResult): Promise<boolean> {
    return new Promise((resolve) => {
      this.safetyChallenge = {
        isOpen: true,
        sql,
        inspection,
        confirmed: false,
        resolve,
      };
    });
  }

  confirmSafetyChallenge() {
    if (this.safetyChallenge.resolve) {
      this.safetyChallenge.resolve(true);
    }
    this.safetyChallenge.isOpen = false;
  }

  cancelSafetyChallenge() {
    if (this.safetyChallenge.resolve) {
      this.safetyChallenge.resolve(false);
    }
    this.safetyChallenge.isOpen = false;
  }

  // Schema & Table Manipulation Operations
  async dropTable(tableName: string) {
    if (!this.activeSessionId) return;
    const isPg = this.activeConnection?.driver === 'postgres';
    const sql = isPg ? `DROP TABLE "${tableName}" CASCADE;` : `DROP TABLE IF EXISTS \`${tableName}\`;`;
    const confirmed = await this.promptSafetyChallenge(sql, {
      risk_level: 'CRITICAL',
      target_table: tableName,
      reason: `Destructive operation: DROP TABLE will permanently erase table '${tableName}' and all its data.`,
      estimated_rows: null,
    });
    if (!confirmed) return;

    try {
      const res = await api.executeQuery(this.activeSessionId, sql, true, this.currentDatabase || '');
      if (res.error) {
        this.showToast(`Failed to drop table: ${res.error}`, 'error');
        return;
      }
      this.showToast(`Table '${tableName}' dropped successfully`, 'success');
      const openTab = this.tabs.find((t) => t.tableName?.toLowerCase() === tableName.toLowerCase());
      if (openTab) {
        this.closeTab(openTab.id);
      }
      await this.refreshCatalog();
    } catch (err: any) {
      this.showToast(`Failed to drop table: ${err.message}`, 'error');
    }
  }

  async truncateTable(tableName: string) {
    if (!this.activeSessionId) return;
    const isPg = this.activeConnection?.driver === 'postgres';
    const sql = isPg ? `TRUNCATE TABLE "${tableName}" CASCADE;` : `TRUNCATE TABLE \`${tableName}\`;`;
    const confirmed = await this.promptSafetyChallenge(sql, {
      risk_level: 'HIGH',
      target_table: tableName,
      reason: `Destructive mutation: TRUNCATE TABLE will delete all records in table '${tableName}'.`,
      estimated_rows: null,
    });
    if (!confirmed) return;

    try {
      const res = await api.executeQuery(this.activeSessionId, sql, true, this.currentDatabase || '');
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

  async renameTable(oldName: string, newName: string) {
    if (!this.activeSessionId) return;
    const isPg = this.activeConnection?.driver === 'postgres';
    const sql = isPg
      ? `ALTER TABLE "${oldName}" RENAME TO "${newName}";`
      : `RENAME TABLE \`${oldName}\` TO \`${newName}\`;`;

    try {
      const res = await api.executeQuery(this.activeSessionId, sql, true, this.currentDatabase || '');
      if (res.error) {
        this.showToast(`Failed to rename table: ${res.error}`, 'error');
        return;
      }
      this.showToast(`Table renamed to '${newName}'`, 'success');
      const openTab = this.tabs.find((t) => t.tableName?.toLowerCase() === oldName.toLowerCase());
      if (openTab) {
        openTab.tableName = newName;
        openTab.title = newName;
      }
      await this.refreshCatalog();
    } catch (err: any) {
      this.showToast(`Failed to rename table: ${err.message}`, 'error');
    }
  }

  async duplicateTable(sourceTable: string, targetTable: string, copyData = true) {
    if (!this.activeSessionId) return;
    const driver = this.activeConnection?.driver;
    try {
      if (driver === 'postgres') {
        let sql = `CREATE TABLE "${targetTable}" (LIKE "${sourceTable}" INCLUDING ALL);`;
        if (copyData) {
          sql += `\nINSERT INTO "${targetTable}" SELECT * FROM "${sourceTable}";`;
        }
        const res = await api.executeQuery(this.activeSessionId, sql, true, this.currentDatabase || '');
        if (res.error) throw new Error(res.error);
      } else if (driver === 'sqlite') {
        const sql = copyData
          ? `CREATE TABLE "${targetTable}" AS SELECT * FROM "${sourceTable}";`
          : `CREATE TABLE "${targetTable}" AS SELECT * FROM "${sourceTable}" WHERE 0 = 1;`;
        const res = await api.executeQuery(this.activeSessionId, sql, true, this.currentDatabase || '');
        if (res.error) throw new Error(res.error);
      } else {
        let sql = `CREATE TABLE \`${targetTable}\` LIKE \`${sourceTable}\`;`;
        if (copyData) {
          sql += `\nINSERT INTO \`${targetTable}\` SELECT * FROM \`${sourceTable}\`;`;
        }
        const res = await api.executeQuery(this.activeSessionId, sql, true, this.currentDatabase || '');
        if (res.error) throw new Error(res.error);
      }
      this.showToast(`Table '${sourceTable}' duplicated to '${targetTable}'`, 'success');
      await this.refreshCatalog();
      this.openTableDataTab(targetTable);
    } catch (err: any) {
      this.showToast(`Failed to duplicate table: ${err.message}`, 'error');
    }
  }

  async createDatabase(dbName: string, charset = 'utf8mb4', collation = 'utf8mb4_unicode_ci') {
    if (!this.activeSessionId) return;
    const isPg = this.activeConnection?.driver === 'postgres';
    const sql = isPg
      ? `CREATE DATABASE "${dbName}";`
      : `CREATE DATABASE \`${dbName}\` CHARACTER SET ${charset} COLLATE ${collation};`;

    try {
      const res = await api.executeQuery(this.activeSessionId, sql, true);
      if (res.error) throw new Error(res.error);
      this.showToast(`Database '${dbName}' created`, 'success');
      await this.refreshDatabases();
      await this.selectDatabase(dbName);
    } catch (err: any) {
      this.showToast(`Failed to create database: ${err.message}`, 'error');
      throw err;
    }
  }

  async dropDatabase(dbName: string) {
    if (!this.activeSessionId) return;
    const isPg = this.activeConnection?.driver === 'postgres';
    const sql = isPg ? `DROP DATABASE "${dbName}";` : `DROP DATABASE IF EXISTS \`${dbName}\`;`;
    const confirmed = await this.promptSafetyChallenge(sql, {
      risk_level: 'CRITICAL',
      target_table: dbName,
      reason: `Destructive operation: DROP DATABASE will permanently erase database '${dbName}' and all tables inside it.`,
      estimated_rows: null,
    });
    if (!confirmed) return;

    try {
      const res = await api.executeQuery(this.activeSessionId, sql, true);
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

  async createTableDirect(ddl: string, tableName: string) {
    if (!this.activeSessionId) return;
    try {
      const res = await api.executeQuery(this.activeSessionId, ddl, true);
      if (res.error) throw new Error(res.error);
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
      isReadOnly,
    };
  }

  saveJsonCellValue(newVal: string) {
    const { tableName, rowIdx, colIdx, colName } = this.jsonCellModal;
    if (!tableName || rowIdx === undefined || colIdx === undefined) return;

    const tab = this.activeTab;
    const result = tab?.activeResult;
    if (!result?.rows || !result.rows[rowIdx]) return;

    const oldVal = result.rows[rowIdx][colIdx];
    if (newVal !== String(oldVal ?? '')) {
      const pkColObj = result.columns.find((c: any) => typeof c === 'object' && c.is_primary_key);
      const pkCol = pkColObj ? (typeof pkColObj === 'string' ? pkColObj : pkColObj.name) : undefined;
      const pkVal = pkCol ? result.rows[rowIdx][result.columns.findIndex((c: any) => (typeof c === 'string' ? c : c.name) === pkCol)] : undefined;

      this.stageEdit(tableName, rowIdx, colIdx, colName, pkCol, pkVal, oldVal, newVal);
      result.rows[rowIdx][colIdx] = newVal;
      this.showToast(`Staged JSON update for '${colName}'`, 'info');
    }
  }
}

export const appState = new AppState();
