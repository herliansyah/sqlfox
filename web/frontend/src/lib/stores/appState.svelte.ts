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
  SessionAuditItem,
} from '../../types';
import { api } from '../api/client';

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
  catalog = $state<SchemaCatalog | null>(null);
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
  isAiSettingsModalOpen = $state<boolean>(false);
  isNotesDrawerOpen = $state<boolean>(false);
  isNoteComposerModalOpen = $state<boolean>(false);
  editingNote = $state<any>(null);
  isActivityDrawerOpen = $state<boolean>(false);
  isUpdateModalOpen = $state<boolean>(false);
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
      if (auth.is_remote && !auth.unlocked) {
        this.isRemoteLocked = true;
        return;
      }
      this.isRemoteLocked = false;
      await this.loadConnections();
    } catch (err) {
      console.warn('Initial connection load deferred:', err);
    }
  }

  async unlockRemote(password: string) {
    const res = await api.unlockVault(password);
    if (res && res.token) {
      sessionStorage.setItem('sqlfox_remote_token', res.token);
      document.cookie = 'sqlfox_token=' + encodeURIComponent(res.token) + '; path=/; SameSite=Lax';
      this.isRemoteLocked = false;
      await this.loadConnections();
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

  async deleteConnection(id: string) {
    try {
      await api.deleteConnection(id);
      this.showToast('Connection deleted', 'info');
      if (this.activeConnection?.id === id) {
        this.activeConnection = null;
        this.activeSessionId = null;
        this.catalog = null;
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
      this.activeConnection = conn;
      localStorage.setItem('sqlfox_last_conn_id', conn.id);
      const sess = await api.connectSession(conn.id);
      this.activeSessionId = sess.id || sess.connection_id || conn.id;

      let dbs = (sess.schema && sess.schema.databases) || [];
      if (!dbs || dbs.length === 0) {
        try {
          const fresh = await api.getSchema(this.activeSessionId, '');
          if (fresh && fresh.databases && fresh.databases.length > 0) {
            dbs = fresh.databases;
            if (fresh.database_name) {
              this.currentDatabase = fresh.database_name;
            }
            this.catalog = fresh;
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
      if (schema.tables && schema.tables.length > 0) {
        this.catalog = schema;
      }
    } catch (err) {
      console.error('Failed to reload databases:', err);
    } finally {
      this.isLoadingCatalog = false;
    }
  }

  async loadFastCatalogBackground(sessionId: string) {
    try {
      const fastCat = await api.getFastCatalog(sessionId);
      if (fastCat && typeof fastCat === 'object') {
        const counts: Record<string, number> = {};
        for (const [db, tbls] of Object.entries(fastCat)) {
          counts[db] = Array.isArray(tbls) ? tbls.length : 0;
        }
        this.databaseTableCounts = counts;
      }
    } catch (err) {
      console.warn('Fast catalog background load failed:', err);
    }
  }

  async selectDatabase(dbName: string) {
    if (!this.activeSessionId || this.currentDatabase === dbName) return;
    this.isLoadingCatalog = true;
    try {
      const schema = await api.selectDatabase(this.activeSessionId, dbName);
      if (schema && (schema.tables || schema.databases)) {
        this.catalog = schema;
        this.currentDatabase = schema.database_name || dbName;
        if (schema.databases && schema.databases.length > 0) {
          this.availableDatabases = schema.databases;
        }
      } else {
        const fetched = await api.getSchema(this.activeSessionId, dbName);
        this.catalog = fetched;
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

  async refreshCatalog() {
    if (!this.activeSessionId) return;
    this.isLoadingCatalog = true;
    try {
      const schema = await api.getSchema(this.activeSessionId, this.currentDatabase);
      this.catalog = schema;
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

  // Tabs Management
  addTab(title = 'New Query.sql', sql = '') {
    const newId = `tab-${Date.now()}`;
    const newTab: EditorTab = {
      id: newId,
      title,
      sql,
      isTableViewer: false,
    };
    this.tabs.push(newTab);
    this.activeTabId = newId;
    return newTab;
  }

  openTableDataTab(tableName: string, initialMode: TableMode = 'data') {
    const existing = this.tabs.find((t) => t.isTableViewer && t.tableName === tableName);
    if (existing) {
      existing.tableMode = initialMode;
      this.activeTabId = existing.id;
      if (initialMode === 'data' && (!existing.activeResult || !existing.activeResult.rows)) {
        this.loadTableData(existing);
      }
      return;
    }
    const newId = `tab-tbl-${Date.now()}`;
    const dbPrefix = this.currentDatabase ? `\`${this.currentDatabase}\`.` : '';
    const newTab: EditorTab = {
      id: newId,
      title: tableName,
      sql: `SELECT * FROM ${dbPrefix}\`${tableName}\` LIMIT 100;`,
      isTableViewer: true,
      tableName,
      tableMode: initialMode,
      pageSize: 100,
      currentPage: 1,
      filterPills: [],
      hiddenColumns: [],
    };
    this.tabs.push(newTab);
    this.activeTabId = newId;
    if (initialMode === 'data') {
      this.loadTableData(newTab);
    }
  }

  openTableStructureTab(tableName: string) {
    this.openTableDataTab(tableName, 'structure');
  }

  openProcesslistTab() {
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
  }

  setTableMode(tabId: string, mode: TableMode) {
    const tab = this.tabs.find((t) => t.id === tabId);
    if (tab) {
      tab.tableMode = mode;
      if (mode === 'data' && (!tab.activeResult || !tab.activeResult.rows?.length)) {
        this.loadTableData(tab);
      }
    }
  }

  async loadTableData(tab: EditorTab) {
    if (!this.activeSessionId || !tab.tableName) return;
    tab.isExecuting = true;
    try {
      const limit = tab.pageSize || 100;
      const offset = ((tab.currentPage || 1) - 1) * limit;

      // If filter pills are present, construct filtered query
      let query = '';
      const dbPrefix = this.currentDatabase ? `\`${this.currentDatabase}\`.` : '';
      if (tab.filterPills && tab.filterPills.length > 0) {
        const whereClauses = tab.filterPills.map((p) => {
          if (p.op === 'IS NULL' || p.op === 'IS NOT NULL') {
            return `\`${p.col}\` ${p.op}`;
          }
          const escapedVal = p.val.replace(/'/g, "''");
          if (p.op === 'LIKE' || p.op === 'NOT LIKE') {
            return `\`${p.col}\` ${p.op} '%${escapedVal}%'`;
          }
          if (p.op === 'IN' || p.op === 'NOT IN') {
            return `\`${p.col}\` ${p.op} (${p.val})`;
          }
          if (p.op === 'BETWEEN') {
            return `\`${p.col}\` BETWEEN ${p.val}`;
          }
          // numeric check
          const isNum = !isNaN(Number(p.val)) && p.val.trim() !== '';
          return isNum ? `\`${p.col}\` ${p.op} ${p.val}` : `\`${p.col}\` ${p.op} '${escapedVal}'`;
        });
        query = `SELECT * FROM ${dbPrefix}\`${tab.tableName}\` WHERE ${whereClauses.join(' AND ')} LIMIT ${limit} OFFSET ${offset};`;
        const res = await api.executeQuery(this.activeSessionId, query, false);
        tab.activeResult = res;
      } else {
        const res = await api.getTableRows(this.activeSessionId, tab.tableName, limit, offset);
        tab.activeResult = res;
      }
    } catch (err: any) {
      tab.activeResult = {
        columns: [],
        rows: [],
        duration_ms: 0,
        row_count: 0,
        error: err.message || 'Error loading table data',
      };
    } finally {
      tab.isExecuting = false;
    }
  }

  async openTableDDLTab(tableName: string) {
    const existing = this.tabs.find((t) => t.title === `${tableName}.ddl.sql`);
    if (existing) {
      this.activeTabId = existing.id;
      return;
    }
    const newTab = this.addTab(`${tableName}.ddl.sql`, `-- Fetching CREATE TABLE for ${tableName}...`);
    if (this.activeSessionId) {
      try {
        const res = await api.getTableDDL(this.activeSessionId, tableName);
        if (res && res.ddl) {
          newTab.sql = res.ddl;
        }
      } catch (err: any) {
        newTab.sql = `-- Error fetching DDL: ${err.message || err}`;
      }
    }
  }

  async openRoutineTab(routineName: string, routineType: 'PROCEDURE' | 'FUNCTION' = 'PROCEDURE') {
    const existing = this.tabs.find((t) => t.title === `${routineName}.sql`);
    if (existing) {
      this.activeTabId = existing.id;
      return;
    }
    const newTab = this.addTab(`${routineName}.sql`, `-- Fetching definition for ${routineType} ${routineName}...`);
    if (this.activeSessionId) {
      try {
        const res = await api.getRoutineDDL(this.activeSessionId, routineName, routineType);
        if (res && res.ddl) {
          newTab.sql = res.ddl;
        }
      } catch (err: any) {
        newTab.sql = `-- Error fetching routine definition: ${err.message || err}`;
      }
    }
  }

  duplicateTab(tabId: string) {
    const tab = this.tabs.find((t) => t.id === tabId);
    if (!tab) return;
    const newTab = this.addTab(`Copy of ${tab.title}`, tab.sql);
    newTab.isTableViewer = tab.isTableViewer;
    newTab.tableName = tab.tableName;
    newTab.tableMode = tab.tableMode;
  }

  closeTab(tabId: string) {
    if (this.tabs.length <= 1) return;
    const idx = this.tabs.findIndex((t) => t.id === tabId);
    if (idx !== -1) {
      this.tabs.splice(idx, 1);
      if (this.activeTabId === tabId) {
        this.activeTabId = this.tabs[Math.max(0, idx - 1)].id;
      }
    }
  }

  closeOtherTabs(tabId: string) {
    this.tabs = this.tabs.filter((t) => t.id === tabId);
    this.activeTabId = tabId;
  }

  // Execution
  async executeCurrentTab() {
    const tab = this.activeTab;
    if (!tab || !this.activeSessionId) return;

    if (tab.isTableViewer) {
      await this.loadTableData(tab);
      return;
    }

    tab.isExecuting = true;
    const startTime = performance.now();
    try {
      const res = await api.executeQuery(this.activeSessionId, tab.sql, false);
      let finalRes = res;
      if (res.is_destructive_blocked && res.inspection) {
        const confirmed = await this.promptSafetyChallenge(tab.sql, res.inspection);
        if (confirmed) {
          finalRes = await api.executeQuery(this.activeSessionId, tab.sql, true);
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
        sql: tab.sql,
        duration_ms: finalRes.duration_ms || Math.round(performance.now() - startTime),
        row_count: finalRes.row_count,
        affected_rows: finalRes.affected_rows,
        database: tab.databaseScope || this.currentDatabase,
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
        database: tab.databaseScope || this.currentDatabase,
      });
    } finally {
      tab.isExecuting = false;
    }
  }

  async executeStatementAtCursor(stmtSql: string) {
    const tab = this.activeTab;
    if (!tab || !this.activeSessionId || !stmtSql.trim()) return;

    tab.isExecuting = true;
    const startTime = performance.now();
    try {
      const res = await api.executeQuery(this.activeSessionId, stmtSql, false);
      let finalRes = res;
      if (res.is_destructive_blocked && res.inspection) {
        const confirmed = await this.promptSafetyChallenge(stmtSql, res.inspection);
        if (confirmed) {
          finalRes = await api.executeQuery(this.activeSessionId, stmtSql, true);
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

      this.auditLogs.unshift({
        id: 'audit-' + Date.now() + '-' + Math.random().toString(36).substring(2, 6),
        timestamp: new Date().toLocaleTimeString(),
        sql: stmtSql,
        duration_ms: finalRes.duration_ms || Math.round(performance.now() - startTime),
        row_count: finalRes.row_count,
        affected_rows: finalRes.affected_rows,
        database: tab.databaseScope || this.currentDatabase,
      });
    } catch (err: any) {
      const errMsg = err.message || 'Execution error';
      tab.activeResult = {
        columns: [],
        rows: [],
        duration_ms: Math.round(performance.now() - startTime),
        row_count: 0,
        error: errMsg,
      };
    } finally {
      tab.isExecuting = false;
    }
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
      const res = await api.executeQuery(this.activeSessionId, pagedSql, true);
      const rows = res.rows || [];
      const hasMore = rows.length > batchSize;
      tab.hasMoreStream = hasMore;
      tab.activeResult = {
        ...res,
        rows: hasMore ? rows.slice(0, batchSize) : rows,
        row_count: Math.min(rows.length, batchSize),
      };
    } catch (err: any) {
      this.showToast(`Batch paging failed: ${err.message}`, 'error');
    } finally {
      tab.isExecuting = false;
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
  async openFkPeek(event: MouseEvent, table: string, col: string, refTable: string, refCol: string, val: any) {
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
      const currentDb = this.currentDatabase || '';
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

  async explainCurrentTab() {
    const tab = this.activeTab;
    if (!tab || !this.activeSessionId || !tab.sql) return;
    tab.isExecuting = true;
    try {
      const res = await api.explainQuery(this.activeSessionId, tab.sql);
      tab.explainResult = res;
      this.showToast('Query plan generated', 'success');
    } catch (err: any) {
      this.showToast(`Explain failed: ${err.message}`, 'error');
    } finally {
      tab.isExecuting = false;
    }
  }

  // Grid Staged Edits & CRUD
  stageEdit(table: string, rIdx: number, cIdx: number, colName: string, pkCol: string | undefined, pkVal: any, oldVal: any, newVal: any) {
    const key = `${table}:${rIdx}:${cIdx}`;
    this.stagedEdits[key] = {
      rIdx,
      cIdx,
      table,
      colName,
      pkCol,
      pkVal,
      oldVal,
      newVal,
    };
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
        database: this.currentDatabase,
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

  // Filter Pills & Column Picker
  addFilterPill(col: string, op: FilterPill['op'], val: string) {
    const tab = this.activeTab;
    if (!tab) return;
    if (!tab.filterPills) tab.filterPills = [];
    tab.filterPills.push({ col, op, val });
    if (tab.isTableViewer) {
      this.loadTableData(tab);
    }
  }

  removeFilterPill(index: number) {
    const tab = this.activeTab;
    if (!tab || !tab.filterPills) return;
    tab.filterPills.splice(index, 1);
    if (tab.isTableViewer) {
      this.loadTableData(tab);
    }
  }

  clearFilterPills() {
    const tab = this.activeTab;
    if (!tab) return;
    tab.filterPills = [];
    if (tab.isTableViewer) {
      this.loadTableData(tab);
    }
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
      const res = await api.executeQuery(this.activeSessionId, sql, true);
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
      const res = await api.executeQuery(this.activeSessionId, sql, true);
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
      const res = await api.executeQuery(this.activeSessionId, sql, true);
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
        const res = await api.executeQuery(this.activeSessionId, sql, true);
        if (res.error) throw new Error(res.error);
      } else if (driver === 'sqlite') {
        const sql = copyData
          ? `CREATE TABLE "${targetTable}" AS SELECT * FROM "${sourceTable}";`
          : `CREATE TABLE "${targetTable}" AS SELECT * FROM "${sourceTable}" WHERE 0 = 1;`;
        const res = await api.executeQuery(this.activeSessionId, sql, true);
        if (res.error) throw new Error(res.error);
      } else {
        let sql = `CREATE TABLE \`${targetTable}\` LIKE \`${sourceTable}\`;`;
        if (copyData) {
          sql += `\nINSERT INTO \`${targetTable}\` SELECT * FROM \`${sourceTable}\`;`;
        }
        const res = await api.executeQuery(this.activeSessionId, sql, true);
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
