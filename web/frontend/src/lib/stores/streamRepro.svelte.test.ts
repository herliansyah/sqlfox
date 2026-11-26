import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, waitFor, cleanup } from '@testing-library/svelte';
import { appState } from './appState.svelte';
import { api } from '../api/client';
import VirtualGrid from '../components/grid/VirtualGrid.svelte';

describe('Stream 872 rows stuck Streaming banner repro', () => {
  beforeEach(() => {
    (globalThis as any).ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    appState.tabs = [
      { id: 'tab-1', title: 'Query 1.sql', sql: 'SELECT * FROM wide;', isTableViewer: false },
    ];
    appState.activeTabId = 'tab-1';
    appState.activeSessionId = 'sess-1';
    appState.currentDatabase = 'testdb';
    appState.activeConnection = {
      id: 'conn-1',
      name: 'Test',
      driver: 'mysql',
      database_name: 'testdb',
      environment: 'Development',
    } as any;
    vi.restoreAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('clears Streaming rows banner and updates row count to 872 after mid-stream render', async () => {
    const cols = Array.from({ length: 69 }, (_, i) => ({ name: `col_${i}`, type: 'VARCHAR(64)' }));
    const makeRows = (start: number, count: number) =>
      Array.from({ length: count }, (_, r) =>
        Array.from({ length: 69 }, (_, c) => `val_${start + r}_${c}`)
      );

    const enc = new TextEncoder();
    let streamCtrl!: ReadableStreamDefaultController<Uint8Array>;

    vi.spyOn(api, 'rawFetch').mockImplementation(async () => {
      const stream = new ReadableStream<Uint8Array>({
        start(controller) {
          streamCtrl = controller;
        },
      });
      return new Response(stream, {
        status: 200,
        headers: { 'Content-Type': 'application/x-ndjson' },
      });
    });

    const { container } = render(VirtualGrid);
    const execPromise = appState.executeCurrentTab('SELECT * FROM wide;');

    await waitFor(() => expect(streamCtrl).toBeDefined());

    // Emit meta + 20 + 200 (220 rows)
    streamCtrl.enqueue(enc.encode(JSON.stringify({ type: 'meta', columns: cols, tx_active: false, database: 'testdb' }) + '\n'));
    streamCtrl.enqueue(enc.encode(JSON.stringify({ type: 'chunk', rows: makeRows(0, 20) }) + '\n'));
    streamCtrl.enqueue(enc.encode(JSON.stringify({ type: 'chunk', rows: makeRows(20, 200) }) + '\n'));

    // Wait until VirtualGrid has rendered the mid-stream "Streaming rows (220 loaded)..." banner
    await waitFor(() => {
      expect(container.textContent).toContain('Streaming rows');
    });

    // Now emit remaining chunks + done 872 + close
    streamCtrl.enqueue(enc.encode(JSON.stringify({ type: 'chunk', rows: makeRows(220, 200) }) + '\n'));
    streamCtrl.enqueue(enc.encode(JSON.stringify({ type: 'chunk', rows: makeRows(420, 200) }) + '\n'));
    streamCtrl.enqueue(enc.encode(JSON.stringify({ type: 'chunk', rows: makeRows(620, 200) }) + '\n'));
    streamCtrl.enqueue(enc.encode(JSON.stringify({ type: 'chunk', rows: makeRows(820, 52) }) + '\n'));
    streamCtrl.enqueue(enc.encode(JSON.stringify({ type: 'done', row_count: 872, execution_ms: 35, has_more: false, tx_active: false }) + '\n'));
    streamCtrl.close();

    await execPromise;

    await waitFor(() => {
      expect(appState.activeTab?.isExecuting).toBe(false);
      expect(appState.activeTab?.activeResult?.rows?.length).toBe(872);
      expect(container.textContent).not.toContain('Streaming rows');
    });
  });
});
