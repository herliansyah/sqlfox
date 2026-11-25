import type { ProcesslistThread } from '../../../types';

export const PROCESSLIST_STATE_TONE: Record<string, string> = {
  red: 'bg-red-950/60 text-red-400 border-red-800/60',
  amber: 'bg-amber-950/60 text-amber-400 border-amber-800/60',
  emerald: 'bg-emerald-950/60 text-emerald-400 border-emerald-800/60',
  sky: 'bg-sky-950/60 text-sky-400 border-sky-800/60',
  slate: 'bg-slate-800/60 text-slate-400 border-slate-700/60',
};

export function isSystemThread(t: ProcesslistThread): boolean {
  const u = (t.user || '').toLowerCase();
  const cmd = (t.command || '').toLowerCase();
  return u === 'system user' || u === 'event_scheduler' || cmd === 'binlog dump' || cmd === 'binlog dump gtid';
}

export function commandTone(command: string): string {
  const c = (command || '').toLowerCase();
  if (!c || c === 'sleep' || c === 'daemon') return PROCESSLIST_STATE_TONE.slate;
  return PROCESSLIST_STATE_TONE.sky;
}
