export interface ParsedActivityLog {
  id: string;
  time: string;
  timeMs?: string;
  level: string;
  module: string;
  message: string;
  attrs?: Record<string, any>;
}

export function parseActivityLogPayload(rawData: string): ParsedActivityLog | null {
  try {
    const data = JSON.parse(rawData);
    const now = data.time ? new Date(data.time) : new Date();
    const hh = String(now.getHours()).padStart(2, '0');
    const mm = String(now.getMinutes()).padStart(2, '0');
    const ss = String(now.getSeconds()).padStart(2, '0');
    const ms = String(now.getMilliseconds()).padStart(3, '0');
    const level = String(data.level || 'INFO').toUpperCase();
    return {
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      time: `${hh}:${mm}:${ss}`,
      timeMs: `${hh}:${mm}:${ss}.${ms}`,
      level,
      module: String(data.module || 'core'),
      message: String(data.msg || data.message || rawData),
      attrs: data.attrs || undefined,
    };
  } catch {
    return null;
  }
}

export function playAudioReminderChime(): void {
  try {
    if (typeof window === 'undefined') return;
    const AudioCtx = window.AudioContext || (window as any).webkitAudioContext;
    if (!AudioCtx) return;
    const ctx = new AudioCtx();
    const osc = ctx.createOscillator();
    const gain = ctx.createGain();
    osc.type = 'sine';
    osc.frequency.setValueAtTime(587.33, ctx.currentTime);
    osc.frequency.setValueAtTime(880, ctx.currentTime + 0.12);
    gain.gain.setValueAtTime(0.18, ctx.currentTime);
    gain.gain.exponentialRampToValueAtTime(0.001, ctx.currentTime + 0.7);
    osc.connect(gain);
    gain.connect(ctx.destination);
    osc.start();
    osc.stop(ctx.currentTime + 0.7);
    osc.onended = () => void ctx.close();
  } catch {
    /* ignore audio context restrictions */
  }
}
