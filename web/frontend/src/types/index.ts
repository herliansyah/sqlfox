export type DatabaseDriver = 'mysql' | 'postgres' | 'sqlite';
export type Environment = 'Development' | 'Staging' | 'Production';
export type RiskLevel = 'NONE' | 'LOW' | 'MEDIUM' | 'WARNING' | 'HIGH' | 'DANGEROUS' | 'CRITICAL';
export type TableMode = 'data' | 'structure' | 'indexes' | 'foreign-keys' | 'ddl' | 'diagnostics' | 'partitions' | 'notes';

export interface ConnectionProfile {
  id: string;
  name: string;
  driver: DatabaseDriver;
  host?: string;
  port?: number;
  user?: string;
  password?: string;
  database?: string;
  database_name?: string;
  ssl_mode?: string;
  use_ssl?: boolean;
  environment: Environment;
  read_only?: boolean;
  color?: string;
  ssh_enabled?: boolean;
  ssh_host?: string;
  ssh_port?: number;
  ssh_user?: string;
  ssh_password?: string;
  ssh_key?: string;
  created_at?: string;
  updated_at?: string;
}

export type ColumnMetadata = ColumnDefinition;

export interface ColumnDefinition {
  name: string;
  type: string;
  is_nullable: boolean;
  is_primary_key: boolean;
  default_value?: string | null;
  comment?: string;
  extra?: string;
}

export interface ForeignKeyInfo {
  name?: string;
  constraint_name?: string;
  column?: string;
  column_name?: string;
  referenced_table: string;
  referenced_column: string;
  on_update?: string;
  on_delete?: string;
}

export interface IndexInfo {
  name: string;
  columns: string[];
  is_unique: boolean;
  is_primary?: boolean;
  type: string;
}

export interface TableMetadata {
  name: string;
  type: string; // 'BASE TABLE' | 'VIEW' | 'TABLE' | 'view'
  row_count?: number;
  rows_count?: number;
  data_length?: number;
  comment?: string;
  columns: ColumnDefinition[];
  primary_key?: string[];
  foreign_keys?: ForeignKeyInfo[];
  indexes?: IndexInfo[];
}

export interface TriggerMetadata {
  name: string;
  table: string;
  timing: string;
  event: string;
  statement: string;
}

export interface RoutineMetadata {
  name: string;
  type: string; // 'PROCEDURE' | 'FUNCTION'
  data_type?: string;
  definition?: string;
}

export interface EventMetadata {
  name: string;
  status: string;
  interval?: string;
  starts_at?: string;
  ends_at?: string;
  definition?: string;
}

export interface SchemaCatalog {
  database_name: string;
  databases?: string[];
  tables: TableMetadata[];
  foreign_keys?: ForeignKeyInfo[];
  views?: string[];
  routines?: RoutineMetadata[];
  triggers?: TriggerMetadata[];
  events?: EventMetadata[];
}

export interface QuerySnippet {
  id: string;
  title: string;
  description?: string;
  sql: string;
  tags: string[];
}

export interface StatementEstimate {
  statement_index: number;
  sql: string;
  operation: string;
  target_table: string;
  target_database?: string;
  has_where: boolean;
  count_query?: string;
  estimated_rows?: number | null;
  estimated_is_exact?: boolean;
  requires_challenge: boolean;
  reason?: string;
}

export interface InspectionResult {
  risk_level: RiskLevel;
  is_destructive?: boolean;
  requires_challenge?: boolean;
  requires_affirmation?: boolean;
  schema_scope_mismatch?: boolean;
  target_database?: string;
  target_table?: string;
  target_type?: string;
  operation?: string;
  reason?: string;
  suggestion?: string;
  has_where?: boolean;
  count_query?: string;
  estimated_rows?: number | null;
  statement_estimates?: StatementEstimate[];
}

export interface FilterPill {
  col: string;
  op: '=' | '!=' | 'LIKE' | 'NOT LIKE' | 'IS NULL' | 'IS NOT NULL' | '>' | '<' | '>=' | '<=' | 'IN' | 'NOT IN' | 'BETWEEN' | 'RAW';
  val: string;
}

export interface QueryProfileStage {
  stage: string;
  duration_us: number;
  percentage?: number;
}

export interface QueryProfile {
  duration_ms: number;
  stages: QueryProfileStage[];
}

export interface QueryExecutionResult {
  columns: (string | ColumnDefinition)[];
  rows: any[][];
  duration_ms: number;
  row_count: number;
  affected_rows?: number;
  error?: string;
  statement?: string;
  message?: string;
  is_destructive_blocked?: boolean;
  inspection?: InspectionResult;
  results?: QueryExecutionResult[];
  profile?: QueryProfile;
}

export interface StagedEdit {
  rIdx: number;
  cIdx: number;
  table: string;
  database?: string;
  colName: string;
  pkCol?: string;
  pkVal?: any;
  oldVal: any;
  newVal: any;
}

export interface ProcesslistThread {
  id: number;
  user: string;
  host: string;
  db: string;
  command: string;
  time: number;
  state: string;
  info: string;
}

export interface ProcessLockWait {
  waiting_thread_id: number;
  waiting_query?: string;
  waiting_user?: string;
  waiting_seconds?: number;
  blocking_thread_id: number;
  blocking_query?: string;
  blocking_user?: string;
  locked_resource?: string;
  lock_mode?: string;
  is_root_blocker?: boolean;
}

export interface TablePartition {
  name: string;
  method?: string;
  rows?: number;
  data_length?: number;
  index_length?: number;
  comment?: string;
}

export interface Toast {
  id: string;
  type: 'info' | 'success' | 'warning' | 'error';
  message: string;
}

export interface SessionAuditItem {
  id: string;
  timestamp: string;
  sql: string;
  duration_ms: number;
  row_count?: number;
  affected_rows?: number;
  error?: string;
  database?: string;
}

export interface NativeDialogState {
  isOpen: boolean;
  type: 'alert' | 'confirm' | 'prompt';
  title: string;
  message: string;
  defaultValue?: string;
  inputValue?: string;
  resolve?: (value: any) => void;
}

export interface DdlReviewState {
  isOpen: boolean;
  sql: string;
  title?: string;
  onConfirm?: () => Promise<void> | void;
}

export interface EditorTab {
  id: string;
  title: string;
  sql: string;
  database?: string;
  isTableViewer?: boolean;
  tableName?: string;
  tableMode?: TableMode;
  isProcesslist?: boolean;
  isFlightRecorder?: boolean;
  isErdCanvas?: boolean;
  isVariables?: boolean;
  activeResult?: QueryExecutionResult | null;
  isExecuting?: boolean;
  filterText?: string;
  filterMode?: 'pills' | 'raw';
  filterRaw?: string;
  appliedFilter?: string;
  filterPills?: FilterPill[];
  showFilterBar?: boolean;
  hiddenColumns?: string[];
  pageSize?: number;
  currentPage?: number;
  totalRows?: number;
  /** Exact COUNT(*) for the current filter, populated on demand by the pager. */
  exactRowCount?: number;
  /** True while an exact COUNT(*) round-trip is in flight. */
  isCountingExact?: boolean;
  sortCol?: string | null;
  sortDir?: 'ASC' | 'DESC' | null;
  explainResult?: any;
  multiResults?: QueryExecutionResult[];
  activeResultIndex?: number;
  streamBatchOffset?: number;
  hasMoreStream?: boolean;
  stagedDraftRows?: Record<string, any>[];
  selectedRowIndices?: number[];
  showQueryProfiler?: boolean;
}
