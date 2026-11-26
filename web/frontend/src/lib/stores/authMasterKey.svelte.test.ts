import { describe, it, expect, vi, afterEach } from 'vitest';
import { appState } from './appState.svelte';
import { api } from '../api/client';

describe('appState.init master-key gating', () => {
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('does not call /api/auth/master-key when accessed remotely (is_remote=true)', async () => {
    vi.spyOn(api, 'getAuthStatus').mockResolvedValue({
      is_remote: true,
      unlocked: true,
      lan_enabled: true,
    } as any);
    const masterKeySpy = vi.spyOn(api, 'getMasterKey').mockResolvedValue({ lan_enabled: true });
    vi.spyOn(appState, 'loadConnections').mockResolvedValue(undefined);
    vi.spyOn(appState, 'loadSnippets').mockResolvedValue([]);
    vi.spyOn(appState, 'initActivityStream').mockImplementation(() => {});
    vi.spyOn(appState, 'startNotesReminderPoller').mockImplementation(() => {});

    await appState.init();

    expect(masterKeySpy).not.toHaveBeenCalled();
  });

  it('calls /api/auth/master-key on localhost (is_remote=false)', async () => {
    vi.spyOn(api, 'getAuthStatus').mockResolvedValue({
      is_remote: false,
      unlocked: true,
      lan_enabled: true,
    } as any);
    const masterKeySpy = vi.spyOn(api, 'getMasterKey').mockResolvedValue({
      lan_enabled: true,
      pairing_code: 'fox-1234',
    });
    vi.spyOn(appState, 'loadConnections').mockResolvedValue(undefined);
    vi.spyOn(appState, 'loadSnippets').mockResolvedValue([]);
    vi.spyOn(appState, 'initActivityStream').mockImplementation(() => {});
    vi.spyOn(appState, 'startNotesReminderPoller').mockImplementation(() => {});

    await appState.init();

    expect(masterKeySpy).toHaveBeenCalledTimes(1);
  });
});
