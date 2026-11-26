/**
 * Pure formatting and prompt helpers for the schema Sidebar.
 */

export const SIDEBAR_WIDTH_STORAGE_KEY = 'sqlfox_sidebar_width';

export function clampSidebarWidth(w: number, min = 200, max = 480): number {
  if (!Number.isFinite(w)) return 260;
  return Math.min(max, Math.max(min, w));
}

export function formatObjectCount(cnt?: number): string {
  if (cnt === undefined || cnt === null) return '';
  if (cnt >= 1_000_000) return `${(cnt / 1_000_000).toFixed(1)}M`;
  if (cnt >= 1_000) return `${(cnt / 1_000).toFixed(1)}k`;
  return String(cnt);
}

export function buildAiTablePrompt(tableName: string): string {
  return `Analyze the schema of table \`${tableName}\`. Suggest useful queries, potential index optimizations, and explain its relationships.`;
}
