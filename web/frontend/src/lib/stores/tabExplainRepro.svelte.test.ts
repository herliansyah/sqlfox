import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, waitFor, cleanup } from '@testing-library/svelte';
import { appState } from './appState.svelte';
import { api } from '../api/client';
import TabBar from '../components/editor/TabBar.svelte';
import SqlEditor from '../components/editor/SqlEditor.svelte';

describe('Tab add & explain reactivity reproduction', () => {
  beforeEach(() => {
    (globalThis as any).ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    appState.tabs = [
      { id: 'tab-1', title: 'Query 1.sql', sql: 'SELECT 1;', isTableViewer: false },
    ];
    appState.activeTabId = 'tab-1';
    appState.activeSessionId = 'sess-1';
    appState.currentDatabase = 'testdb';
    appState.activeConnection = {
      id: 'conn-1',
      name: 'Test',
      driver: 'sqlite',
      database_name: ':memory:',
      environment: 'Development',
    } as any;
    vi.restoreAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('addTab immediately renders the new tab in TabBar without reload', async () => {
    const { container } = render(TabBar);
    expect(container.querySelectorAll('[data-tab-id]').length).toBe(1);

    appState.addTab();

    await waitFor(() => {
      expect(container.querySelectorAll('[data-tab-id]').length).toBe(2);
    });
  });

  it('explainCurrentTab stops TabBar and SqlEditor spinners when explain resolves', async () => {
    let resolveExplain!: (v: any) => void;
    vi.spyOn(api, 'explainQuery').mockImplementation(
      () =>
        new Promise((resolve) => {
          resolveExplain = resolve;
        })
    );

    const { container } = render(TabBar);
    const editorView = render(SqlEditor);

    const explainPromise = appState.explainCurrentTab('SELECT 1;', false);

    await waitFor(() => {
      expect(container.querySelectorAll('.animate-spin').length).toBe(1);
      expect(editorView.container.querySelectorAll('.animate-spin').length).toBe(2);
    });

    resolveExplain({ plan: [], columns: [], rows: [] });
    await explainPromise;

    await waitFor(() => {
      const tab = appState.tabs.find((t) => t.id === 'tab-1');
      expect(tab?.isExplaining).toBe(false);
      expect(tab?.isExecuting).toBe(false);
      expect(container.querySelectorAll('.animate-spin').length).toBe(0);
      expect(editorView.container.querySelectorAll('.animate-spin').length).toBe(0);
    });
  });
});
