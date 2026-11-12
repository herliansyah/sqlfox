<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';
  import SqlCodeEditor from '../editor/SqlCodeEditor.svelte';

  let eventName = $state('');
  let scheduleType = $state<'EVERY' | 'AT'>('EVERY');
  let everyQty = $state('1');
  let everyUnit = $state<'DAY' | 'HOUR' | 'MINUTE' | 'SECOND' | 'WEEK' | 'MONTH' | 'YEAR'>('DAY');
  let atTime = $state('CURRENT_TIMESTAMP + INTERVAL 1 HOUR');
  let status = $state<'ENABLE' | 'DISABLE' | 'DISABLE ON SLAVE'>('ENABLE');
  let preserve = $state(true);
  let body = $state('BEGIN\n  -- Scheduled event actions\n  DELETE FROM sessions WHERE expires_at < NOW();\nEND');
  let isSubmitting = $state(false);

  $effect(() => {
    if (appState.isEventDesignerOpen) {
      if (appState.eventToEdit) {
        eventName = appState.eventToEdit.name;
        status = (appState.eventToEdit.status as any) || 'ENABLE';
        body = appState.eventToEdit.definition || 'BEGIN\n  -- Scheduled event actions\nEND';
      } else {
        eventName = '';
        scheduleType = 'EVERY';
        everyQty = '1';
        everyUnit = 'DAY';
        atTime = 'CURRENT_TIMESTAMP + INTERVAL 1 HOUR';
        status = 'ENABLE';
        preserve = true;
        body = 'BEGIN\n  -- Scheduled event actions\n  DELETE FROM sessions WHERE expires_at < NOW();\nEND';
      }
      isSubmitting = false;
    }
  });

  let generatedDDL = $derived.by(() => {
    const isSQLite = appState.activeConnection?.driver === 'sqlite';
    const quote = isSQLite ? '"' : '`';
    const cleanName = eventName.trim() || 'evt_daily_cleanup';
    const cleanBody = body.trim().replace(/;+$/, '');

    let scheduleStr = scheduleType === 'EVERY'
      ? `EVERY ${everyQty.trim() || '1'} ${everyUnit}`
      : `AT ${atTime.trim()}`;

    let preserveStr = preserve ? ' ON COMPLETION PRESERVE' : '';
    let statusStr = ` ${status}`;

    return `CREATE EVENT ${quote}${cleanName}${quote}\nON SCHEDULE ${scheduleStr}${preserveStr}${statusStr}\nDO\n${cleanBody};`;
  });

  function openInEditor() {
    appState.addTab(`create_event_${eventName.trim() || 'event'}.sql`, generatedDDL);
    appState.isEventDesignerOpen = false;
  }

  async function handleCreate() {
    if (!eventName.trim()) {
      appState.showToast('Event name is required', 'error');
      return;
    }
    if (!body.trim()) {
      appState.showToast('Event body statements are required', 'error');
      return;
    }
    if (!appState.activeSessionId) return;

    isSubmitting = true;
    try {
      const res = await api.executeQuery(appState.activeSessionId, generatedDDL, false);
      if (res.error) {
        appState.showToast(`Failed to save event: ${res.error}`, 'error');
        return;
      }
      appState.showToast(`Event "${eventName.trim()}" created successfully`, 'success');
      appState.isEventDesignerOpen = false;
      await appState.refreshCatalog();
    } catch (err: any) {
      appState.showToast(`Error: ${err.message}`, 'error');
    } finally {
      isSubmitting = false;
    }
  }

  function handleClose() {
    appState.isEventDesignerOpen = false;
  }
</script>

{#if appState.isEventDesignerOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4">
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="event-designer-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-2xl flex flex-col max-h-[90vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-purple-500/10 border border-purple-500/30 text-purple-400">
            <Icon name="clock" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="event-designer-title" class="font-bold text-sm text-slate-100 font-sans">
              {appState.eventToEdit ? `Edit Event "${appState.eventToEdit.name}"` : 'Create Scheduled Event'}
            </h3>
            <p class="text-[11px] text-slate-400">
              Visual scheduler for recurring maintenance, purges, and automated tasks
            </p>
          </div>
        </div>
        <button
          onclick={handleClose}
          class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition"
          aria-label="Close"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Dialog Body -->
      <div class="p-5 overflow-y-auto space-y-4 flex-1">
        <!-- Event Name & Status -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
          <div class="md:col-span-2 space-y-1">
            <label for="event-name" class="text-xs font-semibold text-slate-300">Event Name *</label>
            <input
              id="event-name"
              type="text"
              bind:value={eventName}
              placeholder="e.g. evt_purge_old_logs"
              class="w-full bg-[#090d16] border border-[#334155] focus:border-purple-500 rounded px-3 py-1.5 text-xs text-white font-mono outline-none"
            />
          </div>

          <div class="space-y-1">
            <label for="event-status" class="text-xs font-semibold text-slate-300">Status</label>
            <select
              id="event-status"
              bind:value={status}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-purple-500 rounded px-2.5 py-1.5 text-xs text-slate-200 font-mono outline-none"
            >
              <option value="ENABLE">ENABLE (Active)</option>
              <option value="DISABLE">DISABLE (Paused)</option>
              <option value="DISABLE ON SLAVE">DISABLE ON SLAVE</option>
            </select>
          </div>
        </div>

        <!-- Schedule Mode -->
        <div class="space-y-2 bg-[#090d16] border border-[#1e293b] p-3 rounded-lg">
          <div class="flex items-center space-x-6 text-xs">
            <label class="flex items-center space-x-2 text-slate-200 cursor-pointer">
              <input
                type="radio"
                name="schedule-type"
                value="EVERY"
                bind:group={scheduleType}
                class="text-purple-500 focus:ring-0"
              />
              <span class="font-semibold">Recurring (EVERY)</span>
            </label>
            <label class="flex items-center space-x-2 text-slate-200 cursor-pointer">
              <input
                type="radio"
                name="schedule-type"
                value="AT"
                bind:group={scheduleType}
                class="text-purple-500 focus:ring-0"
              />
              <span class="font-semibold">One-time (AT)</span>
            </label>
          </div>

          {#if scheduleType === 'EVERY'}
            <div class="flex items-center space-x-2 pt-1">
              <span class="text-xs text-slate-400">Every</span>
              <input
                type="number"
                min="1"
                bind:value={everyQty}
                class="w-20 bg-slate-900 border border-slate-700 rounded px-2 py-1 text-xs text-white font-mono outline-none focus:border-purple-500"
              />
              <select
                bind:value={everyUnit}
                class="bg-slate-900 border border-slate-700 rounded px-2.5 py-1 text-xs text-slate-200 font-mono outline-none focus:border-purple-500"
              >
                <option value="DAY">DAY</option>
                <option value="HOUR">HOUR</option>
                <option value="MINUTE">MINUTE</option>
                <option value="SECOND">SECOND</option>
                <option value="WEEK">WEEK</option>
                <option value="MONTH">MONTH</option>
                <option value="YEAR">YEAR</option>
              </select>
            </div>
          {:else}
            <div class="pt-1">
              <input
                type="text"
                bind:value={atTime}
                placeholder="CURRENT_TIMESTAMP + INTERVAL 1 HOUR"
                class="w-full bg-slate-900 border border-slate-700 rounded px-3 py-1 text-xs text-white font-mono outline-none focus:border-purple-500"
              />
            </div>
          {/if}

          <div class="flex items-center space-x-2 pt-2 border-t border-slate-800/80">
            <input
              id="event-preserve"
              type="checkbox"
              bind:checked={preserve}
              class="rounded bg-slate-900 border-slate-700 text-purple-500 focus:ring-0"
            />
            <label for="event-preserve" class="text-xs text-slate-300 select-none cursor-pointer">
              ON COMPLETION PRESERVE (keep event after execution finishes)
            </label>
          </div>
        </div>

        <!-- Body Statements -->
        <div class="space-y-1">
          <label for="event-body" class="text-xs font-semibold text-slate-300 flex items-center justify-between">
            <span>Event Body Statements *</span>
            <span class="text-[10px] text-slate-500 font-mono">DO &lt;statement&gt; or BEGIN ... END</span>
          </label>
          <div class="h-44">
            <SqlCodeEditor
              id="event-body"
              bind:value={body}
              placeholder={'BEGIN\n  DELETE FROM audit_logs WHERE created_at < DATE_SUB(NOW(), INTERVAL 90 DAY);\nEND'}
            />
          </div>
        </div>

        <!-- DDL Preview -->
        <div class="space-y-1">
          <div class="text-[11px] font-semibold text-slate-400 uppercase tracking-wider flex items-center space-x-1">
            <Icon name="file-code" class="w-3.5 h-3.5 text-purple-400" />
            <span>Generated DDL Preview</span>
          </div>
          <pre class="p-3 rounded bg-[#090d16] border border-slate-800 text-[11px] font-mono text-purple-300/90 overflow-x-auto whitespace-pre-wrap select-text">{generatedDDL}</pre>
        </div>
      </div>

      <!-- Dialog Footer -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-between shrink-0">
        <button
          onclick={openInEditor}
          type="button"
          class="px-3 py-1.5 bg-[#1e293b] hover:bg-slate-700 text-slate-300 hover:text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition border border-[#334155]"
        >
          <Icon name="edit" class="w-3.5 h-3.5 text-purple-400" />
          <span>Open in SQL Editor</span>
        </button>

        <div class="flex items-center space-x-2">
          <button
            onclick={handleClose}
            type="button"
            class="px-3.5 py-1.5 text-xs font-semibold text-slate-400 hover:text-white transition"
          >
            Cancel
          </button>
          <button
            onclick={handleCreate}
            disabled={isSubmitting || !eventName.trim()}
            type="button"
            class="px-4 py-1.5 bg-purple-600 hover:bg-purple-500 disabled:opacity-50 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow-lg shadow-purple-950/40"
          >
            {#if isSubmitting}
              <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
              <span>Saving...</span>
            {:else}
              <Icon name="check" class="w-3.5 h-3.5" />
              <span>Create Event</span>
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
