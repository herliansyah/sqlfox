<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { focusTrap } from '../../actions/focusTrap';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let targetTable = $state('');
  let delimiter = $state('auto');
  let hasHeader = $state(true);
  let emptyAsNull = $state(true);
  let skipErrors = $state(false);

  let selectedFile: File | null = $state(null);
  let csvHeaders = $state<string[]>([]);
  let csvPreviewRows = $state<string[][]>([]);
  let detectedDelim = $state(',');
  let columnMappings = $state<Record<string, string>>({}); // csvCol -> tableCol

  let isImporting = $state(false);
  let importStatusMsg = $state('');
  let importErrors = $state<string[]>([]);

  let isPostgres = $derived(
    appState.activeConnection?.driver === 'postgres' ||
      appState.activeConnection?.driver === 'postgresql'
  );
  let isSQLite = $derived(appState.activeConnection?.driver === 'sqlite');

  let wasOpen = false;
  $effect(() => {
    if (appState.isCsvImportModalOpen) {
      if (!wasOpen) {
        targetTable = appState.activeTab?.tableName || appState.filteredTables[0]?.name || '';
        delimiter = 'auto';
        hasHeader = true;
        emptyAsNull = true;
        skipErrors = false;
        selectedFile = null;
        csvHeaders = [];
        csvPreviewRows = [];
        detectedDelim = ',';
        columnMappings = {};
        isImporting = false;
        importStatusMsg = '';
        importErrors = [];
        wasOpen = true;
      }
    } else {
      wasOpen = false;
    }
  });

  let activeCatalog = $derived(
    (appState.activeTab?.database && appState.catalogs[appState.activeTab.database.toLowerCase()]) ||
      (appState.currentDatabase && appState.catalogs[appState.currentDatabase.toLowerCase()]) ||
      appState.catalog
  );

  let targetTableColumns = $derived.by(() => {
    if (!targetTable || !activeCatalog?.tables) return [];
    const tbl = activeCatalog.tables.find((t) => t.name.toLowerCase() === targetTable.toLowerCase());
    return tbl?.columns || [];
  });

  function detectDelimiter(sample: string): string {
    const firstLines = sample.split(/\r?\n/).filter((l) => l.trim().length > 0).slice(0, 5);
    if (firstLines.length === 0) return ',';
    const candidates = [',', ';', '\t', '|'];
    let best = ',';
    let bestScore = -1;
    for (const c of candidates) {
      const counts = firstLines.map((l) => l.split(c).length);
      const minC = Math.min(...counts);
      const maxC = Math.max(...counts);
      if (minC > 1) {
        const consistency = minC === maxC ? minC * 10 : minC;
        if (consistency > bestScore) {
          bestScore = consistency;
          best = c;
        }
      }
    }
    return best;
  }

  function parseCsvRows(text: string, delim: string): string[][] {
    const rows: string[][] = [];
    let row: string[] = [];
    let cur = '';
    let inQuotes = false;
    let quoteChar = '';

    for (let i = 0; i < text.length; i++) {
      const char = text[i];
      if (inQuotes) {
        if (char === quoteChar) {
          if (i + 1 < text.length && text[i + 1] === quoteChar) {
            cur += quoteChar;
            i++;
          } else {
            inQuotes = false;
          }
        } else {
          cur += char;
        }
      } else {
        if (char === '"' || char === "'") {
          inQuotes = true;
          quoteChar = char;
        } else if (char === delim) {
          row.push(cur.trim());
          cur = '';
        } else if (char === '\n' || char === '\r') {
          if (char === '\r' && text[i + 1] === '\n') i++;
          row.push(cur.trim());
          if (row.some((field) => field.length > 0)) {
            rows.push(row);
          }
          row = [];
          cur = '';
        } else {
          cur += char;
        }
      }
    }
    if (cur.length > 0 || row.length > 0) {
      row.push(cur.trim());
      if (row.some((field) => field.length > 0)) {
        rows.push(row);
      }
    }
    return rows;
  }

  async function handleFileSelect(e: Event) {
    const input = e.target as HTMLInputElement;
    if (input.files && input.files[0]) {
      selectedFile = input.files[0];
      importStatusMsg = '';
      importErrors = [];
      await parseFilePreview();
    }
  }

  async function parseFilePreview() {
    if (!selectedFile) return;
    try {
      // Only slice the first 64KB for preview so large CSV files do not stall the browser UI
      const sampleText = await selectedFile.slice(0, 65536).text();
      const effectiveDelim = delimiter === 'auto' ? detectDelimiter(sampleText) : delimiter;
      detectedDelim = effectiveDelim;
      const allRows = parseCsvRows(sampleText, effectiveDelim);
      if (allRows.length === 0) return;

      const firstRow = allRows[0]!;
      if (hasHeader) {
        csvHeaders = firstRow;
        csvPreviewRows = allRows.slice(1, 6);
      } else {
        csvHeaders = firstRow.map((_, i) => `Col_${i + 1}`);
        csvPreviewRows = allRows.slice(0, 5);
      }

      // Auto-map matching columns
      const mapping: Record<string, string> = {};
      csvHeaders.forEach((h) => {
        const cleanH = h.toLowerCase().replace(/[^a-z0-9_]/g, '');
        const matched = targetTableColumns.find(
          (c) => c.name.toLowerCase().replace(/[^a-z0-9_]/g, '') === cleanH
        );
        mapping[h] = matched ? matched.name : '';
      });
      columnMappings = mapping;
    } catch (err: any) {
      appState.showToast(`Failed to preview CSV: ${err.message}`, 'error');
    }
  }

  async function handleImport() {
    if (!selectedFile || !targetTable || !appState.activeSessionId) return;

    const activePairs = Object.entries(columnMappings).filter(([_, tblCol]) => Boolean(tblCol));
    if (activePairs.length === 0) {
      appState.showToast('Please map at least one CSV column to a table column', 'error');
      return;
    }

    const mappingObj: Record<string, string> = {};
    for (const [csvCol, tblCol] of activePairs) {
      mappingObj[csvCol] = tblCol;
    }

    isImporting = true;
    importErrors = [];
    importStatusMsg = `Streaming "${selectedFile.name}" to server importer...`;

    try {
      const targetDb = appState.activeTab?.database || appState.currentDatabase || undefined;
      const res = await api.importCsvFormData(appState.activeSessionId, targetTable, selectedFile, {
        database: targetDb,
        delimiter: delimiter === 'auto' ? detectedDelim : delimiter,
        emptyAsNull,
        skipErrors,
        mapping: mappingObj,
      });

      const inserted = res.inserted ?? 0;
      const failed = res.failed ?? 0;
      importErrors = Array.isArray(res.errors) ? res.errors : [];
      importStatusMsg = `Imported ${inserted.toLocaleString()} rows${failed > 0 ? ` (${failed} failed/skipped)` : ''}.`;

      appState.showToast(
        `CSV import complete: ${inserted.toLocaleString()} row(s) inserted into "${targetTable}"`,
        failed > 0 ? 'warning' : 'success'
      );
      await appState.refreshCatalog();
      if (
        appState.activeTab?.isTableViewer &&
        appState.activeTab.tableName?.toLowerCase() === targetTable.toLowerCase()
      ) {
        await appState.loadTableData(appState.activeTab);
      }
      if (failed === 0) {
        appState.isCsvImportModalOpen = false;
      }
    } catch (err: any) {
      importStatusMsg = `Import failed: ${err.message}`;
      appState.showToast(`Import failed: ${err.message}`, 'error');
    } finally {
      isImporting = false;
    }
  }

  function handleClose() {
    if (isImporting) return;
    appState.isCsvImportModalOpen = false;
  }
</script>

{#if appState.isCsvImportModalOpen}
  <div class="fixed inset-0 z-50 bg-black/70 flex items-center justify-center p-4 font-sans">
    <div
      role="dialog"
      aria-modal="true"
      use:focusTrap
      aria-labelledby="csv-modal-title"
      class="bg-[#0b0f19] border border-[#334155] rounded-xl shadow-2xl w-full max-w-3xl flex flex-col max-h-[88vh] overflow-hidden animate-in fade-in zoom-in-95 duration-150"
    >
      <!-- Dialog Header -->
      <div class="px-5 py-4 bg-[#0f172a] border-b border-[#1e293b] flex items-center justify-between shrink-0">
        <div class="flex items-center space-x-2.5">
          <div class="p-1.5 rounded-lg bg-emerald-500/10 border border-emerald-500/30 text-emerald-400">
            <Icon name="upload" class="w-4 h-4" />
          </div>
          <div>
            <h3 id="csv-modal-title" class="font-bold text-sm text-slate-100">
              CSV Import &amp; Visual Column Mapper
            </h3>
            <p class="text-[11px] text-slate-400">
              Stream delimited files (CSV, TSV) via server importer into database tables
            </p>
          </div>
        </div>
        <button
          type="button"
          onclick={handleClose}
          disabled={isImporting}
          class="text-slate-400 hover:text-white p-1 rounded-lg hover:bg-slate-800 transition disabled:opacity-40"
          aria-label="Close"
        >
          <Icon name="x" class="w-4 h-4" />
        </button>
      </div>

      <!-- Dialog Body -->
      <div class="p-5 overflow-y-auto space-y-4 flex-1 text-xs">
        <!-- Configuration Row -->
        <div class="grid grid-cols-1 md:grid-cols-3 gap-3">
          <div class="space-y-1">
            <label for="csv-target-tbl" class="font-semibold text-slate-300">Target Table *</label>
            <select
              id="csv-target-tbl"
              bind:value={targetTable}
              onchange={parseFilePreview}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-emerald-500 rounded px-2.5 py-1.5 text-xs text-white font-mono outline-none"
            >
              {#each appState.filteredTables as tbl}
                <option value={tbl.name}>{tbl.name}</option>
              {/each}
            </select>
          </div>

          <div class="space-y-1">
            <label for="csv-delimiter" class="font-semibold text-slate-300">Delimiter</label>
            <select
              id="csv-delimiter"
              bind:value={delimiter}
              onchange={parseFilePreview}
              class="w-full bg-[#090d16] border border-[#334155] focus:border-emerald-500 rounded px-2.5 py-1.5 text-xs text-white font-mono outline-none"
            >
              <option value="auto">Auto-Detect ({detectedDelim === '\t' ? 'Tab' : detectedDelim})</option>
              <option value=",">Comma (,)</option>
              <option value=";">Semicolon (;)</option>
              <option value={'\t'}>Tab (\t)</option>
              <option value="|">Pipe (|)</option>
            </select>
          </div>

          <div class="space-y-1">
            <label for="csv-file-picker" class="font-semibold text-slate-300">CSV File *</label>
            <input
              id="csv-file-picker"
              type="file"
              accept=".csv,.tsv,.txt"
              onchange={handleFileSelect}
              class="w-full text-xs text-slate-400 file:mr-2 file:py-1 file:px-2.5 file:rounded file:border-0 file:text-[11px] file:font-semibold file:bg-emerald-950 file:text-emerald-400 hover:file:bg-emerald-900 cursor-pointer bg-[#090d16] border border-[#334155] rounded p-1"
            />
          </div>
        </div>

        <!-- Options Checkboxes -->
        <div class="flex flex-wrap items-center gap-4 bg-[#090d16] border border-[#1e293b] p-2.5 rounded-lg text-xs">
          <label class="flex items-center space-x-2 text-slate-300 cursor-pointer select-none">
            <input
              type="checkbox"
              bind:checked={hasHeader}
              onchange={parseFilePreview}
              class="rounded bg-slate-900 border-slate-700 text-emerald-500 focus:ring-0"
            />
            <span>First row has column headers</span>
          </label>

          <label class="flex items-center space-x-2 text-slate-300 cursor-pointer select-none">
            <input
              type="checkbox"
              bind:checked={emptyAsNull}
              class="rounded bg-slate-900 border-slate-700 text-emerald-500 focus:ring-0"
            />
            <span>Treat empty cells as NULL</span>
          </label>

          <label class="flex items-center space-x-2 text-slate-300 cursor-pointer select-none">
            <input
              type="checkbox"
              bind:checked={skipErrors}
              class="rounded bg-slate-900 border-slate-700 text-emerald-500 focus:ring-0"
            />
            <span>
              Skip duplicate/constraint errors ({isPostgres ? 'ON CONFLICT DO NOTHING' : isSQLite ? 'INSERT OR IGNORE' : 'INSERT IGNORE'})
            </span>
          </label>
        </div>

        <!-- Column Mapping Table -->
        {#if csvHeaders.length > 0}
          <div class="space-y-2">
            <div class="flex items-center justify-between">
              <span class="font-bold text-slate-300">Visual Column Mapping</span>
              <span class="text-[11px] text-slate-500 font-mono">
                {Object.values(columnMappings).filter(Boolean).length} of {csvHeaders.length} mapped
              </span>
            </div>

            <div class="max-h-52 overflow-y-auto border border-[#1e293b] rounded-lg">
              <table class="w-full text-left border-collapse text-xs">
                <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-400 text-[10.5px] uppercase tracking-wider sticky top-0">
                  <tr>
                    <th class="py-1.5 px-3">CSV Column Header</th>
                    <th class="py-1.5 px-3">Sample Value</th>
                    <th class="py-1.5 px-3">Target Table Column</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[#1e293b]/60 font-mono text-[11px]">
                  {#each csvHeaders as header, idx}
                    {@const sampleVal = csvPreviewRows[0]?.[idx] ?? ''}
                    <tr class="hover:bg-slate-800/30">
                      <td class="py-1.5 px-3 font-semibold text-sky-300">{header}</td>
                      <td class="py-1.5 px-3 text-slate-400 truncate max-w-[160px]" title={sampleVal}>{sampleVal || '(empty)'}</td>
                      <td class="py-1.5 px-3">
                        <select
                          bind:value={columnMappings[header]}
                          class="w-full bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-xs text-white font-mono outline-none focus:border-emerald-500"
                        >
                          <option value="">(Skip / Do Not Import)</option>
                          {#each targetTableColumns as col}
                            <option value={col.name}>{col.name} ({col.type})</option>
                          {/each}
                        </select>
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          </div>

          <!-- Preview Table -->
          {#if csvPreviewRows.length > 0}
            <div class="space-y-1">
              <span class="font-bold text-slate-300 block">Row Preview (First {csvPreviewRows.length} Rows from 64KB Sample)</span>
              <div class="overflow-x-auto border border-[#1e293b] rounded-lg max-h-32">
                <table class="w-full text-left border-collapse text-[11px] font-mono">
                  <thead class="bg-[#0f172a] text-slate-400 sticky top-0">
                    <tr>
                      {#each csvHeaders as h}
                        <th class="py-1 px-2.5 truncate max-w-[120px]">{h}</th>
                      {/each}
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-[#1e293b]/60">
                    {#each csvPreviewRows as row}
                      <tr class="hover:bg-slate-800/20">
                        {#each row as cell}
                          <td class="py-1 px-2.5 truncate max-w-[120px] text-slate-300">{cell || '-'}</td>
                        {/each}
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
            </div>
          {/if}
        {/if}

        <!-- Status & Errors -->
        {#if importStatusMsg}
          <div class="space-y-1.5 bg-[#090d16] border border-[#1e293b] p-3 rounded-lg">
            <div class="flex items-center justify-between text-[11px] font-mono">
              <span class="text-slate-200">{importStatusMsg}</span>
              {#if isImporting}
                <Icon name="refresh-cw" class="w-3.5 h-3.5 text-emerald-400 animate-spin" />
              {/if}
            </div>
            {#if importErrors.length > 0}
              <div class="max-h-24 overflow-y-auto text-[10.5px] font-mono text-rose-300 bg-rose-950/30 border border-rose-900/50 rounded p-2 space-y-0.5">
                {#each importErrors.slice(0, 20) as err}
                  <div>{err}</div>
                {/each}
              </div>
            {/if}
          </div>
        {/if}
      </div>

      <!-- Dialog Footer -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-end space-x-2 shrink-0">
        <button
          onclick={handleClose}
          disabled={isImporting}
          type="button"
          class="px-3.5 py-1.5 text-xs font-semibold text-slate-400 hover:text-white transition disabled:opacity-40"
        >
          Cancel
        </button>
        <button
          onclick={handleImport}
          disabled={isImporting || !selectedFile || !targetTable}
          type="button"
          class="px-4 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 text-white rounded text-xs font-semibold flex items-center space-x-1.5 transition shadow"
        >
          {#if isImporting}
            <Icon name="refresh-cw" class="w-3.5 h-3.5 animate-spin" />
            <span>Importing...</span>
          {:else}
            <Icon name="check" class="w-3.5 h-3.5" />
            <span>Import into Table</span>
          {/if}
        </button>
      </div>
    </div>
  </div>
{/if}
