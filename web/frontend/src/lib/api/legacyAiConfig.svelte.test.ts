import { beforeEach, describe, expect, it, vi } from 'vitest';
import { api, loadAiConfig, syncOrMigrateLegacyAiConfig } from './client';

const LEGACY_KEY = 'sqlfox_ai_config';

describe('legacy AI config', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it('removes plaintext config even when migration fails', async () => {
    localStorage.setItem(LEGACY_KEY, JSON.stringify({ apiKey: 'sk-legacy', modelName: 'gpt-4o-mini' }));
    vi.spyOn(console, 'warn').mockImplementation(() => {});
    vi.spyOn(api, 'listAiProfiles').mockRejectedValue(new Error('offline'));

    await syncOrMigrateLegacyAiConfig();

    expect(localStorage.getItem(LEGACY_KEY)).toBeNull();
  });

  it('still migrates the key into a profile when the API works', async () => {
    localStorage.setItem(LEGACY_KEY, JSON.stringify({ apiKey: 'sk-legacy', provider: 'anthropic' }));
    vi.spyOn(api, 'listAiProfiles').mockResolvedValue([{ id: 'p1', is_active: true, provider: 'openai' }] as any);
    const update = vi.spyOn(api, 'updateAiProfile').mockResolvedValue({} as any);

    await syncOrMigrateLegacyAiConfig();

    expect(update).toHaveBeenCalledWith('p1', expect.objectContaining({ api_key: 'sk-legacy' }));
    expect(localStorage.getItem(LEGACY_KEY)).toBeNull();
  });

  it('never restores apiKey from localStorage', () => {
    localStorage.setItem(LEGACY_KEY, JSON.stringify({ apiKey: 'sk-legacy', api_key: 'sk-snake' }));
    expect(loadAiConfig().apiKey).toBe('');
  });

  it('does not send a stored apiKey in aiChat', async () => {
    localStorage.setItem(LEGACY_KEY, JSON.stringify({ apiKey: 'sk-legacy', provider: 'openai' }));
    const fetchMock = vi.fn(async () => ({ ok: true, status: 200, text: async () => '{"content":"ok"}' }));
    vi.stubGlobal('fetch', fetchMock);

    await api.aiChat({ prompt: 'hi' });

    const body = JSON.parse((fetchMock.mock.calls[0] as any)[1].body);
    expect(body.api_key).toBe('');
    expect(JSON.stringify(body)).not.toContain('sk-legacy');
  });
});
