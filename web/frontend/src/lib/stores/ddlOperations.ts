import type { EditorTab, EventMetadata, InspectionResult, RoutineMetadata, Toast } from '../../types';
import { api } from '../api/client';
import { quoteSqlIdentifier } from '../utils/sqlStatements';
import {
  buildCreateDatabaseSQL,
  buildDropDatabaseSQL,
  buildDropTableSQL,
  buildDuplicateTableSQL,
  buildRenameTableSQL,
  buildTruncateTableSQL,
} from './ddlActions';

export interface DdlOperationsContext {
  readonly activeSessionId: string | null;
  readonly isReadOnly: boolean;
  readonly currentDatabase: string;
  readonly availableDatabases: string[];
  readonly driver?: string;
  readonly tabs: EditorTab[];
  showToast(msg: string, type?: Toast['type']): string;
  promptSafetyChallenge(
    sql: string,
    inspection: InspectionResult
  ): Promise<{ confirmed: boolean; challengeInput: string }>;
  openDDLReviewModal(sql: string, onConfirm: () => Promise<void>, title?: string): void;
  executeDDL(sql: string, database?: string, connId?: string): Promise<boolean>;
  closeTab(tabId: string, force?: boolean): Promise<void>;
  loadTableData(tab: EditorTab): Promise<void>;
  refreshCatalog(database?: string): Promise<void>;
  refreshDatabases(): Promise<void>;
  selectDatabase(dbName: string): Promise<void>;
  openTableDataTab(tableName: string): void;
  openDdlViewModal(title: string, ddl: string): void;
  addTab(title?: string, sql?: string): void;
}

export async function dropTableAction(
  ctx: DdlOperationsContext,
  tableName: string,
  database?: string
) {
  if (!ctx.activeSessionId) return;
  if (ctx.isReadOnly) {
    ctx.showToast('Read-only mode is active — DROP TABLE blocked', 'warning');
    return;
  }
  const targetDb =
    database ||
    ctx.tabs.find((t) => t.tableName?.toLowerCase() === tableName.toLowerCase())?.database ||
    ctx.currentDatabase ||
    '';
  const sql = buildDropTableSQL(tableName, ctx.driver);
  const decision = await ctx.promptSafetyChallenge(sql, {
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
    const res = await api.executeQuery(ctx.activeSessionId, sql, {
      confirmed: true,
      challengeInput: decision.challengeInput || tableName,
      database: targetDb,
    });
    if (res.error) {
      ctx.showToast(`Failed to drop table: ${res.error}`, 'error');
      return;
    }
    ctx.showToast(`Table '${tableName}' dropped successfully`, 'success');
    const openTab = ctx.tabs.find((t) => t.tableName?.toLowerCase() === tableName.toLowerCase());
    if (openTab) {
      await ctx.closeTab(openTab.id, true);
    }
    await ctx.refreshCatalog();
  } catch (err: any) {
    ctx.showToast(`Failed to drop table: ${err.message}`, 'error');
  }
}

export async function truncateTableAction(
  ctx: DdlOperationsContext,
  tableName: string,
  database?: string
): Promise<boolean> {
  if (!ctx.activeSessionId) return false;
  if (ctx.isReadOnly) {
    ctx.showToast('Read-only mode is active — TRUNCATE TABLE blocked', 'warning');
    return false;
  }
  const targetDb =
    database ||
    ctx.tabs.find((t) => t.tableName?.toLowerCase() === tableName.toLowerCase())?.database ||
    ctx.currentDatabase ||
    '';
  const sql = buildTruncateTableSQL(tableName, ctx.driver);
  const decision = await ctx.promptSafetyChallenge(sql, {
    risk_level: 'HIGH',
    requires_challenge: true,
    target_table: tableName,
    target_database: targetDb,
    operation: 'TRUNCATE',
    reason: `Destructive mutation: TRUNCATE TABLE will delete all records in table '${tableName}'.`,
    estimated_rows: null,
  });
  if (!decision.confirmed) return false;

  try {
    const res = await api.executeQuery(ctx.activeSessionId, sql, {
      confirmed: true,
      challengeInput: decision.challengeInput || tableName,
      database: targetDb,
    });
    if (res.error) {
      ctx.showToast(`Failed to truncate table: ${res.error}`, 'error');
      return false;
    }
    ctx.showToast(`Table '${tableName}' truncated`, 'success');
    const openTab = ctx.tabs.find((t) => t.tableName?.toLowerCase() === tableName.toLowerCase());
    if (openTab) {
      await ctx.loadTableData(openTab);
    }
    await ctx.refreshCatalog();
    return true;
  } catch (err: any) {
    ctx.showToast(`Failed to truncate table: ${err.message}`, 'error');
    return false;
  }
}

export async function renameTableAction(
  ctx: DdlOperationsContext,
  oldName: string,
  newName: string,
  database?: string
) {
  if (!ctx.activeSessionId) return;
  const targetDb =
    database ||
    ctx.tabs.find((t) => t.tableName?.toLowerCase() === oldName.toLowerCase())?.database ||
    ctx.currentDatabase ||
    '';
  const sql = buildRenameTableSQL(oldName, newName, ctx.driver);

  ctx.openDDLReviewModal(
    sql,
    async () => {
      if (!(await ctx.executeDDL(sql, targetDb))) return;
      ctx.showToast(`Table renamed to '${newName}'`, 'success');
      const openTab = ctx.tabs.find((t) => t.tableName?.toLowerCase() === oldName.toLowerCase());
      if (openTab) {
        openTab.tableName = newName;
        openTab.title = newName;
      }
      await ctx.refreshCatalog(targetDb);
    },
    `Rename Table: ${oldName} → ${newName}`
  );
}

export async function duplicateTableAction(
  ctx: DdlOperationsContext,
  sourceTable: string,
  targetTable: string,
  copyData = true,
  database?: string
) {
  if (!ctx.activeSessionId) return;
  const targetDb =
    database ||
    ctx.tabs.find((t) => t.tableName?.toLowerCase() === sourceTable.toLowerCase())?.database ||
    ctx.currentDatabase ||
    '';
  const sql = buildDuplicateTableSQL(sourceTable, targetTable, copyData, ctx.driver);

  ctx.openDDLReviewModal(
    sql,
    async () => {
      if (!(await ctx.executeDDL(sql, targetDb))) return;
      ctx.showToast(`Table '${sourceTable}' duplicated to '${targetTable}'`, 'success');
      await ctx.refreshCatalog(targetDb);
      ctx.openTableDataTab(targetTable);
    },
    `Duplicate Table: ${sourceTable} → ${targetTable}`
  );
}

export async function createDatabaseAction(
  ctx: DdlOperationsContext,
  dbName: string,
  charset = '',
  collation = ''
) {
  if (!ctx.activeSessionId) return;
  const sql = buildCreateDatabaseSQL(dbName, charset, collation, ctx.driver);

  ctx.openDDLReviewModal(
    sql,
    async () => {
      if (!(await ctx.executeDDL(sql))) return;
      ctx.showToast(`Database '${dbName}' created`, 'success');
      await ctx.refreshDatabases();
      await ctx.selectDatabase(dbName);
    },
    `Create Database: ${dbName}`
  );
}

export async function dropDatabaseAction(ctx: DdlOperationsContext, dbName: string) {
  if (!ctx.activeSessionId) return;
  const sql = buildDropDatabaseSQL(dbName, ctx.driver);
  const decision = await ctx.promptSafetyChallenge(sql, {
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
    const res = await api.executeQuery(ctx.activeSessionId, sql, {
      confirmed: true,
      challengeInput: decision.challengeInput || dbName,
    });
    if (res.error) throw new Error(res.error);
    ctx.showToast(`Database '${dbName}' dropped`, 'success');
    await ctx.refreshDatabases();
    if (ctx.currentDatabase === dbName && ctx.availableDatabases.length > 0) {
      await ctx.selectDatabase(ctx.availableDatabases[0]);
    }
  } catch (err: any) {
    ctx.showToast(`Failed to drop database: ${err.message}`, 'error');
  }
}

export async function toggleEventStatusAction(ctx: DdlOperationsContext, event: EventMetadata) {
  if (!ctx.activeSessionId) return;
  const currentlyEnabled = (event.status || '').toUpperCase() === 'ENABLED';
  const nextEnable = !currentlyEnabled;
  try {
    await api.toggleEvent(ctx.activeSessionId, event.name, nextEnable);
    ctx.showToast(`Event '${event.name}' ${nextEnable ? 'ENABLED' : 'DISABLED'}`, 'success');
    await ctx.refreshCatalog();
  } catch (err: any) {
    ctx.showToast(`Failed to toggle event: ${err.message}`, 'error');
  }
}

export async function openEventDDLTabAction(ctx: DdlOperationsContext, eventName: string) {
  if (!ctx.activeSessionId) return;
  try {
    const res = await api.getEventDDL(ctx.activeSessionId, eventName);
    ctx.openDdlViewModal(`EVENT: ${eventName}`, res?.ddl || `-- No DDL returned for ${eventName}`);
  } catch (err: any) {
    ctx.showToast(`Failed to load event DDL: ${err.message}`, 'error');
  }
}

export async function openRoutineCallerAction(ctx: DdlOperationsContext, routine: RoutineMetadata) {
  const rType = (routine.type || 'PROCEDURE').toUpperCase();
  let ddl = routine.definition || '';
  if (!ddl && ctx.activeSessionId) {
    try {
      const res = await api.getRoutineDDL(ctx.activeSessionId, routine.name, rType);
      ddl = res?.ddl || '';
    } catch {
      /* fallback to empty params */
    }
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
  const qName = quoteSqlIdentifier(routine.name, ctx.driver);
  let callSql =
    rType === 'FUNCTION'
      ? `SELECT ${qName}(${args.join(', ')}) AS result;`
      : `CALL ${qName}(${args.join(', ')});`;
  if (postSelects.length > 0 && rType !== 'FUNCTION') {
    callSql += `\nSELECT ${postSelects.join(', ')};`;
  }
  ctx.addTab(`Call ${routine.name}.sql`, callSql);
}
