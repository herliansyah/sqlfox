<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import type { ProcesslistThread, ProcessLockWait } from '../../../types';
  import Icon from '../common/Icon.svelte';
  import ProcessTrendPanel from './ProcessTrendPanel.svelte';
  import FlightTimeline from './FlightTimeline.svelte';

  type DbaSubTab = 'threads' | 'digest' | 'deadlock' | 'cluster' | 'variables' | 'flight-recorder';
  type FilterChip = 'all' | 'active' | 'sleep' | 'locked' | 'slow';

  let currentSubTab = $state<DbaSubTab>('threads');

  // Header Monitor menu picks the sub-view; this only reacts to that request.
  $effect(() => {
    currentSubTab = appState.processlistSubTab as DbaSubTab;
  });
  let threads = $state<ProcesslistThread[]>([]);
  let locks = $state<ProcessLockWait[]>([]);
  let showBlockerPanel = $state(false);
  let currentThreadId = $state<number>(0);
  let isLoading = $state(false);

  // Column Visibility Management (ADR 0054)
  // `id` and `info` are mandatory: hiding them would leave the table with no
  // meaningful content, and the legacy UI always pinned them visible.
  const MANDATORY_COLUMNS = ['id', 'info'] as const;
  const PL_COLUMN_LABELS: Record<string, string> = {
    id: 'ID',
    user: 'User',
    host: 'Host',
    db: 'Database',
    command: 'Command',
    time: 'Time (s)',
    state: 'State',
    info: 'SQL / Query Info',
  };
  const COLUMN_PREFS_KEY = 'sqlfox-processlist-hidden-cols';

  function loadHiddenColumns(): Record<string, boolean> {
    const defaults: Record<string, boolean> = {
      id: true,
      user: true,
      host: true,
      db: true,
      command: true,
      time: true,
      state: true,
      info: true,
    };
    try {
      const raw = typeof localStorage !== 'undefined' ? localStorage.getItem(COLUMN_PREFS_KEY) : null;
      if (!raw) return defaults;
      const parsed = JSON.parse(raw);
      if (!Array.isArray(parsed)) return defaults;
      // Guard against stale/corrupted prefs: only known, optional columns may be hidden.
      for (const key of parsed) {
        if (key in defaults && !(MANDATORY_COLUMNS as readonly string[]).includes(key)) {
          defaults[key] = false;
        }
      }
    } catch {
      /* ignore malformed prefs */
    }
    return defaults;
  }

  let visibleColumns = $state<Record<string, boolean>>(loadHiddenColumns());
  let showColSelector = $state(false);
  let showTrendPanel = $state(true);

  // Persist only the hidden optional columns, mirroring the legacy storage contract.
  $effect(() => {
    if (typeof localStorage === 'undefined') return;
    const hidden = Object.keys(visibleColumns).filter((k) => !visibleColumns[k]);
    try {
      localStorage.setItem(COLUMN_PREFS_KEY, JSON.stringify(hidden));
    } catch {
      /* storage may be unavailable (private mode / quota) */
    }
  });

  // Threads filtering & selection
  let filterText = $state('');
  let activeChip = $state<FilterChip>('all');
  let selectedThreadIds = $state<Set<number>>(new Set());
  let refreshInterval = $state<number>(0); // 0 = pause, 1000, 2000, 5000
  let intervalTimer: any = null;
  let inspectingThread = $state<ProcesslistThread | null>(null);

  // Sorting: slowest threads first by default, which is what a DBA looks at first.
  type SortKey = 'id' | 'user' | 'host' | 'db' | 'command' | 'time' | 'state' | 'info';
  let sortKey = $state<SortKey>('time');
  let sortDir = $state<'asc' | 'desc'>('desc');

  function toggleSort(key: SortKey) {
    if (sortKey === key) {
      sortDir = sortDir === 'asc' ? 'desc' : 'asc';
    } else {
      sortKey = key;
      sortDir = key === 'time' || key === 'id' ? 'desc' : 'asc';
    }
  }

  // Statement Digest State
  let digestRows = $state<any[]>([]);
  let isDigestLoading = $state(false);
  let digestError = $state('');

  // Deadlock State
  let deadlockData = $state<any>(null);
  let isDeadlockLoading = $state(false);

  // Cluster State
  let clusterReport = $state<any>(null);
  let isClusterLoading = $state(false);

  // Variables State
  let variablesReport = $state<any>(null);
  let isVariablesLoading = $state(false);
  let varFilterText = $state('');
  let varCategory = $state('All');

  // Performance Flight Recorder State
  let isRecording = $state(false);
  let recordingId = $state<string | null>(null);
  let recordingStartedAt = $state<Date | null>(null);
  let recordingSeconds = $state(0);
  let flightIntervalMs = $state(1000);
  let flightSessions = $state<any[]>([]);
  let selectedFlightSessionId = $state<string>('');
  let flightSessionTicks = $state<any[]>([]);
  let selectedTickIndex = $state<number | null>(null);
  let isFlightLoading = $state(false);
  let flightTickerTimer: any = null;
  let flightPollTimer: any = null;

  $effect(() => {
    if (appState.activeSessionId) {
      if (currentSubTab === 'threads') {
        loadProcesslist();
      } else if (currentSubTab === 'digest') {
        loadStatementDigest();
      } else if (currentSubTab === 'deadlock') {
        loadDeadlock();
      } else if (currentSubTab === 'cluster') {
        loadClusterReport();
      } else if (currentSubTab === 'variables') {
        loadVariablesReport();
      } else if (currentSubTab === 'flight-recorder') {
        checkFlightRecorderStatus();
        loadFlightSessions();
      }
    }
  });

  $effect(() => {
    if (intervalTimer) {
      clearInterval(intervalTimer);
      intervalTimer = null;
    }
    if (refreshInterval > 0 && currentSubTab === 'threads') {
      intervalTimer = setInterval(() => {
        loadProcesslist(false);
      }, refreshInterval);
    }
    return () => {
      if (intervalTimer) clearInterval(intervalTimer);
    };
  });

  async function loadProcesslist(showSpinner = true) {
    if (!appState.activeSessionId) return;
    if (showSpinner) isLoading = true;
    try {
      const res = await api.getProcesslist(appState.activeSessionId);
      threads = res.threads || [];
      locks = (res.locks || []) as ProcessLockWait[];
      currentThreadId = res.current_thread_id || 0;
    } catch (err: any) {
      appState.showToast(`Failed to load processlist: ${err.message}`, 'error');
    } finally {
      if (showSpinner) isLoading = false;
    }
  }

  async function loadStatementDigest() {
    if (!appState.activeSessionId) return;
    isDigestLoading = true;
    digestError = '';
    try {
      const res = await api.getStatementDigest(appState.activeSessionId, 30);
      digestRows = res.digest || res.statements || (Array.isArray(res) ? res : []);
    } catch (err: any) {
      digestError = String(err?.message || err).replace(/^Failed to load statement digest:\s*/, '');
      digestRows = [];
    } finally {
      isDigestLoading = false;
    }
  }

  async function handleResetDigest() {
    if (!confirm('Are you sure you want to reset statement digest performance statistics?')) return;
    if (!appState.activeSessionId) return;
    try {
      await api.resetStatementDigest(appState.activeSessionId);
      appState.showToast('Reset statement digest stats', 'success');
      await loadStatementDigest();
    } catch (err: any) {
      appState.showToast(`Failed to reset: ${err.message}`, 'error');
    }
  }

  async function loadDeadlock() {
    if (!appState.activeSessionId) return;
    isDeadlockLoading = true;
    try {
      const res = await api.getDeadlock(appState.activeSessionId);
      deadlockData = res;
    } catch (err: any) {
      appState.showToast(`Failed to inspect deadlock: ${err.message}`, 'error');
    } finally {
      isDeadlockLoading = false;
    }
  }

  async function loadClusterReport() {
    if (!appState.activeSessionId) return;
    isClusterLoading = true;
    try {
      const res = await api.getClusterHealth(appState.activeSessionId);
      clusterReport = res;
    } catch (err: any) {
      clusterReport = null;
    } finally {
      isClusterLoading = false;
    }
  }

  async function loadVariablesReport() {
    if (!appState.activeSessionId) return;
    isVariablesLoading = true;
    try {
      const res = await api.getServerVariables(appState.activeSessionId);
      variablesReport = res;
    } catch (err: any) {
      variablesReport = null;
    } finally {
      isVariablesLoading = false;
    }
  }

  // Performance Flight Recorder Handlers
  async function checkFlightRecorderStatus() {
    if (!appState.activeSessionId) return;
    try {
      const status = await api.getFlightRecorderStatus(appState.activeSessionId);
      isRecording = !!status.is_recording;
      if (status.is_recording) {
        recordingId = status.recording_id;
        recordingStartedAt = status.started_at ? new Date(status.started_at) : new Date();
        recordingSeconds = status.duration_seconds || 0;
        if (!selectedFlightSessionId && Array.isArray(status.recent_ticks)) {
          flightSessionTicks = status.recent_ticks;
        }
        startFlightTimers();
      } else {
        stopFlightTimers();
      }
    } catch (_) {}
  }

  function startFlightTimers() {
    stopFlightTimers();
    flightTickerTimer = setInterval(() => {
      recordingSeconds++;
    }, 1000);
    // Poll every 1000ms for real-time telemetry streaming
    flightPollTimer = setInterval(async () => {
      if (!appState.activeSessionId || !isRecording) return;
      try {
        const status = await api.getFlightRecorderStatus(appState.activeSessionId);
        if (!status.is_recording) {
          isRecording = false;
          stopFlightTimers();
          await loadFlightSessions();
        } else {
          recordingSeconds = status.duration_seconds || recordingSeconds;
          if (!selectedFlightSessionId && Array.isArray(status.recent_ticks)) {
            flightSessionTicks = status.recent_ticks;
          }
        }
      } catch (_) {}
    }, Math.max(800, flightIntervalMs));
  }

  function stopFlightTimers() {
    if (flightTickerTimer) clearInterval(flightTickerTimer);
    if (flightPollTimer) clearInterval(flightPollTimer);
    flightTickerTimer = null;
    flightPollTimer = null;
  }

  async function toggleFlightRecording() {
    if (!appState.activeSessionId) return;
    if (!isRecording) {
      try {
        const res = await api.startFlightRecorder(appState.activeSessionId, flightIntervalMs);
        isRecording = true;
        recordingId = res.recording_id;
        recordingStartedAt = new Date();
        recordingSeconds = 0;
        selectedFlightSessionId = ''; // Enter live recording mode
        flightSessionTicks = [];
        selectedTickIndex = null;
        startFlightTimers();
        appState.showToast('Performance Flight Recorder started (10-min safety guard active)', 'info');
        // Fetch first tick immediately
        setTimeout(async () => {
          try {
            const st = await api.getFlightRecorderStatus(appState.activeSessionId);
            if (st && Array.isArray(st.recent_ticks)) {
              flightSessionTicks = st.recent_ticks;
            }
          } catch (_) {}
        }, 1100);
      } catch (err: any) {
        appState.showToast(`Failed to start flight recorder: ${err.message}`, 'error');
      }
    } else {
      try {
        const finished = await api.stopFlightRecorder(appState.activeSessionId);
        isRecording = false;
        stopFlightTimers();
        appState.showToast('Performance Flight Recorder stopped and saved to session vault', 'success');
        await loadFlightSessions();
        if (finished && finished.id) {
          inspectFlightSession(finished.id);
        }
      } catch (err: any) {
        appState.showToast(`Failed to stop flight recorder: ${err.message}`, 'error');
      }
    }
  }

  async function loadFlightSessions() {
    if (!appState.activeSessionId) return;
    isFlightLoading = true;
    try {
      const sessions = await api.listFlightRecorderSessions(appState.activeSessionId);
      flightSessions = Array.isArray(sessions) ? sessions : [];
      if (flightSessions.length > 0 && !selectedFlightSessionId && !isRecording) {
        inspectFlightSession(flightSessions[0].id);
      }
    } catch (err: any) {
      flightSessions = [];
    } finally {
      isFlightLoading = false;
    }
  }

  async function inspectFlightSession(recId: string) {
    selectedFlightSessionId = recId;
    selectedTickIndex = null;
    if (!recId || !appState.activeSessionId) {
      flightSessionTicks = [];
      return;
    }
    isFlightLoading = true;
    try {
      const res = await api.getFlightRecorderSession(appState.activeSessionId, recId);
      flightSessionTicks = res.ticks || [];
      if (flightSessionTicks.length > 0) {
        selectedTickIndex = flightSessionTicks.length - 1;
      }
    } catch (err: any) {
      appState.showToast(`Failed to load session details: ${err.message}`, 'error');
    } finally {
      isFlightLoading = false;
    }
  }

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

  const flightPeak = $derived.by(() => {
    const t = flightSessionTicks || [];
    if (t.length === 0) return { maxQps: 0, maxThreads: 0, lockDelta: 0 };
    let maxQps = 0;
    let maxThreads = 0;
    let firstLock: number | null = null;
    let lastLock = 0;
    for (const tick of t) {
      maxQps = Math.max(maxQps, Number(tick.qps ?? 0));
      maxThreads = Math.max(maxThreads, Number(tick.threads_running ?? tick.active_threads ?? 0));
      if (firstLock === null && tick.row_lock_waits != null) firstLock = Number(tick.row_lock_waits);
      if (tick.row_lock_waits != null) lastLock = Number(tick.row_lock_waits);
    }
    return {
      maxQps,
      maxThreads,
      lockDelta: Math.max(0, lastLock - (firstLock ?? lastLock)),
    };
  });

  let activeOrSelectedTick = $derived.by(() => {
    if (selectedTickIndex != null && flightSessionTicks[selectedTickIndex]) {
      return flightSessionTicks[selectedTickIndex];
    }
    if (flightSessionTicks.length > 0) {
      return flightSessionTicks[flightSessionTicks.length - 1];
    }
    return null;
  });

  async function deleteFlightSession(recId: string) {
    if (!confirm('Are you sure you want to permanently delete this flight recorder session?')) return;
    if (!appState.activeSessionId) return;
    try {
      await api.deleteFlightRecorderSession(appState.activeSessionId, recId);
      appState.showToast('Flight recorder session deleted', 'info');
      if (selectedFlightSessionId === recId) {
        selectedFlightSessionId = '';
        flightSessionTicks = [];
      }
      await loadFlightSessions();
    } catch (err: any) {
      appState.showToast(`Failed to delete session: ${err.message}`, 'error');
    }
  }

  function exportFlightSessionJSON() {
    if (!flightSessionTicks.length && !selectedFlightSessionId) return;
    const blob = new Blob([JSON.stringify({ session_id: selectedFlightSessionId, ticks: flightSessionTicks }, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `flight-recorder-${selectedFlightSessionId || 'export'}.json`;
    a.click();
    URL.revokeObjectURL(url);
    appState.showToast('Exported flight recording session JSON', 'success');
  }

  async function killSingleThread(id: number) {
    if (!confirm(`Are you sure you want to kill thread ID ${id}?`)) return;
    try {
      await api.killProcess(appState.activeSessionId!, id);
      appState.showToast(`Killed thread ${id}`, 'success');
      selectedThreadIds.delete(id);
      if (inspectingThread?.id === id) inspectingThread = null;
      await loadProcesslist(false);
    } catch (err: any) {
      appState.showToast(`Failed to kill thread: ${err.message}`, 'error');
    }
  }

  async function handleBatchKill() {
    const ids = Array.from(selectedThreadIds);
    if (ids.length === 0) return;
    if (!confirm(`Kill ${ids.length} selected database thread(s)?`)) return;

    try {
      const res = await api.batchKillProcesses(appState.activeSessionId!, ids, 'connection');
      appState.showToast(`Killed ${res.killed_count || ids.length} threads`, 'success');
      selectedThreadIds.clear();
      await loadProcesslist(false);
    } catch (err: any) {
      appState.showToast(`Batch kill failed: ${err.message}`, 'error');
    }
  }

  let rootBlockerCount = $derived.by(
    () => new Set(locks.filter((l) => l.is_root_blocker).map((l) => l.blocking_thread_id)).size
  );

  function inspectLockThread(lock: ProcessLockWait, side: 'blocking' | 'waiting') {
    const id = side === 'blocking' ? lock.blocking_thread_id : lock.waiting_thread_id;
    const existing = threads.find((t) => t.id === id);
    if (existing) {
      inspectingThread = existing;
      return;
    }
    inspectingThread = {
      id,
      user: (side === 'blocking' ? lock.blocking_user : lock.waiting_user) || 'unknown',
      host: '',
      db: '',
      command: '',
      time: side === 'blocking' ? 0 : lock.waiting_seconds || 0,
      state: side === 'blocking' ? `Holding ${lock.lock_mode || 'lock'}` : `Waiting for thread #${lock.blocking_thread_id}`,
      info: (side === 'blocking' ? lock.blocking_query : lock.waiting_query) || '',
    };
  }

  async function killLockThread(id: number, mode: 'query' | 'connection') {
    if (!appState.activeSessionId) return;
    if (!confirm(`Kill ${mode} for thread #${id}?`)) return;
    try {
      await api.batchKillProcesses(appState.activeSessionId, [id], mode);
      appState.showToast(`Killed ${mode} for thread #${id}`, 'success');
      await loadProcesslist(false);
    } catch (err: any) {
      appState.showToast(`Failed to kill thread #${id}: ${err.message}`, 'error');
    }
  }

  function copyText(text: string | undefined, label: string) {
    if (!text) return;
    navigator.clipboard.writeText(text);
    appState.showToast(`${label} copied to clipboard`, 'info');
  }

  let filteredThreads = $derived.by(() => {
    const q = filterText.toLowerCase().trim();
    return threads.filter((t) => {
      if (q) {
        const match =
          t.user?.toLowerCase().includes(q) ||
          t.host?.toLowerCase().includes(q) ||
          t.db?.toLowerCase().includes(q) ||
          t.command?.toLowerCase().includes(q) ||
          t.state?.toLowerCase().includes(q) ||
          t.info?.toLowerCase().includes(q) ||
          String(t.id).includes(q);
        if (!match) return false;
      }

      if (activeChip === 'active') {
        if (t.command === 'Sleep' || t.state === 'Sleep') return false;
      } else if (activeChip === 'sleep') {
        if (t.command !== 'Sleep' && t.state !== 'Sleep') return false;
      } else if (activeChip === 'locked') {
        const isLocked =
          (t.state && t.state.toLowerCase().includes('lock')) ||
          locks.some((l) => l.waiting_thread_id === t.id || l.blocking_thread_id === t.id);
        if (!isLocked) return false;
      } else if (activeChip === 'slow') {
        if (t.time < 5) return false;
      }

      return true;
    });
  });

  let sortedThreads = $derived.by(() => {
    const dir = sortDir === 'asc' ? 1 : -1;
    return [...filteredThreads].sort((a, b) => {
      const av = a[sortKey];
      const bv = b[sortKey];
      if (typeof av === 'number' && typeof bv === 'number') return (av - bv) * dir;
      return (
        String(av ?? '').localeCompare(String(bv ?? ''), undefined, { numeric: true, sensitivity: 'base' }) * dir
      );
    });
  });

  let chipCounts = $derived.by(() => {
    let active = 0;
    let sleep = 0;
    let locked = 0;
    let slow = 0;
    for (const t of threads) {
      if (t.command === 'Sleep' || t.state === 'Sleep') sleep++;
      else active++;
      if (t.time >= 5) slow++;
      if (
        (t.state || '').toLowerCase().includes('lock') ||
        locks.some((l) => l.waiting_thread_id === t.id || l.blocking_thread_id === t.id)
      )
        locked++;
    }
    return { active, sleep, locked, slow };
  });

  const stateTone: Record<string, string> = {
    rose: 'bg-rose-500/10 text-rose-300 border-rose-600/40',
    amber: 'bg-amber-500/10 text-amber-300 border-amber-600/40',
    sky: 'bg-sky-500/10 text-sky-300 border-sky-600/40',
    slate: 'bg-slate-800/60 text-slate-400 border-slate-700/60',
  };

  function commandTone(command: string) {
    const c = (command || '').toLowerCase();
    if (!c || c === 'sleep' || c === 'daemon') return stateTone.slate;
    return stateTone.sky;
  }

  let allFilteredSelected = $derived.by(() => {
    const selectable = filteredThreads.filter((t) => t.id !== currentThreadId);
    if (selectable.length === 0) return false;
    return selectable.every((t) => selectedThreadIds.has(t.id));
  });

  function toggleSelectAll(checked: boolean) {
    const selectable = filteredThreads.filter((t) => t.id !== currentThreadId);
    if (checked) {
      selectable.forEach((t) => selectedThreadIds.add(t.id));
    } else {
      selectable.forEach((t) => selectedThreadIds.delete(t.id));
    }
  }

  function toggleThreadSelection(id: number) {
    if (selectedThreadIds.has(id)) {
      selectedThreadIds.delete(id);
    } else {
      selectedThreadIds.add(id);
    }
  }

  let filteredVariables = $derived.by(() => {
    if (!variablesReport?.variables) return [];
    let list = variablesReport.variables;
    if (varCategory !== 'All') {
      list = list.filter((v: any) => v.category === varCategory);
    }
    if (varFilterText.trim()) {
      const q = varFilterText.toLowerCase();
      list = list.filter(
        (v: any) =>
          v.name.toLowerCase().includes(q) ||
          String(v.value).toLowerCase().includes(q) ||
          (v.description && v.description.toLowerCase().includes(q))
      );
    }
    return list;
  });

  let varCategories = $derived.by(() => {
    if (!variablesReport?.variables) return ['All'];
    const cats = new Set<string>(['All']);
    variablesReport.variables.forEach((v: any) => {
      if (v.category) cats.add(v.category);
    });
    return Array.from(cats);
  });

  function explainThreadQuery(sql?: string) {
    if (!sql) return;
    const tabId = appState.createTab({
      title: 'Explain Plan',
      type: 'worksheet',
      query: `EXPLAIN ${sql};`,
    });
    appState.setActiveTab(tabId);
    appState.executeCurrentTab();
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
    copyText(deadlockRawText(report) || JSON.stringify(report, null, 2), 'Deadlock report');
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

  function sendThreadToAI(sql?: string, thread?: ProcesslistThread | null) {
    const query = (sql || '').trim();
    if (!query) {
      appState.showToast('This thread has no active statement to send', 'warning');
      return;
    }
    const ctx = [
      'Analyse this MySQL thread and its running statement. Explain what it does, why it may be slow or blocking, and propose a concrete fix (index, rewrite, or lock strategy).',
      '',
      thread
        ? `Thread #${thread.id} | user: ${thread.user || '-'} | db: ${thread.db || '-'} | command: ${thread.command || '-'} | time: ${thread.time}s | state: ${thread.state || '-'}`
        : '',
      '```sql',
      query,
      '```',
    ]
      .filter((l, idx) => l !== '' || idx < 2)
      .join('\n');
    appState.sendToAi(ctx);
  }
</script>

  {#snippet th(key: SortKey, label: string, width: string, align: 'left' | 'right' = 'left')}
    <th
      class="h-8 px-2 font-semibold overflow-hidden {width}"
      aria-sort={sortKey === key ? (sortDir === 'asc' ? 'ascending' : 'descending') : 'none'}
    >
      <button
        type="button"
        onclick={() => toggleSort(key)}
        class="group flex w-full items-center gap-1 transition-colors hover:text-slate-200 {align === 'right'
          ? 'justify-end'
          : ''} {sortKey === key ? 'text-sky-300' : ''}"
      >
        <span class="truncate">{label}</span>
        <Icon
          name={sortKey === key && sortDir === 'desc' ? 'arrow-down' : 'arrow-up'}
          class="w-3 h-3 shrink-0 {sortKey === key ? 'text-sky-400' : 'text-slate-600 opacity-0 group-hover:opacity-100'}"
        />
      </button>
    </th>
  {/snippet}


<div class="h-full flex flex-col bg-[#090d16] overflow-hidden select-text font-mono text-xs">
  <!-- Top Navigation & Sub-tabs Bar (Height h-9: 36px - aligned with TabBar and Sidebar) -->
  <div class="h-9 bg-[#0b0f19] border-b border-slate-800 px-4 flex items-center justify-between gap-3 text-xs flex-shrink-0 select-none overflow-hidden">
    <div class="flex items-center space-x-4 min-w-0 flex-1">
      <div class="flex items-center space-x-1.5 font-sans font-bold text-slate-200">
        <Icon name="cpu" class="w-4 h-4 text-sky-400" />
        <span class="shrink-0 whitespace-nowrap">DBA Telemetry</span>
      </div>

      <!-- Sub-Tabs Selector (never collapses: scrolls horizontally when narrow) -->
      <div class="flex items-center space-x-1 bg-[#090d16] p-0.5 rounded-lg border border-slate-700/80 overflow-x-auto flex-1 min-w-0 whitespace-nowrap" style="scrollbar-width: thin">
        <button
          onclick={() => (currentSubTab = 'threads')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 {currentSubTab === 'threads' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Threads ({threads.length})
        </button>
        <button
          onclick={() => (currentSubTab = 'digest')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 {currentSubTab === 'digest' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Statement Digest
        </button>
        <button
          onclick={() => (currentSubTab = 'deadlock')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 {currentSubTab === 'deadlock' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Deadlock Inspector
        </button>
        <button
          onclick={() => (currentSubTab = 'cluster')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 {currentSubTab === 'cluster' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Cluster & Replication
        </button>
        <button
          onclick={() => (currentSubTab = 'variables')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 {currentSubTab === 'variables' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Server Variables
        </button>
        <button
          onclick={() => (currentSubTab = 'flight-recorder')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 flex items-center space-x-1.5 {currentSubTab === 'flight-recorder' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          {#if isRecording}
            <span class="w-2 h-2 rounded-full bg-rose-500 animate-ping"></span>
          {/if}
          <span>Flight Recorder</span>
        </button>
      </div>
    </div>

    <!-- Right Controls for Active SubTab -->
    <div class="flex items-center space-x-3">
      {#if currentSubTab === 'threads'}
        <div class="flex items-center space-x-1.5 bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-[11px]">
          <span class="text-slate-400 font-sans">Refresh:</span>
          <select
            bind:value={refreshInterval}
            class="bg-transparent text-sky-400 font-semibold focus:outline-none cursor-pointer [color-scheme:dark] [&>option]:bg-[#0b1220] [&>option]:text-slate-200"
          >
            <option value={0}>Paused</option>
            <option value={1000}>1s</option>
            <option value={2000}>2s</option>
            <option value={5000}>5s</option>
          </select>
        </div>

        <button
          onclick={() => loadProcesslist(true)}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh thread list"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'digest'}
        <button
          onclick={handleResetDigest}
          class="px-2.5 py-1 rounded bg-rose-950/60 hover:bg-rose-900 border border-rose-800 text-rose-300 text-xs font-semibold transition flex items-center space-x-1 font-sans"
        >
          <Icon name="trash-2" class="w-3.5 h-3.5" />
          <span>Reset Stats</span>
        </button>
        <button
          onclick={loadStatementDigest}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh statement digest"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isDigestLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'deadlock'}
        <button
          onclick={loadDeadlock}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh deadlock report"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isDeadlockLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'cluster'}
        <button
          onclick={loadClusterReport}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh cluster telemetry"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isClusterLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'variables'}
        <button
          onclick={loadVariablesReport}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh server variables"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isVariablesLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'flight-recorder'}
        <div class="flex items-center space-x-2">
          {#if !isRecording}
            <div class="flex items-center space-x-1 bg-[#090d16] border border-[#334155] rounded px-2 py-0.5 text-[11px]">
              <span class="text-slate-400 font-sans">Interval:</span>
              <select
                bind:value={flightIntervalMs}
                class="bg-transparent text-sky-400 font-semibold focus:outline-none cursor-pointer [color-scheme:dark] [&>option]:bg-[#0b1220] [&>option]:text-slate-200"
              >
                <option value={500}>500ms</option>
                <option value={1000}>1s</option>
                <option value={2000}>2s</option>
              </select>
            </div>
          {/if}

          <button
            onclick={toggleFlightRecording}
            class="px-3 py-1 rounded font-bold text-xs font-sans transition flex items-center space-x-1.5 shadow {isRecording ? 'bg-rose-600 hover:bg-rose-500 text-white animate-pulse' : 'bg-emerald-600 hover:bg-emerald-500 text-white'}"
          >
            {#if isRecording}
              <span class="w-2 h-2 rounded-full bg-white"></span>
              <span>STOP ({Math.floor(recordingSeconds / 60).toString().padStart(2, '0')}:{(recordingSeconds % 60).toString().padStart(2, '0')})</span>
            {:else}
              <span class="w-2 h-2 rounded-full bg-rose-400"></span>
              <span>START REC</span>
            {/if}
          </button>

          {#if flightSessionTicks.length > 0}
            <button
              onclick={exportFlightSessionJSON}
              class="px-2.5 py-1 rounded bg-[#090d16] hover:bg-slate-800 border border-[#334155] text-slate-300 text-xs font-sans transition flex items-center space-x-1"
              title="Export flight recording as JSON"
            >
              <Icon name="download" class="w-3.5 h-3.5 text-sky-400" />
              <span>Export JSON</span>
            </button>
          {/if}
        </div>
      {/if}
    </div>
  </div>

  <!-- Content for Threads SubTab -->
  {#if currentSubTab === 'threads'}
    <div class="h-9 bg-[#090d16] border-b border-[#1e293b] px-4 flex items-center justify-between text-xs select-none">
      <div class="flex items-center space-x-1 font-sans text-[11px]">
        <button
          onclick={() => (activeChip = 'all')}
          class="px-2 py-0.5 rounded transition {activeChip === 'all' ? 'bg-sky-500/20 text-sky-300 border border-sky-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
        >
          All ({threads.length})
        </button>
        <button
          onclick={() => (activeChip = 'active')}
          class="px-2 py-0.5 rounded transition {activeChip === 'active' ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
        >
          Active ({chipCounts.active})
        </button>
        <button
          onclick={() => (activeChip = 'sleep')}
          class="px-2 py-0.5 rounded transition {activeChip === 'sleep' ? 'bg-slate-700 text-slate-200 font-bold' : 'text-slate-400 hover:text-slate-200'}"
        >
          Sleep ({chipCounts.sleep})
        </button>
        <button
          onclick={() => (activeChip = 'locked')}
          class="px-2 py-0.5 rounded transition {activeChip === 'locked' ? 'bg-amber-500/20 text-amber-300 border border-amber-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
        >
          Locked ({chipCounts.locked})
        </button>
        <button
          onclick={() => (activeChip = 'slow')}
          class="px-2 py-0.5 rounded transition {activeChip === 'slow' ? 'bg-rose-500/20 text-rose-300 border border-rose-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
        >
          Slow ({chipCounts.slow})
        </button>
      </div>

      <div class="flex items-center space-x-2">
        {#if selectedThreadIds.size > 0}
          <div class="flex items-center space-x-2 bg-rose-950/40 border border-rose-800/80 px-2 py-0.5 rounded text-[11px] animate-in fade-in">
            <span class="text-rose-300 font-semibold">{selectedThreadIds.size} selected</span>
            <button
              onclick={handleBatchKill}
              class="px-2 py-0.5 rounded bg-rose-600 hover:bg-rose-500 text-white font-bold transition flex items-center space-x-1"
            >
              <Icon name="trash-2" class="w-3 h-3" />
              <span>Kill Selected</span>
            </button>
          </div>
        {/if}

        <button
          onclick={() => (showBlockerPanel = true)}
          class="h-7 px-2 rounded border text-[11px] font-sans flex items-center space-x-1 transition {locks.length
            ? 'bg-rose-950/40 border-rose-700/70 text-rose-300 hover:bg-rose-900/50'
            : 'bg-[#0b0f19] border-[#334155] text-slate-400 hover:text-white'}"
          title="Open lock contention and root blocker triage"
        >
          <Icon name="shield-alert" class="w-3 h-3 {locks.length ? 'text-rose-400' : 'text-slate-500'}" />
          <span>Blockers{locks.length ? ` (${locks.length})` : ''}</span>
          {#if rootBlockerCount > 0}
            <span class="px-1 rounded bg-rose-600 text-white text-[9px] font-bold leading-4">{rootBlockerCount} root</span>
          {/if}
        </button>

        <button
          onclick={() => (showTrendPanel = !showTrendPanel)}
          class="h-7 px-2 rounded border border-[#334155] text-[11px] font-sans flex items-center space-x-1 transition {showTrendPanel ? 'bg-sky-500/20 text-sky-300 border-sky-500/40' : 'bg-[#0b0f19] text-slate-400 hover:text-white'}"
          title="Toggle telemetry trend chart"
        >
          <Icon name="activity" class="w-3 h-3 text-sky-400" />
          <span>Trend</span>
        </button>

        <!-- Column Visibility Dropdown (ADR 0054) -->
        <div class="relative">
          <button
            onclick={(e) => { e.stopPropagation(); showColSelector = !showColSelector; }}
            class="h-7 px-2 rounded bg-[#0b0f19] border border-[#334155] text-slate-300 hover:text-white text-[11px] font-sans flex items-center space-x-1 transition"
            title="Manage column visibility"
          >
            <Icon name="columns" class="w-3 h-3 text-sky-400" />
            <span>Columns</span>
            <Icon name="chevron-down" class="w-2.5 h-2.5 text-slate-400" />
          </button>

          {#if showColSelector}
            <button
              tabindex="-1"
              aria-label="Close"
              class="fixed inset-0 z-30 bg-transparent cursor-default"
              onclick={() => (showColSelector = false)}
            ></button>
            <div
              class="absolute right-0 mt-1 w-44 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl z-40 p-2 text-xs font-sans space-y-1 animate-in fade-in duration-100"
            >
              <div class="text-[10px] font-bold text-slate-400 uppercase tracking-wider px-1 pb-1 border-b border-slate-800">
                Visible Columns
              </div>
              {#each Object.keys(visibleColumns) as colKey (colKey)}
                {@const isMandatory = (MANDATORY_COLUMNS as readonly string[]).includes(colKey)}
                <label
                  class="flex items-center justify-between py-1 px-1 rounded text-[11px] {isMandatory
                    ? 'cursor-not-allowed opacity-70'
                    : 'cursor-pointer hover:bg-slate-800 text-slate-300'}"
                >
                  <span class="flex items-center space-x-2">
                    <input
                      type="checkbox"
                      checked={visibleColumns[colKey]}
                      disabled={isMandatory}
                      onchange={(e) => {
                        if (isMandatory) return;
                        visibleColumns[colKey] = (e.target as HTMLInputElement).checked;
                      }}
                      class="rounded bg-[#090d16] border-[#334155] text-sky-500 focus:ring-0"
                    />
                    <span class="truncate {visibleColumns[colKey] ? 'text-slate-200' : 'text-slate-500 line-through'}">
                      {PL_COLUMN_LABELS[colKey] ?? colKey}
                    </span>
                  </span>
                  {#if isMandatory}
                    <span class="text-[9px] font-bold text-amber-400/90" title="Mandatory column cannot be hidden">LOCKED</span>
                  {/if}
                </label>
              {/each}
            </div>
          {/if}
        </div>

        <input
          type="text"
          bind:value={filterText}
          placeholder="Filter thread list..."
          class="bg-[#0b0f19] border border-[#334155] rounded px-2.5 py-1 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 w-44 font-sans"
        />
      </div>
    </div>

    <!-- Active Threads Telemetry Trend (ADR 0054) -->
    {#if showTrendPanel}
      <div class="border-b border-[#1e293b] shrink-0 bg-[#070b12]">
        <ProcessTrendPanel {threads} />
      </div>
    {/if}

        <!-- Threads Table View -->
    <div class="flex-1 overflow-auto">
      <table class="w-full min-w-[1120px] border-collapse table-fixed text-left">
        <colgroup>
          <col class="w-9" />
          {#if visibleColumns.id}<col class="w-16" />{/if}
          {#if visibleColumns.user}<col class="w-32" />{/if}
          {#if visibleColumns.host}<col class="w-40" />{/if}
          {#if visibleColumns.db}<col class="w-32" />{/if}
          {#if visibleColumns.command}<col class="w-28" />{/if}
          {#if visibleColumns.time}<col class="w-20" />{/if}
          {#if visibleColumns.state}<col class="w-44" />{/if}
          {#if visibleColumns.info}<col />{/if}
          <col class="w-[120px]" />
        </colgroup>

        <thead
          class="sticky top-0 z-20 bg-[#0f172a] text-[10px] font-sans uppercase tracking-wider text-slate-400 select-none border-b border-slate-700/80 shadow-[0_1px_0_0_rgba(15,23,42,0.9)]"
        >
          <tr>
            <th class="h-8 px-2 text-center">
              <input
                type="checkbox"
                aria-label="Select all threads"
                checked={allFilteredSelected}
                onchange={(e) => toggleSelectAll((e.target as HTMLInputElement).checked)}
                class="rounded border-slate-600 bg-transparent text-sky-500 focus:ring-0 focus:ring-offset-0 align-middle"
              />
            </th>
            {#if visibleColumns.id}{@render th('id', 'ID', 'w-16', 'right')}{/if}
            {#if visibleColumns.user}{@render th('user', 'User', 'w-32')}{/if}
            {#if visibleColumns.host}{@render th('host', 'Host', 'w-40')}{/if}
            {#if visibleColumns.db}{@render th('db', 'DB', 'w-32')}{/if}
            {#if visibleColumns.command}{@render th('command', 'Command', 'w-28')}{/if}
            {#if visibleColumns.time}{@render th('time', 'Time', 'w-20', 'right')}{/if}
            {#if visibleColumns.state}{@render th('state', 'State', 'w-44')}{/if}
            {#if visibleColumns.info}{@render th('info', 'Query', '')}{/if}
            <th
              class="h-8 px-2 w-[120px] text-center font-semibold sticky right-0 bg-[#0f172a] border-l border-slate-800"
            >
              <span class="sr-only">Actions</span>
            </th>
          </tr>
        </thead>

        <tbody class="font-mono text-[11.5px]">
          {#if filteredThreads.length === 0}
            <tr>
              <td colspan={Object.values(visibleColumns).filter(Boolean).length + 2} class="py-16 text-center">
                <div class="flex flex-col items-center gap-2 font-sans">
                  <Icon
                    name={isLoading ? 'loader' : 'check-circle'}
                    class="w-6 h-6 {isLoading ? 'animate-spin text-sky-400' : 'text-slate-700'}"
                  />
                  <span class="text-slate-500">
                    {isLoading ? 'Loading threads...' : 'No threads match the current filter'}
                  </span>
                </div>
              </td>
            </tr>
          {:else}
            {#each sortedThreads as thread (thread.id)}
              {@const isSelf = thread.id === currentThreadId}
              {@const isSelected = selectedThreadIds.has(thread.id)}
              {@const isSlow = thread.time >= 5}
              {@const isRootBlocker = locks.some(
                (l) => l.is_root_blocker && l.blocking_thread_id === thread.id
              )}
              {@const isWaitingOnLock = locks.some((l) => l.waiting_thread_id === thread.id)}
              {@const rowBg =
                (isRootBlocker
                  ? 'bg-red-950/40 hover:bg-red-950/50 border-l-red-500 font-semibold'
                  : isWaitingOnLock
                    ? 'bg-amber-950/20 hover:bg-amber-950/30 border-l-amber-600'
                    : isSelected
                      ? 'bg-rose-950/25 hover:bg-rose-950/35 border-l-transparent'
                      : isSelf
                        ? 'bg-sky-950/20 hover:bg-sky-950/30 border-l-transparent'
                        : isSlow
                          ? 'bg-amber-950/20 hover:bg-amber-950/30 border-l-amber-500'
                          : 'hover:bg-slate-800/40 border-l-transparent')}
              {@const actionBg = isRootBlocker
                ? 'bg-[#2a0a0a]'
                : isWaitingOnLock
                ? 'bg-[#1f1a0a]'
                : isSelected
                ? 'bg-rose-950/25'
                : isSelf
                ? 'bg-sky-950/20'
                : 'bg-[#090d16]'}
              <tr
                class="group h-8 cursor-pointer border-b border-slate-800/50 border-l-4 transition-colors {rowBg}"
                onclick={() => (inspectingThread = thread)}
              >
                <td class="px-2 text-center overflow-hidden" onclick={(e) => e.stopPropagation()}>
                  {#if !isSelf}
                    <input
                      type="checkbox"
                      aria-label="Select thread {thread.id}"
                      checked={isSelected}
                      onchange={() => toggleThreadSelection(thread.id)}
                      class="rounded border-slate-600 bg-transparent text-rose-500 focus:ring-0 focus:ring-offset-0 align-middle"
                    />
                  {/if}
                </td>
                {#if visibleColumns.id}
                  <td class="px-2 text-right whitespace-nowrap overflow-hidden text-ellipsis tabular-nums font-bold {isSelf ? 'text-sky-400' : 'text-slate-300'}">
                    {thread.id}
                    {#if isRootBlocker}
                      <span
                        class="ml-1 rounded bg-rose-600 px-1.5 py-0.5 font-sans text-[9px] font-bold text-white"
                        title="Root blocker holding locks for other threads"
                      >
                        BLOCKER
                      </span>
                    {:else if isWaitingOnLock}
                      <span
                        class="ml-1 rounded border border-amber-800 bg-amber-950 px-1.5 py-0.5 font-sans text-[9px] font-bold text-amber-300"
                        title="Thread is waiting on a lock"
                      >
                        WAITING
                      </span>
                    {/if}
                    {#if isSelf}
                      <span class="ml-1 font-sans text-[9px] font-normal uppercase text-sky-500">you</span>
                    {/if}
                  </td>
                {/if}
                {#if visibleColumns.user}
                  <td class="px-2 text-slate-200 truncate overflow-hidden" title={thread.user}>{thread.user || '-'}</td>
                {/if}
                {#if visibleColumns.host}
                  <td class="px-2 text-slate-400 truncate overflow-hidden" title={thread.host}>{thread.host || '-'}</td>
                {/if}
                {#if visibleColumns.db}
                  <td class="px-2 text-emerald-400/90 truncate overflow-hidden" title={thread.db}>{thread.db || '-'}</td>
                {/if}
                {#if visibleColumns.command}
                  <td class="px-2 overflow-hidden">
                    <span
                      class="inline-block max-w-full truncate rounded border px-1.5 py-0.5 font-sans text-[10px] leading-tight {commandTone(
                        thread.command
                      )}"
                    >
                      {thread.command || '-'}
                    </span>
                  </td>
                {/if}
                {#if visibleColumns.time}
                  <td
                    class="px-2 text-right tabular-nums {isSlow
                      ? 'font-bold text-rose-400'
                      : thread.time >= 1
                      ? 'text-amber-300'
                      : 'text-slate-400'}"
                    title={isSlow ? 'Running longer than 5 seconds' : ''}
                  >
                    {thread.time.toFixed(1)}
                  </td>
                {/if}
                {#if visibleColumns.state}
                  <td class="px-2 truncate text-slate-400 overflow-hidden" title={thread.state}>{thread.state || '-'}</td>
                {/if}
                {#if visibleColumns.info}
                  <td class="px-2 truncate text-slate-300 group-hover:text-sky-300 overflow-hidden" title={thread.info}>
                    {thread.info || '-'}
                  </td>
                {/if}
                <td
                  class="px-2 text-center whitespace-nowrap overflow-hidden sticky right-0 border-l border-slate-800/60 transition-colors {actionBg} group-hover:bg-slate-800/40"
                  onclick={(e) => e.stopPropagation()}
                >
                  {#if !isSelf}
                    <button
                      type="button"
                      onclick={() => killSingleThread(thread.id)}
                      class="rounded p-1 text-slate-500 transition hover:bg-rose-950/60 hover:text-rose-400"
                      title="Kill thread {thread.id}"
                      aria-label="Kill thread {thread.id}"
                    >
                      <Icon name="trash-2" class="w-3.5 h-3.5" />
                    </button>
                  {/if}
                </td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  {:else if currentSubTab === 'digest'}
    <!-- Statement Digest View -->
    <div class="flex-1 overflow-auto">
      <table class="w-full text-left border-collapse">
        <thead class="bg-[#0f172a] sticky top-0 z-10 border-b border-[#334155] text-slate-400 text-[11px] uppercase tracking-wider font-sans select-none">
          <tr>
            <th class="py-2 px-3 w-32">Database</th>
            <th class="py-2 px-3">Normalized Statement</th>
            <th class="py-2 px-3 w-24 text-right">Exec Count</th>
            <th class="py-2 px-3 w-28 text-right">Total Latency</th>
            <th class="py-2 px-3 w-28 text-right">Avg Latency</th>
            <th class="py-2 px-3 w-28 text-right">Max Latency</th>
            <th class="py-2 px-3 w-28 text-right">Rows Examined</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[#1e293b]/60">
          {#if isDigestLoading}
            <tr>
              <td colspan="7" class="p-8 text-center text-slate-500">
                Aggregating slow query digest from performance_schema...
              </td>
            </tr>
          {:else if digestError}
            <tr>
              <td colspan="7" class="p-8">
                <div class="flex flex-col items-center gap-2 text-center">
                  <Icon name="alert-triangle" class="w-5 h-5 text-amber-400" />
                  <p class="text-sm font-semibold text-amber-300">Statement digest unavailable</p>
                  <p class="text-xs text-slate-400 max-w-2xl font-mono break-words">{digestError}</p>
                  <button
                    type="button"
                    onclick={loadStatementDigest}
                    class="mt-1 px-3 py-1 rounded bg-slate-800 hover:bg-slate-700 border border-slate-700 text-slate-200 text-xs font-semibold"
                  >
                    Retry
                  </button>
                </div>
              </td>
            </tr>
          {:else if digestRows.length === 0}
            <tr>
              <td colspan="7" class="p-8 text-center text-slate-500 space-y-1">
                <p class="text-sm">No statement digests recorded yet on this server.</p>
                <p class="text-[11px] text-slate-600">
                  performance_schema is enabled but has not aggregated any statement yet — run some
                  traffic, then hit refresh.
                </p>
              </td>
            </tr>
          {:else}
            {#each digestRows as row}
              <tr class="hover:bg-slate-800/40 transition-colors">
                <td class="py-2 px-3 text-emerald-400 font-semibold">{row.schema_name || row.db || '-'}</td>
                <td class="py-2 px-3 text-slate-200 font-mono text-[11px] truncate max-w-md" title={row.digest_text || row.query}>
                  {row.digest_text || row.query || '-'}
                </td>
                <td class="py-2 px-3 text-right font-bold text-sky-300">{row.count_star || row.exec_count || 0}</td>
                <td class="py-2 px-3 text-right text-slate-300">{row.sum_timer_wait || row.total_time || '-'}</td>
                <td class="py-2 px-3 text-right font-semibold text-amber-300">{row.avg_timer_wait || row.avg_latency || '-'}</td>
                <td class="py-2 px-3 text-right font-bold text-rose-400">{row.max_timer_wait || row.max_latency || '-'}</td>
                <td class="py-2 px-3 text-right text-slate-400">{row.sum_rows_examined || row.rows_examined || 0}</td>
              </tr>
            {/each}
          {/if}
        </tbody>
      </table>
    </div>
  {:else if currentSubTab === 'deadlock'}
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
            onclick={loadDeadlock}
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
              onclick={loadDeadlock}
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
                      <button
                        type="button"
                        onclick={() => copyText(tx.query, 'Statement')}
                        class="text-[10px] text-sky-400 hover:underline font-sans flex items-center gap-1"
                      >
                        <Icon name="copy" class="w-3 h-3" />
                        <span>Copy</span>
                      </button>
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
  {:else if currentSubTab === 'cluster'}
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
              <span class="text-[13px] font-bold font-mono truncate block {card.tone}" title={card.title || card.value}>
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
                </tr>
              </thead>
              <tbody class="divide-y divide-[#1e293b]/60">
                {#each clusterReport.nodes as node}
                  <tr class="hover:bg-slate-800/40 {node.is_culprit ? 'bg-amber-950/20' : ''}">
                    <td class="py-1.5 px-3 font-bold text-sky-300 truncate max-w-[12rem]" title={node.name}>
                      {node.name || '(unnamed)'}
                    </td>
                    <td class="py-1.5 px-3 text-slate-400">{node.address || '-'}</td>
                    <td class="py-1.5 px-3">
                      <span class="px-1.5 py-0.5 rounded text-[10px] font-bold font-sans uppercase border {nodeStateTone(node.state)}">
                        {node.state || 'unknown'}
                      </span>
                    </td>
                    <td class="py-1.5 px-3 text-right {node.receive_queue > 0 ? 'text-amber-400 font-bold' : 'text-slate-300'}">
                      {node.receive_queue ?? 0}
                    </td>
                    <td class="py-1.5 px-3 text-right {node.send_queue > 0 ? 'text-amber-400 font-bold' : 'text-slate-300'}">
                      {node.send_queue ?? 0}
                    </td>
                    <td class="py-1.5 px-3 text-right text-slate-300">{(node.flow_control_sent ?? 0).toLocaleString()}</td>
                    <td class="py-1.5 px-3">
                      <div class="flex items-center gap-2">
                        <div class="h-1.5 flex-1 rounded-full bg-slate-800 overflow-hidden">
                          {#if nodeQueueTotal(node) > 0}
                            <div
                              class="h-full rounded-full {node.is_culprit || nodeQueueTotal(node) > 1000 ? 'bg-amber-500' : 'bg-sky-500'}"
                              style="width: {queueWidth(node)}%"
                              title="receive queue {node.receive_queue ?? 0} / send queue {node.send_queue ?? 0}"
                            ></div>
                          {/if}
                        </div>
                        <span class="text-[10px] font-mono {nodeQueueTotal(node) > 1000 ? 'text-amber-400' : 'text-slate-500'} w-10 text-right">
                          {nodeQueueTotal(node) > 0 ? nodeQueueTotal(node).toLocaleString() : 'clear'}
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
              Flow Control &amp; Durability
            </div>
            <div class="p-3 space-y-2.5">
              <div>
                <div class="flex items-center justify-between text-[11px] font-sans">
                  <span class="text-slate-400">Flow control paused</span>
                  <span class="font-mono {fcTone}">{flowControlPct}%</span>
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
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">Cert failures</dt><dd class={clusterReport.tri_metric?.cert_failures ? 'text-rose-300' : 'text-slate-200'}>{(clusterReport.tri_metric?.cert_failures ?? 0).toLocaleString()}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">BF aborts</dt><dd class={clusterReport.tri_metric?.bf_aborts ? 'text-amber-300' : 'text-slate-200'}>{(clusterReport.tri_metric?.bf_aborts ?? 0).toLocaleString()}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">Log waits</dt><dd class={clusterReport.tri_metric?.log_waits ? 'text-amber-300' : 'text-slate-200'}>{(clusterReport.tri_metric?.log_waits ?? 0).toLocaleString()}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">FSyncs</dt><dd class="text-slate-200">{(clusterReport.tri_metric?.os_log_fsyncs ?? 0).toLocaleString()}</dd></div>
                <div class="flex justify-between"><dt class="text-slate-500 font-sans">Net latency avg</dt><dd class="text-slate-200">{netLatency}</dd></div>
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
                <div class="p-2.5 bg-[#0b0f19] border border-[#1e293b] rounded-lg space-y-1">
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
                  <p class="text-[11px] text-slate-500 font-sans leading-snug">{chk.summary}</p>
                  {#if chk.remediation}
                    <p class="text-[10.5px] text-slate-600 font-mono truncate" title={chk.remediation}>{chk.remediation}</p>
                  {/if}
                </div>
              {/each}
            </div>
          </div>
        {/if}
      {/if}
    </div>
  {:else if currentSubTab === 'variables'}
    <!-- Server Variables View -->
    <div class="flex-1 flex flex-col overflow-hidden">
      <!-- Variables Filter Bar -->
      <div class="h-9 bg-[#090d16] border-b border-[#1e293b] px-4 flex items-center justify-between text-xs select-none">
        <div class="flex items-center space-x-2">
          <span class="text-slate-400 font-sans text-xs">Category:</span>
          <select
            bind:value={varCategory}
            class="bg-[#0b0f19] border border-[#334155] rounded px-2 py-0.5 text-xs text-sky-400 font-semibold focus:outline-none cursor-pointer [color-scheme:dark] [&>option]:bg-[#0b1220] [&>option]:text-slate-200"
          >
            {#each varCategories as cat}
              <option value={cat}>{cat}</option>
            {/each}
          </select>
        </div>

        <input
          type="text"
          bind:value={varFilterText}
          placeholder="Search variables by name or value..."
          class="bg-[#0b0f19] border border-[#334155] rounded px-2.5 py-1 text-xs text-slate-200 placeholder-slate-500 focus:outline-none focus:border-sky-500 w-56 font-sans"
        />
      </div>

      <div class="flex-1 overflow-auto">
        <table class="w-full text-left border-collapse text-xs">
          <thead class="bg-[#0f172a] sticky top-0 z-10 border-b border-[#334155] text-slate-400 text-[11px] uppercase tracking-wider font-sans select-none">
            <tr>
              <th class="py-2 px-3 w-64">Variable Name</th>
              <th class="py-2 px-3 w-48">Value</th>
              <th class="py-2 px-3 w-28">Category</th>
              <th class="py-2 px-3 w-20 text-center">Dynamic</th>
              <th class="py-2 px-3">Tuning Recommendation</th>
              <th class="py-2 px-3 w-20 text-center">Snippet</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-[#1e293b]/60">
            {#if isVariablesLoading}
              <tr>
                <td colspan="6" class="p-8 text-center text-slate-500">
                  Inspecting server variables...
                </td>
              </tr>
            {:else if filteredVariables.length === 0}
              <tr>
                <td colspan="6" class="p-8 text-center text-slate-500">
                  No server variables match your filter.
                </td>
              </tr>
            {:else}
              {#each filteredVariables as v}
                <tr class="hover:bg-slate-800/40">
                  <td class="py-2 px-3 font-mono font-bold text-sky-300">{v.name}</td>
                  <td class="py-2 px-3 font-mono text-slate-200 truncate max-w-[200px]" title={String(v.value)}>{v.value}</td>
                  <td class="py-2 px-3 text-slate-400">{v.category || 'General'}</td>
                  <td class="py-2 px-3 text-center">
                    {#if v.is_dynamic}
                      <span class="px-1.5 py-0.2 rounded bg-emerald-950 text-emerald-300 border border-emerald-800 text-[10px] font-bold">YES</span>
                    {:else}
                      <span class="text-slate-600 text-[10px]">RESTART</span>
                    {/if}
                  </td>
                  <td class="py-2 px-3 text-slate-400 text-[11px] font-sans">
                    {#if v.recommendation || v.suggested_value}
                      <span class="text-amber-300 font-semibold">{v.suggested_value ? `Suggested: ${v.suggested_value}` : ''}</span>
                      <span class="text-slate-400 block">{v.recommendation || v.description}</span>
                    {:else}
                      <span class="text-slate-600">-</span>
                    {/if}
                  </td>
                  <td class="py-2 px-3 text-center">
                    {#if v.set_global_snippet}
                      <button
                        onclick={() => {
                          navigator.clipboard.writeText(v.set_global_snippet);
                          appState.showToast('Copied SET GLOBAL statement to clipboard!', 'info');
                        }}
                        class="p-1 rounded text-sky-400 hover:text-white hover:bg-slate-800 transition"
                        title="Copy SET GLOBAL snippet"
                      >
                        <Icon name="copy" class="w-3.5 h-3.5" />
                      </button>
                    {:else}
                      <span class="text-slate-600">-</span>
                    {/if}
                  </td>
                </tr>
              {/each}
            {/if}
          </tbody>
        </table>
      </div>
    </div>
  {:else if currentSubTab === 'flight-recorder'}
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
                Performance Flight Recorder captures a blackbox audit of database telemetry (threads, QPS, row locks, buffer stalls, active SQL) sampled every 500ms-2s. Click <strong>START REC</strong> to capture real-time behavior during load tests or incidents.
              </p>
            </div>
            <button
              type="button"
              onclick={toggleFlightRecording}
              class="px-4 py-2 rounded-lg bg-emerald-600 hover:bg-emerald-500 text-white font-bold text-xs transition flex items-center space-x-1.5 shadow-lg shadow-emerald-950/40"
            >
              <Icon name="play" class="w-4 h-4" />
              <span>Start Performance Recording</span>
            </button>
          </div>
        {:else}
          <!-- Session Summary Banner -->
          <div class="grid grid-cols-2 md:grid-cols-5 gap-3 font-sans text-xs">
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
              <span class="text-[10px] text-slate-500 font-sans">over session · counter {activeOrSelectedTick?.row_lock_waits ?? 0}</span>
            </div>
            <div class="p-3 rounded-xl bg-[#0b0f19] border border-[#1e293b]">
              <span class="text-slate-500 text-[10px] uppercase font-bold block">Peak load</span>
              <span class="text-sm font-bold text-slate-100 font-mono">
                {flightPeak.maxQps} qps peak
              </span>
              <span class="text-[10px] text-slate-500 font-sans">
                {flightPeak.maxThreads} active threads peak · {flightSessionTicks.length} samples
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
                        onclick={() => (selectedTickIndex = idx)}
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
                  <span class="px-1.5 py-0.2 rounded-full text-[10px] font-mono bg-sky-950 text-sky-300 border border-sky-800">
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
                    <div class="p-2.5 rounded-lg border border-slate-800 bg-[#050811] space-y-1.5 font-sans">
                      <div class="flex items-center justify-between text-[11px]">
                        <div class="flex items-center space-x-2 font-mono">
                          <span class="text-slate-500">#{qIdx + 1}</span>
                          {#if q.id}<span class="text-slate-400">ID: {q.id}</span>{/if}
                          {#if q.user}<span class="text-slate-400">User: {q.user}</span>{/if}
                          {#if q.db}<span class="text-sky-400 font-bold">DB: {q.db}</span>{/if}
                          {#if q.state}<span class="text-amber-400">State: {q.state}</span>{/if}
                          {#if q.time != null}<span class="text-slate-500 font-semibold">{q.time}s</span>{/if}
                        </div>
                        <div class="flex items-center space-x-1.5">
                          <button
                            onclick={() => {
                              navigator.clipboard.writeText(q.info || q.query);
                              appState.showToast('SQL copied to clipboard!', 'info');
                            }}
                            class="px-2 py-0.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-[10px] transition flex items-center space-x-1"
                            title="Copy Query SQL"
                          >
                            <Icon name="copy" class="w-3 h-3" />
                            <span>Copy</span>
                          </button>
                          <button
                            onclick={() => {
                              appState.addTab('Flight Query.sql', q.info || q.query);
                              appState.showToast('SQL inserted into worksheet!', 'success');
                            }}
                            class="px-2 py-0.5 rounded bg-sky-950 hover:bg-sky-900 border border-sky-800 text-sky-300 text-[10px] transition flex items-center space-x-1"
                            title="Open Query in Worksheet"
                          >
                            <Icon name="play" class="w-2.5 h-2.5" />
                            <span>Worksheet</span>
                          </button>
                          <button
                            onclick={() => appState.saveQueryToNote(q.info || q.query, `Forensic Query (${formatTickTime(activeOrSelectedTick)})`)}
                            class="px-2 py-0.5 rounded bg-amber-950 hover:bg-amber-900 border border-amber-800 text-amber-300 text-[10px] transition flex items-center space-x-1"
                            title="Save Query to Operational Notes"
                          >
                            <Icon name="bookmark" class="w-2.5 h-2.5" />
                            <span>Note</span>
                          </button>
                        </div>
                      </div>
                      <pre class="p-2 rounded bg-[#090d16] border border-slate-900 text-sky-200 font-mono text-[11px] overflow-x-auto whitespace-pre-wrap select-text">{q.info || q.query}</pre>
                    </div>
                  {/each}
                {/if}
              </div>
            </div>
          {/if}
        {/if}
      </div>
    </div>
  {/if}
</div>

<!-- Thread Detail Modal -->
{#if inspectingThread}
  <div class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-[#1e293b] rounded-2xl w-full max-w-xl shadow-2xl overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <div class="px-5 py-4 border-b border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2">
          <Icon name="cpu" class="w-4 h-4 text-sky-400" />
          <h3 class="font-bold text-slate-100 text-sm">Thread Inspector (ID: {inspectingThread.id})</h3>
        </div>
        <button onclick={() => (inspectingThread = null)} class="text-slate-400 hover:text-white">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-4">
        <div class="grid grid-cols-3 gap-3">
          <div class="p-2.5 bg-[#050811] border border-[#1e293b] rounded-lg">
            <span class="text-slate-500 text-[10px] uppercase block">User @ Host</span>
            <span class="font-bold text-slate-200 truncate block">{inspectingThread.user}@{inspectingThread.host}</span>
          </div>
          <div class="p-2.5 bg-[#050811] border border-[#1e293b] rounded-lg">
            <span class="text-slate-500 text-[10px] uppercase block">Database</span>
            <span class="font-bold text-emerald-400 truncate block">{inspectingThread.db || '-'}</span>
          </div>
          <div class="p-2.5 bg-[#050811] border border-[#1e293b] rounded-lg">
            <span class="text-slate-500 text-[10px] uppercase block">Execution Time</span>
            <span class="font-bold {inspectingThread.time > 5 ? 'text-rose-400' : 'text-slate-200'}">{inspectingThread.time} sec</span>
          </div>
        </div>

        <div>
          <div class="flex items-center justify-between mb-1.5">
            <span class="text-slate-400 text-xs font-medium">SQL Statement</span>
            <div class="flex items-center space-x-2">
              <button
                onclick={() => {
                  navigator.clipboard.writeText(inspectingThread?.info || '');
                  appState.showToast('Thread SQL copied to clipboard', 'info');
                }}
                class="text-[11px] text-sky-400 hover:underline flex items-center space-x-1"
              >
                <Icon name="copy" class="w-3 h-3" />
                <span>Copy SQL</span>
              </button>
              <button
                onclick={() => explainThreadQuery(inspectingThread?.info)}
                class="text-[11px] text-emerald-400 hover:underline flex items-center space-x-1"
              >
                <Icon name="play" class="w-3 h-3" />
                <span>Explain Query</span>
              </button>
              <button
                onclick={() => sendThreadToAI(inspectingThread?.info, inspectingThread)}
                class="text-[11px] text-purple-400 hover:underline flex items-center space-x-1"
              >
                <Icon name="bot" class="w-3 h-3" />
                <span>Send to AI</span>
              </button>
            </div>
          </div>
          <pre class="p-3 bg-[#050811] border border-[#1e293b] rounded-xl text-sky-300 font-mono text-xs overflow-x-auto whitespace-pre-wrap max-h-48">{inspectingThread.info || '(No active query statement)'}</pre>
        </div>
      </div>

      <div class="px-5 py-3 border-t border-[#1e293b] flex items-center justify-between bg-[#0f172a]">
        <button
          onclick={() => killSingleThread(inspectingThread!.id)}
          class="px-3.5 py-1.5 rounded bg-rose-600 hover:bg-rose-500 text-white text-xs font-bold transition flex items-center space-x-1.5 shadow-lg shadow-rose-950/30"
        >
          <Icon name="trash-2" class="w-3.5 h-3.5" />
          <span>Kill Thread</span>
        </button>

        <button
          onclick={() => (inspectingThread = null)}
          class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 text-xs font-medium transition"
        >
          Close
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Lock Contention & Root Blocker Triage -->
{#if showBlockerPanel}
  <div
    class="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) showBlockerPanel = false;
    }}
  >
    <div class="bg-[#0b0f19] border border-[#334155] rounded-xl w-full max-w-4xl max-h-[86vh] shadow-2xl overflow-hidden flex flex-col text-xs font-sans">
      <div class="px-5 py-3.5 border-b border-[#334155] flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2.5">
          <Icon name="shield-alert" class="w-4 h-4 {rootBlockerCount > 0 ? 'text-rose-400' : 'text-slate-400'}" />
          <div>
            <h3 class="font-bold text-slate-100 text-sm">Lock Contention &amp; Root Blocker Triage</h3>
            <p class="text-[11px] text-slate-500 mt-0.5">
              {rootBlockerCount > 0
                ? `${rootBlockerCount} root blocker(s) holding transactions`
                : locks.length > 0
                  ? `${locks.length} lock wait(s), no root blocker`
                  : 'No active lock contention'}
            </p>
          </div>
        </div>
        <div class="flex items-center space-x-2">
          <span class="px-2 py-0.5 rounded border text-[10px] font-bold {rootBlockerCount > 0
            ? 'bg-rose-950/60 border-rose-700 text-rose-300'
            : 'bg-slate-900 border-slate-700 text-slate-400'}">
            {rootBlockerCount > 0 ? `${rootBlockerCount} ROOT BLOCKER${rootBlockerCount > 1 ? 'S' : ''}` : `${locks.length} LOCK WAITS`}
          </span>
          <button onclick={() => (showBlockerPanel = false)} class="p-1 rounded text-slate-400 hover:text-white hover:bg-slate-800 transition" title="Close">
            <Icon name="x" class="w-4 h-4" />
          </button>
        </div>
      </div>

      <div class="flex-1 overflow-y-auto p-4 space-y-3">
        {#if locks.length === 0}
          <div class="py-10 text-center space-y-2">
            <div class="w-10 h-10 mx-auto rounded-full bg-emerald-500/10 border border-emerald-600/30 flex items-center justify-center text-emerald-400">
              <Icon name="check-circle" class="w-5 h-5" />
            </div>
            <div class="font-bold text-slate-200">No Active Lock Contention</div>
            <div class="text-slate-500 text-[11px]">All threads and transactions are running unobstructed.</div>
          </div>
        {:else}
          {#each locks as lock, idx (idx)}
            {@const isRoot = !!lock.is_root_blocker}
            <div class="rounded-lg border {isRoot ? 'border-rose-800 bg-rose-950/25' : 'border-slate-800 bg-[#0f172a]'}">
              <div class="px-3.5 py-2.5 flex flex-wrap items-center justify-between gap-2 border-b {isRoot ? 'border-rose-900/60' : 'border-slate-800'}">
                <div class="flex flex-wrap items-center gap-2 min-w-0">
                  {#if isRoot}
                    <span class="px-1.5 py-0.5 rounded bg-rose-600 text-white text-[10px] font-bold tracking-wide flex items-center space-x-1">
                      <Icon name="alert-triangle" class="w-3 h-3" />
                      <span>ROOT BLOCKER</span>
                    </span>
                  {:else}
                    <span class="px-1.5 py-0.5 rounded bg-amber-950/60 border border-amber-800/70 text-amber-300 text-[10px] font-bold">
                      LOCK WAIT
                    </span>
                  {/if}
                  <span class="font-bold text-slate-100">Thread #{lock.blocking_thread_id}</span>
                  <span class="text-slate-500">{lock.blocking_user || 'unknown'}</span>
                  <span class="px-1.5 py-0.5 rounded bg-amber-950/50 border border-amber-900/70 text-amber-300 text-[10px] font-mono">
                    holding {lock.lock_mode || 'LOCK'}
                  </span>
                  {#if lock.locked_resource}
                    <span class="px-1.5 py-0.5 rounded bg-sky-950/50 border border-sky-900/70 text-sky-300 text-[10px] font-mono truncate max-w-[18rem]" title={lock.locked_resource}>
                      on {lock.locked_resource}
                    </span>
                  {/if}
                </div>
                <div class="flex items-center space-x-1.5 shrink-0">
                  <button
                    onclick={() => inspectLockThread(lock, 'blocking')}
                    class="px-2 py-1 rounded border border-slate-700 bg-slate-900 hover:bg-slate-800 text-slate-300 transition flex items-center space-x-1"
                  >
                    <Icon name="search" class="w-3 h-3 text-sky-400" />
                    <span>Detail</span>
                  </button>
                  <button
                    onclick={() => killLockThread(lock.blocking_thread_id, 'query')}
                    class="px-2 py-1 rounded border border-amber-700/70 bg-amber-950/50 hover:bg-amber-900/60 text-amber-300 font-semibold transition"
                  >
                    Kill Query
                  </button>
                  <button
                    onclick={() => killLockThread(lock.blocking_thread_id, 'connection')}
                    class="px-2 py-1 rounded bg-rose-600 hover:bg-rose-500 text-white font-bold transition"
                  >
                    Kill Blocker
                  </button>
                </div>
              </div>

              <div class="p-3 space-y-2">
                {#if lock.blocking_query}
                  <div class="rounded border border-slate-800 overflow-hidden">
                    <div class="px-2.5 py-1.5 bg-[#090d16] border-b border-slate-800 flex items-center justify-between">
                      <span class="text-[10px] font-semibold text-slate-400 uppercase tracking-wide">Blocking Query</span>
                      <div class="flex items-center space-x-2">
                        <button onclick={() => copyText(lock.blocking_query, 'Blocking query')} class="text-[10px] text-slate-400 hover:text-white flex items-center space-x-1">
                          <Icon name="copy" class="w-3 h-3" />
                          <span>Copy</span>
                        </button>
                        <button onclick={() => inspectLockThread(lock, 'blocking')} class="text-[10px] text-sky-400 hover:text-sky-300 flex items-center space-x-1">
                          <Icon name="search" class="w-3 h-3" />
                          <span>Inspect</span>
                        </button>
                      </div>
                    </div>
                    <pre class="p-2.5 bg-[#050811] text-sky-200 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-all max-h-40 overflow-y-auto select-text">{lock.blocking_query}</pre>
                  </div>
                {:else}
                  <div class="px-2.5 py-2 rounded border border-slate-800 bg-[#090d16] text-slate-500 italic flex items-center justify-between">
                    <span>Idle in transaction: holding locks without a running query.</span>
                    <button onclick={() => inspectLockThread(lock, 'blocking')} class="text-sky-400 hover:text-sky-300 not-italic text-[10px]">Show detail</button>
                  </div>
                {/if}

                <div class="pl-3 border-l-2 border-rose-700/70 flex flex-wrap items-center justify-between gap-2">
                  <div class="flex flex-wrap items-center gap-1.5 text-[11px]">
                    <span class="text-rose-300">Blocked:</span>
                    <span class="font-bold text-slate-100">Thread #{lock.waiting_thread_id}</span>
                    <span class="text-slate-500">{lock.waiting_user || 'unknown'}</span>
                    <span class="text-slate-400">waiting</span>
                    <span class="text-amber-300 font-mono font-bold">{lock.waiting_seconds ?? 0}s</span>
                    <span class="text-slate-400">for {lock.lock_mode || 'lock'}</span>
                  </div>
                  <div class="flex items-center space-x-1.5">
                    <button
                      onclick={() => inspectLockThread(lock, 'waiting')}
                      class="px-2 py-0.5 rounded border border-slate-700 bg-slate-900 hover:bg-slate-800 text-slate-300 text-[10px] transition flex items-center space-x-1"
                    >
                      <Icon name="search" class="w-3 h-3 text-sky-400" />
                      <span>Detail</span>
                    </button>
                    <button
                      onclick={() => killLockThread(lock.waiting_thread_id, 'connection')}
                      class="px-2 py-0.5 rounded border border-rose-800 bg-rose-950/50 hover:bg-rose-900/60 text-rose-300 text-[10px] transition"
                    >
                      Kill Waiting #{lock.waiting_thread_id}
                    </button>
                  </div>
                </div>

                {#if lock.waiting_query}
                  <div class="rounded border border-slate-800 overflow-hidden">
                    <div class="px-2.5 py-1.5 bg-[#090d16] border-b border-slate-800 flex items-center justify-between">
                      <span class="text-[10px] font-semibold text-slate-400 uppercase tracking-wide">Waiting Query</span>
                      <button onclick={() => copyText(lock.waiting_query, 'Waiting query')} class="text-[10px] text-slate-400 hover:text-white flex items-center space-x-1">
                        <Icon name="copy" class="w-3 h-3" />
                        <span>Copy</span>
                      </button>
                    </div>
                    <pre class="p-2.5 bg-[#050811] text-slate-300 font-mono text-[11px] leading-relaxed whitespace-pre-wrap break-all max-h-32 overflow-y-auto select-text">{lock.waiting_query}</pre>
                  </div>
                {/if}
              </div>
            </div>
          {/each}
        {/if}
      </div>

      <div class="px-5 py-2.5 border-t border-[#334155] bg-[#0f172a] flex items-center justify-between text-[10px] text-slate-500 font-mono">
        <span>Root blockers are blocking threads that are not waiting on another lock.</span>
        <button onclick={() => (showBlockerPanel = false)} class="px-2.5 py-1 rounded border border-slate-700 bg-slate-900 hover:bg-slate-800 text-slate-300 font-sans text-xs transition">
          Close
        </button>
      </div>
    </div>
  </div>
{/if}
