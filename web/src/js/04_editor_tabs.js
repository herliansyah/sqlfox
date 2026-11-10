// --- SQL Autocomplete & CodeMirror Engine ---
    const SQL_KEYWORDS = [
      'SELECT', 'FROM', 'WHERE', 'INSERT INTO', 'UPDATE', 'DELETE FROM',
      'JOIN', 'INNER JOIN', 'LEFT JOIN', 'RIGHT JOIN', 'CROSS JOIN',
      'ON', 'GROUP BY', 'ORDER BY', 'HAVING', 'LIMIT', 'OFFSET',
      'UNION ALL', 'UNION', 'DISTINCT', 'AS', 'AND', 'OR', 'NOT',
      'IN', 'BETWEEN', 'LIKE', 'IS NULL', 'IS NOT NULL', 'EXISTS',
      'CASE', 'WHEN', 'THEN', 'ELSE', 'END', 'CREATE TABLE',
      'ALTER TABLE', 'DROP TABLE', 'TRUNCATE TABLE', 'INDEX',
      'PRIMARY KEY', 'FOREIGN KEY', 'SET', 'VALUES', 'ASC', 'DESC',
      'DEFAULT', 'AUTO_INCREMENT', 'DATABASE', 'SCHEMA', 'SHOW TABLES'
    ];

    const SQL_FUNCTIONS = [
      'COUNT()', 'SUM()', 'AVG()', 'MIN()', 'MAX()', 'COALESCE()',
      'CONCAT()', 'NOW()', 'DATE()', 'IFNULL()', 'LOWER()', 'UPPER()',
      'SUBSTRING()', 'TRIM()', 'ROUND()', 'LENGTH()', 'CURRENT_TIMESTAMP()'
    ];

    function computeSQLCompletions(fullText, cursorIndex, schema) {
      // Statement-bounded scoping: find statement surrounding cursor between semicolons
      const lastSemi = fullText.lastIndexOf(';', cursorIndex > 0 ? cursorIndex - 1 : 0);
      const nextSemi = fullText.indexOf(';', cursorIndex);
      const stmtStart = lastSemi >= 0 ? lastSemi + 1 : 0;
      const stmtEnd = nextSemi >= 0 ? nextSemi : fullText.length;
      const stmt = fullText.slice(stmtStart, stmtEnd);
      const textBeforeCursor = fullText.slice(0, cursorIndex);
      const textBeforeInStmt = fullText.slice(stmtStart, cursorIndex);

      // Multi-database schema index
      const activeDBName = (schema && schema.database_name) ? schema.database_name.toLowerCase() : '';
      const availableDatabases = new Set();
      if (schema && schema.databases) {
        schema.databases.forEach(db => availableDatabases.add(db.toLowerCase()));
      }
      if (typeof multiDBCatalogCache === 'object') {
        Object.keys(multiDBCatalogCache).forEach(db => availableDatabases.add(db.toLowerCase()));
      }

      // Extract table aliases in active statement: e.g. FROM users u, JOIN orders AS o, FROM finance.invoices inv
      const aliases = {};
      const aliasRegex = /\b(?:FROM|JOIN)\s+([`"]?[a-zA-Z0-9_]+(?:\.[a-zA-Z0-9_]+)?[`"]?)(?:\s+(?:AS\s+)?([`"]?[a-zA-Z0-9_]+[`"]?))?/gi;
      let match;
      while ((match = aliasRegex.exec(stmt)) !== null) {
        const rawTable = match[1].replace(/[`"]/g, '');
        const rawAlias = match[2] ? match[2].replace(/[`"]/g, '') : null;
        if (rawAlias && !/^(WHERE|ON|JOIN|LEFT|RIGHT|INNER|GROUP|ORDER|LIMIT|SET)$/i.test(rawAlias)) {
          aliases[rawAlias.toLowerCase()] = rawTable;
        }
      }

      // Detect dot-dereferencing:
      // Case A: two dots -> db.table.col_partial (e.g. finance.invoices.am)
      // Case B: one dot -> qualifier.partial (e.g. finance. or users. or alias.)
      const lineUpToCursor = textBeforeCursor.slice(textBeforeCursor.lastIndexOf('\n') + 1);
      const twoDotMatch = lineUpToCursor.match(/([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]*)$/);
      const oneDotMatch = lineUpToCursor.match(/([a-zA-Z0-9_]+)\.([a-zA-Z0-9_]*)$/);
      const wordMatch = lineUpToCursor.match(/([a-zA-Z0-9_]+)$/);

      let candidates = [];

      // 0. Explicit snippet prefix: snip: or snip:search
      const snipPrefixMatch = lineUpToCursor.match(/(?:^|\s)snip:([a-zA-Z0-9_]*)$/i);
      if (snipPrefixMatch) {
        const snipQuery = (snipPrefixMatch[1] || '').toLowerCase();
        let snipCandidates = [];
        (loadedSnippets || []).forEach(s => {
          const lowerTitle = (s.title || '').toLowerCase();
          const lowerTags = (s.tags || '').toLowerCase();
          if (!snipQuery || lowerTitle.includes(snipQuery) || lowerTags.includes(snipQuery)) {
            let score = 250;
            if (lowerTitle.startsWith(snipQuery)) score += 50;
            snipCandidates.push({
              text: s.title,
              displayText: s.title,
              kind: 'SNP',
              badgeClass: 'cm-badge-snp',
              type: s.tags ? `SNIPPET (${s.tags})` : 'SNIPPET',
              score: score,
              snippet: s
            });
          }
        });
        snipCandidates.sort((a, b) => b.score - a.score || a.text.localeCompare(b.text));
        return {
          list: snipCandidates,
          partial: 'snip:' + (snipPrefixMatch[1] || ''),
          hasDot: false
        };
      }

      // 1. Double dot dereferencing: db.table.column (e.g. finance.invoices.id)

      if (twoDotMatch) {
        const dbPart = twoDotMatch[1].toLowerCase();
        const tblPart = twoDotMatch[2].toLowerCase();
        const colPartial = (twoDotMatch[3] || '').toLowerCase();

        const targetCatalog = (typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[dbPart]) || (dbPart === activeDBName ? schema : null);
        let matchedTable = null;
        if (targetCatalog && targetCatalog.tables) {
          matchedTable = targetCatalog.tables.find(t => t.name.toLowerCase() === tblPart);
        }

        if (matchedTable && matchedTable.columns) {
          matchedTable.columns.forEach(col => {
            const lowerCol = col.name.toLowerCase();
            if (!colPartial || lowerCol.includes(colPartial)) {
              let score = 60;
              if (lowerCol === colPartial) score += 100;
              else if (lowerCol.startsWith(colPartial)) score += 50;
              candidates.push({
                text: col.name,
                displayText: col.name,
                kind: 'COL',
                badgeClass: 'cm-badge-col',
                type: col.type || '',
                score: score
              });
            }
          });
        }
        candidates.sort((a, b) => b.score - a.score || a.text.localeCompare(b.text));
        return {
          list: candidates,
          partial: twoDotMatch[3] || '',
          hasDot: true
        };
      }

      // 2. Single dot dereferencing: db.table OR table.col OR alias.col
      if (oneDotMatch) {
        const qualifier = oneDotMatch[1];
        const partial = oneDotMatch[2] || '';
        const lowerQualifier = qualifier.toLowerCase();
        const lowerPartial = partial.toLowerCase();

        // 2a. If qualifier matches a known Database name -> suggest tables in that database!
        if (availableDatabases.has(lowerQualifier)) {
          const dbCatalog = (typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[lowerQualifier]) || (lowerQualifier === activeDBName ? schema : null);
          if (dbCatalog && dbCatalog.tables) {
            dbCatalog.tables.forEach(tbl => {
              const lowerTbl = tbl.name.toLowerCase();
              if (!lowerPartial || lowerTbl.includes(lowerPartial)) {
                let score = 60;
                if (lowerTbl === lowerPartial) score += 100;
                else if (lowerTbl.startsWith(lowerPartial)) score += 50;
                candidates.push({
                  text: tbl.name,
                  displayText: tbl.name,
                  kind: 'TBL',
                  badgeClass: 'cm-badge-tbl',
                  type: tbl.type || 'TABLE',
                  score: score
                });
              }
            });
          }
          candidates.sort((a, b) => b.score - a.score || a.text.localeCompare(b.text));
          return {
            list: candidates,
            partial: partial,
            hasDot: true
          };
        }

        // 2b. Otherwise, qualifier is a table name or alias (either local or cross-db alias)
        let resolved = aliases[lowerQualifier] || qualifier;
        let targetDB = activeDBName;
        let targetTable = resolved;
        if (resolved.includes('.')) {
          const parts = resolved.split('.');
          targetDB = parts[0].toLowerCase();
          targetTable = parts[1];
        }

        const catalog = (typeof multiDBCatalogCache === 'object' && multiDBCatalogCache[targetDB]) || schema;
        let matchedTable = null;
        if (catalog && catalog.tables) {
          matchedTable = catalog.tables.find(t => t.name.toLowerCase() === targetTable.toLowerCase());
        }

        if (matchedTable && matchedTable.columns) {
          matchedTable.columns.forEach(col => {
            const lowerCol = col.name.toLowerCase();
            if (!lowerPartial || lowerCol.includes(lowerPartial)) {
              let score = 50;
              if (lowerCol === lowerPartial) score += 100;
              else if (lowerCol.startsWith(lowerPartial)) score += 50;
              candidates.push({
                text: col.name,
                displayText: col.name,
                kind: 'COL',
                badgeClass: 'cm-badge-col',
                type: col.type || '',
                score: score
              });
            }
          });
        }
        candidates.sort((a, b) => b.score - a.score || a.text.localeCompare(b.text));
        return {
          list: candidates,
          partial: partial,
          hasDot: true
        };
      }

      // 3. No qualifier: Context detection based on preceding clause in statement before cursor
      let partial = '';
      if (wordMatch) {
        partial = wordMatch[1] || '';
      }
      const lowerPartial = partial.toLowerCase();

      const cleanBefore = textBeforeInStmt.replace(/--.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '');
      const keywords = [...cleanBefore.matchAll(/\b(SELECT|FROM|JOIN|LEFT\s+JOIN|RIGHT\s+JOIN|INNER\s+JOIN|WHERE|ON|GROUP\s+BY|ORDER\s+BY|SET|INTO|UPDATE)\b/gi)];
      const clause = keywords.length > 0 ? keywords[keywords.length - 1][1].toUpperCase().replace(/\s+/g, ' ') : '';

      const isTableClause = /^(FROM|JOIN|LEFT JOIN|RIGHT JOIN|INNER JOIN|INTO|UPDATE)$/.test(clause);
      const isColumnClause = /^(SELECT|WHERE|ON|GROUP BY|ORDER BY|SET|HAVING)$/.test(clause);

      // In table clauses (FROM/JOIN): Suggest Databases as well for cross-database querying
      if (isTableClause && availableDatabases.size > 0) {
        availableDatabases.forEach(dbName => {
          if (!lowerPartial || dbName.includes(lowerPartial)) {
            let score = 115;
            if (dbName === lowerPartial) score += 80;
            else if (dbName.startsWith(lowerPartial)) score += 40;
            candidates.push({
              text: dbName,
              displayText: dbName,
              kind: 'DB',
              badgeClass: 'cm-badge-db',
              type: 'DATABASE',
              score: score
            });
          }
        });
      }

      // Collect all tables in current schema
      if (schema && schema.tables) {
        schema.tables.forEach(tbl => {
          const lowerTbl = tbl.name.toLowerCase();
          if (!lowerPartial || lowerTbl.includes(lowerPartial)) {
            let score = 30;
            if (lowerTbl === lowerPartial) score += 100;
            else if (lowerTbl.startsWith(lowerPartial)) score += 50;
            if (isTableClause) score += 120; // Massive boost in FROM/JOIN
            candidates.push({
              text: tbl.name,
              displayText: tbl.name,
              kind: 'TBL',
              badgeClass: 'cm-badge-tbl',
              type: tbl.type || 'TABLE',
              score: score
            });
          }

          // Collect columns if in column clause or as secondary candidates
          if (tbl.columns) {
            const isMentionedTable = stmt.toLowerCase().includes(tbl.name.toLowerCase());
            tbl.columns.forEach(col => {
              const lowerCol = col.name.toLowerCase();
              if (!lowerPartial || lowerCol.includes(lowerPartial)) {
                let score = 20;
                if (lowerCol === lowerPartial) score += 100;
                else if (lowerCol.startsWith(lowerPartial)) score += 50;
                if (isColumnClause) score += 60;
                if (isMentionedTable) score += 30; // Mentioned table columns prioritized!
                candidates.push({
                  text: col.name,
                  displayText: `${col.name}`,
                  kind: 'COL',
                  badgeClass: 'cm-badge-col',
                  type: col.type ? `${tbl.name}.${col.type}` : tbl.name,
                  score: score
                });
              }
            });
          }
        });
      }

      // Collect SQL Functions
      SQL_FUNCTIONS.forEach(fn => {
        const baseName = fn.replace('()', '');
        const lowerFn = baseName.toLowerCase();
        if (!lowerPartial || lowerFn.includes(lowerPartial)) {
          let score = 25;
          if (lowerFn === lowerPartial) score += 100;
          else if (lowerFn.startsWith(lowerPartial)) score += 40;
          if (isColumnClause) score += 30;
          if (isTableClause) score -= 40;
          candidates.push({
            text: fn,
            displayText: fn,
            kind: 'FN',
            badgeClass: 'cm-badge-fn',
            type: 'FUNCTION',
            score: score
          });
        }
      });

      // Collect SQL Keywords (UPPERCASE)
      SQL_KEYWORDS.forEach(kw => {
        const lowerKw = kw.toLowerCase();
        if (!lowerPartial || lowerKw.includes(lowerPartial)) {
          let score = 10;
          if (lowerKw === lowerPartial) score += 100;
          else if (lowerKw.startsWith(lowerPartial)) score += 45;
          if (!isTableClause && !isColumnClause) score += 30;
          candidates.push({
            text: kw,
            displayText: kw,
            kind: 'KW',
            badgeClass: 'cm-badge-kw',
            type: 'KEYWORD',
            score: score
          });
        }
      });

      // Collect Snippets matching partial word
      if (typeof loadedSnippets !== 'undefined' && loadedSnippets && loadedSnippets.length > 0 && lowerPartial && lowerPartial.length >= 2) {
        loadedSnippets.forEach(s => {
          const lowerTitle = (s.title || '').toLowerCase();
          const lowerTags = (s.tags || '').toLowerCase();
          if (lowerTitle.includes(lowerPartial) || lowerTags.includes(lowerPartial)) {
            let score = 35;
            if (lowerTitle.startsWith(lowerPartial)) score += 55;
            candidates.push({
              text: s.title,
              displayText: s.title,
              kind: 'SNP',
              badgeClass: 'cm-badge-snp',
              type: s.tags ? `SNIPPET (${s.tags})` : 'SNIPPET',
              score: score,
              snippet: s
            });
          }
        });
      }

      // Remove duplicates, sort by score descending
      const seen = new Set();
      candidates = candidates.filter(item => {
        const key = `${item.kind}:${item.text}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
      });

      candidates.sort((a, b) => b.score - a.score || a.text.localeCompare(b.text));

      return {
        list: candidates.slice(0, 50),
        partial: partial,
        hasDot: false
      };
    }

    function sqlContextHint(cm) {
      const cur = cm.getCursor();
      const fullText = cm.getValue();
      const cursorIndex = cm.indexFromPos(cur);

      const res = computeSQLCompletions(fullText, cursorIndex, schemaCatalogCache);
      if (!res.list || res.list.length === 0) return null;

      const fromCh = cur.ch - (res.partial ? res.partial.length : 0);

      return {
        from: CodeMirror.Pos(cur.line, fromCh),
        to: CodeMirror.Pos(cur.line, cur.ch),
        list: res.list.map(item => {
          const entry = {
            text: item.text,
            displayText: item.displayText,
            render: function(element, self, data) {
              const badge = document.createElement('span');
              badge.className = 'cm-hint-badge ' + item.badgeClass;
              badge.textContent = item.kind;
              element.appendChild(badge);

              const label = document.createElement('span');
              label.className = 'cm-hint-label';
              label.textContent = item.displayText || item.text;
              element.appendChild(label);

              if (item.type) {
                const typeSpan = document.createElement('span');
                typeSpan.className = 'cm-hint-type';
                typeSpan.textContent = item.type;
                element.appendChild(typeSpan);
              }
            }
          };

          if (item.kind === 'SNP' && item.snippet) {
            entry.hint = function(cmInstance, self, data) {
              cmInstance.replaceRange('', self.from, self.to);
              invokeSnippet(item.snippet);
            };
          }

          return entry;
        })
      };
    }

    function cm6SqlCompletionSource(context) {
      const word = context.matchBefore(/[a-zA-Z0-9_.]*/);
      if (!word && !context.explicit) return null;
      if (!context.explicit && (!word || word.text === '' || word.from === word.to)) return null;
      const fullText = context.state.doc.toString();
      const cursorIndex = context.pos;

      const res = computeSQLCompletions(fullText, cursorIndex, schemaCatalogCache);
      if (!res || !res.list || res.list.length === 0) return null;

      const fromPos = res.partial ? context.pos - res.partial.length : (word ? word.to : context.pos);
      return {
        from: Math.max(0, fromPos),
        to: context.pos,
        options: res.list.map(item => ({
          label: item.displayText || item.text,
          displayLabel: item.displayText || item.text,
          kind: item.kind,
          badgeClass: item.badgeClass,
          type: item.kind ? item.kind.toLowerCase() : 'text',
          detail: item.type || '',
          boost: (item.score || 0) / 100,
          apply: (view, completion, fromPos, toPos) => {
            if (item.kind === 'SNP' && item.snippet && typeof invokeSnippet === 'function') {
              view.dispatch({
                changes: { from: fromPos, to: toPos, insert: '' },
                selection: { anchor: fromPos, head: fromPos }
              });
              invokeSnippet(item.snippet);
            } else {
              view.dispatch({
                changes: { from: fromPos, to: toPos, insert: item.text },
                selection: { anchor: fromPos + item.text.length, head: fromPos + item.text.length }
              });
            }
          }
        })),
        filter: false
      };
    }

    function initCodeMirror() {
      const container = document.getElementById('sql-editor-container');
      if (!container || cmEditor || typeof CodeMirror === 'undefined') return;

      const currentDriver = (typeof activeConnection !== 'undefined' && activeConnection && activeConnection.driver) ? activeConnection.driver : 'mysql';

      cmEditor = CodeMirror(container, {
        value: (tabs.find(t => t.id === activeTabId) || {}).sql || '',
        mode: currentDriver,
        lineNumbers: true,
        matchBrackets: true,
        autoCloseBrackets: true,
        lineWrapping: true,
        completionSource: cm6SqlCompletionSource,
        extraKeys: {
          'Ctrl-Space': function(cm) {
            if (typeof cm.showHint === 'function') {
              cm.showHint({ hint: sqlContextHint, completeSingle: false });
            }
          },
          'Ctrl-Enter': function() {
            runActiveQuery(false, '', null, null, false);
          },
          'Cmd-Enter': function() {
            runActiveQuery(false, '', null, null, false);
          },
          'Ctrl-Alt-Enter': function() {
            runActiveQuery(false, '', null, null, true);
          },
          'Cmd-Alt-Enter': function() {
            runActiveQuery(false, '', null, null, true);
          },
          'F5': function() {
            runActiveQuery(false, '', null, null, true);
          },
          'Ctrl-Shift-Enter': function() {
            explainActiveQuery();
          },
          'Cmd-Shift-Enter': function() {
            explainActiveQuery();
          },
          'Ctrl-Shift-F': function() {
            formatActiveQuery();
          },
          'Cmd-Shift-F': function() {
            formatActiveQuery();
          }
        }
      });
      window.editorInstance = cmEditor;

      cmEditor.on('inputRead', function(cm, change) {
        if (typeof cm.showHint === 'function') {
          cm.showHint({ hint: sqlContextHint, completeSingle: false });
        }
      });

      cmEditor.on('change', function(cm) {
        const val = cm.getValue();
        const curr = tabs.find(t => t.id === activeTabId);
        if (curr && !curr.isTableViewer) {
          curr.sql = val;
        }
        const ta = document.getElementById('sql-editor-textarea');
        if (ta) ta.value = val;
        scheduleSaveTabs();

        // ponytail: re-render tabs dirty dot only when dirty state actually flips
        const isDirtyNow = val.trim().length > 0;
        if (curr && curr._lastDirty !== isDirtyNow) {
          curr._lastDirty = isDirtyNow;
          renderTabs();
        }
      });

      container.addEventListener('mousedown', function(e) {
        if (cmEditor && !e.target.closest('.cm-content')) {
          if (e.target.closest('#notes-tab-preview')) return;
          const view = cmEditor.view;
          if (!view || !view.contentDOM) return;
          const lastLineEl = view.contentDOM.lastElementChild;
          if (lastLineEl) {
            const lastLineRect = lastLineEl.getBoundingClientRect();
            if (e.clientY > lastLineRect.bottom) {
              e.preventDefault();
              cmEditor.focus();
              const endPos = view.state.doc.length;
              view.dispatch({
                selection: { anchor: endPos, head: endPos },
                scrollIntoView: true
              });
            }
          }
        }
      });
    }

    function getStatementAtCursor(cm) {
      if (!cm) return '';
      const text = cm.getValue();
      if (!text || !text.trim()) return '';

      const cursorIdx = cm.indexFromPos(cm.getCursor());
      const statements = [];
      let inSingleQuote = false;
      let inDoubleQuote = false;
      let inBacktick = false;
      let inLineComment = false;
      let inBlockComment = false;
      let inDollarQuote = false;
      let dollarTag = '';
      let stmtStart = 0;

      const len = text.length;
      for (let i = 0; i < len; i++) {
        const ch = text[i];
        const next = i + 1 < len ? text[i + 1] : '';

        if (inLineComment) {
          if (ch === '\n' || ch === '\r') inLineComment = false;
          continue;
        }
        if (inBlockComment) {
          if (ch === '*' && next === '/') {
            inBlockComment = false;
            i++;
          }
          continue;
        }
        if (inSingleQuote) {
          if (ch === '\\') { i++; continue; }
          if (ch === "'") {
            if (next === "'") { i++; continue; }
            inSingleQuote = false;
          }
          continue;
        }
        if (inDoubleQuote) {
          if (ch === '\\') { i++; continue; }
          if (ch === '"') {
            if (next === '"') { i++; continue; }
            inDoubleQuote = false;
          }
          continue;
        }
        if (inBacktick) {
          if (ch === '`') inBacktick = false;
          continue;
        }
        if (inDollarQuote) {
          if (ch === '$' && text.startsWith(dollarTag, i)) {
            i += dollarTag.length - 1;
            inDollarQuote = false;
          }
          continue;
        }

        // Line comment
        if ((ch === '-' && next === '-') || ch === '#') {
          inLineComment = true;
          if (ch === '-') i++;
          continue;
        }
        // Block comment
        if (ch === '/' && next === '*') {
          inBlockComment = true;
          i++;
          continue;
        }

        // Quotes
        if (ch === "'") { inSingleQuote = true; continue; }
        if (ch === '"') { inDoubleQuote = true; continue; }
        if (ch === '`') { inBacktick = true; continue; }

        // Postgres dollar quote
        if (ch === '$') {
          const match = text.slice(i).match(/^(\$[a-zA-Z0-9_]*\$)/);
          if (match) {
            dollarTag = match[1];
            inDollarQuote = true;
            i += dollarTag.length - 1;
            continue;
          }
        }

        // Semicolon
        if (ch === ';') {
          statements.push({
            start: stmtStart,
            end: i + 1,
            sql: text.slice(stmtStart, i + 1).trim()
          });
          stmtStart = i + 1;
        }
      }

      if (stmtStart < len) {
        statements.push({
          start: stmtStart,
          end: len,
          sql: text.slice(stmtStart, len).trim()
        });
      }

      const validStmts = statements.filter(s => {
        const stripped = s.sql.replace(/--.*$/gm, '').replace(/#.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '').replace(/;/g, '').trim();
        return stripped.length > 0;
      });

      if (validStmts.length === 0) return text;
      if (validStmts.length === 1) return validStmts[0].sql;

      let target = validStmts.find(s => cursorIdx >= s.start && cursorIdx <= s.end);
      if (!target) {
        for (let i = validStmts.length - 1; i >= 0; i--) {
          if (cursorIdx >= validStmts[i].start) {
            target = validStmts[i];
            break;
          }
        }
        if (!target) target = validStmts[0];
      }

      // Visual flash indicator on statement execution
      if (target && cm.markText) {
        try {
          const fromPos = cm.posFromIndex(target.start);
          const toPos = cm.posFromIndex(target.end);
          const marker = cm.markText(fromPos, toPos, { className: 'cm-flash-exec' });
          setTimeout(() => {
            try { marker.clear(); } catch (_) {}
          }, 350);
        } catch (_) {}
      }

      return target ? target.sql : text;
    }

    function getEditorSQL(fullScript = false) {
      if (cmEditor) {
        const sel = cmEditor.getSelection();
        if (sel && sel.trim()) return sel.trim();
        if (fullScript) {
          return cmEditor.getValue();
        }
        const stmt = getStatementAtCursor(cmEditor);
        if (stmt && stmt.trim()) return stmt.trim();
        return cmEditor.getValue();
      }
      const el = document.getElementById('sql-editor-textarea');
      return el ? el.value : '';
    }

    function setEditorSQL(val) {
      val = val || '';
      if (cmEditor) {
        if (cmEditor.getValue() !== val) {
          cmEditor.setValue(val);
        }
      }
      const el = document.getElementById('sql-editor-textarea');
      if (el) el.value = val;
    }

    function extractSingleTableFromSQL(sql) {
      if (!sql || typeof sql !== 'string') return null;
      const clean = sql.replace(/--.*$/gm, '').replace(/\/\*[\s\S]*?\*\//g, '').trim();
      if (!/^\s*SELECT\b/i.test(clean)) return null;
      if (/\b(?:JOIN|UNION)\b/i.test(clean)) return null;
      const match = clean.match(/\bFROM\s+(?:[`"\[]?([a-zA-Z0-9_$\u0080-\uffff]+)[`"\]]?\.)?[`"\[]?([a-zA-Z0-9_$\u0080-\uffff]+)[`"\]]?(?:\s+(?:AS\s+)?[`"\[]?[a-zA-Z0-9_$\u0080-\uffff]+[`"\]]?)?(?:\s+(?:WHERE|ORDER|GROUP|HAVING|LIMIT|OFFSET|PROCEDURE|FOR|LOCK|WINDOW|;)|;|\s*$)/i);
      if (!match) return null;
      return { database: match[1] || '', table: match[2] };
    }

    // Driver-aware SQL identifier quoter (PostgreSQL/SQLite use double-quotes, MySQL/MariaDB use backticks)
    function quoteIdentifierForDriver(name) {
      const driver = (activeConnection && activeConnection.driver) ? activeConnection.driver.toLowerCase() : 'mysql';
      const clean = String(name || '').replace(/^[`"\[]+|[`"\]]+$/g, '');
      if (driver === 'postgres' || driver === 'postgresql' || driver === 'pg' || driver === 'sqlite' || driver === 'sqlite3') {
        return `"${clean.replace(/"/g, '""')}"`;
      }
      return `\`${clean.replace(/`/g, '``')}\``;
    }

    // --- Pure Client-Side SQL Formatter ---
    function formatSQL(sql) {
      if (!sql || typeof sql !== 'string') return '';
      const trimmed = sql.trim();
      if (!trimmed) return '';

      // ponytail: zero-dependency SQL formatter breaking clauses, indenting subqueries, and capitalizing keywords
      const majorClauses = [
        'SELECT', 'FROM', 'WHERE', 'HAVING', 'GROUP BY', 'ORDER BY',
        'LIMIT', 'OFFSET', 'UNION ALL', 'UNION',
        'INSERT INTO', 'VALUES', 'UPDATE', 'SET', 'DELETE FROM',
        'LEFT JOIN', 'RIGHT JOIN', 'INNER JOIN', 'CROSS JOIN', 'FULL JOIN', 'JOIN', 'ON',
        'CREATE TABLE', 'ALTER TABLE', 'DROP TABLE',
        'CREATE VIEW', 'ALTER VIEW', 'DROP VIEW',
        'CREATE TRIGGER', 'DROP TRIGGER'
      ];

      const tokens = [];
      let current = '';
      let inString = false;
      let stringChar = '';

      for (let i = 0; i < trimmed.length; i++) {
        const char = trimmed[i];
        if (inString) {
          current += char;
          if (char === stringChar && trimmed[i - 1] !== '\\') {
            inString = false;
            tokens.push({ type: 'string', value: current });
            current = '';
          }
        } else if (char === "'" || char === '"' || char === '`') {
          if (current.trim()) tokens.push({ type: 'word', value: current.trim() });
          current = char;
          inString = true;
          stringChar = char;
        } else if (char === '(' || char === ')' || char === ',' || char === ';') {
          if (current.trim()) tokens.push({ type: 'word', value: current.trim() });
          tokens.push({ type: 'symbol', value: char });
          current = '';
        } else if (/\s/.test(char)) {
          if (current.trim()) {
            tokens.push({ type: 'word', value: current.trim() });
            current = '';
          }
        } else {
          current += char;
        }
      }
      if (current.trim()) tokens.push({ type: inString ? 'string' : 'word', value: current.trim() });

      let result = '';
      let indentLevel = 0;
      const getIndent = () => '  '.repeat(indentLevel);

      for (let i = 0; i < tokens.length; i++) {
        const t = tokens[i];
        if (t.type === 'string') {
          result += t.value + ' ';
          continue;
        }

        const upper = t.value.toUpperCase();
        let matchedClause = '';

        if (i + 1 < tokens.length && tokens[i + 1].type === 'word') {
          const pair = upper + ' ' + tokens[i + 1].value.toUpperCase();
          if (majorClauses.includes(pair)) {
            matchedClause = pair;
            i++;
          }
        }
        if (!matchedClause && majorClauses.includes(upper)) {
          matchedClause = upper;
        }

        if (matchedClause) {
          if (matchedClause === 'ON') {
            result += 'ON ';
          } else {
            if (result.trim() && !result.endsWith('\n')) result += '\n';
            result += getIndent() + matchedClause + ' ';
          }
        } else if (upper === 'AND' || upper === 'OR') {
          result += '\n' + getIndent() + '  ' + upper + ' ';
        } else if (t.value === '(') {
          result += ' (';
          indentLevel++;
        } else if (t.value === ')') {
          if (indentLevel > 0) indentLevel--;
          result = result.trimEnd() + ') ';
        } else if (t.value === ',') {
          result = result.trimEnd() + ',\n' + getIndent() + '  ';
        } else if (t.value === ';') {
          result = result.trimEnd() + ';\n\n';
          indentLevel = 0;
        } else {
          if (SQL_KEYWORDS.includes(upper)) {
            result += upper + ' ';
          } else {
            result += t.value + ' ';
          }
        }
      }

      return result.trim();
    }

    function formatActiveQuery() {
      const sql = getEditorSQL();
      if (!sql || !sql.trim()) return;
      setEditorSQL(formatSQL(sql));
    }

    // --- Global Format & Transaction Shortcuts ---
    window.addEventListener('keydown', (e) => {
      if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'F' || e.key === 'f')) {
        e.preventDefault();
        formatActiveQuery();
      }
      if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'C' || e.key === 'c')) {
        if (isCurrentSessionTxActive) {
          e.preventDefault();
          commitActiveTransaction();
        }
      }
      if ((e.ctrlKey || e.metaKey) && e.shiftKey && (e.key === 'R' || e.key === 'r')) {
        if (isCurrentSessionTxActive) {
          e.preventDefault();
          rollbackActiveTransaction();
        }
      }
    });

    // --- Workspace Pane Splitter (Query Editor vs Data Grid) ---
    // ponytail: interactive draggable horizontal divider with min-height constraints (editor: 80px, grid: 120px),
    // global localStorage persistence, double-click 50:50 reset, and CodeMirror auto-refresh.
    function initWorkspacePaneSplitter() {
      const splitter = document.getElementById('workspace-pane-splitter');
      const editor = document.getElementById('editor-container');
      if (!splitter || !editor) return;

      const minEditorHeight = 80;
      const minGridHeight = 120;

      // Restore saved preference if any
      try {
        const savedHeight = localStorage.getItem('sqlfox_workspace_editor_height');
        if (savedHeight) {
          editor.style.height = savedHeight;
        }
      } catch (_) {}

      let isDragging = false;
      let startY = 0;
      let startHeight = 0;

      splitter.addEventListener('mousedown', (e) => {
        isDragging = true;
        startY = e.clientY;
        startHeight = editor.offsetHeight;
        document.body.style.cursor = 'row-resize';
        document.body.style.userSelect = 'none';
        e.preventDefault();
      });

      // Double-click to reset to 50:50 default
      splitter.addEventListener('dblclick', () => {
        editor.style.height = '50%';
        try {
          localStorage.removeItem('sqlfox_workspace_editor_height');
        } catch (_) {}
        if (cmEditor) cmEditor.refresh();
      });

      window.addEventListener('mousemove', (e) => {
        if (!isDragging) return;
        const deltaY = e.clientY - startY;
        const parent = editor.parentElement;
        const parentHeight = parent ? parent.clientHeight : window.innerHeight;
        const tabsBar = parent ? parent.querySelector('#tabs-container')?.parentElement : null;
        const tabsHeight = tabsBar ? tabsBar.offsetHeight : 36;
        const availableHeight = parentHeight - tabsHeight - splitter.offsetHeight;

        const maxEditorHeight = Math.max(minEditorHeight, availableHeight - minGridHeight);
        const newHeight = Math.max(minEditorHeight, Math.min(maxEditorHeight, startHeight + deltaY));

        editor.style.height = `${newHeight}px`;
        if (cmEditor) cmEditor.refresh();
      });

      window.addEventListener('mouseup', () => {
        if (!isDragging) return;
        isDragging = false;
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
        try {
          localStorage.setItem('sqlfox_workspace_editor_height', editor.style.height);
        } catch (_) {}
        if (cmEditor) cmEditor.refresh();
      });

      window.addEventListener('resize', () => {
        if (!editor || editor.classList.contains('hidden')) return;
        const parent = editor.parentElement;
        if (!parent) return;
        const tabsBar = parent.querySelector('#tabs-container')?.parentElement;
        const tabsHeight = tabsBar ? tabsBar.offsetHeight : 36;
        const availableHeight = parent.clientHeight - tabsHeight - splitter.offsetHeight;
        if (availableHeight > minEditorHeight + minGridHeight) {
          const currentHeight = editor.offsetHeight;
          const maxEditorHeight = availableHeight - minGridHeight;
          if (currentHeight > maxEditorHeight) {
            editor.style.height = `${maxEditorHeight}px`;
            if (cmEditor) cmEditor.refresh();
          }
        }
      });
    }

    // --- Tabs & Editor ---
    function renderTabs(hasSwapped = false) {
      const container = document.getElementById('tabs-container');
      container.innerHTML = tabs.map(t => {
        const isActive = t.id === activeTabId;
        const isDirty = !t.isTableViewer && !t.isProcesslist && !t.isCluster && !t.isFlightRecorder && !t.isVariables && !t.isNote && ((t.id === activeTabId && cmEditor) ? cmEditor.getValue().trim().length > 0 : (t.sql || '').trim().length > 0);
        const icon = t.isExecuting ? `<span class="animate-spin inline-block text-sky-400">${renderIcon('refresh-cw', 'w-3.5 h-3.5')}</span>` :
                     t.isVariables ? renderIcon('sliders', 'w-3.5 h-3.5 text-cyan-400') :
                     t.isFlightRecorder ? renderIcon('disc', 'w-3.5 h-3.5 text-rose-400') :
                     t.isCluster ? renderIcon('network', 'w-3.5 h-3.5 text-emerald-400') :
                     t.isProcesslist ? renderIcon('cpu', 'w-3.5 h-3.5 text-sky-400') :
                     t.isTableViewer ? renderIcon('table', 'w-3.5 h-3.5 text-amber-400') :
                     t.isNote ? renderIcon('file-text', 'w-3.5 h-3.5 text-purple-400') :
                     renderIcon('terminal', 'w-3.5 h-3.5 text-sky-400');
        const activeClasses = 'bg-[#0b1120] text-slate-100 font-semibold border-t-2 border-t-app-accent border-r border-slate-800 shadow-inner';
        const inactiveClasses = 'bg-[#0c1324]/50 hover:bg-slate-800/60 text-slate-400 hover:text-slate-200 border-r border-slate-800/80';
        return `
        <div onclick="switchTab('${t.id}')" oncontextmenu="showTabContextMenu(event, '${t.id}')" class="group flex items-center space-x-1.5 px-3 py-1 cursor-pointer text-xs transition-colors tab-label shrink-0 whitespace-nowrap ${isActive ? activeClasses : inactiveClasses}" title="${escapeHtml(t.title)} (Right-click for options)">
          <span class="text-xs shrink-0">${icon}</span>
          <span class="truncate max-w-[130px]">${escapeHtml(t.title)}</span>
          ${isDirty ? '<span class="w-1.5 h-1.5 rounded-full bg-amber-400 shrink-0" title="Unsaved query"></span>' : ''}
          <button onclick="event.stopPropagation(); closeTab('${t.id}')" class="opacity-0 group-hover:opacity-100 p-0.5 rounded hover:bg-red-500/20 hover:text-red-400 text-slate-400 transition-opacity ml-1 shrink-0" title="Close Tab (Ctrl+W)">
            <svg class="w-3.5 h-3.5" viewBox="0 0 16 16" fill="currentColor"><path d="M4.646 4.646a.5.5 0 0 1 .708 0L8 7.293l2.646-2.647a.5.5 0 0 1 .708.708L8.707 8l2.647 2.646a.5.5 0 0 1-.708.708L8 8.707l-2.646 2.647a.5.5 0 0 1-.708-.708L7.293 8 4.646 5.354a.5.5 0 0 1 0-.708z"/></svg>
          </button>
        </div>
      `;
      }).join('');

      const activeTab = tabs.find(t => t.id === activeTabId);
      const modeTabs = document.getElementById('table-mode-tabs');
      const editorContainer = document.getElementById('editor-container');
      const gridContainer = document.getElementById('grid-container');
      const plContainer = document.getElementById('processlist-view-container');
      const clContainer = document.getElementById('cluster-view-container');
      const frContainer = document.getElementById('flight-recorder-view-container');
      const svContainer = document.getElementById('server-variables-view-container');

      if (activeTab && activeTab.isVariables) {
        if (modeTabs) modeTabs.classList.add('hidden');
        if (editorContainer) editorContainer.classList.add('hidden');
        if (gridContainer) gridContainer.classList.add('hidden');
        if (plContainer) plContainer.classList.add('hidden');
        if (clContainer) clContainer.classList.add('hidden');
        if (frContainer) frContainer.classList.add('hidden');
        if (svContainer) svContainer.classList.remove('hidden');
        filterServerVariables();
      } else if (activeTab && activeTab.isFlightRecorder) {
        if (modeTabs) modeTabs.classList.add('hidden');
        if (editorContainer) editorContainer.classList.add('hidden');
        if (gridContainer) gridContainer.classList.add('hidden');
        if (plContainer) plContainer.classList.add('hidden');
        if (clContainer) clContainer.classList.add('hidden');
        if (svContainer) svContainer.classList.add('hidden');
        if (frContainer) frContainer.classList.remove('hidden');
        refreshFlightRecorderUI();
      } else if (activeTab && activeTab.isCluster) {
        if (modeTabs) modeTabs.classList.add('hidden');
        if (editorContainer) editorContainer.classList.add('hidden');
        if (gridContainer) gridContainer.classList.add('hidden');
        if (plContainer) plContainer.classList.add('hidden');
        if (frContainer) frContainer.classList.add('hidden');
        if (svContainer) svContainer.classList.add('hidden');
        if (clContainer) clContainer.classList.remove('hidden');
        fetchClusterHealth(true);
      } else if (activeTab && activeTab.isProcesslist) {
        if (modeTabs) modeTabs.classList.add('hidden');
        if (editorContainer) editorContainer.classList.add('hidden');
        if (gridContainer) gridContainer.classList.add('hidden');
        if (clContainer) clContainer.classList.add('hidden');
        if (frContainer) frContainer.classList.add('hidden');
        if (svContainer) svContainer.classList.add('hidden');
        if (plContainer) plContainer.classList.remove('hidden');
        if (plTrendPanelOpen) requestAnimationFrame(() => renderProcesslistTrendChart());
        fetchProcesslist(true);
      } else {
        if (plContainer) plContainer.classList.add('hidden');
        if (clContainer) clContainer.classList.add('hidden');
        if (frContainer) frContainer.classList.add('hidden');
        if (svContainer) svContainer.classList.add('hidden');
        if (gridContainer) gridContainer.classList.remove('hidden');
        if (activeTab && activeTab.isTableViewer) {
          if (modeTabs) modeTabs.classList.remove('hidden');
          if (editorContainer) editorContainer.classList.add('hidden');
          setTableMode(activeTab.tableMode || 'data', false);
        } else {
          if (modeTabs) modeTabs.classList.add('hidden');
          if (editorContainer) editorContainer.classList.remove('hidden');
          setTableMode('data', false);
          if (activeTab) {
            setEditorSQL(activeTab.sql || '');
          }
        }
      }

      const exportNotesBtn = document.getElementById('export-notes-btn');
      const notesModeToggle = document.getElementById('notes-tab-mode-toggle');
      const saveAsNoteBtn = document.getElementById('btn-save-as-note');
      if (activeTab && activeTab.isNote) {
        if (exportNotesBtn) exportNotesBtn.classList.remove('hidden');
        if (notesModeToggle) notesModeToggle.classList.remove('hidden');
        if (saveAsNoteBtn) saveAsNoteBtn.classList.add('hidden');
        setNotesTabMode(currentNotesTabMode || 'edit');
      } else {
        if (exportNotesBtn) exportNotesBtn.classList.add('hidden');
        if (notesModeToggle) notesModeToggle.classList.add('hidden');
        if (saveAsNoteBtn) saveAsNoteBtn.classList.remove('hidden');
        const previewEl = document.getElementById('notes-tab-preview');
        if (previewEl) previewEl.classList.add('hidden');
        const cmWrapper = cmEditor ? cmEditor.getWrapperElement() : null;
        if (cmWrapper) cmWrapper.classList.remove('hidden');
      }

      const splitter = document.getElementById('workspace-pane-splitter');
      if (splitter) {
        const isEditorVisible = editorContainer && !editorContainer.classList.contains('hidden');
        splitter.classList.toggle('hidden', !isEditorVisible);
      }

      updateTabDatabasePill(activeTab);

      setTimeout(() => {
        if (cmEditor) cmEditor.refresh();
      }, 10);

      updateTabOverflow(hasSwapped);
    }

    // --- Tab Overflow Dropdown ---
    function updateTabOverflow(hasSwapped = false) {
      const tabsBar = document.getElementById('tabs-bar');
      const container = document.getElementById('tabs-container');
      const overflowWrapper = document.getElementById('tab-overflow-wrapper');
      const overflowCountEl = document.getElementById('tab-overflow-count');
      const addBtn = document.getElementById('add-tab-btn');
      if (!tabsBar || !container) return;

      const barWidth = tabsBar.clientWidth;
      if (!barWidth || barWidth <= 0) {
        if (overflowWrapper) overflowWrapper.classList.add('hidden');
        return;
      }

      const tabEls = Array.from(container.children);
      if (tabEls.length === 0) {
        if (overflowWrapper) overflowWrapper.classList.add('hidden');
        closeTabOverflowMenu();
        return;
      }

      tabEls.forEach(el => el.classList.remove('hidden'));

      const addBtnWidth = addBtn ? (addBtn.offsetWidth || 28) : 28;
      const overflowReserve = 58;
      const maxAllowedWidth = barWidth - addBtnWidth - overflowReserve - 16;

      let currentWidth = 0;
      let visibleCount = 0;

      for (let i = 0; i < tabEls.length; i++) {
        const w = tabEls[i].offsetWidth || 120;
        if (currentWidth + w <= maxAllowedWidth) {
          currentWidth += w + 4;
          visibleCount++;
        } else {
          break;
        }
      }

      if (visibleCount < tabEls.length) {
        const maxWithoutOverflow = barWidth - addBtnWidth - 16;
        let totalW = 0;
        for (let i = 0; i < tabEls.length; i++) {
          totalW += (tabEls[i].offsetWidth || 120) + 4;
        }
        if (totalW <= maxWithoutOverflow) {
          visibleCount = tabEls.length;
        }
      }

      if (visibleCount === 0 && tabEls.length > 0) {
        visibleCount = 1;
      }

      // Rule: active tab must always be in visible portion
      const activeIdx = tabs.findIndex(t => t.id === activeTabId);
      if (!hasSwapped && activeIdx >= visibleCount && visibleCount > 0) {
        const [activeTabObj] = tabs.splice(activeIdx, 1);
        tabs.splice(visibleCount - 1, 0, activeTabObj);
        saveTabsState();
        renderTabs(true);
        return;
      }

      const overflowTabsList = [];
      for (let i = 0; i < tabEls.length; i++) {
        if (i >= visibleCount) {
          tabEls[i].classList.add('hidden');
          if (tabs[i]) overflowTabsList.push(tabs[i]);
        } else {
          tabEls[i].classList.remove('hidden');
        }
      }

      if (overflowTabsList.length > 0) {
        if (overflowWrapper) overflowWrapper.classList.remove('hidden');
        if (overflowCountEl) overflowCountEl.textContent = overflowTabsList.length;
        renderTabOverflowDropdown(overflowTabsList);
      } else {
        if (overflowWrapper) overflowWrapper.classList.add('hidden');
        if (overflowCountEl) overflowCountEl.textContent = '0';
        closeTabOverflowMenu();
        const dd = document.getElementById('tab-overflow-dropdown');
        if (dd) {
          dd.innerHTML = '';
          dd.classList.add('hidden');
        }
      }
    }

    function renderTabOverflowDropdown(overflowTabsList) {
      const dd = document.getElementById('tab-overflow-dropdown');
      if (!dd) return;
      dd.innerHTML = `
        <div class="px-3 py-1.5 text-[10px] uppercase tracking-wider font-bold text-slate-500 border-b border-slate-800 flex items-center justify-between">
          <span>More Open Tabs</span>
          <span class="text-sky-400 font-mono">${overflowTabsList.length}</span>
        </div>
        <div class="py-1">
          ${overflowTabsList.map(t => {
            const isDirty = !t.isTableViewer && !t.isProcesslist && !t.isCluster && !t.isFlightRecorder && !t.isVariables && !t.isNote && ((t.id === activeTabId && cmEditor) ? cmEditor.getValue().trim().length > 0 : (t.sql || '').trim().length > 0);
            const icon = t.isExecuting ? `<span class="animate-spin inline-block text-sky-400">${renderIcon('refresh-cw', 'w-3.5 h-3.5')}</span>` :
                         t.isVariables ? renderIcon('sliders', 'w-3.5 h-3.5 text-cyan-400') :
                         t.isFlightRecorder ? renderIcon('disc', 'w-3.5 h-3.5 text-rose-400') :
                         t.isCluster ? renderIcon('network', 'w-3.5 h-3.5 text-emerald-400') :
                         t.isProcesslist ? renderIcon('cpu', 'w-3.5 h-3.5 text-sky-400') :
                         t.isTableViewer ? renderIcon('table', 'w-3.5 h-3.5 text-amber-400') :
                         t.isNote ? renderIcon('file-text', 'w-3.5 h-3.5 text-purple-400') :
                         renderIcon('terminal', 'w-3.5 h-3.5 text-sky-400');
            return `
              <div onclick="selectOverflowTab('${t.id}')" class="flex items-center justify-between px-3 py-1.5 hover:bg-slate-800 cursor-pointer text-xs group transition">
                <div class="flex items-center space-x-2 truncate mr-2">
                  <span class="text-xs shrink-0">${icon}</span>
                  <span class="text-slate-300 group-hover:text-white truncate max-w-[170px]">${escapeHtml(t.title)}</span>
                  ${isDirty ? '<span class="w-1.5 h-1.5 rounded-full bg-amber-400 shrink-0" title="Unsaved query"></span>' : ''}
                </div>
                <button onclick="event.stopPropagation(); closeTabFromOverflow('${t.id}')" class="text-slate-500 hover:text-red-400 p-0.5 rounded hover:bg-red-500/20 transition shrink-0 ml-1" title="Close Tab"><svg class="w-3.5 h-3.5" aria-hidden="true"><use href="#icon-x"/></svg></button>
              </div>
            `;
          }).join('')}
        </div>
      `;
    }

    function toggleTabOverflowMenu(e) {
      if (e) {
        e.preventDefault();
        e.stopPropagation();
      }
      const dd = document.getElementById('tab-overflow-dropdown');
      const btn = document.getElementById('tab-overflow-btn');
      if (!dd || !btn) return;
      const isHidden = dd.classList.contains('hidden');
      closeAllContextMenus();
      if (isHidden) {
        const rect = btn.getBoundingClientRect();
        const x = Math.min(rect.right - 220, window.innerWidth - 230);
        dd.style.left = `${Math.max(10, x)}px`;
        dd.style.top = `${rect.bottom + 4}px`;
        dd.classList.remove('hidden');
      } else {
        dd.classList.add('hidden');
      }
    }

    function closeTabOverflowMenu() {
      const dd = document.getElementById('tab-overflow-dropdown');
      if (dd) dd.classList.add('hidden');
    }

    function selectOverflowTab(tabId) {
      closeTabOverflowMenu();
      switchTab(tabId);
    }

    async function closeTabFromOverflow(tabId) {
      await closeTab(tabId);
      const overflowWrapper = document.getElementById('tab-overflow-wrapper');
      const dd = document.getElementById('tab-overflow-dropdown');
      if (overflowWrapper && !overflowWrapper.classList.contains('hidden')) {
        if (dd) dd.classList.remove('hidden');
      } else {
        closeTabOverflowMenu();
      }
    }

    let currentNotesTabMode = 'edit';
    function setNotesTabMode(mode) {
      currentNotesTabMode = mode;
      const btnEdit = document.getElementById('btn-notes-mode-edit');
      const btnPreview = document.getElementById('btn-notes-mode-preview');
      const previewEl = document.getElementById('notes-tab-preview');
      const cmWrapper = cmEditor ? cmEditor.getWrapperElement() : null;

      if (mode === 'preview') {
        if (btnEdit) btnEdit.className = 'px-2 py-0.5 rounded text-[11px] text-slate-400 hover:text-white';
        if (btnPreview) btnPreview.className = 'px-2 py-0.5 rounded text-[11px] font-semibold bg-slate-800 text-white';
        if (previewEl) {
          previewEl.innerHTML = renderCopilotMarkdown(getEditorSQL());
          previewEl.classList.remove('hidden');
        }
        if (cmWrapper) cmWrapper.classList.add('hidden');
      } else {
        if (btnEdit) btnEdit.className = 'px-2 py-0.5 rounded text-[11px] font-semibold bg-slate-800 text-white';
        if (btnPreview) btnPreview.className = 'px-2 py-0.5 rounded text-[11px] text-slate-400 hover:text-white';
        if (previewEl) previewEl.classList.add('hidden');
        if (cmWrapper) cmWrapper.classList.remove('hidden');
        if (cmEditor) cmEditor.refresh();
      }
    }

    function exportCurrentNotes() {
      const activeTab = tabs.find(t => t.id === activeTabId);
      const text = activeTab && activeTab.isNote ? getEditorSQL() : '';
      if (!text) { showNativeAlert('No note content to export.', 'Export Notes'); return; }
      const blob = new Blob([text], { type: 'text/markdown' });
      const a = document.createElement('a');
      a.href = URL.createObjectURL(blob);
      a.download = `${(activeTab.title || 'Notes').replace(/\.md$/, '')}.md`;
      a.click();
    }

    // ponytail: Session Tab State Persistence via localStorage scoped per active connection or fallback
    function getTabsStorageKey(targetConnId) {
      if (targetConnId) return `sqlfox_tabs_${targetConnId}`;
      if (activeConnection && activeConnection.id) return `sqlfox_tabs_${activeConnection.id}`;
      try {
        const lastConnId = sessionStorage.getItem('sqlfox_active_conn_id') || localStorage.getItem('sqlfox_last_conn_id');
        if (lastConnId) return `sqlfox_tabs_${lastConnId}`;
      } catch {}
      return 'sqlfox_tabs_default';
    }

    let _saveTabsTimeout = null;
    function scheduleSaveTabs() {
      if (_saveTabsTimeout) clearTimeout(_saveTabsTimeout);
      _saveTabsTimeout = setTimeout(() => {
        saveTabsState();
      }, 250);
    }

    function saveTabsState() {
      const key = getTabsStorageKey();
      try {
        const curr = tabs.find(t => t.id === activeTabId);
        if (curr && !curr.isTableViewer && !curr.isProcesslist && !curr.isCluster && !curr.isFlightRecorder && !curr.isVariables) {
          if (cmEditor) {
            curr.sql = cmEditor.getValue();
          } else {
            const el = document.getElementById('sql-editor-textarea');
            if (el) curr.sql = el.value;
          }
        }
        const storableTabs = tabs
          .map(t => ({
            id: t.id,
            title: t.title,
            sql: t.sql || '',
            isTableViewer: !!t.isTableViewer,
            tableName: t.tableName || '',
            database: t.database || '',
            tableMode: t.tableMode || 'data',
            filterMode: t.filterMode || 'pills',
            filterPills: t.filterPills || [],
            filterRaw: t.filterRaw || '',
            appliedFilter: t.appliedFilter || '',
            queryFilters: t.queryFilters || {},
            sortCol: t.sortCol || '',
            sortDir: t.sortDir || '',
            offset: t.offset || 0,
            pageSize: t.pageSize || 50,
            queryPage: t.queryPage || 0,
            isNote: !!t.isNote,
            isProcesslist: !!t.isProcesslist,
            isCluster: !!t.isCluster,
            isFlightRecorder: !!t.isFlightRecorder,
            isVariables: !!t.isVariables
          }));

        if (storableTabs.length === 0) {
          storableTabs.push({ id: 'tab-1', title: 'Query 1.sql', sql: '', isTableViewer: false });
        }

        const state = {
          activeTabId: tabs.some(t => t.id === activeTabId) ? activeTabId : storableTabs[0].id,
          tabs: storableTabs
        };
        localStorage.setItem(key, JSON.stringify(state));
      } catch (err) {
        console.warn('Failed to save tabs state:', err);
      }
    }

    function restoreTabsState(connId) {
      const key = getTabsStorageKey(connId);
      try {
        const raw = localStorage.getItem(key);
        if (raw) {
          const parsed = JSON.parse(raw);
          if (parsed && Array.isArray(parsed.tabs) && parsed.tabs.length > 0) {
            tabs = parsed.tabs;
            if (activeConnection && activeConnection.driver !== 'mysql' && activeConnection.driver !== 'mariadb' && activeConnection.driver !== 'postgres' && activeConnection.driver !== 'postgresql' && activeConnection.driver !== 'pg') {
              tabs = tabs.filter(t => !t.isProcesslist && !t.isCluster);
              if (tabs.length === 0) {
                tabs = [{ id: 'tab-1', title: 'Query 1.sql', sql: '', isTableViewer: false }];
              }
            }
            activeTabId = (tabs.some(t => t.id === parsed.activeTabId)) ? parsed.activeTabId : tabs[0].id;
            const activeTab = tabs.find(t => t.id === activeTabId);
            if (activeTab && !activeTab.isTableViewer && !activeTab.isProcesslist && !activeTab.isCluster && !activeTab.isFlightRecorder && !activeTab.isVariables) {
              setEditorSQL(activeTab.sql || '');
            }
            renderTabs();

            if (activeTab && activeConnection) {
              if (activeTab.isTableViewer && activeTab.tableName) {
                setTableMode(activeTab.tableMode || 'data', true);
              } else if (activeTab.isProcesslist) {
                initProcesslistFilters();
                fetchProcesslist(false);
              } else if (activeTab.isCluster) {
                fetchClusterHealth(false);
              } else if (activeTab.isVariables) {
                fetchServerVariables();
              } else if (activeTab.isFlightRecorder) {
                loadFlightSessions();
                checkFlightRecorderStatus();
                refreshFlightRecorderUI();
              }
            }
            return true;
          }
        }
      } catch (err) {
        console.warn('Failed to restore tabs state:', err);
      }
      tabs = [{ id: 'tab-1', title: 'Query 1.sql', sql: '', isTableViewer: false }];
      activeTabId = 'tab-1';
      renderTabs();
      setEditorSQL('');
      return false;
    }

    function clearGridToInitialState() {
      currentColumns = [];
      currentResults = [];
      activeGridTable = null;
      activeGridPKCol = null;
      activeGridDB = '';
      activeEditCell = null;
      const multiBar = document.getElementById('multi-result-bar');
      if (multiBar) multiBar.classList.add('hidden');
      activeGridSelectedRow = null;
      activeGridSelectedCol = null;
      activeGridSelectedTd = null;
      if (typeof closeCellDetailDrawer === 'function') closeCellDetailDrawer();
      selectedRowIndices.clear();
      isVirtualGridActive = false;
      virtualGridRows = [];
      virtualGridStartIdx = 0;
      updateGridEditModeUI();
      updateStagedEditsUI();
      updateSelectedRowsUI();
      syncGridToolbarMode();
      const table = document.getElementById('results-table');
      if (table) {
        table.classList.add('table-empty');
        table.style.width = '100%';
      }
      const thead = document.getElementById('grid-thead');
      if (thead) thead.innerHTML = '<tr><th class="p-2.5 text-center text-slate-400 font-semibold w-full">No data</th></tr>';
      const tbody = document.getElementById('grid-tbody');
      if (tbody) tbody.innerHTML = '<tr><td class="p-12 text-center text-slate-500 w-full">Run a query or double click a table in the sidebar to view records</td></tr>';
      const stats = document.getElementById('grid-result-stats');
      if (stats) {
        stats.textContent = '0 rows';
        stats.classList.remove('text-amber-400', 'font-medium');
      }
      const streamPager = document.getElementById('query-stream-pager');
      if (streamPager) {
        streamPager.classList.add('hidden');
        streamPager.classList.remove('flex');
      }
      const bottomStats = document.getElementById('grid-bottom-stats');
      if (bottomStats) bottomStats.textContent = '0 rows';
      const pageInput = document.getElementById('grid-page-input');
      if (pageInput) pageInput.value = '1';
      const totalPagesEl = document.getElementById('grid-total-pages');
      if (totalPagesEl) totalPagesEl.textContent = '1';
      const btnFirst = document.getElementById('btn-page-first');
      if (btnFirst) btnFirst.disabled = true;
      const btnPrev = document.getElementById('btn-page-prev');
      if (btnPrev) btnPrev.disabled = true;
      const btnNext = document.getElementById('btn-page-next');
      if (btnNext) btnNext.disabled = true;
      const btnLast = document.getElementById('btn-page-last');
      if (btnLast) btnLast.disabled = true;
      const btnCount = document.getElementById('btn-count-accurate');
      if (btnCount) btnCount.classList.add('hidden');
      const filterBar = document.getElementById('table-filter-bar');
      if (filterBar) filterBar.classList.add('hidden');
      const paginationBar = document.getElementById('grid-pagination-bar');
      if (paginationBar) paginationBar.classList.add('hidden');
      const profileBtn = document.getElementById('btn-view-profile');
      if (profileBtn) profileBtn.classList.add('hidden');
    }

    // ponytail: Countdown progress bar indicator using GPU-accelerated CSS transition.
    // Drains smoothly from 100% to 0% over interval duration with zero continuous JS frame loops.
    function triggerCountdownBar(barId, interval) {
      const bar = document.getElementById(barId);
      if (!bar) return;
      if (!interval || interval === 'pause' || typeof interval !== 'number' || document.hidden) {
        bar.style.transition = 'none';
        bar.style.width = '0%';
        return;
      }
      // Instant reset to 100% without transition
      bar.style.transition = 'none';
      bar.style.width = '100%';
      // Force layout reflow so next transition triggers properly
      void bar.offsetWidth;
      // Animate smoothly to 0% over interval duration
      bar.style.transition = `width ${interval}ms linear`;
      bar.style.width = '0%';
    }

    // ponytail: silent background auto-wakeup on tab focus when left idle (>5m)
    let lastTabVisibilityTimestamp = Date.now();
    let isWakingUpSession = false;

    async function wakeUpSessionIfNeeded() {
      if (isWakingUpSession) return;
      const now = Date.now();
      const elapsedMinutes = (now - lastTabVisibilityTimestamp) / 1000 / 60;
      lastTabVisibilityTimestamp = now;

      if (elapsedMinutes >= 5) {
        isWakingUpSession = true;
        try {
          if (typeof checkAuthStatus === 'function') {
            await checkAuthStatus();
          }
          if (activeConnection && typeof pingActiveSession === 'function') {
            await pingActiveSession();
          }
        } catch (_) {
          // quiet auto-wakeup probe
        } finally {
          isWakingUpSession = false;
        }
      }
    }

    document.addEventListener('visibilitychange', () => {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (document.hidden) {
        lastTabVisibilityTimestamp = Date.now();
        triggerCountdownBar('pl-refresh-bar', 'pause');
        triggerCountdownBar('cl-refresh-bar', 'pause');
      } else {
        wakeUpSessionIfNeeded();
        if (activeTab && activeTab.isProcesslist && typeof processlistRefreshInterval !== 'undefined' && processlistRefreshInterval !== 'pause') {
          fetchProcesslist(true);
          triggerCountdownBar('pl-refresh-bar', processlistRefreshInterval);
        }
        if (activeTab && activeTab.isCluster && typeof clusterRefreshInterval !== 'undefined' && clusterRefreshInterval !== 'pause') {
          fetchClusterHealth(true);
          triggerCountdownBar('cl-refresh-bar', clusterRefreshInterval);
        }
      }
    });

    window.addEventListener('focus', () => {
      if (!document.hidden) {
        wakeUpSessionIfNeeded();
      }
    });

    function switchTab(tabId) {
      const prevTabId = activeTabId;
      if (tabId !== activeTabId) {
        selectedRowIndices.clear();
        updateSelectedRowsUI();
      }
      const curr = tabs.find(t => t.id === activeTabId);
      if (curr && !curr.isTableViewer && !curr.isProcesslist && !curr.isCluster && !curr.isFlightRecorder && !curr.isVariables) {
        curr.sql = cmEditor ? cmEditor.getValue() : (document.getElementById('sql-editor-textarea') || {}).value || '';
      }
      activeTabId = tabId;
      renderTabs();
      syncGridToolbarMode();
      const nextTab = tabs.find(t => t.id === tabId);
      if (nextTab && nextTab.isTableViewer && nextTab.tableName) {
        // The grid is shared by all tabs, so a different tab's rows (another DB's same-named table,
        // or a query result on that table) must never be reused: reload whenever the tab changes.
        if (prevTabId !== tabId || activeGridTable !== nextTab.tableName || currentResults.length === 0) {
          setTableMode(nextTab.tableMode || 'data', true);
        }
      } else if (nextTab && !nextTab.isTableViewer && nextTab.rawQueryError) {
        renderQueryErrorBanner(nextTab.rawQueryError.sql, nextTab.rawQueryError.message);
      } else if (nextTab && !nextTab.isTableViewer && nextTab.rawQueryRows && nextTab.rawQueryCols) {
        renderGridResults(nextTab.rawQueryCols, nextTab.rawQueryRows, null, nextTab.queryTable || null, nextTab.queryDB || nextTab.database);
        if (nextTab.queryFilters && Object.keys(nextTab.queryFilters).length > 0) {
          applyQueryResultsFilter(nextTab);
        }
        if (nextTab.sortCol) {
          renderGridResults(currentColumns, sortQueryRows(nextTab, currentResults), activeGridPKCol, activeGridTable, activeGridDB, true);
        }
        const stats = document.getElementById('grid-result-stats');
        if (stats && nextTab.queryStats) {
          stats.textContent = nextTab.queryStats;
          if (nextTab.hasMoreStream) {
            stats.classList.add('text-amber-400', 'font-medium');
          } else {
            stats.classList.remove('text-amber-400', 'font-medium');
          }
        }
      } else if (nextTab && !nextTab.isTableViewer && nextTab.rawQueryExec) {
        renderQuerySuccessBanner(
          nextTab.rawQueryExec.sql,
          nextTab.rawQueryExec.message,
          nextTab.rawQueryExec.executionMs,
          nextTab.rawQueryExec.rowsAffected,
          nextTab.rawQueryExec.backupFile
        );
        const stats = document.getElementById('grid-result-stats');
        if (stats && nextTab.queryStats) {
          stats.textContent = nextTab.queryStats;
          if (nextTab.hasMoreStream) {
            stats.classList.add('text-amber-400', 'font-medium');
          } else {
            stats.classList.remove('text-amber-400', 'font-medium');
          }
        }
      } else if (nextTab && !nextTab.isTableViewer && !nextTab.isProcesslist && !nextTab.isCluster && !nextTab.isFlightRecorder && !nextTab.isVariables) {
        // ponytail: Clean state isolation. Query tab with no execution results shows a clean empty grid
        // instead of retaining previous tab's table or query results.
        clearGridToInitialState();
      }

      // Sync auto-refresh countdown bar on active tab
      if (nextTab && nextTab.isProcesslist) {
        if (typeof processlistRefreshInterval !== 'undefined' && processlistRefreshInterval !== 'pause') {
          triggerCountdownBar('pl-refresh-bar', processlistRefreshInterval);
        }
      } else {
        triggerCountdownBar('pl-refresh-bar', 'pause');
      }

      if (nextTab && nextTab.isCluster) {
        if (typeof clusterRefreshInterval !== 'undefined' && clusterRefreshInterval !== 'pause') {
          triggerCountdownBar('cl-refresh-bar', clusterRefreshInterval);
        }
      }

      updateTabDatabasePill(nextTab);
      updateQueryStreamPagerUI(nextTab);
      updateTransactionStatusUI(nextTab?.txActive || false);
      renderMultiResultTabsUI(nextTab);
      if (isQueryExecuting) {
        const executingTab = tabs.find(t => t.isExecuting);
        const runBtn = document.getElementById('btn-run-query');
        if (runBtn) {
          runBtn.title = executingTab && executingTab.id !== tabId
            ? `Query is currently executing in '${executingTab.title}'`
            : 'Executing query...';
        }
      }
      saveTabsState();
    }

    function getNextQueryTabTitle() {
      let maxNum = 0;
      const existingTitles = new Set(tabs.map(t => (t.title || '').toLowerCase().trim()));
      const queryRegex = /^query\s+(\d+)\.sql$/i;
      for (const t of tabs) {
        const m = (t.title || '').trim().match(queryRegex);
        if (m) {
          const num = parseInt(m[1], 10);
          if (num > maxNum) maxNum = num;
        }
      }
      let nextNum = maxNum + 1;
      while (existingTitles.has(`query ${nextNum}.sql`)) {
        nextNum++;
      }
      return `Query ${nextNum}.sql`;
    }

    function addNewQueryTab() {
      const newId = 'tab-' + Date.now();
      const currentActiveTab = tabs.find(t => t.id === activeTabId);
      const inheritedDB = (currentActiveTab && currentActiveTab.database) || schemaCatalogCache?.database_name || activeConnection?.database_name || '';
      tabs.push({ id: newId, title: getNextQueryTabTitle(), sql: '', isTableViewer: false, database: inheritedDB });
      switchTab(newId);
      saveTabsState();
    }

    // --- Tab Database Scope & Pill (ADR 0055) ---
    function updateTabDatabasePill(tab) {
      const pillContainer = document.getElementById('tab-database-pill-container');
      const pillName = document.getElementById('tab-database-pill-name');
      if (!pillContainer || !pillName) return;
      if (!tab || tab.isTableViewer || tab.isProcesslist || tab.isCluster || tab.isFlightRecorder || tab.isVariables) {
        pillContainer.classList.add('hidden');
        return;
      }
      pillContainer.classList.remove('hidden');
      if (!tab.database) {
        tab.database = schemaCatalogCache?.database_name || activeConnection?.database_name || 'default';
      }
      pillName.textContent = tab.database || 'default';
    }

    let cachedTabDatabases = [];
    let tabDbDropdownSelectedIndex = -1;
    let currentFilteredTabDatabases = [];

    function renderTabDatabaseList(filterQuery = '') {
      const activeTab = tabs.find(t => t.id === activeTabId);
      const itemsContainer = document.getElementById('tab-db-switcher-items');
      if (!itemsContainer) return;
      const currentDb = (activeTab && activeTab.database) || schemaCatalogCache?.database_name || activeConnection?.database_name || 'default';
      const q = (filterQuery || '').trim().toLowerCase();
      const filtered = q ? cachedTabDatabases.filter(d => d.toLowerCase().includes(q)) : cachedTabDatabases;
      currentFilteredTabDatabases = filtered;

      if (filtered.length === 0) {
        tabDbDropdownSelectedIndex = -1;
        itemsContainer.innerHTML = '<div class="text-slate-500 text-center py-3 text-xs italic">No matching database</div>';
        return;
      }

      if (tabDbDropdownSelectedIndex >= filtered.length) {
        tabDbDropdownSelectedIndex = filtered.length > 0 ? filtered.length - 1 : -1;
      }

      itemsContainer.innerHTML = filtered.map((db, idx) => {
        const isCurrent = db.toLowerCase() === currentDb.toLowerCase();
        const isHighlighted = idx === tabDbDropdownSelectedIndex;
        let itemClasses = 'w-full text-left px-3 py-1.5 flex items-center justify-between transition cursor-pointer ';
        if (isHighlighted) {
          itemClasses += 'bg-amber-900/80 text-white font-semibold ring-1 ring-amber-400';
        } else if (isCurrent) {
          itemClasses += 'text-amber-400 font-bold bg-amber-950/30 hover:bg-slate-800';
        } else {
          itemClasses += 'text-slate-300 hover:bg-slate-800';
        }

        return `
          <button id="tab-db-item-${idx}" onclick="selectTabDatabase(${jsArg(db)})" class="${itemClasses}">
            <span class="font-mono text-xs truncate max-w-[190px]">${escapeHtml(db)}</span>
            ${isCurrent ? '<span class="text-xs shrink-0">✓</span>' : ''}
          </button>
        `;
      }).join('');

      if (tabDbDropdownSelectedIndex >= 0) {
        const activeItem = document.getElementById(`tab-db-item-${tabDbDropdownSelectedIndex}`);
        if (activeItem) {
          activeItem.scrollIntoView({ block: 'nearest' });
        }
      }
    }

    function filterTabDatabaseList(query) {
      tabDbDropdownSelectedIndex = -1;
      renderTabDatabaseList(query);
    }

    function handleTabDbSearchKeydown(e) {
      if (e.key === 'Escape') {
        e.preventDefault();
        closeAllContextMenus();
      } else if (e.key === 'ArrowDown') {
        e.preventDefault();
        if (currentFilteredTabDatabases.length > 0) {
          tabDbDropdownSelectedIndex = (tabDbDropdownSelectedIndex + 1) % currentFilteredTabDatabases.length;
          const searchInput = document.getElementById('tab-db-search-input');
          renderTabDatabaseList(searchInput ? searchInput.value : '');
        }
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        if (currentFilteredTabDatabases.length > 0) {
          tabDbDropdownSelectedIndex = tabDbDropdownSelectedIndex <= 0
            ? currentFilteredTabDatabases.length - 1
            : tabDbDropdownSelectedIndex - 1;
          const searchInput = document.getElementById('tab-db-search-input');
          renderTabDatabaseList(searchInput ? searchInput.value : '');
        }
      } else if (e.key === 'Enter') {
        e.preventDefault();
        const targetIdx = tabDbDropdownSelectedIndex >= 0 ? tabDbDropdownSelectedIndex : 0;
        if (currentFilteredTabDatabases.length > 0 && targetIdx < currentFilteredTabDatabases.length) {
          selectTabDatabase(currentFilteredTabDatabases[targetIdx]);
        }
      }
    }

    function openTabDatabaseSwitcher(e) {
      if (e) {
        e.preventDefault();
        e.stopPropagation();
      }
      closeAllContextMenus();
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (!activeTab) return;
      const menu = document.getElementById('tab-db-switcher-menu');
      const pill = document.getElementById('tab-database-pill-container');
      const searchInput = document.getElementById('tab-db-search-input');
      if (!menu || !pill) return;

      const dbs = (schemaCatalogCache && Array.isArray(schemaCatalogCache.databases) && schemaCatalogCache.databases.length > 0)
        ? schemaCatalogCache.databases
        : (activeConnection ? [activeConnection.database_name || 'default'] : ['default']);

      cachedTabDatabases = dbs;
      tabDbDropdownSelectedIndex = -1;
      if (searchInput) searchInput.value = '';
      renderTabDatabaseList('');

      const rect = pill.getBoundingClientRect();
      menu.style.top = `${rect.bottom + 4}px`;
      menu.style.left = `${rect.left}px`;
      menu.classList.remove('hidden');
      setTimeout(() => { if (searchInput) searchInput.focus(); }, 50);
    }

    function selectTabDatabase(dbName) {
      const activeTab = tabs.find(t => t.id === activeTabId);
      if (activeTab) {
        activeTab.database = dbName;
        updateTabDatabasePill(activeTab);
        saveTabsState();
        showToast(`Tab database context switched to '${dbName}'`);
      }
      closeAllContextMenus();
    }

    // --- Tab Context Menu & Lifecycle Management ---
    let selectedContextMenuTabId = null;

    function isTabDirtyOrNonEmpty(tab) {
      if (!tab) return false;
      if (!tab.isTableViewer && !tab.isProcesslist && !tab.isCluster && !tab.isFlightRecorder && !tab.isVariables && !tab.isNote) {
        const sql = (tab.id === activeTabId && cmEditor) ? cmEditor.getValue().trim() : (tab.sql || '').trim();
        return sql.length > 0;
      }
      if (tab.isNote) {
        const text = (tab.id === activeTabId && cmEditor) ? cmEditor.getValue().trim() : (tab.sql || '').trim();
        return text.length > 0;
      }
      if (tab.isTableViewer) {
        if (typeof stagedEdits !== 'undefined' && stagedEdits && stagedEdits.size > 0 && activeGridTable === tab.tableName) return true;
        return false;
      }
      return false;
    }

    function showTabContextMenu(e, tabId) {
      if (e) {
        e.preventDefault();
        e.stopPropagation();
      }
      closeAllContextMenus();
      const targetTab = tabs.find(t => t.id === tabId);
      if (!targetTab) return;
      selectedContextMenuTabId = tabId;

      const menu = document.getElementById('tab-context-menu');
      if (!menu) return;

      const idx = tabs.findIndex(t => t.id === tabId);
      const isOnlyTab = tabs.length <= 1;
      const isLastTabRight = idx === tabs.length - 1;

      const btnCloseOthers = document.getElementById('tab-ctx-btn-close-others');
      if (btnCloseOthers) {
        btnCloseOthers.disabled = isOnlyTab;
        btnCloseOthers.classList.toggle('opacity-50', isOnlyTab);
        btnCloseOthers.classList.toggle('cursor-not-allowed', isOnlyTab);
        btnCloseOthers.classList.toggle('cursor-pointer', !isOnlyTab);
      }
      const btnCloseRight = document.getElementById('tab-ctx-btn-close-right');
      if (btnCloseRight) {
        btnCloseRight.disabled = isLastTabRight;
        btnCloseRight.classList.toggle('opacity-50', isLastTabRight);
        btnCloseRight.classList.toggle('cursor-not-allowed', isLastTabRight);
        btnCloseRight.classList.toggle('cursor-pointer', !isLastTabRight);
      }

      const isSingleton = !!(targetTab.isProcesslist || targetTab.isCluster || targetTab.isFlightRecorder || targetTab.isVariables);
      const btnDuplicate = document.getElementById('tab-ctx-btn-duplicate');
      if (btnDuplicate) {
        btnDuplicate.disabled = isSingleton;
        btnDuplicate.classList.toggle('opacity-50', isSingleton);
        btnDuplicate.classList.toggle('cursor-not-allowed', isSingleton);
        btnDuplicate.classList.toggle('cursor-pointer', !isSingleton);
      }

      const btnSaveNote = document.getElementById('tab-ctx-btn-save-note');
      if (btnSaveNote) {
        const canSave = !targetTab.isNote && !targetTab.isProcesslist && !targetTab.isCluster && !targetTab.isFlightRecorder && !targetTab.isVariables;
        btnSaveNote.disabled = !canSave;
        btnSaveNote.classList.toggle('opacity-50', !canSave);
        btnSaveNote.classList.toggle('cursor-not-allowed', !canSave);
        btnSaveNote.classList.toggle('cursor-pointer', canSave);
      }

      const clientX = (e && typeof e.clientX === 'number') ? e.clientX : 0;
      const clientY = (e && typeof e.clientY === 'number') ? e.clientY : 0;
      const x = Math.min(clientX, window.innerWidth - 200);
      const y = Math.min(clientY, window.innerHeight - 250);
      menu.style.left = `${Math.max(10, x)}px`;
      menu.style.top = `${Math.max(10, y)}px`;
      menu.classList.remove('hidden');
    }

    function closeTabContextMenu() {
      const menu = document.getElementById('tab-context-menu');
      if (menu) menu.classList.add('hidden');
      selectedContextMenuTabId = null;
    }

    async function tabContextMenuAction(action) {
      const tabId = selectedContextMenuTabId;
      closeTabContextMenu();
      if (!tabId) return;

      if (action === 'close') {
        await closeTab(tabId);
      } else if (action === 'close-others') {
        await closeOtherTabs(tabId);
      } else if (action === 'close-right') {
        await closeTabsToTheRight(tabId);
      } else if (action === 'close-all') {
        await closeAllTabs();
      } else if (action === 'duplicate') {
        duplicateTab(tabId);
      } else if (action === 'rename') {
        await renameTab(tabId);
      } else if (action === 'save-note') {
        const targetTab = tabs.find(t => t.id === tabId);
        if (targetTab) saveTabQueryToNotes(targetTab);
      }
    }

    function cleanupClosedTabResources(closedTabs) {
      const list = Array.isArray(closedTabs) ? closedTabs : [closedTabs];
      if (list.some(t => t && t.isCluster)) {
        if (typeof clusterTimer !== 'undefined' && clusterTimer) {
          clearInterval(clusterTimer);
          clusterTimer = null;
        }
      }
      if (list.some(t => t && t.isExecuting)) {
        cancelRunningQuery().catch(() => {});
      }
    }

    async function closeTab(tabId) {
      const tab = tabs.find(t => t.id === tabId);
      if (!tab) return;

      if (tab.isExecuting) {
        await cancelRunningQuery();
      }

      if (isCurrentSessionTxActive && !tab.isTableViewer && !tab.isProcesslist && !tab.isCluster && !tab.isFlightRecorder && !tab.isVariables) {
        const ok = await showNativeConfirm('An uncommitted transaction is currently active in this session.\nClosing this tab will ROLLBACK the transaction. Continue?', 'Active Transaction Warning');
        if (!ok) return;
        await rollbackActiveTransaction();
      }

      const isQueryTab = !tab.isTableViewer && !tab.isProcesslist && !tab.isCluster && !tab.isFlightRecorder && !tab.isVariables && !tab.isNote;
      const sql = (tab.id === activeTabId && cmEditor) ? cmEditor.getValue().trim() : (tab.sql || '').trim();
      const isEmptyQuery = isQueryTab && sql.length === 0;

      if (!isEmptyQuery) {
        const title = tab.title || 'this tab';
        const msg = isQueryTab
          ? `Are you sure you want to close "${title}"?\nAny unsaved query or modifications will be discarded.`
          : `Are you sure you want to close "${title}"?`;
        const ok = await showNativeConfirm(msg, 'Close Tab');
        if (!ok) return;
      }

      cleanupClosedTabResources(tab);

      if (tabs.length === 1) {
        tabs = [{ id: 'tab-' + Date.now(), title: 'Query 1.sql', sql: '', isTableViewer: false }];
        activeTabId = tabs[0].id;
        renderTabs();
        setEditorSQL('');
        clearGridToInitialState();
        saveTabsState();
        return;
      }

      const closeIdx = tabs.findIndex(t => t.id === tabId);
      tabs = tabs.filter(t => t.id !== tabId);

      if (activeTabId === tabId) {
        const nextIdx = Math.min(closeIdx, tabs.length - 1);
        switchTab(tabs[nextIdx].id);
      } else {
        renderTabs();
        saveTabsState();
      }
    }

    async function closeOtherTabs(keepTabId) {
      const otherTabs = tabs.filter(t => t.id !== keepTabId);
      if (otherTabs.length === 0) return;

      const hasDirty = otherTabs.some(t => isTabDirtyOrNonEmpty(t));
      if (hasDirty) {
        const ok = await showNativeConfirm(`Close other ${otherTabs.length} tab(s)?\nAny unsaved queries will be discarded.`, 'Close Other Tabs');
        if (!ok) return;
      }

      cleanupClosedTabResources(otherTabs);

      tabs = tabs.filter(t => t.id === keepTabId);
      if (activeTabId !== keepTabId) {
        switchTab(keepTabId);
      } else {
        renderTabs();
        saveTabsState();
      }
    }

    async function closeTabsToTheRight(fromTabId) {
      const fromIdx = tabs.findIndex(t => t.id === fromTabId);
      if (fromIdx === -1 || fromIdx === tabs.length - 1) return;

      const rightTabs = tabs.slice(fromIdx + 1);
      if (rightTabs.length === 0) return;

      const hasDirty = rightTabs.some(t => isTabDirtyOrNonEmpty(t));
      if (hasDirty) {
        const ok = await showNativeConfirm(`Close ${rightTabs.length} tab(s) to the right?\nAny unsaved queries will be discarded.`, 'Close Tabs to the Right');
        if (!ok) return;
      }

      cleanupClosedTabResources(rightTabs);

      tabs = tabs.slice(0, fromIdx + 1);
      if (!tabs.some(t => t.id === activeTabId)) {
        switchTab(fromTabId);
      } else {
        renderTabs();
        saveTabsState();
      }
    }

    async function closeAllTabs() {
      const hasDirty = tabs.some(t => isTabDirtyOrNonEmpty(t));
      if (hasDirty || tabs.length > 1) {
        const ok = await showNativeConfirm(`Close all ${tabs.length} tab(s)?\nWorkspace will be reset to a clean query tab.`, 'Close All Tabs');
        if (!ok) return;
      }

      cleanupClosedTabResources(tabs);

      tabs = [{ id: 'tab-' + Date.now(), title: 'Query 1.sql', sql: '', isTableViewer: false }];
      activeTabId = tabs[0].id;
      renderTabs();
      setEditorSQL('');
      clearGridToInitialState();
      saveTabsState();
    }

    function duplicateTab(tabId) {
      const sourceIdx = tabs.findIndex(t => t.id === tabId);
      if (sourceIdx === -1) return;
      const src = tabs[sourceIdx];

      let currentSql = src.sql || '';
      if (src.id === activeTabId && !src.isTableViewer && !src.isProcesslist && !src.isCluster && !src.isFlightRecorder && !src.isVariables) {
        currentSql = cmEditor ? cmEditor.getValue() : (document.getElementById('sql-editor-textarea')?.value || '');
      }

      let newTitle = src.title || 'Query.sql';
      const isSql = newTitle.endsWith('.sql');
      const base = isSql ? newTitle.slice(0, -4) : newTitle;
      const cleanBase = base.replace(/\s+\(Copy(\s+\d+)?\)$/i, '');
      const existingTitles = new Set(tabs.map(t => (t.title || '').toLowerCase().trim()));

      let candidate = isSql ? `${cleanBase} (Copy).sql` : `${cleanBase} (Copy)`;
      let copyIndex = 2;
      while (existingTitles.has(candidate.toLowerCase())) {
        candidate = isSql ? `${cleanBase} (Copy ${copyIndex}).sql` : `${cleanBase} (Copy ${copyIndex})`;
        copyIndex++;
      }
      newTitle = candidate;

      const newId = 'tab-' + Date.now();
      const clonedTab = {
        id: newId,
        title: newTitle,
        sql: currentSql,
        isTableViewer: !!src.isTableViewer,
        tableName: src.tableName || '',
        database: src.database || '',
        tableMode: src.tableMode || 'data',
        filterMode: src.filterMode || 'pills',
        filterPills: Array.isArray(src.filterPills) ? JSON.parse(JSON.stringify(src.filterPills)) : [],
        filterRaw: src.filterRaw || '',
        appliedFilter: src.appliedFilter || '',
        queryFilters: src.queryFilters ? JSON.parse(JSON.stringify(src.queryFilters)) : {},
        sortCol: src.sortCol || '',
        sortDir: src.sortDir || '',
        offset: 0,
        isNote: !!src.isNote
      };

      tabs.splice(sourceIdx + 1, 0, clonedTab);
      switchTab(newId);
      saveTabsState();
    }

    async function renameTab(tabId) {
      const tab = tabs.find(t => t.id === tabId);
      if (!tab) return;

      const newTitle = await showNativePrompt('Enter new tab name:', tab.title, 'Rename Tab');
      if (newTitle && newTitle.trim()) {
        tab.title = newTitle.trim();
        renderTabs();
        saveTabsState();
      }
    }

