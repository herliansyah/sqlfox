<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { copyText } from '../../utils/clipboard';
  import Icon from '../common/Icon.svelte';

  let {
    deadlockData = null,
    isDeadlockLoading = false,
    onOpenSqlInEditor,
    onRetry,
  }: {
    deadlockData: any;
    isDeadlockLoading: boolean;
    onOpenSqlInEditor: (sql?: string, title?: string) => void;
    onRetry?: () => void;
  } = $props();

  function copyWithToast(text: string | undefined, label: string) {
    if (!text) return;
    copyText(text);
    appState.showToast(`Copied ${label} to clipboard`, 'success');
  }

  // Deadlock Inspector helpers -------------------------------------------------
  function parseLockDescriptor(text: string): { table: string; index: string; mode: string; scope: string } {
    if (!text) return { table: '', index: '', mode: '', scope: '' };
    const rec = text.match(/index\s+(\S+)\s+of table\s+`?([\w$]+)`?\.`?([\w$]+)`?/i);
    if (rec) {
      const mode = (text.match(/lock_mode\s+([\w-]+)/i) || [])[1] || '';
      const scope = /locks rec but not gap/i.test(text) ? 'record' : /gap before rec/i.test(text) ? 'gap' : 'record';
      return { table: `${rec[2]}.${rec[3]}`, index: rec[1], mode: mode.toUpperCase(), scope };
    }
    const tbl = text.match(/TABLE LOCK table\s+`?([\w$]+)`?\.`?([\w$]+)`?[^,]*lock mode\s+([\w-]+)/i);
    if (tbl) return { table: `${tbl[1]}.${tbl[2]}`, index: '', mode: tbl[3].toUpperCase(), scope: 'table' };
    return { table: '', index: '', mode: '', scope: '' };
  }

  // What a transaction holds, falling back to the first lock descriptor in its block.
  function lockHeldInfo(tx: any) {
    const fromSection = parseLockDescriptor(tx?.lock_held || '');
    if (fromSection.table) return fromSection;
    return parseLockDescriptor(tx?.lock_mode || '');
  }

  // What a transaction is blocked on. Empty when the dump did not carry the descriptor.
  function lockWaitedInfo(tx: any) {
    return parseLockDescriptor(tx?.lock_waited || '');
  }

  function fmtLock(info: { table: string; index: string; mode: string; scope: string }, withMode = true): string {
    if (!info.table) return 'not captured';
    const parts = [info.table];
    if (info.index) parts.push(info.index);
    if (withMode && info.mode) parts.push(info.mode);
    if (info.scope) parts.push(info.scope);
    return parts.join(' · ');
  }

  // Advice derived from the parsed report, not from guesswork: only mention what the dump proves.
  function deadlockAdvice(report: any): string[] {
    const txs: any[] = report?.transactions || [];
    if (txs.length < 2) return [];
    const a = txs[0];
    const b = txs[1];
    const aHeld = lockHeldInfo(a);
    const aWaited = lockWaitedInfo(a);
    const bHeld = lockHeldInfo(b);
    const bWaited = lockWaitedInfo(b);
    const tips: string[] = [];

    // A true cycle: each side waits on what the other holds.
    if (
      aWaited.table && bHeld.table && aWaited.table === bHeld.table && aWaited.index === bHeld.index &&
      bWaited.table && aHeld.table && bWaited.table === aHeld.table && bWaited.index === aHeld.index
    ) {
      tips.push(
        `Classic cycle on ${aWaited.table}: each transaction waits for the ${aWaited.index} index entry the other one holds. Both statements touch the same rows in opposite order.`
      );
      tips.push('Order the writes the same way on every code path (for example, sort the primary keys before updating a batch).');
    } else if (aWaited.table && aWaited.table === bWaited.table && aWaited.index === bWaited.index) {
      tips.push(`Both transactions wait for the same ${aWaited.index} index entry in ${aWaited.table}, so the cycle runs through a single hot row.`);
    } else if (aWaited.table && bWaited.table) {
      tips.push(`The two waits land on different objects (${fmtLock(aWaited)} vs ${fmtLock(bWaited)}), so the cycle spans more than one statement.`);
    }

    const longest = Math.max(a.active_seconds || 0, b.active_seconds || 0);
    if (longest >= 5) {
      tips.push(`The older transaction had been open for ${longest}s before the conflict. Commit or roll back earlier instead of holding locks across more work.`);
    }
    const victim = txs.find((t) => t.is_victim);
    if (victim && (victim.undo_log_entries || 0) > 0) {
      tips.push(`The victim had ${victim.undo_log_entries} undo log entries to roll back, so the deadlock discarded real work, not just a single statement.`);
    }
    const scanHint = txs.find((t) => /index\s+PRIMARY/.test(String(t.lock_mode || '')));
    if (scanHint) {
      tips.push('One side locked through the PRIMARY key. Narrow the WHERE clause to the exact key range so fewer records get locked.');
    }
    tips.push('Retry the rolled-back transaction on error 1213 with backoff, and add a covering index if the conflicting statements still lock more rows than they change.');
    return tips;
  }

  function deadlockRawText(report: any): string {
    return String(report?.raw_section || '').trim();
  }

  function copyDeadlockReport(report: any) {
    copyWithToast(deadlockRawText(report) || JSON.stringify(report, null, 2), 'Deadlock report');
  }

  function sendDeadlockToAI(report: any) {
    const txs: any[] = report?.transactions || [];
    const lines = [
      'Analyse this InnoDB deadlock and explain how to prevent it recurring.',
      '',
      `Summary: ${report?.summary || '-'}`,
      report?.detected_at ? `Detected at: ${report.detected_at}` : '',
      '',
    ];
    txs.forEach((tx, i) => {
      lines.push(
        `Transaction ${tx.index ?? i + 1}${tx.is_victim ? ' (rolled back victim)' : ' (committed)'}`,
        `  thread: ${tx.thread_id || '-'} | user: ${tx.app_user || '-'}@${tx.host || '-'} | active: ${tx.active_seconds ?? '-'}s | undo entries: ${tx.undo_log_entries ?? '-'}`,
        `  holds: ${fmtLock(lockHeldInfo(tx))}`,
        `  waits for: ${fmtLock(lockWaitedInfo(tx))}`,
        `  query: ${tx.query || '-'}`,
        ''
      );
    });
    const raw = deadlockRawText(report);
    if (raw) lines.push('Raw InnoDB section:', raw);
    lines.push('', 'Give the lock cycle, the root cause, and the concrete fix (index, statement order, transaction scope, retry policy).');
    appState.sendToAi(lines.filter((l) => l !== null).join('\n'), 'Deadlock report');
  }


  const openSqlInEditor = (sql?: string, title?: string) => onOpenSqlInEditor(sql, title);
</script>

    <!-- Deadlock Inspector -->
    <div class="flex-1 overflow-auto p-3 space-y-3">
      {#if isDeadlockLoading}
        <div class="p-12 text-center text-slate-400 flex flex-col items-center gap-2">
          <Icon name="refresh-cw" class="w-6 h-6 animate-spin text-sky-400" />
          <span>Reading SHOW ENGINE INNODB STATUS for the latest deadlock...</span>
        </div>
      {:else if !deadlockData}
        <div class="bg-[#0b0f19] border border-[#334155] rounded-xl p-8 text-center space-y-3">
          <div class="inline-flex p-3 bg-slate-800/60 rounded-full text-slate-400">
            <Icon name="shield-alert" class="w-7 h-7" />
          </div>
          <h3 class="text-sm font-bold text-slate-200 font-sans">Deadlock report not loaded</h3>
          <p class="text-xs text-slate-400 max-w-md mx-auto font-sans">
            The engine status was not read for this session yet.
          </p>
          <button
            type="button"
            onclick={() => onRetry?.()}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-xs font-semibold font-sans"
          >
            Read engine status
          </button>
        </div>
      {:else if !deadlockData.has_deadlock}
        <div class="bg-[#0b0f19] border border-emerald-800/60 rounded-xl p-6">
          <div class="flex items-start gap-3">
            <div class="p-2 bg-emerald-500/10 text-emerald-400 rounded-lg shrink-0">
              <Icon name="check-circle" class="w-5 h-5" />
            </div>
            <div class="min-w-0 space-y-1">
              <h3 class="text-sm font-bold text-emerald-300 font-sans">No deadlock recorded</h3>
              <p class="text-xs text-slate-400 font-sans">{deadlockData.summary || 'No deadlocks detected since the database engine was started.'}</p>
              <p class="text-[11px] text-slate-500 font-sans">
                MySQL keeps only the <span class="text-slate-400">latest</span> deadlock in the engine status. Older incidents are gone once a new one replaces them.
              </p>
            </div>
          </div>
          <div class="mt-4 flex items-center gap-2">
            <button
              type="button"
              onclick={() => onRetry?.()}
              class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-xs font-semibold font-sans flex items-center gap-1.5"
            >
              <Icon name="refresh-cw" class="w-3.5 h-3.5" />
              <span>Check again</span>
            </button>
            <button
              type="button"
              onclick={() => sendDeadlockToAI(deadlockData)}
              class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-xs font-semibold font-sans flex items-center gap-1.5"
            >
              <Icon name="bot" class="w-3.5 h-3.5" />
              <span>Ask AI about lock waits</span>
            </button>
          </div>
        </div>
      {:else}
        <!-- Verdict -->
        <div class="bg-[#0b0f19] border border-rose-800/70 rounded-xl p-3">
          <div class="flex items-start justify-between gap-3">
            <div class="flex items-start gap-2.5 min-w-0">
              <div class="p-1.5 bg-rose-500/15 text-rose-400 rounded-md shrink-0 mt-0.5">
                <Icon name="alert-triangle" class="w-4 h-4" />
              </div>
              <div class="min-w-0">
                <h3 class="font-bold text-rose-300 font-sans text-sm">
                  {deadlockData.summary || 'Deadlock detected between concurrent transactions'}
                </h3>
                <div class="mt-1.5 flex flex-wrap items-center gap-x-4 gap-y-1 text-[11px] font-mono text-slate-400">
                  {#if deadlockData.detected_at}
                    <span><span class="text-slate-500">detected</span> {deadlockData.detected_at}</span>
                  {/if}
                  {#if deadlockData.victim_thread_id}
                    <span><span class="text-slate-500">victim thread</span> <span class="text-rose-300 font-bold">#{deadlockData.victim_thread_id}</span></span>
                  {/if}
                  {#if deadlockData.victim_tx_id}
                    <span><span class="text-slate-500">victim trx</span> {deadlockData.victim_tx_id}</span>
                  {/if}
                  <span><span class="text-slate-500">transactions</span> {deadlockData.transactions?.length ?? 0}</span>
                </div>
              </div>
            </div>
            <div class="flex items-center gap-1.5 shrink-0">
              <button
                type="button"
                onclick={() => copyDeadlockReport(deadlockData)}
                class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-300 text-[11px] font-sans flex items-center gap-1"
                title="Copy the raw InnoDB deadlock section"
              >
                <Icon name="copy" class="w-3 h-3" />
                <span>Copy raw</span>
              </button>
              <button
                type="button"
                onclick={() => sendDeadlockToAI(deadlockData)}
                class="px-2 py-1 rounded bg-purple-950/60 hover:bg-purple-900/60 border border-purple-800 text-purple-200 text-[11px] font-sans flex items-center gap-1"
              >
                <Icon name="bot" class="w-3 h-3" />
                <span>Send to AI</span>
              </button>
            </div>
          </div>
        </div>

        {#if (deadlockData.transactions?.length ?? 0) > 0}
          <!-- Lock conflict matrix: what each transaction holds and what it waits for -->
          <div class="bg-[#0b0f19] border border-[#334155] rounded-xl overflow-hidden">
            <div class="px-3 py-2 border-b border-[#1e293b] flex items-center justify-between">
              <h4 class="text-xs font-bold text-slate-200 font-sans">Lock conflict</h4>
              <span class="text-[10px] font-sans text-slate-500">who holds what, who waits for what</span>
            </div>
            <table class="w-full border-collapse text-left font-mono text-[11px]">
              <thead class="bg-[#090d16] text-slate-500 font-sans text-[10px] uppercase">
                <tr>
                  <th class="py-1.5 px-3">Transaction</th>
                  <th class="py-1.5 px-3">Holds</th>
                  <th class="py-1.5 px-3">Waits for</th>
                  <th class="py-1.5 px-3 text-right">Active</th>
                  <th class="py-1.5 px-3 text-right">Undo</th>
                  <th class="py-1.5 px-3">Outcome</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-[#1e293b]/60">
                {#each deadlockData.transactions as tx, i}
                  {@const held = lockHeldInfo(tx)}
                  {@const waited = lockWaitedInfo(tx)}
                  <tr class={tx.is_victim ? 'bg-rose-950/20' : ''}>
                    <td class="py-1.5 px-3 whitespace-nowrap">
                      <span class="text-slate-200 font-bold">Tx {tx.index ?? i + 1}</span>
                      <span class="text-slate-500"> · thread {tx.thread_id || '-'}</span>
                    </td>
                    <td class="py-1.5 px-3 text-slate-300">
                      <span class="text-sky-300">{fmtLock(held)}</span>
                    </td>
                    <td class="py-1.5 px-3 text-slate-300">
                      <span class={waited.table ? 'text-amber-300' : 'text-slate-500 italic'}>{fmtLock(waited)}</span>
                    </td>
                    <td class="py-1.5 px-3 text-right tabular-nums text-slate-300">{tx.active_seconds ?? '-'}s</td>
                    <td class="py-1.5 px-3 text-right tabular-nums text-slate-300">{tx.undo_log_entries ?? '-'}</td>
                    <td class="py-1.5 px-3">
                      <span
                        class="px-1.5 py-0.5 rounded text-[10px] font-bold font-sans uppercase border {tx.is_victim
                          ? 'bg-rose-950 text-rose-300 border-rose-800'
                          : 'bg-emerald-950 text-emerald-300 border-emerald-800'}"
                      >
                        {tx.is_victim ? 'rolled back' : 'committed'}
                      </span>
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>

          <!-- Per-transaction detail -->
          <div class="grid grid-cols-1 xl:grid-cols-2 gap-3">
            {#each deadlockData.transactions as tx, i}
              {@const held = lockHeldInfo(tx)}
              {@const waited = lockWaitedInfo(tx)}
              <div class="bg-[#0b0f19] border rounded-xl overflow-hidden {tx.is_victim ? 'border-rose-800/70' : 'border-[#334155]'}">
                <div class="px-3 py-2 border-b border-[#1e293b] flex items-center justify-between">
                  <div class="flex items-center gap-2">
                    <span class="text-xs font-bold text-slate-200 font-sans">Transaction {tx.index ?? i + 1}</span>
                    <span class="text-[10px] font-mono text-slate-500 truncate max-w-[10rem]" title={tx.tx_id}>trx {tx.tx_id || '-'}</span>
                  </div>
                  <span
                    class="px-1.5 py-0.5 rounded text-[10px] font-bold font-sans uppercase border {tx.is_victim
                      ? 'bg-rose-950 text-rose-300 border-rose-800'
                      : 'bg-emerald-950 text-emerald-300 border-emerald-800'}"
                  >
                    {tx.is_victim ? 'victim · rolled back' : 'survivor · committed'}
                  </span>
                </div>
                <div class="p-3 space-y-3">
                  <dl class="grid grid-cols-2 gap-x-3 gap-y-1.5 text-[11px] font-mono">
                    <div>
                      <dt class="text-slate-500 text-[10px] uppercase font-sans">Thread</dt>
                      <dd class="text-slate-200">{tx.thread_id || '-'}</dd>
                    </div>
                    <div>
                      <dt class="text-slate-500 text-[10px] uppercase font-sans">Account</dt>
                      <dd class="text-slate-200 truncate" title="{tx.app_user || '-'}@{tx.host || '-'}">
                        {tx.app_user || '-'}<span class="text-slate-500">@</span>{tx.host || '-'}
                      </dd>
                    </div>
                    <div>
                      <dt class="text-slate-500 text-[10px] uppercase font-sans">Active for</dt>
                      <dd class="text-slate-200">{tx.active_seconds ?? '-'}s</dd>
                    </div>
                    <div>
                      <dt class="text-slate-500 text-[10px] uppercase font-sans">Undo entries</dt>
                      <dd class="text-slate-200">{tx.undo_log_entries ?? '-'}</dd>
                    </div>
                    <div class="col-span-2">
                      <dt class="text-slate-500 text-[10px] uppercase font-sans">Holds lock</dt>
                      <dd class="text-sky-300 break-words" title={tx.lock_held || tx.lock_mode}>{fmtLock(held)}</dd>
                    </div>
                    <div class="col-span-2">
                      <dt class="text-slate-500 text-[10px] uppercase font-sans">Waits for lock</dt>
                      <dd class={waited.table ? 'text-amber-300 break-words' : 'text-slate-500 italic'} title={tx.lock_waited}>
                        {fmtLock(waited)}
                      </dd>
                    </div>
                  </dl>

                  {#if held.mode || waited.mode}
                    <div class="flex flex-wrap items-center gap-1.5 text-[10px] font-sans">
                      {#if held.mode}
                        <span class="px-1.5 py-0.5 rounded bg-sky-950 text-sky-300 border border-sky-800 uppercase font-bold">holds {held.mode} · {held.scope}</span>
                      {/if}
                      {#if waited.mode}
                        <span class="px-1.5 py-0.5 rounded bg-amber-950 text-amber-300 border border-amber-800 uppercase font-bold">waits {waited.mode} · {waited.scope}</span>
                      {/if}
                    </div>
                  {/if}

                  <div>
                    <div class="flex items-center justify-between mb-1">
                      <span class="text-slate-500 text-[10px] uppercase font-sans">Statement</span>
                      <div class="flex items-center space-x-2">
                        <button
                          type="button"
                          onclick={() => copyWithToast(tx.query, 'Statement')}
                          class="text-[10px] text-sky-400 hover:underline font-sans flex items-center gap-1"
                        >
                          <Icon name="copy" class="w-3 h-3" />
                          <span>Copy</span>
                        </button>
                        {#if tx.query}
                          <button
                            type="button"
                            onclick={() => openSqlInEditor(tx.query, `Deadlock_Tx${tx.index ?? 1}.sql`)}
                            class="text-[10px] text-emerald-400 hover:underline font-sans flex items-center gap-1"
                          >
                            <Icon name="file-code" class="w-3 h-3" />
                            <span>Open in Editor</span>
                          </button>
                        {/if}
                      </div>
                    </div>
                    <pre class="p-2 rounded bg-[#090d16] border border-slate-800 text-[11px] text-sky-300 font-mono overflow-x-auto whitespace-pre-wrap break-words">{tx.query || 'Statement not captured in the engine status.'}</pre>
                  </div>
                </div>
              </div>
            {/each}
          </div>

          <!-- What the dump proves -->
          <div class="bg-[#0b0f19] border border-[#334155] rounded-xl p-3">
            <h4 class="text-xs font-bold text-slate-200 font-sans mb-2 flex items-center gap-1.5">
              <Icon name="zap" class="w-3.5 h-3.5 text-amber-400" />
              <span>What to change</span>
            </h4>
            <ul class="space-y-1.5">
              {#each deadlockAdvice(deadlockData) as tip}
                <li class="flex items-start gap-2 text-[11.5px] text-slate-300 font-sans leading-relaxed">
                  <span class="mt-1.5 w-1 h-1 rounded-full bg-slate-500 shrink-0"></span>
                  <span>{tip}</span>
                </li>
              {/each}
            </ul>
          </div>
        {:else}
          <div class="bg-[#0b0f19] border border-amber-800/60 rounded-xl p-4 text-xs text-slate-300 font-sans">
            The engine reported a deadlock but its transaction detail could not be parsed. Open the raw section below and read the
            <span class="font-mono text-amber-300">TRANSACTION</span> blocks directly.
          </div>
        {/if}

        {#if deadlockRawText(deadlockData)}
          <details class="bg-[#0b0f19] border border-[#334155] rounded-xl overflow-hidden group">
            <summary class="px-3 py-2 cursor-pointer text-xs font-bold text-slate-300 font-sans flex items-center gap-1.5 hover:text-white">
              <Icon name="chevron-right" class="w-3.5 h-3.5 transition-transform group-open:rotate-90" />
              <span>Raw InnoDB deadlock section</span>
              <span class="text-[10px] font-mono font-normal text-slate-500">{deadlockRawText(deadlockData).split('\n').length} lines</span>
            </summary>
            <pre class="max-h-96 overflow-auto m-0 px-3 pb-3 text-[11px] leading-relaxed text-slate-400 font-mono whitespace-pre">{deadlockRawText(deadlockData)}</pre>
          </details>
        {/if}
      {/if}
    </div>
