<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { copyText } from '../../utils/clipboard';
  import Icon from '../common/Icon.svelte';
  import FlightTimeline from './FlightTimeline.svelte';

  let {
    isRecording = false,
    recordingSeconds = 0,
    flightSessions = [],
    selectedFlightSessionId = '',
    flightSessionTicks = [],
    selectedTickIndex = $bindable(null),
    isFlightLoading = false,
    flightPeak,
    activeOrSelectedTick = null,
    onToggleRecording,
    onInspectSession,
    onDeleteSession,
    onOpenSqlInEditor,
    onExplainQuery,
    onSendThreadToAI,
  }: {
    isRecording: boolean;
    recordingSeconds?: number;
    flightSessions: any[];
    selectedFlightSessionId: string;
    flightSessionTicks: any[];
    selectedTickIndex: number | null;
    isFlightLoading: boolean;
    flightPeak: { maxQps: number; maxThreads: number; lockDelta: number; bufferDelta: number };
    activeOrSelectedTick: any;
    onToggleRecording?: () => void;
    onInspectSession: (recId: string) => void;
    onDeleteSession: (recId: string) => void;
    onOpenSqlInEditor: (sql?: string, title?: string) => void;
    onExplainQuery: (sql?: string) => void;
    onSendThreadToAI?: (sql: string, thread: any) => void;
  } = $props();

  function formatTickTime(tick: any): string {
    const raw = tick?.sampled_at || tick?.timestamp || tick?.time;
    if (!raw) return '--:--:--';
    const d = new Date(raw);
    return isNaN(d.getTime()) ? String(raw) : d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  }

  function getTickQueries(tick: any): any[] {
    if (!tick) return [];
    if (tick.top_queries_json) {
      try {
        const parsed = JSON.parse(tick.top_queries_json);
        if (Array.isArray(parsed)) return parsed.filter((q: any) => q && (q.info || q.query));
      } catch (_) {}
    }
    if (tick.top_query || tick.query) {
      return [{ info: tick.top_query || tick.query, time: 0, user: 'client', command: 'Query' }];
    }
    return [];
  }

  function getTopQueryText(tick: any): string {
    const qList = getTickQueries(tick);
    if (qList.length > 0) {
      return qList[0].info || qList[0].query || '-';
    }
    return '-';
  }

  function formatSessionLabel(s: any): string {
    if (!s) return '';
    const d = s.started_at ? new Date(s.started_at) : null;
    const timeStr = d && !isNaN(d.getTime()) ? d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : (s.name || s.id);
    const count = s.sample_count ?? 0;
    const dur = s.duration_seconds ?? 0;
    return `${s.name || 'Flight Run'} (${timeStr} · ${count} ticks, ${dur}s)`;
  }

  const inspectFlightSession = (recId: string) => onInspectSession(recId);
  const deleteFlightSession = (recId: string) => onDeleteSession(recId);
  const openSqlInEditor = (sql?: string, title?: string) => onOpenSqlInEditor(sql, title);
  const explainThreadQuery = (sql?: string) => onExplainQuery(sql);
</script>

    <!-- Performance Flight Recorder View -->
    <div class="flex-1 flex flex-col overflow-hidden bg-[#090d16]">
      <!-- Flight Sessions Bar -->
      <div class="h-10 bg-[#090d16] border-b border-[#1e293b] px-4 flex items-center justify-between text-xs select-none">
        <div class="flex items-center space-x-3">
          <div class="flex items-center space-x-1.5 font-sans">
            <span class="text-slate-400">Session:</span>
            <select
              value={selectedFlightSessionId}
              onchange={(e) => inspectFlightSession((e.target as HTMLSelectElement).value)}
              class="bg-[#0f172a] border border-[#334155] rounded px-2.5 py-1 text-slate-200 text-xs focus:outline-none focus:border-sky-500 font-mono cursor-pointer [color-scheme:dark] [&>option]:bg-[#0b1220] [&>option]:text-slate-200"
            >
              <option value="">{isRecording ? '● Live In-Progress Recording' : '-- Select Saved Recording --'}</option>
              {#each flightSessions as s}
                <option value={s.id}>
                  {formatSessionLabel(s)}
                </option>
              {/each}
            </select>
          </div>

          {#if selectedFlightSessionId}
            <button
              type="button"
              onclick={() => deleteFlightSession(selectedFlightSessionId)}
              class="px-2 py-1 rounded bg-rose-950/60 hover:bg-rose-900 border border-rose-800 text-rose-300 text-xs font-sans transition flex items-center space-x-1"
              title="Delete this flight session"
            >
              <Icon name="trash-2" class="w-3 h-3" />
              <span>Delete Session</span>
            </button>
          {/if}
        </div>

        <div class="flex items-center space-x-3 font-sans text-[11px] text-slate-400">
          <span>{flightSessions.length} recorded session(s)</span>
          {#if isRecording}
            <span class="px-2 py-0.5 rounded bg-rose-950/80 border border-rose-700 text-rose-300 font-bold flex items-center space-x-1.5 animate-pulse">
              <span class="w-2 h-2 rounded-full bg-rose-500 animate-ping"></span>
              <span>LIVE RECORDING ACTIVE ({Math.floor(recordingSeconds / 60)}m {recordingSeconds % 60}s)</span>
            </span>
          {/if}
        </div>
      </div>

      <!-- Flight Recorder Body -->
      <div class="flex-1 overflow-auto p-4 space-y-4">
        {#if isFlightLoading}
          <div class="p-16 text-center text-slate-500 flex flex-col items-center gap-2 font-sans">
            <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
            <span>Loading flight recorder telemetry ticks...</span>
          </div>
        {:else if !isRecording && !selectedFlightSessionId && flightSessions.length === 0}
          <div class="p-12 text-center bg-[#0b0f19] border border-dashed border-[#1e293b] rounded-2xl flex flex-col items-center justify-center space-y-3 my-12 max-w-lg mx-auto font-sans">
            <div class="p-3 rounded-full bg-rose-500/10 text-rose-400">
              <Icon name="activity" class="w-8 h-8" />
            </div>
            <div class="space-y-1">
              <h4 class="font-bold text-slate-200 text-sm">No Flight Recording Sessions Yet</h4>
              <p class="text-slate-400 text-xs">
                Performance Flight Recorder captures a blackbox audit of database telemetry (threads, QPS, row locks, buffer stalls, active SQL) sampled every 1s-5s. Click <strong>START REC</strong> to capture real-time behavior during load tests or incidents.
              </p>
            </div>
            <button
              type="button"
              onclick={() => onToggleRecording?.()}
              class="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-bold text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-950/40"
            >
              <Icon name="play" class="w-4 h-4" />
              <span>Start Performance Recording</span>
            </button>
          </div>
        {:else}
          <!-- Session Summary Banner -->
          <div class="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-3 font-sans text-xs">
            <div class="p-3 rounded-xl bg-[#0b0f19] border border-[#1e293b]">
              <span class="text-slate-500 text-[10px] uppercase font-bold block">Status</span>
              <span class="text-sm font-bold {isRecording ? 'text-rose-400 animate-pulse' : 'text-emerald-400'}">
                {isRecording ? 'Capturing Live Ticks' : 'Historical Vault Run'}
              </span>
            </div>
            <div class="p-3 rounded-xl bg-[#0b0f19] border border-[#1e293b]">
              <span class="text-slate-500 text-[10px] uppercase font-bold block">Active QPS</span>
              <span class="text-sm font-bold text-sky-400 font-mono">
                {activeOrSelectedTick?.qps != null ? activeOrSelectedTick.qps.toLocaleString() : 0} qps
              </span>
            </div>
            <div class="p-3 rounded-xl bg-[#0b0f19] border border-[#1e293b]">
              <span class="text-slate-500 text-[10px] uppercase font-bold block">Threads (Run / Conn)</span>
              <span class="text-sm font-bold text-emerald-400 font-mono">
                {activeOrSelectedTick?.threads_running ?? 0}
                <span class="text-slate-400 text-xs font-normal"> / {activeOrSelectedTick?.threads_connected ?? 0} conn</span>
              </span>
            </div>
            <div class="p-3 rounded-xl bg-[#0b0f19] border border-[#1e293b]">
              <span class="text-slate-500 text-[10px] uppercase font-bold block">Row-lock waits (Δ)</span>
              <span class="text-sm font-bold {flightPeak.lockDelta > 0 ? 'text-amber-400' : 'text-slate-200'} font-mono">
                +{flightPeak.lockDelta.toLocaleString()}
              </span>
              <span class="text-[10px] text-slate-500 font-sans block">counter {activeOrSelectedTick?.row_lock_waits ?? 0}</span>
            </div>
            <div class="p-3 rounded-xl bg-[#0b0f19] border border-[#1e293b]">
              <span class="text-slate-500 text-[10px] uppercase font-bold block">Buffer Pool Waits</span>
              <span class="text-sm font-bold {flightPeak.bufferDelta > 0 ? 'text-purple-400' : 'text-slate-200'} font-mono">
                +{flightPeak.bufferDelta.toLocaleString()}
              </span>
              <span class="text-[10px] text-slate-500 font-sans block">counter {activeOrSelectedTick?.buffer_pool_wait_free ?? 0}</span>
            </div>
            <div class="p-3 rounded-xl bg-[#0b0f19] border border-[#1e293b]">
              <span class="text-slate-500 text-[10px] uppercase font-bold block">Peak load</span>
              <span class="text-sm font-bold text-slate-100 font-mono">
                {flightPeak.maxQps} qps peak
              </span>
              <span class="text-[10px] text-slate-500 font-sans block">
                {flightPeak.maxThreads} active threads · {flightSessionTicks.length} samples
              </span>
            </div>
          </div>

          <!-- Telemetry Timeline (click a sample to inspect it) -->
          <FlightTimeline
            ticks={flightSessionTicks}
            selectedIndex={selectedTickIndex}
            onselect={(idx) => (selectedTickIndex = idx)}
          />

          <!-- Ticks Samples Table -->
          <div class="border border-[#1e293b] rounded-xl overflow-hidden bg-[#0b0f19]">
            <div class="bg-[#0f172a] px-3.5 py-2 border-b border-[#1e293b] flex items-center justify-between text-xs">
              <span class="font-bold text-slate-200 font-sans">Timeline Samples ({flightSessionTicks.length} points)</span>
              <span class="text-slate-500 text-[11px] font-sans">Click row to inspect running queries</span>
            </div>
            <div class="max-h-80 overflow-y-auto">
              <table class="w-full text-left border-collapse text-xs">
                <thead class="sticky top-0 bg-[#0f172a] text-slate-400 text-[11px] font-sans uppercase border-b border-[#1e293b] z-10">
                  <tr>
                    <th class="py-2 px-3 w-12">#</th>
                    <th class="py-2 px-3 w-32">Timestamp</th>
                    <th class="py-2 px-3 text-right w-36">Threads (Run/Conn)</th>
                    <th class="py-2 px-3 text-right w-24">QPS</th>
                    <th class="py-2 px-3 text-right w-24">Row Locks</th>
                    <th class="py-2 px-3 text-right w-24" title="Cumulative buffer-pool waits — the delta between samples is what matters">Buffer Wait (cum.)</th>
                    <th class="py-2 px-3">Top Active Query</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[#1e293b] font-mono text-[11.5px]">
                  {#if flightSessionTicks.length === 0}
                    <tr>
                      <td colspan="7" class="p-8 text-center text-slate-500 italic font-sans">
                        {isRecording ? 'Sampling in progress... ticks will appear shortly.' : 'No tick samples recorded in this session.'}
                      </td>
                    </tr>
                  {:else}
                    {#each flightSessionTicks as tick, idx}
                      {@const isSel = selectedTickIndex === idx || (selectedTickIndex == null && idx === flightSessionTicks.length - 1)}
                      <tr
                        role="button"
                        tabindex="0"
                        aria-pressed={isSel}
                        onclick={() => (selectedTickIndex = idx)}
                        onkeydown={(e) => {
                          if (e.key === 'Enter' || e.key === ' ') {
                            e.preventDefault();
                            selectedTickIndex = idx;
                          }
                        }}
                        class="cursor-pointer transition {isSel ? 'bg-sky-950/40 border-l-2 border-l-sky-400' : 'hover:bg-slate-800/40'}"
                      >
                        <td class="py-2 px-3 text-slate-500">{idx + 1}</td>
                        <td class="py-2 px-3 text-slate-300">{formatTickTime(tick)}</td>
                        <td class="py-2 px-3 text-right text-emerald-400 font-bold">
                          {tick.threads_running ?? tick.active_threads ?? 0}
                          <span class="text-slate-500 text-[10.5px] font-normal"> / {tick.threads_connected ?? 0}</span>
                        </td>
                        <td class="py-2 px-3 text-right text-sky-400 font-semibold">{tick.qps != null ? tick.qps : '--'}</td>
                        <td class="py-2 px-3 text-right {(tick.row_lock_waits || 0) > 0 ? 'text-amber-400 font-bold' : 'text-slate-500'}">
                          {tick.row_lock_waits ?? 0}
                        </td>
                        <td class="py-2 px-3 text-right {(tick.buffer_pool_wait_free || 0) > 0 ? 'text-purple-400 font-bold' : 'text-slate-500'}">
                          {tick.buffer_pool_wait_free ?? 0}
                        </td>
                        <td class="py-2 px-3 text-slate-300 truncate max-w-sm">
                          {getTopQueryText(tick)}
                        </td>
                      </tr>
                    {/each}
                  {/if}
                </tbody>
              </table>
            </div>
          </div>

          <!-- Correlated Active Queries Forensics Panel -->
          {#if activeOrSelectedTick}
            {@const queries = getTickQueries(activeOrSelectedTick)}
            <div class="border border-[#1e293b] rounded-xl overflow-hidden bg-[#0b0f19]">
              <div class="bg-[#0f172a] px-3.5 py-2.5 border-b border-[#1e293b] flex items-center justify-between text-xs">
                <div class="flex items-center space-x-2">
                  <Icon name="search" class="w-3.5 h-3.5 text-sky-400" />
                  <span class="font-bold text-slate-100 font-sans">
                    Correlated Queries Forensics at {formatTickTime(activeOrSelectedTick)}
                  </span>
                  <span class="px-1.5 py-0.5 rounded-full text-[10px] font-mono bg-sky-950 text-sky-300 border border-sky-800">
                    {queries.length} active queries
                  </span>
                </div>
                {#if selectedTickIndex != null}
                  <button
                    onclick={() => (selectedTickIndex = null)}
                    class="text-[11px] text-slate-400 hover:text-white"
                  >
                    Reset to Latest Tick
                  </button>
                {/if}
              </div>

              <div class="p-3 space-y-2.5">
                {#if queries.length === 0}
                  <div class="p-6 text-center text-slate-500 italic text-xs font-sans">
                    No active SQL statements running at this sampled second. Database was idle or handling rapid sub-millisecond queries.
                  </div>
                {:else}
                  {#each queries as q, qIdx}
                    {@const qSql = q.info || q.query || ''}
                    {@const tid = q.thread_id ?? q.id}
                    <div class="p-2.5 rounded-lg border border-slate-800 bg-[#050811] space-y-1.5 font-sans">
                      <div class="flex items-center justify-between text-[11px] flex-wrap gap-2">
                        <div class="flex items-center space-x-2 font-mono flex-wrap">
                          <span class="text-slate-500">#{qIdx + 1}</span>
                          {#if tid}<span class="text-slate-400">Thread: #{tid}</span>{/if}
                          {#if q.user}<span class="text-slate-400">User: {q.user}</span>{/if}
                          {#if q.db}<span class="text-sky-400 font-bold">DB: {q.db}</span>{/if}
                          {#if q.state}<span class="text-amber-400">State: {q.state}</span>{/if}
                          {#if q.time != null}<span class="text-slate-500 font-semibold">{q.time}s</span>{/if}
                        </div>
                        <div class="flex items-center space-x-1.5 flex-wrap">
                          <button
                            type="button"
                            onclick={() => {
                              copyText(qSql);
                              appState.showToast('SQL copied to clipboard!', 'info');
                            }}
                            class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] transition flex items-center space-x-1"
                            title="Copy Query SQL"
                          >
                            <Icon name="copy" class="w-3 h-3" />
                            <span>Copy</span>
                          </button>
                          <button
                            type="button"
                            onclick={() => {
                              appState.addTab('Flight Query.sql', qSql);
                              appState.showToast('SQL inserted into worksheet!', 'success');
                            }}
                            class="px-2 py-0.5 rounded bg-sky-950 hover:bg-sky-900 border border-sky-800 text-sky-300 text-[10px] transition flex items-center space-x-1"
                            title="Open Query in Worksheet"
                          >
                            <Icon name="file-code" class="w-2.5 h-2.5" />
                            <span>Worksheet</span>
                          </button>
                          <button
                            type="button"
                            onclick={() => explainThreadQuery(qSql)}
                            class="px-2 py-0.5 rounded bg-emerald-950 hover:bg-emerald-900 border border-emerald-800 text-emerald-300 text-[10px] transition flex items-center space-x-1"
                            title="Explain Query Execution Plan"
                          >
                            <Icon name="play" class="w-2.5 h-2.5" />
                            <span>Explain</span>
                          </button>
                          <button
                            type="button"
                            onclick={() => onSendThreadToAI?.(qSql, { id: tid || 0, user: q.user || '', host: q.host || '', db: q.db || '', command: q.command || 'Query', time: q.time || 0, state: q.state || '', info: qSql })}
                            class="px-2 py-0.5 rounded bg-purple-950 hover:bg-purple-900 border border-purple-800 text-purple-300 text-[10px] transition flex items-center space-x-1"
                            title="Ask AI Copilot"
                          >
                            <Icon name="bot" class="w-2.5 h-2.5" />
                            <span>Ask Copilot</span>
                          </button>
                          <button
                            type="button"
                            onclick={() => appState.saveQueryToNote(qSql, `Forensic Query (${formatTickTime(activeOrSelectedTick)})`)}
                            class="px-2 py-0.5 rounded bg-amber-950 hover:bg-amber-900 border border-amber-800 text-amber-300 text-[10px] transition flex items-center space-x-1"
                            title="Save Query to Operational Notes"
                          >
                            <Icon name="bookmark" class="w-2.5 h-2.5" />
                            <span>Note</span>
                          </button>
                        </div>
                      </div>
                      <pre class="p-2 rounded bg-[#090d16] border border-slate-900 text-sky-200 font-mono text-[11px] overflow-x-auto whitespace-pre-wrap select-text">{qSql}</pre>
                    </div>
                  {/each}
                {/if}
              </div>
            </div>
          {/if}
        {/if}
      </div>
    </div>
