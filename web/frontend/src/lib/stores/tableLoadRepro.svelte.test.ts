import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/svelte';
import { appState } from './appState.svelte';
import { api } from '../api/client';
import TabBar from '../components/editor/TabBar.svelte';
import TableViewer from '../components/table/TableViewer.svelte';

describe('Table load state reproduction', () => {
  beforeEach(() => {
    (globalThis as any).ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    appState.tabs = [];
    appState.activeTabId = '';
    appState.activeSessionId = 'sess-1';
    appState.currentDatabase = 'testdb';
    appState.activeConnection = {
      id: 'conn-1',
      name: 'Test',
      driver: 'mysql',
      host: '127.0.0.1',
      port: 3306,
      database: 'testdb',
      username: 'root',
      environment: 'Development',
    } as any;
    vi.restoreAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('single table_a updates VirtualGrid and stops TabBar spinner without switching tabs', async () => {
    vi.spyOn(api, 'getTableRows').mockImplementation(async (_sid, tableName) => {
      await new Promise((r) => setTimeout(r, 10));
      return {
        columns: ['id', 'name'],
        rows: [[1, `${tableName}-row`]],
        row_count: 1,
        total_rows: 1,
        duration_ms: 5,
      } as any;
    });

    const { container } = render(TabBar);
    render(TableViewer);

    appState.openTableDataTab('table_a');

    await waitFor(() => {
      expect(screen.getByText(/Executing query on testdb/)).toBeTruthy();
    });

    await waitFor(() => {
      const tabA = appState.tabs.find((t) => t.tableName === 'table_a');
      expect(tabA?.isExecuting).toBe(false);
      expect(screen.getByText('table_a-row')).toBeTruthy();
      expect(container.querySelectorAll('.animate-spin').length).toBe(0);
    });
  });

  it('clears background tab spinner when switching to table_b while table_a is loading', async () => {
    let resolveA!: (v: any) => void;
    const promiseA = new Promise((r) => {
      resolveA = r;
    });

    vi.spyOn(api, 'getTableRows').mockImplementation(async (_sid, tableName) => {
      if (tableName === 'table_a') {
        return promiseA as any;
      }
      return {
        columns: ['id', 'name'],
        rows: [[1, `${tableName}-row`]],
        row_count: 1,
        total_rows: 1,
        duration_ms: 5,
      } as any;
    });

    const { container } = render(TabBar);
    render(TableViewer);

    appState.openTableDataTab('table_a');
    await waitFor(() => {
      expect(screen.getByText(/Executing query on testdb/)).toBeTruthy();
    });

    appState.openTableDataTab('table_b');
    await waitFor(() => {
      expect(screen.getByText('table_b-row')).toBeTruthy();
    });

    resolveA({
      columns: ['id', 'name'],
      rows: [[1, 'table_a-row']],
      row_count: 1,
      total_rows: 1,
      duration_ms: 5,
    });

    await waitFor(() => {
      const tabA = appState.tabs.find((t) => t.tableName === 'table_a');
      expect(tabA?.isExecuting).toBe(false);
      expect(container.querySelectorAll('.animate-spin').length).toBe(0);
    });

    appState.openTableDataTab('table_a');
    await waitFor(() => {
      expect(screen.getByText('table_a-row')).toBeTruthy();
    });
  });
});
