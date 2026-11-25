const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]):not([type="hidden"]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * Svelte action that traps Tab focus inside a dialog node and restores focus to the
 * previously active element when the dialog unmounts.
 */
export function focusTrap(node: HTMLElement) {
  const previouslyFocused =
    typeof document !== 'undefined' ? (document.activeElement as HTMLElement | null) : null;

  const getFocusable = (): HTMLElement[] =>
    Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR)).filter(
      (el) => el.offsetParent !== null || el.getClientRects().length > 0
    );

  const handleKeyDown = (e: KeyboardEvent) => {
    if (e.key !== 'Tab') return;
    const focusable = getFocusable();
    if (focusable.length === 0) {
      e.preventDefault();
      return;
    }
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (e.shiftKey) {
      if (document.activeElement === first || !node.contains(document.activeElement)) {
        e.preventDefault();
        last.focus();
      }
    } else if (document.activeElement === last || !node.contains(document.activeElement)) {
      e.preventDefault();
      first.focus();
    }
  };

  node.addEventListener('keydown', handleKeyDown);
  // Move initial focus inside dialog if focus is still outside
  if (typeof document !== 'undefined' && !node.contains(document.activeElement)) {
    const first = getFocusable()[0];
    if (first) first.focus();
  }

  return {
    destroy() {
      node.removeEventListener('keydown', handleKeyDown);
      if (previouslyFocused && typeof previouslyFocused.focus === 'function' && previouslyFocused.isConnected) {
        previouslyFocused.focus();
      }
    },
  };
}
