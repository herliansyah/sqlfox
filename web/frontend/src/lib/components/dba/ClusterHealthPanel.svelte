<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { copyText } from '../../utils/clipboard';
  import Icon from '../common/Icon.svelte';

  let {
    clusterReport = null,
    isClusterLoading = false,
    onSwitchNode,
    onViewThreads,
  }: {
    clusterReport: any;
    isClusterLoading: boolean;
    onSwitchNode: (addr?: string) => void;
    onViewThreads?: () => void;
  } = $props();

  function copyWithToast(text: string | undefined, label: string) {
    if (!text) return;
    copyText(text);
    appState.showToast(`Copied ${label} to clipboard`, 'success');
  }

  export function sendClusterToAI() {
    if (!clusterReport) return;
    const prompt = [
      'Analyse this MySQL cluster & replication telemetry report and recommend prioritized remediation steps:',
      '',
      `Topology: ${fmtTopology(clusterReport.topology_type)} | Status: ${clusterReport.cluster_status || 'standalone'} | Size: ${clusterReport.cluster_size || 1}`,
      `Severity: ${severityLabel}`,
      `Flow Control Paused: ${flowControlPct.toFixed(2)}% | Diagnosis: ${clusterReport.bottleneck_diagnosis || '-'}`,
      clusterReport.replication
        ? `Replication: ${clusterReport.replication.role} | Seconds behind: ${clusterReport.replication.seconds_behind_master ?? 'n/a'} | IO: ${clusterReport.replication.io_running} | SQL: ${clusterReport.replication.sql_running}`
        : '',
      clusterReport.tuning_checks?.length
        ? `Tuning Checks:\n${clusterReport.tuning_checks.map((c: any) => `- ${c.variable}: current=${c.current_value}, advised=${c.recommended} (${c.status}) — ${c.summary}`).join('\n')}`
        : '',
    ]
      .filter(Boolean)
      .join('\n');
    appState.sendToAi(prompt, 'Cluster & Replication Report');
  }

  // ---- Cluster / replication presentation helpers -------------------------
  // The label must not contradict the banner text underneath it: run both through the same
  // classifier so a degraded diagnosis can never sit under a green "Healthy" heading.
  const severityTrue = $derived.by(() => {
    const report: any = clusterReport;
    if (!report) return 'ok';
    const fc = (report.flow_control_paused ?? 0) * 100;
    const rep: any = report.replication;
    const nodes: any[] = report.nodes || [];
    const down =
      (rep && (rep.io_running === false || rep.sql_running === false)) ||
      String(report.local_node_state || '').toLowerCase().includes('error') ||
      String(report.cluster_status || '').toLowerCase().includes('non-primary');
    const worstQueue = nodes.reduce(
      (max, n) => Math.max(max, (n.receive_queue ?? 0) + (n.send_queue ?? 0)),
      0
    );
    const checks: any[] = report.tuning_checks || [];
    if (
      down ||
      fc > 5 ||
      worstQueue > 5000 ||
      (rep?.seconds_behind_master ?? 0) > 60 ||
      checks.some((c) => c.status === 'critical')
    ) {
      return 'critical';
    }
    if (
      fc > 1 ||
      worstQueue > 1000 ||
      (rep?.seconds_behind_master ?? 0) > 5 ||
      checks.some((c) => c.status === 'warning')
    ) {
      return 'warning';
    }
    return 'ok';
  });
  const severityLabel = $derived(
    severityTrue === 'critical'
      ? 'Critical — cluster needs attention'
      : severityTrue === 'warning'
        ? 'Warning — degraded replication'
        : 'Healthy — cluster optimal'
  );

  const severityTone = $derived(
    severityTrue === 'critical'
      ? 'bg-rose-950/40 border-rose-800 text-rose-300'
      : severityTrue === 'warning'
        ? 'bg-amber-950/30 border-amber-800 text-amber-300'
        : 'bg-emerald-950/30 border-emerald-800 text-emerald-300'
  );
  const severityIcon = $derived(
    severityTrue === 'critical'
      ? 'shield-alert'
      : severityTrue === 'warning'
        ? 'alert-triangle'
        : 'shield-check'
  );

  const flowControlPct = $derived(
    Math.max(0, (clusterReport?.flow_control_paused ?? 0) * 100)
  );
  const fcTone = $derived(
    flowControlPct > 5 ? 'text-rose-300 font-bold' : flowControlPct > 1 ? 'text-amber-300' : 'text-emerald-300'
  );
  const netLatency = $derived(
    clusterReport?.tri_metric
      ? `${clusterReport.tri_metric.network_latency_avg_ms?.toFixed(2) ?? '0'} ms (max ${clusterReport.tri_metric.network_latency_max_ms?.toFixed(2) ?? '0'})`
      : '—'
  );

  const clusterCards = $derived([
    {
      label: 'Topology',
      value: fmtTopology(clusterReport?.topology_type),
      hint: clusterReport?.cluster_name || '',
      tone: 'text-sky-300',
    },
    {
      label: 'Status',
      value: clusterReport?.cluster_status || 'standalone',
      hint: clusterReport?.local_node_state ? `local ${clusterReport.local_node_state}` : '',
      tone:
        (clusterReport?.cluster_status || '').toLowerCase() === 'primary' ||
        !clusterReport?.cluster_status
          ? 'text-emerald-300'
          : 'text-rose-300',
    },
    {
      label: 'Members',
      value: String(clusterReport?.cluster_size || clusterReport?.nodes?.length || 1),
      hint: clusterReport?.slowest_node ? `slowest ${clusterReport.slowest_node}` : '',
      tone: 'text-slate-100',
    },
    {
      label: 'Replication',
      value: clusterReport?.replication
        ? `${clusterReport.replication.role}${clusterReport.replication.seconds_behind_master !== null && clusterReport.replication.seconds_behind_master !== undefined ? ` · ${clusterReport.replication.seconds_behind_master}s` : ''}`
        : 'sync (wsrep)',
      hint: clusterReport?.replication && (!clusterReport.replication.io_running || !clusterReport.replication.sql_running)
        ? 'thread down'
        : clusterReport?.replication?.seconds_behind_master != null
          ? `${clusterReport.replication.seconds_behind_master}s behind source`
          : 'in sync',
      tone: clusterReport?.replication && (!clusterReport.replication.io_running || !clusterReport.replication.sql_running)
        ? 'text-rose-300'
        : 'text-emerald-300',
    },
    {
      label: 'Flow control',
      value: `${flowControlPct.toFixed(2)}%`,
      hint: 'time commits were paused',
      tone: fcTone,
    },
    {
      label: 'Local node',
      value: clusterReport?.local_node_address || clusterReport?.local_node_name || '—',
      hint: clusterReport?.local_node_name || '',
      tone: 'text-slate-200',
    },
  ]);

  function fmtTopology(t?: string): string {
    if (!t) return 'standalone';
    const k = t.toLowerCase();
    if (k === 'galera') return 'Galera / wsrep';
    if (k === 'replication') return 'Source → Replica';
    if (k === 'group_replication') return 'Group Replication';
    return t;
  }

  function nodeStateTone(state?: string): string {
    const k = (state || '').toLowerCase();
    if (k.includes('sync') || k.includes('primary') || k.includes('online')) {
      return 'bg-emerald-950 text-emerald-300 border-emerald-800';
    }
    if (k.includes('donor') || k.includes('join') || k.includes('desync')) {
      return 'bg-amber-950 text-amber-300 border-amber-800';
    }
    if (k.includes('error') || k.includes('non-primary')) {
      return 'bg-rose-950 text-rose-300 border-rose-800';
    }
    return 'bg-slate-800 text-slate-300 border-slate-700';
  }

  function nodeQueueTotal(node: any): number {
    return (node?.receive_queue ?? 0) + (node?.send_queue ?? 0);
  }

  function queueWidth(node: any): number {
    const total = nodeQueueTotal(node);
    if (total <= 0) return 2;
    // 1000+ queued writesets is already a severe backlog.
    return Math.min(100, Math.max(3, (total / 1000) * 100));
  }

  function lagTone(seconds?: number | null): string {
    if (seconds === null || seconds === undefined) return 'text-slate-300';
    if (seconds > 60) return 'text-rose-300 font-bold';
    if (seconds > 5) return 'text-amber-300 font-bold';
    return 'text-emerald-300 font-bold';
  }


  const switchSessionToNode = (addr?: string) => onSwitchNode(addr);
</script>

    <!-- Cluster & Replication Telemetry -->
    <div class="flex-1 overflow-auto p-3 space-y-3">
      {#if isClusterLoading}
        <div class="p-6 text-center text-slate-400 flex items-center justify-center gap-2 text-xs">
          <Icon name="refresh-cw" class="w-4 h-4 animate-spin text-sky-400" />
          <span>Introspecting cluster topology and replication status...</span>
        </div>
      {:else if !clusterReport}
        <div class="p-4 flex items-start gap-2.5 rounded-lg bg-[#0b0f19] border border-[#1e293b]">
          <Icon name="network" class="w-4 h-4 mt-0.5 shrink-0 text-slate-500" />
          <div class="min-w-0">
            <p class="text-xs text-slate-300 font-sans">No cluster or replication telemetry for this session.</p>
            <p class="text-[11px] text-slate-500 font-sans mt-0.5">
              Needs a MySQL/MariaDB connection (wsrep, replication or performance_schema). SQLite and PostgreSQL sessions report nothing here.
            </p>
          </div>
        </div>
      {:else}
        <!-- Verdict banner -->
        <div
          class="flex items-start gap-2.5 px-3 py-2.5 rounded-lg border {severityTone} font-sans"
        >
          <Icon name={severityIcon} class="w-4 h-4 mt-0.5 shrink-0" />
          <div class="min-w-0">
            <p class="text-xs font-bold uppercase tracking-wide">{severityLabel}</p>
            <p class="text-[11.5px] text-slate-300 leading-snug">
              {clusterReport.bottleneck_diagnosis || 'Cluster is healthy: no flow-control stalls, replica lag or queue backlog detected.'}
            </p>
          </div>
        </div>

        <!-- Topology summary -->
        <div class="grid grid-cols-2 md:grid-cols-4 xl:grid-cols-6 gap-2">
          {#each clusterCards as card}
            <div class="px-3 py-2 bg-[#0b0f19] border border-[#1e293b] rounded-lg min-w-0">
              <span class="text-slate-500 text-[10px] uppercase tracking-wide font-sans block truncate">
                {card.label}
              </span>
              <span class="text-[13px] font-bold font-mono truncate block {card.tone}" title={card.value}>
                {card.value}
              </span>
              {#if card.hint}
                <span class="text-[10px] text-slate-500 block truncate" title={card.hint}>{card.hint}</span>
              {/if}
            </div>
          {/each}
        </div>

        <!-- Members table -->
        {#if clusterReport.nodes && clusterReport.nodes.length > 0}
          <div class="border border-[#1e293b] rounded-lg overflow-hidden bg-[#0b0f19]">
            <div class="px-3 py-2 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between">
              <span class="font-sans font-bold text-slate-300 text-xs">
                Cluster Members ({clusterReport.nodes.length})
              </span>
              <span class="text-[10px] text-slate-500 font-sans">
                queue depth & flow control per node · culprit node flagged
              </span>
            </div>
            <table class="w-full text-left border-collapse text-[11.5px] font-mono">
              <thead class="bg-[#0b0f19] border-b border-[#1e293b] text-slate-400 text-[10.5px] uppercase font-sans tracking-wide">
                <tr>
                  <th class="py-1.5 px-3">Node</th>
                  <th class="py-1.5 px-3">Address</th>
                  <th class="py-1.5 px-3">State</th>
                  <th class="py-1.5 px-3 text-right">Recv Q</th>
                  <th class="py-1.5 px-3 text-right">Send Q</th>
                  <th class="py-1.5 px-3 text-right">FC Sent</th>
                  <th class="py-1.5 px-3 w-24">Queue load</th>
                  <th class="py-1.5 px-3">Role</th>
                  <th class="py-1.5 px-3 text-right">Action</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-[#1e293b]/60">
                {#each clusterReport.nodes as node}
                  {@const isUnreachable = String(node.status || '').toLowerCase() === 'unreachable'}
                  <tr class="hover:bg-slate-800/40 {node.is_culprit ? 'bg-amber-950/20' : ''} {isUnreachable ? 'opacity-75' : ''}">
                    <td class="py-1.5 px-3 font-bold text-sky-300 truncate max-w-[12rem]" title={node.name}>
                      {node.name || '(unnamed)'}
                    </td>
                    <td class="py-1.5 px-3 text-slate-400">{node.address || '-'}</td>
                    <td class="py-1.5 px-3">
                      <span class="px-1.5 py-0.5 rounded text-[10px] font-bold font-sans uppercase border {isUnreachable ? 'bg-slate-800 text-slate-400 border-slate-700' : nodeStateTone(node.state)}">
                        {isUnreachable ? 'unreachable' : (node.state || 'unknown')}
                      </span>
                      {#if isUnreachable && node.message}
                        <span class="block text-[9.5px] text-slate-500 font-sans truncate max-w-[10rem]" title={node.message}>{node.message}</span>
                      {/if}
                    </td>
                    <td class="py-1.5 px-3 text-right {node.receive_queue > 0 ? 'text-amber-400 font-bold' : 'text-slate-300'}">
                      {isUnreachable ? '—' : (node.receive_queue ?? 0)}
                    </td>
                    <td class="py-1.5 px-3 text-right {node.send_queue > 0 ? 'text-amber-400 font-bold' : 'text-slate-300'}">
                      {isUnreachable ? '—' : (node.send_queue ?? 0)}
                    </td>
                    <td class="py-1.5 px-3 text-right text-slate-300">{isUnreachable ? '—' : (node.flow_control_sent ?? 0).toLocaleString()}</td>
                    <td class="py-1.5 px-3">
                      <div class="flex items-center gap-2">
                        <div class="h-1.5 flex-1 rounded-full bg-slate-800 overflow-hidden">
                          {#if !isUnreachable && nodeQueueTotal(node) > 0}
                            <div
                              class="h-full rounded-full {node.is_culprit || nodeQueueTotal(node) > 1000 ? 'bg-amber-500' : 'bg-sky-500'}"
                              style="width: {queueWidth(node)}%"
                              title="receive queue {node.receive_queue ?? 0} / send queue {node.send_queue ?? 0}"
                            ></div>
                          {/if}
                        </div>
                        <span class="text-[10px] font-mono {nodeQueueTotal(node) > 1000 ? 'text-amber-400' : 'text-slate-500'} w-10 text-right">
                          {isUnreachable ? 'locked' : nodeQueueTotal(node) > 0 ? nodeQueueTotal(node).toLocaleString() : 'clear'}
                        </span>
                      </div>
                    </td>
                    <td class="py-1.5 px-3 font-sans space-x-1">
                      {#if node.is_local}
                        <span class="px-1.5 py-0.5 rounded bg-sky-950 border border-sky-800 text-sky-300 text-[9.5px] font-bold uppercase">local</span>
                      {/if}
                      {#if node.is_culprit}
                        <span class="px-1.5 py-0.5 rounded bg-amber-950 border border-amber-800 text-amber-300 text-[9.5px] font-bold uppercase" title="This node is throttling the cluster">culprit</span>
                      {/if}
                    </td>
                    <td class="py-1.5 px-3 text-right font-sans">
                      {#if node.is_local}
                        <button
                          type="button"
                          onclick={() => onViewThreads?.()}
                          class="px-2 py-0.5 rounded bg-sky-950 hover:bg-sky-900 border border-sky-800 text-sky-300 text-[10px] font-semibold transition"
                        >
                          Inspect Processlist
                        </button>
                      {:else if node.address}
                        <button
                          type="button"
                          onclick={() => switchSessionToNode(node.address)}
                          class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-[10px] font-medium transition"
                        >
                          Switch Session
                        </button>
                      {/if}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
          </div>
        {/if}

        <div class="grid grid-cols-1 xl:grid-cols-2 gap-3">
          <!-- Replication (async / source-replica) -->
          <div class="border border-[#1e293b] rounded-lg overflow-hidden bg-[#0b0f19]">
            <div class="px-3 py-2 bg-[#0f172a] border-b border-[#1e293b] font-sans font-bold text-slate-300 text-xs flex items-center justify-between">
              <span>Replication Threads</span>
              {#if clusterReport.replication}
                <span class="px-1.5 py-0.5 rounded text-[10px] font-bold uppercase border {clusterReport.replication.role === 'Replica' ? 'bg-sky-950 border-sky-800 text-sky-300' : 'bg-emerald-950 border-emerald-800 text-emerald-300'}">
                  {clusterReport.replication.role}
                </span>
              {/if}
            </div>
            {#if clusterReport.replication}
              <dl class="p-3 grid grid-cols-2 gap-x-3 gap-y-2 text-[11.5px] font-mono">
                <div>
                  <dt class="text-slate-500 text-[10px] uppercase font-sans">Source host</dt>
                  <dd class="text-slate-200 truncate" title={clusterReport.replication.master_host}>
                    {clusterReport.replication.master_host || '—'}
                  </dd>
                </div>
                <div>
                  <dt class="text-slate-500 text-[10px] uppercase font-sans">Seconds behind</dt>
                  <dd class={lagTone(clusterReport.replication.seconds_behind_master)}>
                    {clusterReport.replication.seconds_behind_master === null || clusterReport.replication.seconds_behind_master === undefined
                      ? '—'
                      : `${clusterReport.replication.seconds_behind_master}s`}
                  </dd>
                </div>
                <div>
                  <dt class="text-slate-500 text-[10px] uppercase font-sans">IO thread</dt>
                  <dd class="flex items-center gap-1.5">
                    <span class="w-1.5 h-1.5 rounded-full {clusterReport.replication.io_running ? 'bg-emerald-400' : 'bg-rose-500'}"></span>
                    <span class={clusterReport.replication.io_running ? 'text-emerald-300' : 'text-rose-300'}>
                      {clusterReport.replication.io_running ? 'Running' : 'Stopped'}
                    </span>
                  </dd>
                </div>
                <div>
                  <dt class="text-slate-500 text-[10px] uppercase font-sans">SQL thread</dt>
                  <dd class="flex items-center gap-1.5">
                    <span class="w-1.5 h-1.5 rounded-full {clusterReport.replication.sql_running ? 'bg-emerald-400' : 'bg-rose-500'}"></span>
                    <span class={clusterReport.replication.sql_running ? 'text-emerald-300' : 'text-rose-300'}>
                      {clusterReport.replication.sql_running ? 'Running' : 'Stopped'}
                    </span>
                  </dd>
                </div>
                {#if clusterReport.replication.last_error}
                  <div class="col-span-2">
                    <dt class="text-slate-500 text-[10px] uppercase font-sans">Last error</dt>
                    <dd class="text-rose-300 break-words whitespace-pre-wrap">{clusterReport.replication.last_error}</dd>
                  </div>
                {/if}
              </dl>
            {:else}
              <p class="p-3 text-[11.5px] text-slate-500">
                No asynchronous replication configured on this node — traffic is sychronous via cluster
                (wsrep) or the node is a standalone instance.
              </p>
            {/if}
          </div>

          <!-- Flow control & durability tri-metric -->
          <div class="border border-[#1e293b] rounded-lg overflow-hidden bg-[#0b0f19]">
            <div class="px-3 py-2 bg-[#0f172a] border-b border-[#1e293b] font-sans font-bold text-slate-300 text-xs">
              Flow Control &amp; Durability Tri-Metric
            </div>
            <div class="p-3 space-y-2.5">
              <div>
                <div class="flex items-center justify-between text-[11px] font-sans">
                  <span class="text-slate-400">Flow control paused</span>
                  <span class="font-mono {fcTone}">{flowControlPct.toFixed(2)}%</span>
                </div>
                <div class="mt-1 h-1.5 w-full rounded-full bg-slate-800 overflow-hidden">
                  <div class="h-full {flowControlPct > 5 ? 'bg-rose-500' : flowControlPct > 1 ? 'bg-amber-500' : 'bg-emerald-500'}"
                       style="width: {Math.min(100, Math.max(0.5, flowControlPct * 10))}%"></div>
                </div>
                <p class="text-[10px] text-slate-500 mt-1 font-sans">
                  {flowControlPct > 5
                    ? 'Cluster replication is throttling commits — apply threads are the bottleneck.'
                    : flowControlPct > 1
                      ? 'Occasional throttling; watch wsrep_slave_threads and per-node disk latency.'
                      : 'Cluster commits are flowing without throttling.'}
                </p>
              </div>
              <dl class="grid grid-cols-2 gap-x-3 gap-y-1.5 text-[11.5px] font-mono">
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">WS sent</dt><dd class="text-slate-200">{(clusterReport.flow_control_sent ?? 0).toLocaleString()}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">WS received</dt><dd class="text-slate-200">{(clusterReport.flow_control_recv ?? 0).toLocaleString()}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">Apply threads</dt><dd class="text-slate-200">{clusterReport.tri_metric?.slave_threads ? clusterReport.tri_metric.slave_threads : 'n/a'}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">Buffer waits</dt><dd class={clusterReport.tri_metric?.buffer_pool_wait_free ? 'text-amber-300 font-bold' : 'text-slate-200'}>{(clusterReport.tri_metric?.buffer_pool_wait_free ?? 0).toLocaleString()}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">Cert failures</dt><dd class={clusterReport.tri_metric?.cert_failures ? 'text-rose-300' : 'text-slate-200'}>{(clusterReport.tri_metric?.cert_failures ?? 0).toLocaleString()}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">BF aborts</dt><dd class={clusterReport.tri_metric?.bf_aborts ? 'text-amber-300' : 'text-slate-200'}>{(clusterReport.tri_metric?.bf_aborts ?? 0).toLocaleString()}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">Log waits</dt><dd class={clusterReport.tri_metric?.log_waits ? 'text-amber-300' : 'text-slate-200'}>{(clusterReport.tri_metric?.log_waits ?? 0).toLocaleString()}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">FSyncs</dt><dd class="text-slate-200">{(clusterReport.tri_metric?.os_log_fsyncs ?? 0).toLocaleString()}</dd></div>
                <div class="col-span-2 flex justify-between border-t border-slate-800/70 pt-1">
                  <dt class="text-slate-500 font-sans">Net latency (min / avg / max)</dt>
                  <dd class="text-slate-200">
                    {clusterReport.tri_metric
                      ? `${(clusterReport.tri_metric.network_latency_min_ms ?? 0).toFixed(2)} / ${(clusterReport.tri_metric.network_latency_avg_ms ?? 0).toFixed(2)} / ${(clusterReport.tri_metric.network_latency_max_ms ?? 0).toFixed(2)} ms`
                      : netLatency}
                  </dd>
                </div>
              </dl>
            </div>
          </div>
        </div>

        <!-- Tuning checks -->
        {#if clusterReport.tuning_checks && clusterReport.tuning_checks.length > 0}
          <div class="space-y-2">
            <span class="font-sans font-bold text-slate-300 text-xs">
              Configuration Tuning Checks ({clusterReport.tuning_checks.length})
            </span>
            <div class="grid grid-cols-1 xl:grid-cols-2 gap-2">
              {#each clusterReport.tuning_checks as chk}
                <div class="p-3 bg-[#0b0f19] border border-[#1e293b] rounded-lg space-y-1.5">
                  <div class="flex items-center justify-between gap-2">
                    <span class="font-mono font-bold text-sky-300 text-[11.5px] truncate">{chk.variable}</span>
                    <span class="px-1.5 py-0.5 rounded text-[9.5px] font-bold uppercase shrink-0 border {chk.status === 'optimal' ? 'bg-emerald-950 text-emerald-300 border-emerald-800' : chk.status === 'critical' ? 'bg-rose-950 text-rose-300 border-rose-800' : 'bg-amber-950 text-amber-300 border-amber-800'}">
                      {chk.status}
                    </span>
                  </div>
                  <div class="text-[10.5px] text-slate-400 font-mono">
                    current <strong class="text-slate-200">{chk.current_value}</strong>
                    <span class="text-slate-600">→</span>
                    advised <strong class="text-emerald-400">{chk.recommended}</strong>
                  </div>
                  <p class="text-[11px] text-slate-400 font-sans leading-snug">{chk.summary}</p>
                  {#if chk.trade_off}
                    <div class="text-[10.5px] text-amber-300/90 font-sans italic flex items-start gap-1.5">
                      <Icon name="scale" class="w-3 h-3 shrink-0 mt-0.5 text-slate-400" />
                      <span>Trade-off: {chk.trade_off}</span>
                    </div>
                  {/if}
                  {#if chk.remediation}
                    <p class="text-[10.5px] text-slate-400 bg-slate-950 border border-slate-800 rounded px-2 py-1 font-mono break-all">{chk.remediation}</p>
                  {/if}
                  <div class="flex items-center justify-end space-x-2 pt-1 font-sans">
                    {#if chk.remediation}
                      <button
                        type="button"
                        onclick={() => copyWithToast(chk.remediation, 'Remediation snippet')}
                        class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-200 text-[10px] border border-slate-700 transition"
                      >
                        Copy Fix
                      </button>
                    {/if}
                    <button
                      type="button"
                      onclick={() => appState.sendToAi(`Explain cluster parameter "${chk.variable}" (current="${chk.current_value}", recommended="${chk.recommended}", status="${chk.status}"). ${chk.summary || ''}\nTrade-off: ${chk.trade_off || 'N/A'}\nRemediation: ${chk.remediation || 'N/A'}`, `Cluster Tuning: ${chk.variable}`)}
                      class="px-2 py-0.5 rounded bg-purple-950/70 hover:bg-purple-900 text-purple-300 text-[10px] border border-purple-800 transition flex items-center space-x-1"
                    >
                      <Icon name="sparkles" class="w-2.5 h-2.5" />
                      <span>Ask Copilot</span>
                    </button>
                  </div>
                </div>
              {/each}
            </div>
          </div>
        {/if}
      {/if}
    </div>
