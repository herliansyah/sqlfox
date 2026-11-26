import { describe, it, expect, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import SafetyModal from './SafetyModal.svelte';
import { appState } from '../../stores/appState.svelte';

describe('SafetyModal', () => {
  beforeEach(() => {
    appState.safetyChallenge = {
      isOpen: false,
      sql: '',
      inspection: null,
      confirmed: false,
      resolve: undefined,
    };
  });

  it('does not render dialog when safetyChallenge is closed', () => {
    render(SafetyModal);
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('requires typing target table/db in Production destructive challenge', async () => {
    let resolvedWith: { confirmed: boolean; challengeInput: string } | null = null;
    appState.safetyChallenge = {
      isOpen: true,
      sql: 'DROP TABLE users;',
      inspection: {
        risk_level: 'CRITICAL',
        is_destructive: true,
        requires_challenge: true,
        target_table: 'users',
        target_database: 'prod_db',
        reason: 'DROP TABLE permanently removes table data.',
      },
      confirmed: false,
      resolve: (val) => {
        resolvedWith = val;
      },
    };

    render(SafetyModal);
    expect(screen.getByRole('dialog')).toBeTruthy();
    expect(screen.getByText(/DETERMINISTIC SAFETY GUARD/)).toBeTruthy();

    const confirmBtn = screen.getByRole('button', { name: /Execute/i }) as HTMLButtonElement;
    expect(confirmBtn.disabled).toBe(true);

    const input = screen.getByRole('textbox') as HTMLInputElement;
    await fireEvent.input(input, { target: { value: 'users' } });
    expect(confirmBtn.disabled).toBe(false);

    await fireEvent.click(confirmBtn);
    expect(resolvedWith).toEqual({ confirmed: true, challengeInput: 'users' });
  });
});
