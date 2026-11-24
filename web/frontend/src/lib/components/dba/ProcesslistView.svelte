<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity';
  import { copyText } from '../../utils/clipboard';
  import { formatSQL } from '../../utils/sqlFormatter';
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import type { ProcesslistThread, ProcessLockWait } from '../../../types';
  import Icon from '../common/Icon.svelte';
  import ProcessTrendPanel, { type TrendSample } from './ProcessTrendPanel.svelte';
  import DigestPanel from './DigestPanel.svelte';
  import DeadlockPanel from './DeadlockPanel.svelte';
  import ClusterHealthPanel from './ClusterHealthPanel.svelte';
  import ServerVariablesPanel from './ServerVariablesPanel.svelte';
  import FlightRecorderPanel from './FlightRecorderPanel.svelte';

  let clusterPanelRef = $state<any>(null);
  import { viewportFit } from '../../actions/viewportFit';

  type DbaSubTab = 'threads' | 'digest' | 'deadlock' | 'cluster' | 'variables' | 'flight-recorder';
  type FilterChip = 'all' | 'active' | 'sleep' | 'locked' | 'slow' | 'idle60';

  let currentSubTab = $state<DbaSubTab>('threads');

  function setSubTab(tab: DbaSubTab) {
    currentSubTab = tab;
    appState.processlistSubTab = tab;
  }

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
  const MANDATORY_COLUMNS = ['id', 'info'] as const;
  const PL_COLUMN_LABELS: Record<string, string> = {
    rowNum: '#',
    id: 'ID',
    user: 'User',
    host: 'Host',
    db: 'Database',
    command: 'Command',
    time: 'Time (s)',
    state: 'State',
    info: 'SQL / Query Info',
    actions: 'Actions',
  };
  const COLUMN_PREFS_KEY = 'sqlfox-processlist-hidden-cols';

  function defaultColumnVisibility(): Record<string, boolean> {
    return {
      rowNum: true,
      id: true,
      user: true,
      host: true,
      db: true,
      command: true,
      time: true,
      state: true,
      info: true,
      actions: true,
    };
  }

  function loadHiddenColumns(): Record<string, boolean> {
    const defaults = defaultColumnVisibility();
    try {
      const raw = typeof localStorage !== 'undefined' ? localStorage.getItem(COLUMN_PREFS_KEY) : null;
      if (!raw) return defaults;
      const parsed = JSON.parse(raw);
      if (!Array.isArray(parsed)) return defaults;
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
  let totalQuestions = $state<number>(0);

  function showAllColumns() {
    visibleColumns = defaultColumnVisibility();
  }

  function resetColumnPreferences() {
    visibleColumns = defaultColumnVisibility();
    if (typeof localStorage !== 'undefined') {
      try { localStorage.removeItem(COLUMN_PREFS_KEY); } catch { /* ignore */ }
    }
  }

  $effect(() => {
    if (typeof localStorage === 'undefined') return;
    const hidden = Object.keys(visibleColumns).filter((k) => !visibleColumns[k]);
    try {
      localStorage.setItem(COLUMN_PREFS_KEY, JSON.stringify(hidden));
    } catch {
      /* storage may be unavailable (private mode / quota) */
    }
  });

  // Threads filtering, selection, polling, trend & menus
  let filterText = $state('');
  let activeChip = $state<FilterChip>('all');
  let hideSleep = $state(true);
  let hideSystem = $state(true);
  let slowThreshold = $state(5);
  let selectedThreadIds = new SvelteSet<number>();
  let refreshInterval = $state<number>(0); // 0 = pause, 1000, 3000, 5000
  let intervalTimer: any = null;
  let isDocHidden = $state<boolean>(typeof document !== 'undefined' ? document.hidden : false);
  let inspectingThread = $state<ProcesslistThread | null>(null);
  let formatInspectSql = $state(false);
  let openRowMenuId = $state<number | null>(null);
  let openRowMenuAnchor = $state<HTMLElement | null>(null);

  // Rolling 60-sample activity trend panel state
  let trendOpen = $state<boolean>(
    typeof localStorage !== 'undefined' && localStorage.getItem('sqlfox_pl_trend_open') === 'true'
  );
  let trendHistory = $state<TrendSample[]>([]);
  let prevQuestions: number | null = null;
  let prevQuestionsAt = 0;

  function toggleTrendPanel() {
    trendOpen = !trendOpen;
    if (typeof localStorage !== 'undefined') {
      try { localStorage.setItem('sqlfox_pl_trend_open', trendOpen ? 'true' : 'false'); } catch { /* ignore */ }
    }
  }

  // Batch Kill Safety Challenge Modal state
  let showBatchKillModal = $state(false);
  let batchKillMode = $state<'query' | 'connection'>('connection');
  let batchKillTargetIds = $state<number[]>([]);
  let batchKillChallengeInput = $state('');
  let isBatchKilling = $state(false);

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

  function isSystemThread(t: ProcesslistThread): boolean {
    const u = (t.user || '').toLowerCase();
    const cmd = (t.command || '').toLowerCase();
    return u === 'system user' || u === 'event_scheduler' || cmd === 'binlog dump' || cmd === 'binlog dump gtid';
  }

  function isProductionConn(): boolean {
    return (appState.activeConnection?.environment || '').toLowerCase() === 'production';
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
  let clusterPollInterval = $state<number>(0); // 0 = paused, 5000, 10000, 30000
  let clusterPollTimer: any = null;
  let clusterUpdatedAt = $state<string>('');

  // Variables State
  let variablesReport = $state<any>(null);
  let isVariablesLoading = $state(false);

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
    if (typeof document === 'undefined') return;
    const onVis = () => {
      isDocHidden = document.hidden;
    };
    document.addEventListener('visibilitychange', onVis);
    return () => document.removeEventListener('visibilitychange', onVis);
  });

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
    if (refreshInterval > 0 && currentSubTab === 'threads' && !isDocHidden) {
      intervalTimer = setInterval(() => {
        loadProcesslist(false);
      }, refreshInterval);
    }
    return () => {
      if (intervalTimer) clearInterval(intervalTimer);
    };
  });

  $effect(() => {
    if (clusterPollTimer) {
      clearInterval(clusterPollTimer);
      clusterPollTimer = null;
    }
    if (clusterPollInterval > 0 && currentSubTab === 'cluster' && !isDocHidden) {
      clusterPollTimer = setInterval(() => {
        loadClusterReport();
      }, clusterPollInterval);
    }
    return () => {
      if (clusterPollTimer) clearInterval(clusterPollTimer);
    };
  });

  let isPollingProcesslist = false;

  async function loadProcesslist(showSpinner = true) {
    const sessionId = appState.activeSessionId;
    if (!sessionId) return;
    if (!showSpinner && isPollingProcesslist) return;
    isPollingProcesslist = true;
    if (showSpinner) isLoading = true;
    try {
      const res = await api.getProcesslist(sessionId);
      if (appState.activeSessionId !== sessionId) return;
      const list = res.threads || [];
      const lockList = (res.locks || []) as ProcessLockWait[];
      threads = list;
      locks = lockList;
      currentThreadId = res.current_thread_id || 0;
      totalQuestions = res.questions || 0;

      // Sync global header DBA badges
      appState.dbaBlockersCount = lockList.length;
      appState.dbaSlowQueriesCount = list.filter(
        (t) => t.command !== 'Sleep' && t.state !== 'Sleep' && t.time >= slowThreshold
      ).length;

      // Update rolling 60-sample activity trend telemetry
      const now = Date.now();
      const running = list.filter((t) => t.command !== 'Sleep' && t.state !== 'Sleep').length;
      let qps = 0;
      if (prevQuestions !== null && prevQuestionsAt > 0 && totalQuestions >= prevQuestions) {
        const elapsedSec = (now - prevQuestionsAt) / 1000;
        if (elapsedSec > 0.1) qps = (totalQuestions - prevQuestions) / elapsedSec;
      }
      prevQuestions = totalQuestions;
      prevQuestionsAt = now;
      const next = [...trendHistory, { ts: now, running, total: list.length, qps }];
      trendHistory = next.length > 60 ? next.slice(next.length - 60) : next;
    } catch (err: any) {
      if (appState.activeSessionId !== sessionId) return;
      appState.showToast(`Failed to load processlist: ${err.message}`, 'error');
    } finally {
      isPollingProcesslist = false;
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
    const ok = await appState.showNativeConfirm(
      'Are you sure you want to reset statement digest performance statistics?',
      'Reset Statement Digest'
    );
    if (!ok || !appState.activeSessionId) return;
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
      clusterUpdatedAt = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
      // Sync global cluster badge when telemetry reports cluster/replication
      if (res && (res.topology_type !== 'standalone' || res.replication || (res.nodes && res.nodes.length > 0))) {
        const sev = String(res.bottleneck_severity || 'ok').toLowerCase();
        appState.dbaClusterStatus = sev === 'critical' ? 'critical' : sev === 'warning' ? 'warn' : 'ok';
        appState.dbaClusterLabel = sev === 'critical' ? 'CRIT' : sev === 'warning' ? 'WARN' : 'OK';
      } else {
        appState.dbaClusterStatus = null;
        appState.dbaClusterLabel = '';
      }
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
      appState.flightRecordingActive = isRecording;
      if (status.is_recording) {
        recordingId = status.recording_id;
        recordingStartedAt = status.started_at ? new Date(status.started_at) : new Date();
        recordingSeconds = status.duration_seconds || 0;
        appState.flightRecordingRemainingSec = Math.max(0, 600 - recordingSeconds);
        if (!selectedFlightSessionId && Array.isArray(status.recent_ticks)) {
          flightSessionTicks = status.recent_ticks;
        }
        startFlightTimers();
      } else {
        appState.flightRecordingRemainingSec = 0;
        stopFlightTimers();
      }
    } catch (_) {}
  }

  function startFlightTimers() {
    stopFlightTimers();
    flightTickerTimer = setInterval(() => {
      recordingSeconds++;
      appState.flightRecordingRemainingSec = Math.max(0, 600 - recordingSeconds);
    }, 1000);
    // Poll every >= 1000ms for real-time telemetry streaming
    flightPollTimer = setInterval(async () => {
      if (!appState.activeSessionId || !isRecording) return;
      try {
        const status = await api.getFlightRecorderStatus(appState.activeSessionId);
        if (!status.is_recording) {
          isRecording = false;
          appState.flightRecordingActive = false;
          appState.flightRecordingRemainingSec = 0;
          stopFlightTimers();
          await loadFlightSessions();
        } else {
          recordingSeconds = status.duration_seconds || recordingSeconds;
          appState.flightRecordingActive = true;
          appState.flightRecordingRemainingSec = Math.max(0, 600 - recordingSeconds);
          if (!selectedFlightSessionId && Array.isArray(status.recent_ticks)) {
            flightSessionTicks = status.recent_ticks;
          }
        }
      } catch (_) {}
    }, Math.max(1000, flightIntervalMs));
  }

  // Stop all timers if the component unmounts (sub-tab switch or view change).
  $effect(() => {
    return () => {
      stopFlightTimers();
      if (intervalTimer) clearInterval(intervalTimer);
      if (clusterPollTimer) clearInterval(clusterPollTimer);
    };
  });

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
        const interval = Math.max(1000, flightIntervalMs);
        const res = await api.startFlightRecorder(appState.activeSessionId, interval);
        isRecording = true;
        appState.flightRecordingActive = true;
        recordingId = res.recording_id;
        recordingStartedAt = new Date();
        recordingSeconds = 0;
        appState.flightRecordingRemainingSec = 600;
        selectedFlightSessionId = ''; // Enter live recording mode
        flightSessionTicks = [];
        selectedTickIndex = null;
        startFlightTimers();
        appState.showToast('Performance Flight Recorder started (10-min safety guard active)', 'info');
        const sessionId = appState.activeSessionId;
        setTimeout(async () => {
          try {
            if (!isRecording || selectedFlightSessionId || appState.activeSessionId !== sessionId) return;
            const st = await api.getFlightRecorderStatus(sessionId);
            if (!isRecording || selectedFlightSessionId || appState.activeSessionId !== sessionId) return;
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
        appState.flightRecordingActive = false;
        appState.flightRecordingRemainingSec = 0;
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

  const flightPeak = $derived.by(() => {
    const t = flightSessionTicks || [];
    if (t.length === 0) return { maxQps: 0, maxThreads: 0, lockDelta: 0, bufferDelta: 0 };
    let maxQps = 0;
    let maxThreads = 0;
    let firstLock: number | null = null;
    let lastLock = 0;
    let firstBuf: number | null = null;
    let lastBuf = 0;
    for (const tick of t) {
      maxQps = Math.max(maxQps, Number(tick.qps ?? 0));
      maxThreads = Math.max(maxThreads, Number(tick.threads_running ?? tick.active_threads ?? 0));
      if (firstLock === null && tick.row_lock_waits != null) firstLock = Number(tick.row_lock_waits);
      if (tick.row_lock_waits != null) lastLock = Number(tick.row_lock_waits);
      if (firstBuf === null && tick.buffer_pool_wait_free != null) firstBuf = Number(tick.buffer_pool_wait_free);
      if (tick.buffer_pool_wait_free != null) lastBuf = Number(tick.buffer_pool_wait_free);
    }
    return {
      maxQps,
      maxThreads,
      lockDelta: Math.max(0, lastLock - (firstLock ?? lastLock)),
      bufferDelta: Math.max(0, lastBuf - (firstBuf ?? lastBuf)),
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
    const ok = await appState.showNativeConfirm(
      'Are you sure you want to permanently delete this flight recorder session?',
      'Delete Flight Recording'
    );
    if (!ok || !appState.activeSessionId) return;
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
    const sessionMeta = flightSessions.find((s) => s.id === selectedFlightSessionId) || null;
    const payload = {
      exported_at: new Date().toISOString(),
      connection: appState.activeConnection?.name || '',
      session_id: selectedFlightSessionId || recordingId || 'live',
      session: sessionMeta,
      peak_summary: flightPeak,
      tick_count: flightSessionTicks.length,
      ticks: flightSessionTicks,
    };
    const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `flight-recorder-${selectedFlightSessionId || 'live'}-${Date.now()}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    appState.showToast('Exported flight recording session JSON', 'success');
  }

  async function killSingleThread(id: number, mode: 'query' | 'connection' = 'query') {
    const thread = threads.find((t) => t.id === id);
    if (thread && isSystemThread(thread)) {
      appState.showToast('System daemon thread is protected from termination', 'warning');
      return;
    }
    if (mode === 'connection' && id === currentThreadId) {
      appState.showToast('Cannot kill your own active session connection', 'warning');
      return;
    }
    const modeLabel = mode === 'connection' ? 'KILL CONNECTION' : 'KILL QUERY';
    const ok = await appState.showNativeConfirm(
      `Execute ${modeLabel} on thread #${id}${thread?.user ? ` (${thread.user}@${thread.host})` : ''}?`,
      `Confirm ${modeLabel}`
    );
    if (!ok) return;
    const sessionId = appState.activeSessionId;
    if (!sessionId) return;
    try {
      await api.killProcess(sessionId, id, mode);
      appState.showToast(`${modeLabel} succeeded for thread #${id}`, 'success');
      if (mode === 'connection') {
        selectedThreadIds.delete(id);
        if (inspectingThread?.id === id) inspectingThread = null;
      }
      await loadProcesslist(false);
    } catch (err: any) {
      appState.showToast(`Failed to kill thread: ${err.message}`, 'error');
    }
  }

  function openBatchKillModal(mode: 'query' | 'connection') {
    const validIds = Array.from(selectedThreadIds).filter((id) => {
      const t = threads.find((x) => x.id === id);
      if (t && isSystemThread(t)) return false;
      if (mode === 'connection' && id === currentThreadId) return false;
      return true;
    });
    if (validIds.length === 0) {
      appState.showToast('No eligible non-system threads selected', 'warning');
      return;
    }
    batchKillMode = mode;
    batchKillTargetIds = validIds;
    batchKillChallengeInput = '';
    showBatchKillModal = true;
  }

  function removeBatchKillTarget(id: number) {
    batchKillTargetIds = batchKillTargetIds.filter((x) => x !== id);
    selectedThreadIds.delete(id);
    if (batchKillTargetIds.length === 0) showBatchKillModal = false;
  }

  let needsBatchChallenge = $derived(isProductionConn() || batchKillTargetIds.length > 5);
  let batchChallengeValid = $derived(
    !needsBatchChallenge || batchKillChallengeInput.trim().toUpperCase() === 'KILL'
  );

  async function executeBatchKill() {
    if (!appState.activeSessionId || batchKillTargetIds.length === 0 || !batchChallengeValid) return;
    isBatchKilling = true;
    try {
      const res = await api.batchKillProcesses(appState.activeSessionId, batchKillTargetIds, batchKillMode);
      const okCount = res.success_count ?? 0;
      const failCount = res.failed_count ?? 0;
      if (failCount > 0) {
        const firstErr = res.results?.find((r) => !r.success)?.error || '';
        appState.showToast(
          `Batch ${batchKillMode}: ${okCount} terminated, ${failCount} failed${firstErr ? ` (${firstErr})` : ''}`,
          okCount > 0 ? 'warning' : 'error'
        );
      } else {
        appState.showToast(
          `Batch ${batchKillMode === 'connection' ? 'KILL CONNECTION' : 'KILL QUERY'}: ${okCount} thread(s) terminated`,
          'success'
        );
      }
      showBatchKillModal = false;
      selectedThreadIds.clear();
      await loadProcesslist(false);
    } catch (err: any) {
      appState.showToast(`Batch kill failed: ${err.message}`, 'error');
    } finally {
      isBatchKilling = false;
    }
  }

  function copySelectedThreadIds() {
    const ids = Array.from(selectedThreadIds).join(', ');
    if (!ids) return;
    copyText(ids);
    appState.showToast(`Copied ${selectedThreadIds.size} thread ID(s) to clipboard`, 'info');
  }

  function exportSelectedThreadsJSON() {
    const rows = threads.filter((t) => selectedThreadIds.has(t.id));
    if (rows.length === 0) return;
    const blob = new Blob([JSON.stringify(rows, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `processlist-threads-${Date.now()}.json`;
    a.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
    appState.showToast(`Exported ${rows.length} thread(s) to JSON`, 'success');
  }

  function saveSelectedThreadsToNotes() {
    const rows = threads.filter((t) => selectedThreadIds.has(t.id));
    if (rows.length === 0) return;
    const body = rows
      .map(
        (t) =>
          `### Thread #${t.id} (${t.user}@${t.host} · db=${t.db || '-'} · ${t.time}s)\nState: ${t.state || '-'}\n\`\`\`sql\n${t.info || '-- Idle'}\n\`\`\``
      )
      .join('\n\n');
    appState.saveMarkdownToNote(body, `Processlist Snapshot (${rows.length} threads)`, {
      category: 'Diagnostics',
      userTags: ['#processlist'],
    });
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
    const ok = await appState.showNativeConfirm(
      `Execute KILL ${mode.toUpperCase()} for thread #${id}?`,
      'Terminate Lock Contention Thread'
    );
    if (!ok) return;
    try {
      const res = await api.batchKillProcesses(appState.activeSessionId, [id], mode);
      if ((res.failed_count ?? 0) > 0 && (res.success_count ?? 0) === 0) {
        const msg = res.results?.[0]?.error || 'Kill denied';
        appState.showToast(`Failed to kill thread #${id}: ${msg}`, 'error');
      } else {
        appState.showToast(`Killed ${mode} for thread #${id}`, 'success');
      }
      await loadProcesslist(false);
    } catch (err: any) {
      appState.showToast(`Failed to kill thread #${id}: ${err.message}`, 'error');
    }
  }

  function copyWithToast(text: string | undefined, label: string) {
    if (!text) return;
    copyText(text);
    appState.showToast(`${label} copied to clipboard`, 'info');
  }

  let filteredThreads = $derived.by(() => {
    const q = filterText.toLowerCase().trim();
    return threads.filter((t) => {
      if (hideSleep && (t.command === 'Sleep' || t.state === 'Sleep')) return false;
      if (hideSystem && isSystemThread(t)) return false;

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
      } else if (activeChip === 'idle60') {
        if ((t.command !== 'Sleep' && t.state !== 'Sleep') || t.time < 60) return false;
      } else if (activeChip === 'locked') {
        const isLocked =
          (t.state && t.state.toLowerCase().includes('lock')) ||
          locks.some((l) => l.waiting_thread_id === t.id || l.blocking_thread_id === t.id);
        if (!isLocked) return false;
      } else if (activeChip === 'slow') {
        if (t.command === 'Sleep' || t.state === 'Sleep' || t.time < slowThreshold) return false;
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
    let idle60 = 0;
    for (const t of threads) {
      const sleeping = t.command === 'Sleep' || t.state === 'Sleep';
      if (sleeping) {
        sleep++;
        if (t.time >= 60) idle60++;
      } else {
        active++;
        if (t.time >= slowThreshold) slow++;
      }
      if (
        (t.state || '').toLowerCase().includes('lock') ||
        locks.some((l) => l.waiting_thread_id === t.id || l.blocking_thread_id === t.id)
      )
        locked++;
    }
    return { active, sleep, locked, slow, idle60 };
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
    const selectable = filteredThreads.filter((t) => t.id !== currentThreadId && !isSystemThread(t));
    if (selectable.length === 0) return false;
    return selectable.every((t) => selectedThreadIds.has(t.id));
  });

  function toggleSelectAll(checked: boolean) {
    const selectable = filteredThreads.filter((t) => t.id !== currentThreadId && !isSystemThread(t));
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

  function explainThreadQuery(sql?: string) {
    const q = (sql || '').trim();
    if (!q) return;
    appState.addTab('Explain.sql', q);
    appState.explainCurrentTab(q, true);
  }

  function openSqlInEditor(sql?: string, title = 'Query.sql') {
    const q = (sql || '').trim();
    if (!q) return;
    appState.addTab(title, q);
  }

  function switchSessionToNode(addr?: string) {
    if (!addr) return;
    const parts = addr.split(':');
    const host = parts[0] || '';
    const port = parts[1] ? parseInt(parts[1], 10) : 3306;
    const match = appState.connections.find(
      (c) => c.host === host && Number(c.port || 3306) === port
    );
    if (match) {
      appState.selectConnection(match);
      appState.showToast(`Switched session to node ${host}:${port}`, 'success');
      return;
    }
    const base = appState.activeConnection;
    appState.connectionToEdit = {
      id: '',
      name: `${base?.name || 'Cluster'} (${host})`,
      driver: base?.driver || 'mysql',
      environment: base?.environment || 'Development',
      host,
      port,
      user: base?.user || 'root',
      database: base?.database || base?.database_name || '',
    } as any;
    appState.isConnectionModalOpen = true;
    appState.showToast(`Pre-filled node ${host}:${port}. Enter credentials and Connect.`, 'info');
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
          onclick={() => setSubTab('threads')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 {currentSubTab === 'threads' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Threads ({threads.length})
        </button>
        <button
          onclick={() => setSubTab('digest')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 {currentSubTab === 'digest' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Statement Digest
        </button>
        <button
          onclick={() => setSubTab('deadlock')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 {currentSubTab === 'deadlock' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Deadlock Inspector
        </button>
        <button
          onclick={() => setSubTab('cluster')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 {currentSubTab === 'cluster' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Cluster & Replication
        </button>
        <button
          onclick={() => setSubTab('variables')}
          class="px-2.5 py-1 rounded text-xs font-semibold transition font-sans shrink-0 {currentSubTab === 'variables' ? 'bg-[#1e293b] text-sky-400 shadow-sm' : 'text-slate-400 hover:text-slate-200'}"
        >
          Server Variables
        </button>
        <button
          onclick={() => setSubTab('flight-recorder')}
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
    <div class="flex items-center space-x-2 shrink-0">
      {#if currentSubTab === 'threads'}
        <button
          type="button"
          onclick={toggleTrendPanel}
          class="px-2 py-1 rounded border text-[11px] font-sans flex items-center space-x-1 transition {trendOpen ? 'bg-sky-950/70 border-sky-700 text-sky-300' : 'bg-[#090d16] border-[#334155] text-slate-300 hover:text-white'}"
          title="Toggle live Activity Trend chart (persisted)"
        >
          <Icon name="trending-up" class="w-3.5 h-3.5 text-sky-400" />
          <span>Trend</span>
          <Icon name={trendOpen ? 'chevron-up' : 'chevron-down'} class="w-3 h-3 text-slate-400" />
        </button>

        <div class="flex items-center space-x-1.5 bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-[11px]">
          <span class="text-slate-400 font-sans">Refresh:</span>
          <select
            bind:value={refreshInterval}
            class="bg-transparent text-sky-400 font-semibold focus:outline-none cursor-pointer [color-scheme:dark] [&>option]:bg-[#0b1220] [&>option]:text-slate-200"
          >
            <option value={0}>Paused</option>
            <option value={1000}>1s</option>
            <option value={3000}>3s</option>
            <option value={5000}>5s</option>
          </select>
          {#if isDocHidden && refreshInterval > 0}
            <span class="text-[10px] text-amber-400 font-sans" title="Polling auto-paused while browser tab is inactive">(paused)</span>
          {/if}
        </div>

        <button
          type="button"
          onclick={() => loadProcesslist(true)}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh thread list"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'digest'}
        <button
          type="button"
          onclick={handleResetDigest}
          class="px-2.5 py-1 rounded bg-rose-950/60 hover:bg-rose-900 border border-rose-800 text-rose-300 text-xs font-semibold transition flex items-center space-x-1 font-sans"
        >
          <Icon name="trash-2" class="w-3.5 h-3.5" />
          <span>Reset Stats</span>
        </button>
        <button
          type="button"
          onclick={loadStatementDigest}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh statement digest"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isDigestLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'deadlock'}
        <button
          type="button"
          onclick={loadDeadlock}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh deadlock report"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isDeadlockLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'cluster'}
        {#if clusterUpdatedAt}
          <span class="hidden md:inline text-[10px] text-slate-500 font-mono">Updated: {clusterUpdatedAt}</span>
        {/if}
        <div class="flex items-center space-x-1.5 bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-[11px]">
          <span class="text-slate-400 font-sans">Poll:</span>
          <select
            bind:value={clusterPollInterval}
            class="bg-transparent text-sky-400 font-semibold focus:outline-none cursor-pointer [color-scheme:dark] [&>option]:bg-[#0b1220] [&>option]:text-slate-200"
          >
            <option value={0}>Paused</option>
            <option value={5000}>5s</option>
            <option value={10000}>10s</option>
            <option value={30000}>30s</option>
          </select>
        </div>
        {#if clusterReport}
          <button
            type="button"
            onclick={() => clusterPanelRef?.sendClusterToAI()}
            class="px-2 py-1 rounded bg-purple-950/60 hover:bg-purple-900 border border-purple-800 text-purple-300 text-[11px] font-sans font-semibold flex items-center space-x-1 transition"
          >
            <Icon name="bot" class="w-3 h-3" />
            <span>Ask Copilot</span>
          </button>
        {/if}
        <button
          type="button"
          onclick={loadClusterReport}
          class="p-1.5 text-slate-400 hover:text-white rounded hover:bg-slate-800 transition"
          title="Refresh cluster telemetry"
        >
          <Icon name="refresh-cw" class="w-3.5 h-3.5 {isClusterLoading ? 'animate-spin text-sky-400' : ''}" />
        </button>
      {:else if currentSubTab === 'variables'}
        <button
          type="button"
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
                <option value={1000}>1s</option>
                <option value={2000}>2s</option>
                <option value={5000}>5s</option>
              </select>
            </div>
          {/if}

          <button
            type="button"
            onclick={toggleFlightRecording}
            class="px-3 py-1 rounded font-bold text-xs font-sans transition flex items-center space-x-1.5 shadow {isRecording ? 'bg-rose-600 hover:bg-rose-500 text-white animate-pulse' : 'bg-emerald-600 hover:bg-emerald-500 text-white'}"
          >
            {#if isRecording}
              {@const rem = Math.max(0, 600 - recordingSeconds)}
              <span class="w-2 h-2 rounded-full bg-white"></span>
              <span>STOP ({Math.floor(recordingSeconds / 60).toString().padStart(2, '0')}:{(recordingSeconds % 60).toString().padStart(2, '0')} · {Math.floor(rem / 60)}:{(rem % 60).toString().padStart(2, '0')} left)</span>
            {:else}
              <span class="w-2 h-2 rounded-full bg-rose-400"></span>
              <span>START REC</span>
            {/if}
          </button>

          {#if flightSessionTicks.length > 0}
            <button
              type="button"
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
    <div class="relative flex-1 flex flex-col overflow-hidden">
      <ProcessTrendPanel
        history={trendHistory}
        open={trendOpen}
        onToggle={toggleTrendPanel}
        onEscalate={() => setSubTab('flight-recorder')}
      />

      <div class="min-h-9 py-1 bg-[#090d16] border-b border-[#1e293b] px-4 flex items-center justify-between flex-wrap gap-2 text-xs select-none">
        <div class="flex items-center space-x-1 font-sans text-[11px] flex-wrap gap-y-1">
          <button
            type="button"
            onclick={() => (activeChip = 'all')}
            class="px-2 py-0.5 rounded transition {activeChip === 'all' ? 'bg-sky-500/20 text-sky-300 border border-sky-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
          >
            All ({threads.length})
          </button>
          <button
            type="button"
            onclick={() => (activeChip = 'active')}
            class="px-2 py-0.5 rounded transition {activeChip === 'active' ? 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
          >
            Active ({chipCounts.active})
          </button>
          <button
            type="button"
            onclick={() => (activeChip = 'sleep')}
            class="px-2 py-0.5 rounded transition {activeChip === 'sleep' ? 'bg-slate-700 text-slate-200 font-bold' : 'text-slate-400 hover:text-slate-200'}"
          >
            Sleep ({chipCounts.sleep})
          </button>
          <button
            type="button"
            onclick={() => (activeChip = 'idle60')}
            class="px-2 py-0.5 rounded transition {activeChip === 'idle60' ? 'bg-amber-900/50 text-amber-200 border border-amber-600/50 font-bold' : 'text-slate-400 hover:text-slate-200'}"
            title="Idle connections sleeping >= 60 seconds"
          >
            Sleep ≥ 60s ({chipCounts.idle60})
          </button>
          <button
            type="button"
            onclick={() => (activeChip = 'locked')}
            class="px-2 py-0.5 rounded transition {activeChip === 'locked' ? 'bg-amber-500/20 text-amber-300 border border-amber-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
          >
            Locked ({chipCounts.locked})
          </button>
          <button
            type="button"
            onclick={() => (activeChip = 'slow')}
            class="px-2 py-0.5 rounded transition {activeChip === 'slow' ? 'bg-rose-500/20 text-rose-300 border border-rose-500/40 font-bold' : 'text-slate-400 hover:text-slate-200'}"
          >
            Slow ≥{slowThreshold}s ({chipCounts.slow})
          </button>
        </div>

        <div class="flex items-center space-x-2 flex-wrap gap-y-1">
          <label class="flex items-center space-x-1 text-[11px] font-sans text-slate-400 hover:text-slate-200 cursor-pointer">
            <input type="checkbox" bind:checked={hideSleep} class="rounded bg-[#090d16] border-[#334155] text-sky-500 focus:ring-0" />
            <span>Hide Sleep</span>
          </label>
          <label class="flex items-center space-x-1 text-[11px] font-sans text-slate-400 hover:text-slate-200 cursor-pointer">
            <input type="checkbox" bind:checked={hideSystem} class="rounded bg-[#090d16] border-[#334155] text-sky-500 focus:ring-0" />
            <span>Hide Sys</span>
          </label>
          <label class="flex items-center space-x-1 text-[11px] font-sans text-slate-400">
            <span>Slow &gt;</span>
            <select
              bind:value={slowThreshold}
              class="h-6 bg-[#0b0f19] border border-[#334155] rounded px-1.5 text-[11px] text-amber-300 font-mono focus:outline-none [color-scheme:dark] [&>option]:bg-[#0b1220]"
            >
              <option value={1}>1s</option>
              <option value={3}>3s</option>
              <option value={5}>5s</option>
              <option value={10}>10s</option>
              <option value={30}>30s</option>
              <option value={60}>60s</option>
            </select>
          </label>

          <button
            type="button"
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

          <!-- Column Visibility Dropdown (ADR 0054) -->
          <div class="relative">
            <button
              type="button"
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
                type="button"
                tabindex="-1"
                aria-label="Close"
                class="fixed inset-0 z-30 bg-transparent cursor-default"
                onclick={() => (showColSelector = false)}
              ></button>
              <div
                use:viewportFit
                class="absolute right-0 mt-1 w-48 bg-[#0f172a] border border-[#334155] rounded-lg shadow-2xl z-40 p-2 text-xs font-sans space-y-1 animate-in fade-in duration-100"
              >
                <div class="flex items-center justify-between px-1 pb-1 border-b border-slate-800">
                  <span class="text-[10px] font-bold text-slate-400 uppercase tracking-wider">Visible Columns</span>
                  <div class="flex items-center space-x-1.5 text-[10px]">
                    <button type="button" onclick={showAllColumns} class="text-sky-400 hover:underline">Show All</button>
                    <button type="button" onclick={resetColumnPreferences} class="text-slate-400 hover:text-white">Reset</button>
                  </div>
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

      <!-- Threads Table View -->
      <div class="flex-1 overflow-auto pb-16">
        <table class="w-full min-w-[1150px] border-collapse table-fixed text-left">
          <colgroup>
            <col class="w-9" />
            {#if visibleColumns.rowNum}<col class="w-10" />{/if}
            {#if visibleColumns.id}<col class="w-32" />{/if}
            {#if visibleColumns.user}<col class="w-32" />{/if}
            {#if visibleColumns.host}<col class="w-40" />{/if}
            {#if visibleColumns.db}<col class="w-28" />{/if}
            {#if visibleColumns.command}<col class="w-28" />{/if}
            {#if visibleColumns.time}<col class="w-24" />{/if}
            {#if visibleColumns.state}<col class="w-40" />{/if}
            {#if visibleColumns.info}<col />{/if}
            {#if visibleColumns.actions}<col class="w-[104px]" />{/if}
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
              {#if visibleColumns.rowNum}
                <th class="h-8 px-2 text-right font-semibold w-10">#</th>
              {/if}
              {#if visibleColumns.id}{@render th('id', 'ID', 'w-32', 'right')}{/if}
              {#if visibleColumns.user}{@render th('user', 'User', 'w-32')}{/if}
              {#if visibleColumns.host}{@render th('host', 'Host', 'w-40')}{/if}
              {#if visibleColumns.db}{@render th('db', 'DB', 'w-28')}{/if}
              {#if visibleColumns.command}{@render th('command', 'Command', 'w-28')}{/if}
              {#if visibleColumns.time}{@render th('time', 'Time', 'w-24', 'right')}{/if}
              {#if visibleColumns.state}{@render th('state', 'State', 'w-40')}{/if}
              {#if visibleColumns.info}{@render th('info', 'Query', '')}{/if}
              {#if visibleColumns.actions}
                <th
                  class="h-8 px-2 w-[104px] text-center font-semibold sticky right-0 bg-[#0f172a] border-l border-slate-800"
                >
                  Actions
                </th>
              {/if}
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
              {#each sortedThreads as thread, rowIdx (thread.id)}
                {@const isSelf = thread.id === currentThreadId}
                {@const isSys = isSystemThread(thread)}
                {@const isSleeping = thread.command === 'Sleep' || thread.state === 'Sleep'}
                {@const isSelected = selectedThreadIds.has(thread.id)}
                {@const isCriticalSlow = !isSleeping && thread.time >= 30}
                {@const isSlow = !isSleeping && thread.time >= slowThreshold}
                {@const isIdle60 = isSleeping && thread.time >= 60}
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
                          : isCriticalSlow
                            ? 'bg-rose-950/25 hover:bg-rose-950/35 border-l-rose-500'
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
                  onclick={() => { inspectingThread = thread; formatInspectSql = false; }}
                >
                  <td class="px-2 text-center overflow-hidden" onclick={(e) => e.stopPropagation()}>
                    {#if !isSelf && !isSys}
                      <input
                        type="checkbox"
                        aria-label="Select thread {thread.id}"
                        checked={isSelected}
                        onchange={() => toggleThreadSelection(thread.id)}
                        class="rounded border-slate-600 bg-transparent text-rose-500 focus:ring-0 focus:ring-offset-0 align-middle"
                      />
                    {/if}
                  </td>
                  {#if visibleColumns.rowNum}
                    <td class="px-2 text-right text-slate-500 text-[10px] tabular-nums">{rowIdx + 1}</td>
                  {/if}
                  {#if visibleColumns.id}
                    <td class="px-2 font-mono tabular-nums font-bold {isSelf ? 'text-sky-400' : 'text-slate-300'}">
                      <div class="flex items-center justify-between gap-1 overflow-hidden">
                        <div class="flex items-center gap-1 shrink-0">
                          {#if isRootBlocker}
                            <span
                              class="rounded bg-rose-600 px-1 py-0.5 font-sans text-[9px] font-bold tracking-tight text-white shadow-xs"
                              title="Root blocker holding locks for other threads"
                            >
                              BLOCK
                            </span>
                          {:else if isWaitingOnLock}
                            <span
                              class="rounded border border-amber-700/60 bg-amber-950/80 px-1 py-0.5 font-sans text-[9px] font-bold tracking-tight text-amber-300"
                              title="Thread is waiting on a lock"
                            >
                              WAIT
                            </span>
                          {/if}
                          {#if isSelf}
                            <span
                              class="rounded border border-sky-800/60 bg-sky-950/80 px-1 py-0.5 font-sans text-[9px] font-bold tracking-tight text-sky-300 uppercase"
                              title="Your current active session"
                            >
                              YOU
                            </span>
                          {/if}
                        </div>
                        <span class="truncate text-right ml-auto">{thread.id}</span>
                      </div>
                    </td>
                  {/if}
                  {#if visibleColumns.user}
                    <td class="px-2 text-slate-200 truncate overflow-hidden" title={thread.user}>
                      {thread.user || '-'}
                      {#if isSys}
                        <span class="ml-1 px-1 rounded bg-sky-950/70 border border-sky-800 text-sky-300 text-[9px] font-sans font-bold uppercase" title="Protected system daemon thread">Sys</span>
                      {/if}
                    </td>
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
                      class="px-2 text-right tabular-nums whitespace-nowrap {isCriticalSlow
                        ? 'font-bold text-rose-400'
                        : isSlow
                        ? 'font-bold text-amber-300'
                        : isIdle60
                        ? 'text-amber-400/90'
                        : thread.time >= 1
                        ? 'text-slate-300'
                        : 'text-slate-500'}"
                      title={isCriticalSlow ? 'Critical slow query >= 30s' : isSlow ? `Slow query >= ${slowThreshold}s` : isIdle60 ? 'Idle connection >= 60s' : ''}
                    >
                      {thread.time.toFixed(1)}{isIdle60 ? 's 💤' : ''}
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
                  {#if visibleColumns.actions}
                    <td
                      class="px-1.5 text-center whitespace-nowrap sticky right-0 border-l border-slate-800/60 transition-colors {actionBg} group-hover:bg-slate-800/40 {openRowMenuId === thread.id ? 'z-30' : ''}"
                      onclick={(e) => e.stopPropagation()}
                    >
                      <div class="inline-flex items-center space-x-1 relative">
                        <button
                          type="button"
                          disabled={isSys}
                          onclick={() => killSingleThread(thread.id, 'query')}
                          class="rounded p-1 text-amber-400/80 hover:bg-amber-950/60 hover:text-amber-300 disabled:opacity-25 disabled:cursor-not-allowed transition"
                          title={isSys ? 'Protected system thread' : 'Kill running query (KILL QUERY)'}
                          aria-label="Kill query on thread {thread.id}"
                        >
                          <Icon name="square" class="w-3 h-3" />
                        </button>
                        <button
                          type="button"
                          disabled={isSelf || isSys}
                          onclick={() => killSingleThread(thread.id, 'connection')}
                          class="rounded p-1 text-rose-400/80 hover:bg-rose-950/60 hover:text-rose-300 disabled:opacity-25 disabled:cursor-not-allowed transition"
                          title={isSelf ? 'Cannot kill own active connection' : isSys ? 'Protected system thread' : 'Terminate client connection (KILL)'}
                          aria-label="Kill connection {thread.id}"
                        >
                          <Icon name="x-circle" class="w-3 h-3" />
                        </button>
                        <button
                          type="button"
                          onclick={(e) => {
                            e.stopPropagation();
                            if (openRowMenuId === thread.id) {
                              openRowMenuId = null;
                              openRowMenuAnchor = null;
                            } else {
                              openRowMenuAnchor = e.currentTarget as HTMLElement;
                              openRowMenuId = thread.id;
                            }
                          }}
                          class="rounded p-1 text-slate-400 hover:bg-slate-700 hover:text-white transition"
                          title="More thread actions"
                          aria-label="Thread {thread.id} menu"
                        >
                          <Icon name="more-vertical" class="w-3 h-3" />
                        </button>

                        {#if openRowMenuId === thread.id}
                          <button
                            type="button"
                            tabindex="-1"
                            aria-label="Close thread menu"
                            class="fixed inset-0 z-40 bg-transparent cursor-default"
                            onclick={() => { openRowMenuId = null; openRowMenuAnchor = null; }}
                          ></button>
                          <div
                            use:viewportFit={{ anchor: openRowMenuAnchor, align: 'right' }}
                            class="fixed w-56 bg-[#0f172a] border border-slate-700 rounded-lg shadow-2xl z-50 py-1 divide-y divide-slate-800 text-left font-sans text-[11px] text-slate-200"
                          >
                            <div class="py-0.5">
                              <button
                                type="button"
                                onclick={() => { inspectingThread = thread; formatInspectSql = false; openRowMenuId = null; openRowMenuAnchor = null; }}
                                class="w-full px-3 py-1.5 hover:bg-slate-800 flex items-center space-x-2"
                              >
                                <Icon name="eye" class="w-3 h-3 text-sky-400" /><span>Inspect Thread</span>
                              </button>
                            </div>
                            <div class="px-2.5 py-1.5 space-y-1">
                              <div class="text-[9.5px] font-bold uppercase tracking-wider text-slate-500">SQL Actions</div>
                              <div class="grid grid-cols-2 gap-1 text-[10px]">
                                <button
                                  type="button"
                                  disabled={!thread.info}
                                  onclick={() => { copyWithToast(thread.info, 'SQL'); openRowMenuId = null; openRowMenuAnchor = null; }}
                                  class="px-2 py-1 rounded bg-slate-800/80 hover:bg-slate-700 flex items-center gap-1.5 disabled:opacity-40"
                                >
                                  <Icon name="copy" class="w-3 h-3 text-slate-400" /><span>Copy SQL</span>
                                </button>
                                <button
                                  type="button"
                                  disabled={!thread.info}
                                  onclick={() => { openSqlInEditor(thread.info, `Thread_${thread.id}.sql`); openRowMenuId = null; openRowMenuAnchor = null; }}
                                  class="px-2 py-1 rounded bg-slate-800/80 hover:bg-slate-700 flex items-center gap-1.5 disabled:opacity-40"
                                >
                                  <Icon name="file-code" class="w-3 h-3 text-emerald-400" /><span>Editor</span>
                                </button>
                                <button
                                  type="button"
                                  disabled={!thread.info}
                                  onclick={() => { explainThreadQuery(thread.info); openRowMenuId = null; openRowMenuAnchor = null; }}
                                  class="px-2 py-1 rounded bg-slate-800/80 hover:bg-slate-700 flex items-center gap-1.5 disabled:opacity-40"
                                >
                                  <Icon name="play" class="w-3 h-3 text-amber-400" /><span>Explain</span>
                                </button>
                                <button
                                  type="button"
                                  disabled={!thread.info}
                                  onclick={() => { sendThreadToAI(thread.info, thread); openRowMenuId = null; openRowMenuAnchor = null; }}
                                  class="px-2 py-1 rounded bg-slate-800/80 hover:bg-slate-700 flex items-center gap-1.5 disabled:opacity-40"
                                >
                                  <Icon name="bot" class="w-3 h-3 text-purple-400" /><span>Ask AI</span>
                                </button>
                              </div>
                            </div>
                            <div class="px-2.5 py-1.5 flex items-center justify-between gap-1.5">
                              <span class="text-[10px] font-bold uppercase tracking-wider text-slate-500">Kill</span>
                              <div class="flex items-center gap-1 text-[10px]">
                                <button
                                  type="button"
                                  disabled={isSys}
                                  onclick={() => { openRowMenuId = null; openRowMenuAnchor = null; killSingleThread(thread.id, 'query'); }}
                                  class="px-2 py-0.5 rounded bg-amber-950/60 hover:bg-amber-900/70 text-amber-300 font-medium disabled:opacity-35"
                                  title="Kill Query (KILL QUERY)"
                                >Query</button>
                                <button
                                  type="button"
                                  disabled={isSelf || isSys}
                                  onclick={() => { openRowMenuId = null; openRowMenuAnchor = null; killSingleThread(thread.id, 'connection'); }}
                                  class="px-2 py-0.5 rounded bg-rose-950/60 hover:bg-rose-900/70 text-rose-300 font-medium disabled:opacity-35"
                                  title="Kill Connection (KILL)"
                                >Connection</button>
                              </div>
                            </div>
                          </div>
                        {/if}
                      </div>
                    </td>
                  {/if}
                </tr>
              {/each}
            {/if}
          </tbody>
        </table>
      </div>

      <!-- Floating Batch Action Dock -->
      {#if selectedThreadIds.size > 0}
        <div
          class="absolute bottom-4 left-1/2 -translate-x-1/2 z-30 bg-slate-900/95 border border-slate-700 shadow-2xl rounded-full px-3.5 py-2 flex items-center space-x-2 backdrop-blur text-xs font-sans select-none"
        >
          <span class="px-2.5 py-0.5 rounded-full bg-sky-950 text-sky-300 font-mono text-[11px] font-bold border border-sky-800">
            {selectedThreadIds.size} selected
          </span>
          <button
            type="button"
            onclick={() => openBatchKillModal('query')}
            class="px-2.5 py-1 rounded-full bg-amber-950 hover:bg-amber-900 text-amber-200 border border-amber-700/80 text-[11px] font-semibold transition flex items-center space-x-1"
          >
            <Icon name="square" class="w-3 h-3" />
            <span>Kill Query ({selectedThreadIds.size})</span>
          </button>
          <button
            type="button"
            onclick={() => openBatchKillModal('connection')}
            class="px-2.5 py-1 rounded-full bg-rose-600 hover:bg-rose-500 text-white text-[11px] font-bold transition flex items-center space-x-1 shadow"
          >
            <Icon name="x-circle" class="w-3 h-3" />
            <span>Kill Conn ({selectedThreadIds.size})</span>
          </button>
          <button
            type="button"
            onclick={copySelectedThreadIds}
            class="px-2 py-1 rounded-full bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-[11px] transition"
          >
            Copy IDs
          </button>
          <button
            type="button"
            onclick={exportSelectedThreadsJSON}
            class="px-2 py-1 rounded-full bg-slate-800 hover:bg-slate-700 text-slate-200 border border-slate-700 text-[11px] transition"
          >
            Export JSON
          </button>
          <button
            type="button"
            onclick={saveSelectedThreadsToNotes}
            class="px-2 py-1 rounded-full bg-slate-800 hover:bg-slate-700 text-amber-300 border border-slate-700 text-[11px] transition"
          >
            Save to Notes
          </button>
          <button
            type="button"
            onclick={() => selectedThreadIds.clear()}
            class="px-2 py-1 rounded-full text-slate-400 hover:text-white text-[11px] transition"
          >
            Clear
          </button>
        </div>
      {/if}
    </div>
  {:else if currentSubTab === 'digest'}
    <DigestPanel {digestRows} {isDigestLoading} {digestError} onOpenSqlInEditor={openSqlInEditor} onRetry={loadStatementDigest} />
  {:else if currentSubTab === 'deadlock'}
    <DeadlockPanel {deadlockData} {isDeadlockLoading} onOpenSqlInEditor={openSqlInEditor} onRetry={loadDeadlock} />
  {:else if currentSubTab === 'cluster'}
    <ClusterHealthPanel bind:this={clusterPanelRef} {clusterReport} {isClusterLoading} onSwitchNode={switchSessionToNode} onViewThreads={() => setSubTab('threads')} />
  {:else if currentSubTab === 'variables'}
    <ServerVariablesPanel {variablesReport} {isVariablesLoading} />
  {:else if currentSubTab === 'flight-recorder'}
    <FlightRecorderPanel
      {isRecording}
      {recordingSeconds}
      {flightSessions}
      {selectedFlightSessionId}
      {flightSessionTicks}
      bind:selectedTickIndex
      {isFlightLoading}
      {flightPeak}
      {activeOrSelectedTick}
      onToggleRecording={toggleFlightRecording}
      onInspectSession={inspectFlightSession}
      onDeleteSession={deleteFlightSession}
      onOpenSqlInEditor={openSqlInEditor}
      onExplainQuery={explainThreadQuery}
      onSendThreadToAI={sendThreadToAI}
    />
  {/if}
</div>

<svelte:window onkeydown={(e) => { if (e.key === 'Escape' && inspectingThread) inspectingThread = null; }} />

<!-- Thread Detail Modal -->
{#if inspectingThread}
  {@const t = inspectingThread}
  {@const isSelfInspect = t.id === currentThreadId}
  {@const isSysInspect = isSystemThread(t)}
  {@const blockingLocks = locks.filter((l) => l.blocking_thread_id === t.id)}
  {@const waitingLocks = locks.filter((l) => l.waiting_thread_id === t.id)}
  {@const isBlocking = blockingLocks.length > 0}
  {@const isWaiting = waitingLocks.length > 0}
  {@const isCritical = isBlocking || t.time >= 30}
  {@const isWarning = !isCritical && (isWaiting || t.time >= slowThreshold)}
  {@const isIdle = !t.info && t.command === 'Sleep'}
  {@const formattedSql = formatInspectSql && t.info ? formatSQL(t.info) : t.info}
  {@const accentBarClass = isCritical
    ? 'from-rose-500 via-rose-400 to-amber-500'
    : isWarning
      ? 'from-amber-500 via-amber-400 to-orange-400'
      : isIdle
        ? 'from-slate-600 via-slate-500 to-slate-600'
        : 'from-sky-500 via-cyan-400 to-emerald-400'}
  {@const iconWrapClass = isCritical
    ? 'bg-rose-500/15 border-rose-500/30 text-rose-400'
    : isWarning
      ? 'bg-amber-500/15 border-amber-500/30 text-amber-400'
      : isIdle
        ? 'bg-slate-800/80 border-slate-700/80 text-slate-400'
        : 'bg-sky-500/15 border-sky-500/30 text-sky-400'}
  <div
    role="dialog"
    aria-modal="true"
    aria-label="Thread Inspector"
    tabindex="-1"
    onclick={(e) => { if (e.target === e.currentTarget) inspectingThread = null; }}
    onkeydown={(e) => { if (e.key === 'Escape') inspectingThread = null; }}
    class="fixed inset-0 z-50 bg-black/75 backdrop-blur-sm flex items-center justify-center p-4"
  >
    <div class="bg-[#0b0f19] border border-slate-800 rounded-2xl w-full max-w-3xl shadow-2xl shadow-black/60 overflow-hidden flex flex-col animate-scale-in text-xs font-sans">
      <!-- Top status accent hairline -->
      <div class="h-0.5 w-full bg-gradient-to-r {accentBarClass}"></div>

      <!-- Header -->
      <div class="px-5 py-4 border-b border-slate-800/90 flex items-start justify-between gap-4 bg-gradient-to-b from-slate-900/90 to-[#0b0f19]">
        <div class="flex items-start gap-3 min-w-0">
          <div class="w-9 h-9 rounded-xl border flex items-center justify-center shrink-0 {iconWrapClass}">
            <Icon name={isBlocking ? 'shield-alert' : isWaiting ? 'clock' : 'cpu'} class="w-4 h-4" />
          </div>
          <div class="min-w-0">
            <div class="flex items-center flex-wrap gap-2">
              <h3 class="font-bold text-slate-100 text-sm tracking-tight">Thread Inspector</h3>
              <span class="px-2 py-0.5 rounded-md bg-slate-900 border border-slate-700/80 text-sky-300 font-mono text-[11px] font-semibold">
                #{t.id}
              </span>
              {#if isSelfInspect}
                <span class="px-2 py-0.5 rounded-full bg-sky-500/15 text-sky-300 border border-sky-500/30 text-[10px] font-semibold tracking-wide">
                  CURRENT SESSION
                </span>
              {/if}
              {#if isSysInspect}
                <span class="px-2 py-0.5 rounded-full bg-slate-800 text-slate-300 border border-slate-700 text-[10px] font-semibold tracking-wide">
                  SYSTEM DAEMON
                </span>
              {/if}
              {#if isBlocking}
                <span class="px-2 py-0.5 rounded-full bg-rose-500/15 text-rose-300 border border-rose-500/35 text-[10px] font-semibold tracking-wide">
                  BLOCKING {blockingLocks.length}
                </span>
              {:else if isWaiting}
                <span class="px-2 py-0.5 rounded-full bg-amber-500/15 text-amber-300 border border-amber-500/35 text-[10px] font-semibold tracking-wide">
                  WAITING LOCK
                </span>
              {:else if t.time >= 30}
                <span class="px-2 py-0.5 rounded-full bg-rose-500/15 text-rose-300 border border-rose-500/35 text-[10px] font-semibold tracking-wide">
                  LONG RUNNING
                </span>
              {:else if t.time >= slowThreshold}
                <span class="px-2 py-0.5 rounded-full bg-amber-500/15 text-amber-300 border border-amber-500/35 text-[10px] font-semibold tracking-wide">
                  SLOW
                </span>
              {:else if isIdle}
                <span class="px-2 py-0.5 rounded-full bg-slate-800/90 text-slate-400 border border-slate-700/80 text-[10px] font-semibold tracking-wide">
                  IDLE
                </span>
              {:else}
                <span class="px-2 py-0.5 rounded-full bg-emerald-500/15 text-emerald-300 border border-emerald-500/30 text-[10px] font-semibold tracking-wide">
                  ACTIVE
                </span>
              {/if}
            </div>
            <p class="text-[11px] text-slate-400 mt-1 truncate font-mono">
              {t.user || 'unknown'}@{t.host || 'unknown'}
              <span class="text-slate-600 mx-1.5">•</span>
              <span class={t.db ? 'text-emerald-400/90' : 'text-slate-500 italic'}>{t.db || 'no database'}</span>
            </p>
          </div>
        </div>
        <button
          type="button"
          onclick={() => (inspectingThread = null)}
          class="p-1.5 rounded-lg text-slate-400 hover:text-white hover:bg-slate-800/80 transition shrink-0"
          aria-label="Close thread inspector"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Body -->
      <div class="p-5 space-y-4 overflow-y-auto max-h-[75vh]">
        <!-- Telemetry Grid -->
        <div class="grid grid-cols-2 sm:grid-cols-3 gap-2.5">
          <div class="p-3 bg-slate-950/75 border border-slate-800/90 hover:border-slate-700/80 rounded-xl transition">
            <div class="flex items-center gap-1.5 text-slate-500 text-[10px] font-semibold uppercase tracking-wider mb-1">
              <Icon name="user" class="w-3 h-3 text-slate-500" />
              <span>User @ Host</span>
            </div>
            <div class="font-mono text-[11.5px] truncate" title="{t.user}@{t.host}">
              <span class="font-semibold text-slate-100">{t.user || '-'}</span><span class="text-slate-500">@{t.host || '-'}</span>
            </div>
          </div>

          <div class="p-3 bg-slate-950/75 border border-slate-800/90 hover:border-slate-700/80 rounded-xl transition">
            <div class="flex items-center gap-1.5 text-slate-500 text-[10px] font-semibold uppercase tracking-wider mb-1">
              <Icon name="database" class="w-3 h-3 text-slate-500" />
              <span>Database</span>
            </div>
            <div class="font-mono text-[11.5px] font-semibold truncate {t.db ? 'text-emerald-400' : 'text-slate-500 italic'}" title={t.db || 'None'}>
              {t.db || 'None'}
            </div>
          </div>

          <div class="p-3 bg-slate-950/75 border border-slate-800/90 hover:border-slate-700/80 rounded-xl transition">
            <div class="flex items-center gap-1.5 text-slate-500 text-[10px] font-semibold uppercase tracking-wider mb-1">
              <Icon name="clock" class="w-3 h-3 text-slate-500" />
              <span>Execution Time</span>
            </div>
            <div class="flex items-center gap-1.5 font-mono text-[12px] font-bold {t.time >= 30 ? 'text-rose-400' : t.time >= slowThreshold ? 'text-amber-300' : 'text-slate-100'}">
              <span class="w-1.5 h-1.5 rounded-full {t.time >= 30 ? 'bg-rose-400' : t.time >= slowThreshold ? 'bg-amber-400' : 'bg-emerald-400'}"></span>
              <span>{t.time}s</span>
            </div>
          </div>

          <div class="p-3 bg-slate-950/75 border border-slate-800/90 hover:border-slate-700/80 rounded-xl transition">
            <div class="flex items-center gap-1.5 text-slate-500 text-[10px] font-semibold uppercase tracking-wider mb-1">
              <Icon name="terminal" class="w-3 h-3 text-slate-500" />
              <span>Command</span>
            </div>
            <span class="inline-flex items-center px-1.5 py-0.5 rounded bg-sky-500/10 border border-sky-500/20 text-sky-300 font-mono text-[11px] font-semibold">
              {t.command || '-'}
            </span>
          </div>

          <div class="p-3 bg-slate-950/75 border border-slate-800/90 hover:border-slate-700/80 rounded-xl transition">
            <div class="flex items-center gap-1.5 text-slate-500 text-[10px] font-semibold uppercase tracking-wider mb-1">
              <Icon name="activity" class="w-3 h-3 text-slate-500" />
              <span>Thread State</span>
            </div>
            <div class="text-[11.5px] font-medium truncate {t.state ? 'text-slate-200' : 'text-slate-500 italic'}" title={t.state || 'None'}>
              {t.state || 'None'}
            </div>
          </div>

          <div class="p-3 bg-slate-950/75 border border-slate-800/90 hover:border-slate-700/80 rounded-xl transition">
            <div class="flex items-center gap-1.5 text-slate-500 text-[10px] font-semibold uppercase tracking-wider mb-1">
              <Icon name={isBlocking || isWaiting ? 'lock' : 'unlock'} class="w-3 h-3 text-slate-500" />
              <span>Lock Role</span>
            </div>
            <div class="text-[11.5px] font-semibold truncate {isBlocking ? 'text-rose-400' : isWaiting ? 'text-amber-300' : 'text-slate-400'}">
              {isBlocking
                ? `Blocking ${blockingLocks.length} thread${blockingLocks.length > 1 ? 's' : ''}`
                : isWaiting
                  ? `Waiting on #${waitingLocks[0]?.blocking_thread_id}`
                  : 'Unblocked'}
            </div>
          </div>
        </div>

        <!-- Lock Waiter / Blocker Banner -->
        {#if isBlocking || isWaiting}
          <div class="p-3.5 rounded-xl border text-[11px] space-y-2.5 {isBlocking ? 'bg-rose-950/25 border-rose-500/35 text-rose-100' : 'bg-amber-950/25 border-amber-500/35 text-amber-100'}">
            {#if isBlocking}
              <div class="flex items-center justify-between flex-wrap gap-2">
                <div class="font-semibold flex items-center gap-2">
                  <Icon name="shield-alert" class="w-4 h-4 text-rose-400 shrink-0" />
                  <span>Holding <span class="font-mono text-rose-300">{blockingLocks[0]?.lock_mode || 'lock'}</span>{blockingLocks[0]?.locked_resource ? ` on ${blockingLocks[0].locked_resource}` : ''}</span>
                </div>
                <span class="px-1.5 py-0.5 rounded bg-rose-500/20 border border-rose-500/30 text-rose-300 text-[10px] font-bold uppercase tracking-wide">
                  Root Blocker
                </span>
              </div>
              <div class="flex items-center flex-wrap gap-1.5">
                <span class="text-[10.5px] text-rose-300/80 mr-1">Blocked threads:</span>
                {#each blockingLocks as lock (lock.waiting_thread_id)}
                  {@const targetThread = threads.find((x) => x.id === lock.waiting_thread_id)}
                  <button
                    type="button"
                    disabled={!targetThread}
                    onclick={() => { if (targetThread) { inspectingThread = targetThread; formatInspectSql = false; } }}
                    title={targetThread ? `Inspect blocked thread #${lock.waiting_thread_id}` : `Thread #${lock.waiting_thread_id} is no longer in list`}
                    class="px-2 py-0.5 rounded-md bg-rose-950/60 border border-rose-500/30 text-rose-200 font-mono text-[10.5px] transition {targetThread ? 'hover:bg-rose-900/70 hover:border-rose-400/60 cursor-pointer' : 'opacity-65 cursor-default'}"
                  >
                    #{lock.waiting_thread_id} · {lock.waiting_seconds ?? 0}s
                  </button>
                {/each}
              </div>
            {/if}
            {#if isWaiting}
              {@const blockerId = waitingLocks[0]?.blocking_thread_id}
              {@const blockerThread = threads.find((x) => x.id === blockerId)}
              <div class="flex items-center justify-between flex-wrap gap-2">
                <div class="font-semibold flex items-center gap-2">
                  <Icon name="clock" class="w-4 h-4 text-amber-400 shrink-0" />
                  <span>
                    Blocked by
                    <button
                      type="button"
                      disabled={!blockerThread}
                      onclick={() => { if (blockerThread) { inspectingThread = blockerThread; formatInspectSql = false; } }}
                      title={blockerThread ? `Inspect blocker thread #${blockerId}` : `Thread #${blockerId} is no longer in list`}
                      class="mx-1 px-1.5 py-0.5 rounded bg-amber-500/20 border border-amber-500/40 text-amber-200 font-mono transition {blockerThread ? 'hover:bg-amber-500/30 cursor-pointer' : 'opacity-70 cursor-default'}"
                    >
                      #{blockerId}
                    </button>
                    for <span class="font-mono text-amber-300">{waitingLocks[0]?.waiting_seconds ?? 0}s</span> on <span class="font-mono text-amber-300">{waitingLocks[0]?.lock_mode || 'lock'}</span>
                  </span>
                </div>
                <span class="px-1.5 py-0.5 rounded bg-amber-500/20 border border-amber-500/30 text-amber-300 text-[10px] font-bold uppercase tracking-wide">
                  Lock Waiter
                </span>
              </div>
            {/if}
          </div>
        {/if}

        <!-- SQL Statement Panel -->
        <div class="rounded-xl border border-slate-800/90 bg-slate-950/80 overflow-hidden">
          <div class="px-3.5 py-2.5 bg-slate-900/70 border-b border-slate-800/90 flex items-center justify-between flex-wrap gap-2">
            <div class="flex items-center gap-2">
              <Icon name="code" class="w-3.5 h-3.5 text-sky-400" />
              <span class="text-slate-200 text-xs font-semibold">SQL Statement</span>
              {#if t.info}
                <span class="px-1.5 py-0.5 rounded bg-slate-800/90 text-slate-400 font-mono text-[10px]">
                  {t.info.length} chars
                </span>
              {/if}
            </div>

            <div class="flex items-center flex-wrap gap-1.5">
              {#if t.info}
                <button
                  type="button"
                  onclick={() => (formatInspectSql = !formatInspectSql)}
                  class="px-2 py-1 rounded-md border text-[11px] font-medium transition flex items-center gap-1 {formatInspectSql ? 'bg-sky-500/15 border-sky-500/40 text-sky-300' : 'bg-slate-900/80 border-slate-700/80 text-slate-300 hover:text-white hover:border-slate-600'}"
                >
                  <Icon name="code" class="w-3 h-3" />
                  <span>{formatInspectSql ? 'Raw' : 'Format'}</span>
                </button>
              {/if}
              <button
                type="button"
                onclick={() => copyWithToast(t.info || '', 'Thread SQL')}
                disabled={!t.info}
                class="px-2 py-1 rounded-md bg-slate-900/80 border border-slate-700/80 text-[11px] font-medium text-slate-300 hover:text-white hover:border-slate-600 transition flex items-center gap-1 disabled:opacity-40 disabled:cursor-not-allowed"
              >
                <Icon name="copy" class="w-3 h-3 text-sky-400" />
                <span>Copy SQL</span>
              </button>
              <button
                type="button"
                onclick={() => copyWithToast(JSON.stringify(t, null, 2), 'Thread JSON')}
                class="px-2 py-1 rounded-md bg-slate-900/80 border border-slate-700/80 text-[11px] font-medium text-slate-300 hover:text-white hover:border-slate-600 transition flex items-center gap-1"
              >
                <Icon name="copy" class="w-3 h-3 text-slate-400" />
                <span>Copy JSON</span>
              </button>

              {#if t.info}
                <span class="w-px h-4 bg-slate-800 mx-0.5 hidden sm:inline-block"></span>
                <button
                  type="button"
                  onclick={() => { openSqlInEditor(t.info, `Thread_${t.id}.sql`); inspectingThread = null; }}
                  class="px-2 py-1 rounded-md bg-cyan-500/10 border border-cyan-500/25 text-[11px] font-medium text-cyan-300 hover:bg-cyan-500/20 transition flex items-center gap-1"
                >
                  <Icon name="file-code" class="w-3 h-3" />
                  <span>To Tab</span>
                </button>
                <button
                  type="button"
                  onclick={() => appState.saveQueryToNote(t.info, `Thread #${t.id} (${t.user})`)}
                  class="px-2 py-1 rounded-md bg-amber-500/10 border border-amber-500/25 text-[11px] font-medium text-amber-300 hover:bg-amber-500/20 transition flex items-center gap-1"
                >
                  <Icon name="bookmark" class="w-3 h-3" />
                  <span>Note</span>
                </button>
                <button
                  type="button"
                  onclick={() => explainThreadQuery(t.info)}
                  class="px-2 py-1 rounded-md bg-emerald-500/10 border border-emerald-500/25 text-[11px] font-medium text-emerald-300 hover:bg-emerald-500/20 transition flex items-center gap-1"
                >
                  <Icon name="play" class="w-3 h-3" />
                  <span>Explain</span>
                </button>
                <button
                  type="button"
                  onclick={() => sendThreadToAI(t.info, t)}
                  class="px-2 py-1 rounded-md bg-purple-500/10 border border-purple-500/25 text-[11px] font-medium text-purple-300 hover:bg-purple-500/20 transition flex items-center gap-1"
                >
                  <Icon name="bot" class="w-3 h-3" />
                  <span>Ask AI</span>
                </button>
              {/if}
            </div>
          </div>

          {#if formattedSql}
            <pre class="p-4 text-sky-200/95 font-mono text-[12px] leading-relaxed overflow-auto whitespace-pre-wrap break-words max-h-60 selection:bg-sky-500/30">{formattedSql}</pre>
          {:else}
            <div class="p-6 text-center text-slate-500 italic text-xs flex flex-col items-center gap-1.5">
              <Icon name="terminal" class="w-4 h-4 text-slate-600" />
              <span>No active query statement ({t.command || 'idle'})</span>
            </div>
          {/if}
        </div>
      </div>

      <!-- Footer -->
      <div class="px-5 py-3.5 border-t border-slate-800/90 flex items-center justify-between flex-wrap gap-3 bg-slate-900/70">
        <div class="flex items-center flex-wrap gap-2">
          <button
            type="button"
            disabled={isSysInspect}
            onclick={() => killSingleThread(t.id, 'query')}
            class="px-3 py-1.5 rounded-lg bg-amber-950/70 hover:bg-amber-900/80 border border-amber-700/70 text-amber-200 text-xs font-semibold transition flex items-center gap-1.5 disabled:opacity-35 disabled:cursor-not-allowed"
          >
            <Icon name="square" class="w-3.5 h-3.5" />
            <span>Kill Query</span>
          </button>
          <button
            type="button"
            disabled={isSelfInspect || isSysInspect}
            onclick={() => killSingleThread(t.id, 'connection')}
            class="px-3.5 py-1.5 rounded-lg bg-rose-600 hover:bg-rose-500 text-white text-xs font-semibold transition flex items-center gap-1.5 shadow-lg shadow-rose-950/40 disabled:opacity-35 disabled:cursor-not-allowed"
          >
            <Icon name="x-circle" class="w-3.5 h-3.5" />
            <span>Kill Connection</span>
          </button>
          {#if isSysInspect}
            <span class="text-[11px] text-slate-500 ml-1">System daemon protected</span>
          {:else if isSelfInspect}
            <span class="text-[11px] text-slate-500 ml-1">Current session cannot be disconnected</span>
          {/if}
        </div>

        <button
          type="button"
          onclick={() => (inspectingThread = null)}
          class="px-3.5 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 border border-slate-700/80 text-slate-200 text-xs font-medium transition flex items-center gap-2"
        >
          <span>Close</span>
          <kbd class="px-1.5 py-0.5 rounded bg-slate-900 text-slate-400 font-mono text-[10px] border border-slate-700/80">Esc</kbd>
        </button>
      </div>
    </div>
  </div>
{/if}

<!-- Batch Kill Safety Challenge Modal -->
{#if showBatchKillModal}
  <div class="fixed inset-0 z-50 bg-black/80 flex items-center justify-center p-4">
    <div class="bg-[#0b0f19] border border-slate-700 rounded-2xl w-full max-w-2xl shadow-2xl overflow-hidden flex flex-col text-xs font-sans">
      <div class="px-5 py-4 border-b border-slate-800 flex items-center justify-between bg-[#0f172a]">
        <div class="flex items-center space-x-2.5">
          <Icon name={batchKillMode === 'connection' ? 'x-circle' : 'square'} class="w-5 h-5 text-rose-400" />
          <div>
            <h3 class="font-bold text-slate-100 text-sm flex items-center space-x-2">
              <span>Confirm Batch {batchKillMode === 'connection' ? 'KILL CONNECTION' : 'KILL QUERY'}</span>
              <span class="px-2 py-0.5 rounded bg-rose-950 text-rose-300 text-[11px] font-mono border border-rose-800">
                {batchKillTargetIds.length} Threads
              </span>
            </h3>
            <p class="text-[11px] text-slate-400">
              {batchKillMode === 'connection'
                ? 'Disconnecting client connections terminates sessions immediately and rolls back uncommitted work.'
                : 'Cancelling running queries aborts active statements while keeping client connections open.'}
            </p>
          </div>
        </div>
        <button type="button" onclick={() => (showBatchKillModal = false)} class="text-slate-400 hover:text-white" aria-label="Close">
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <div class="p-5 space-y-4">
        <div class="max-h-56 overflow-y-auto bg-[#050811] border border-slate-800 rounded-xl divide-y divide-slate-800/70 p-2 font-mono text-[11px]">
          {#each batchKillTargetIds as id (id)}
            {@const bt = threads.find((x) => x.id === id)}
            <div class="py-1.5 px-2 flex items-center justify-between gap-2 hover:bg-slate-900/60 rounded">
              <div class="min-w-0 flex-1">
                <div class="flex items-center space-x-1.5">
                  <span class="font-bold text-sky-400">#{id}</span>
                  {#if bt}
                    <span class="text-slate-300 font-semibold">{bt.user}@{bt.host}</span>
                    <span class="text-slate-500">({bt.db || '-'})</span>
                    <span class="px-1 rounded bg-slate-800 text-slate-300 text-[10px]">{bt.command}</span>
                  {/if}
                </div>
                {#if bt}
                  <div class="text-[10px] text-slate-400 truncate mt-0.5" title={bt.info || ''}>
                    {bt.info || (bt.command === 'Sleep' ? 'Idle connection (Sleep)' : '-')}
                  </div>
                {/if}
              </div>
              <div class="flex items-center space-x-2 shrink-0">
                {#if bt}
                  <span class="font-bold {bt.time >= 30 ? 'text-rose-400' : bt.time >= 5 ? 'text-amber-400' : 'text-slate-300'}">
                    {bt.time}s
                  </span>
                {/if}
                <button
                  type="button"
                  onclick={() => removeBatchKillTarget(id)}
                  class="text-slate-500 hover:text-rose-400 p-1 rounded hover:bg-slate-800"
                  title="Remove thread #{id} from batch"
                >
                  <Icon name="x" class="w-3.5 h-3.5" />
                </button>
              </div>
            </div>
          {/each}
        </div>

        {#if needsBatchChallenge}
          <div class="p-3.5 bg-rose-950/40 border border-rose-800/80 rounded-xl space-y-2">
            <div class="font-bold text-rose-300 flex items-center space-x-1.5">
              <Icon name="shield-alert" class="w-4 h-4 text-rose-400" />
              <span>
                {isProductionConn()
                  ? 'Production Environment Safety Guardrail'
                  : `Large Batch Safety Guardrail (${batchKillTargetIds.length} threads)`}
              </span>
            </div>
            <p class="text-[11px] text-slate-300">
              Type <strong class="text-white font-mono bg-rose-900/60 px-1.5 py-0.5 rounded border border-rose-700/80">KILL</strong> below to unlock batch execution:
            </p>
            <input
              type="text"
              bind:value={batchKillChallengeInput}
              placeholder="Type KILL here..."
              class="w-full bg-slate-900 border border-rose-700/80 px-3 py-1.5 rounded text-white font-mono text-xs outline-none focus:border-rose-500 uppercase tracking-widest"
            />
          </div>
        {/if}
      </div>

      <div class="px-5 py-3 border-t border-slate-800 flex items-center justify-between bg-[#0f172a]">
        <span class="text-[11px] text-slate-500">Recorded in Session Audit Log.</span>
        <div class="flex items-center space-x-2">
          <button
            type="button"
            onclick={() => (showBatchKillModal = false)}
            class="px-3 py-1.5 rounded bg-slate-800 hover:bg-slate-700 text-slate-300 font-medium transition"
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={!batchChallengeValid || isBatchKilling}
            onclick={executeBatchKill}
            class="px-3.5 py-1.5 rounded bg-rose-600 hover:bg-rose-500 text-white font-bold transition flex items-center space-x-1.5 disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <Icon name="zap" class="w-3.5 h-3.5" />
            <span>{isBatchKilling ? 'Executing...' : `Execute ${batchKillMode === 'connection' ? 'KILL CONNECTION' : 'KILL QUERY'}`}</span>
          </button>
        </div>
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
                        <button onclick={() => copyWithToast(lock.blocking_query, 'Blocking query')} class="text-[10px] text-slate-400 hover:text-white flex items-center space-x-1">
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
                      <button onclick={() => copyWithToast(lock.waiting_query, 'Waiting query')} class="text-[10px] text-slate-400 hover:text-white flex items-center space-x-1">
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
