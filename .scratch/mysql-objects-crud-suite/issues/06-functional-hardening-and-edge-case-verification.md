# Issue: Functional Hardening and Edge Case Verification Suite

Status: resolved
Type: task
Blocked by: 03, 04, 05

## Question

How should the entire MySQL/MariaDB Object CRUD & Designer Suite be verified end-to-end across realistic database schemas, complex formatting, whitespace/comment edge cases, and rename lifecycles:
1. Verifying that `stripDefiner` does not destroy vertical indentation or collapse multi-line SQL comments (`-- comment`).
2. Ensuring `extractViewSelectQuery` handles view names with `as`, column alias lists, and CTEs without mangling.
3. Ensuring sidebar schema search filter (`schemaFilterQuery`) correctly matches and displays Stored Procedures, Functions, and Events without premature exit.
4. Ensuring DDL assemblers eliminate double-semicolon (`;;`) syntax glitches and safely drop old object names when renamed during edit.
5. Providing automated runnable verification tests in Go (`web/embed_test.go`) and standalone self-checks (`.scratch/mysql-objects-crud-suite/crud_functional_self_check.js`).

## Answer

1. **Targeted DEFINER Stripping**:
   - Replaced global whitespace collapsing with targeted regex `/\s*(?:\/\*!\d+\s*)?DEFINER\s*=\s*(?:`[^`]+`|'[^']+'|[^\s@]+)@(?:`[^`]+`|'[^']+'|[^\s]+)(?:\s*\*\/)?/gi`.
   - Leaves 100% of body indentation, vertical line breaks, and single-line `-- comment` statements completely intact.
2. **Robust View Extraction**:
   - Prioritized standard view DDL regex `/\bVIEW\s+(?:`[^`]+`|"[^"]+"|[^\s(]+)(?:\s*\([^)]*\))?\s+AS\s+([\s\S]+)$/i` before fallback, cleanly supporting view names containing `as` and explicit column lists.
3. **Comprehensive Schema Search Filtering**:
   - Updated `renderSchemaTree` empty-check to inspect `proceduresList`, `functionsList`, and `eventsList`, allowing users to search across all schema object types without false "No objects matching" screens.
4. **DDL Normalization & Rename Safety**:
   - Added trailing semicolon trimming `replace(/;+$/, '')` across all DDL generators.
   - Wired `originalName` tracking into `assembleTriggerDDL`, `assembleViewDDL`, `assembleRoutineDDL`, and `assembleEventDDL` to cleanly drop prior object names during rename edit cycles.
5. **Automated Verification**:
   - Verified via `node .scratch/mysql-objects-crud-suite/crud_functional_self_check.js` (7 comprehensive test suites passing).
   - Added `TestMySQLObjectsCRUDHardeningAndSafetyGuards` in `web/embed_test.go` and verified via `go test -v ./web` (61 tests passing).
