<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { api } from '../../api/client';
  import Icon from '../common/Icon.svelte';

  let targetTable = $state('');
  let delimiter = $state(',');
  let hasHeader = $state(true);
  let emptyAsNull = $state(true);
  let skipErrors = $state(true);

  let selectedFile: File | null = $state(null);
  let csvHeaders = $state<string[]>([]);
  let csvPreviewRows = $state<string[][]>([]);
  let columnMappings = $state<Record<string, string>>({}); // csvCol -> tableCol

  let isImporting = $state(false);
  let importProgress = $state(0);
  let importStatusMsg = $state('');

  $effect(() => {
    if (appState.isCsvImportModalOpen) {
      targetTable = appState.activeTab?.tableName || appState.filteredTables[0]?.name || '';
      delimiter = ',';
      hasHeader = true;
      emptyAsNull = true;
      skipErrors = true;
      selectedFile = null;
      csvHeaders = [];
      csvPreviewRows = [];
      columnMappings = {};
      isImporting = false;
      importProgress = 0;
      importStatusMsg = '';
    }
  });

  let targetTableColumns = $derived.by(() => {
    if (!targetTable || !appState.catalog?.tables) return [];
    const tbl = appState.catalog.tables.find((t) => t.name === targetTable);
    return (tbl?.columns || []).map((c) => c.name);
  });

  function parseCsvLine(line: string, delim: string): string[] {
    const result: string[] = [];
    let cur = '';
    let inQuotes = false;

    for (let i = 0; i < line.length; i++) {
      const char = line[i];
      if (char === '"' || char === "'") {
        if (inQuotes && line[i + 1] === char) {
          cur += char;
          i++;
        } else {
          inQuotes = !inQuotes;
        }
      } else if (char === delim && !inQuotes) {
        result.push(cur.trim());
        cur = '';
      } else {
        cur += char;
      }
    }
    result.push(cur.trim());
    return result;
  }

  async function handleFileSelect(e: Event) {
    const input = e.target as HTMLInputElement;
    if (input.files && input.files[0]) {
      selectedFile = input.files[0];
      await parseFilePreview();
    }
  }

  async function parseFilePreview() {
    if (!selectedFile) return;
    try {
      const text = await selectedFile.text();
      const lines = text.split(/\r?\n/).filter((l) => l.trim().length > 0);
      if (lines.length === 0) return;

      const firstLineCols = parseCsvLine(lines[0], delimiter);
      if (hasHeader) {
        csvHeaders = firstLineCols;
        csvPreviewRows = lines.slice(1, 6).map((l) => parseCsvLine(l, delimiter));
      } else {
        csvHeaders = firstLineCols.map((_, i) => `Col_${i + 1}`);
        csvPreviewRows = lines.slice(0, 5).map((l) => parseCsvLine(l, delimiter));
      }

      // Auto-map matching columns
      const mapping: Record<string, string> = {};
      csvHeaders.forEach((h) => {
        const cleanH = h.toLowerCase().replace(/[^a-z0-9_]/g, '');
        const matched = targetTableColumns.find(
          (c) => c.toLowerCase().replace(/[^a-z0-9_]/g, '') === cleanH
        );
        mapping[h] = matched || '';
      });
      columnMappings = mapping;
    } catch (err: any) {
      appState.showToast(`Failed to parse CSV: ${err.message}`, 'error');
    }
  }

  async function handleImport() {
    if (!selectedFile || !targetTable || !appState.activeSessionId) return;

    // Check which columns are mapped
    const activePairs = Object.entries(columnMappings).filter(([_, tblCol]) => Boolean(tblCol));
    if (activePairs.length === 0) {
      appState.showToast('Please map at least one CSV column to a table column', 'error');
      return;
    }

    isImporting = true;
    importProgress = 0;
    importStatusMsg = 'Reading full CSV file...';

    try {
      const text = await selectedFile.text();
      const lines = text.split(/\r?\n/).filter((l) => l.trim().length > 0);
      const dataLines = hasHeader ? lines.slice(1) : lines;

      if (dataLines.length === 0) {
        appState.showToast('CSV file contains no records to import', 'warning');
        isImporting = false;
        return;
      }

      const totalRows = dataLines.length;
      const isSQLite = appState.activeConnection?.driver === 'sqlite';
      const quote = isSQLite ? '"' : '`';
      const insertKeyword = skipErrors ? (isSQLite ? 'INSERT OR IGNORE' : 'INSERT IGNORE') : 'INSERT';

      const targetColsSql = activePairs.map(([_, tblCol]) => `${quote}${tblCol}${quote}`).join(', ');

      const batchSize = 100;
      let insertedCount = 0;

      for (let i = 0; i < dataLines.length; i += batchSize) {
        const chunk = dataLines.slice(i, i + batchSize);
        const valuesRows: string[] = [];

        for (const line of chunk) {
          const cells = parseCsvLine(line, delimiter);
          const valTokens: string[] = [];

          for (const [csvCol, _] of activePairs) {
            const colIdx = csvHeaders.indexOf(csvCol);
            const val = colIdx >= 0 && colIdx < cells.length ? cells[colIdx] : '';

            if (emptyAsNull && (val === '' || val.toUpperCase() === 'NULL')) {
              valTokens.push('NULL');
            } else {
              valTokens.push(`'${val.replace(/'/g, "''")}'`);
            }
          }
          valuesRows.push(`(${valTokens.join(', ')})`);
        }

        const query = `${insertKeyword} INTO ${quote}${targetTable}${quote} (${targetColsSql}) VALUES\n${valuesRows.join(',\n')};`;
        await api.executeQuery(appState.activeSessionId, query, false);

        insertedCount += chunk.length;
        importProgress = Math.min(100, Math.round((insertedCount / totalRows) * 100));
        importStatusMsg = `Imported ${insertedCount} of ${totalRows} rows (${importProgress}%)...`;
      }

      appState.showToast(`Imported ${totalRows} rows into ${targetTable}!`, 'success');
      appState.isCsvImportModalOpen = false;
      await appState.refreshCatalog();
    } catch (err: any) {
      appState.showToast(`Import failed: ${err.message}`, 'error');
    } finally {
      isImporting = false;
    }
  }

  function handleClose() {
    appState.isCsvImportModalOpen = false;
  }
</script>

{#if appState.isCsvImportModalOpen}
  <div class="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-4 font-sans">
    <div
      role="dialog"
      aria-modal="true"
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
              CSV Import & Visual Column Mapper
            </h3>
            <p class="text-[11px] text-slate-400">
              Upload delimited files (CSV, TSV) and map fields into database tables
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
              <option value=",">Comma (,)</option>
              <option value=";">Semicolon (;)</option>
              <option value="\t">Tab (\t)</option>
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
            <span>Skip duplicate key errors (INSERT IGNORE)</span>
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

            <div class="max-h-48 overflow-y-auto border border-[#1e293b] rounded-lg">
              <table class="w-full text-left border-collapse text-xs">
                <thead class="bg-[#0f172a] border-b border-[#334155] text-slate-400 text-[10.5px] uppercase tracking-wider sticky top-0">
                  <tr>
                    <th class="py-1.5 px-3">CSV Column Header</th>
                    <th class="py-1.5 px-3">Target Table Column</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-[#1e293b]/60 font-mono text-[11px]">
                  {#each csvHeaders as header}
                    <tr class="hover:bg-slate-800/30">
                      <td class="py-1.5 px-3 font-semibold text-sky-300">{header}</td>
                      <td class="py-1.5 px-3">
                        <select
                          bind:value={columnMappings[header]}
                          class="w-full bg-[#090d16] border border-[#334155] rounded px-2 py-1 text-xs text-white font-mono outline-none focus:border-emerald-500"
                        >
                          <option value="">(Skip / Do Not Import)</option>
                          {#each targetTableColumns as col}
                            <option value={col}>{col}</option>
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
              <span class="font-bold text-slate-300 block">Row Preview (First 5 Rows)</span>
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

        <!-- Progress (during import) -->
        {#if isImporting}
          <div class="space-y-1.5 bg-[#090d16] border border-[#1e293b] p-3 rounded-lg">
            <div class="flex items-center justify-between text-[11px] font-mono">
              <span class="text-slate-300">{importStatusMsg}</span>
              <span class="text-emerald-400 font-bold">{importProgress}%</span>
            </div>
            <div class="w-full bg-slate-800 rounded-full h-2 overflow-hidden">
              <div
                class="bg-emerald-500 h-2 transition-all duration-150"
                style="width: {importProgress}%;"
              ></div>
            </div>
          </div>
        {/if}
      </div>

      <!-- Dialog Footer -->
      <div class="px-5 py-3 bg-[#0f172a] border-t border-[#1e293b] flex items-center justify-end space-x-2 shrink-0">
        <button
          onclick={handleClose}
          type="button"
          class="px-3.5 py-1.5 text-xs font-semibold text-slate-400 hover:text-white transition"
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
