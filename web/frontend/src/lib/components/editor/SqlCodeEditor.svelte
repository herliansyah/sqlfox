<script lang="ts">
  import { appState } from '../../stores/appState.svelte';
  import { codemirror } from '../../actions/codemirror';
  import type { QuerySnippet } from '../../../types';

  let { value = $bindable(''), placeholder = 'Write SQL…', id = 'sql-editor' }: {
    value?: string;
    placeholder?: string;
    id?: string;
  } = $props();

  function quoteIdentifier(name: string): string {
    if (appState.activeConnection?.driver === 'postgres' || appState.activeConnection?.driver === 'sqlite') {
      return `"${name.replace(/"/g, '""')}"`;
    }
    return `\`${name.replace(/`/g, '``')}\``;
  }
</script>

<div
  id={id}
  class="h-full min-h-0 w-full overflow-hidden rounded-lg border border-slate-700/80 bg-[#090d16] focus-within:border-sky-600/70 transition-colors"
  use:codemirror={{
    id: `designer-${id}`,
    doc: value,
    dialect: appState.activeConnection?.driver,
    activeDatabase: appState.currentDatabase,
    catalogs: appState.catalogs,
    snippets: appState.snippets,
    loadCatalog: appState.ensureEditorCatalog.bind(appState),
    quoteIdentifier,
    placeholder,
    onChange: (next) => (value = next),
    onSnippet: (snippet: QuerySnippet, insert: (sql: string) => void) => {
      appState.requestSnippetParameters(snippet, insert);
    },
  }}
></div>
