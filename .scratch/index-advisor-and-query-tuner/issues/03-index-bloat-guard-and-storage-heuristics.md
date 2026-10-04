# Issue: Index Bloat Guard & Storage Overhead Heuristics

Status: resolved
Type: prototype
Blocked by: 01

## Question
How should the advisor detect and calculate when a table has reached an over-indexed state (Index Bloat), and how should the warning be presented so that the developer is clearly informed of the write overhead while still receiving the candidate index recommendation?

Specifically:
1. What heuristic criteria define index bloat? (e.g., secondary index count > 5, or total index storage length exceeding data storage length from `information_schema.tables` / `pg_class`).
2. How should the warning banner be phrased and rendered (amber cautionary banner highlighting current index count, total index size, and potential INSERT/UPDATE latency impact)?
3. How can the bloat guard suggest unused or redundant indexes that could be safely dropped or merged to make room for the new recommendation?

## Answer
1. **Bloat Heuristic**: Implemented in `internal/advisor/advisor.go` (`EvaluateIndexBloat`). A table is flagged as bloated when secondary index count > 5 or index storage size exceeds row data size.
2. **Alert Banner with Unsuppressed Recommendation**: Emits a prominent amber warning banner (`⚠️ INDEX BLOAT WARNING`) highlighting the index count and warning about write degradation on `INSERT`/`UPDATE` operations, while explicitly fulfilling the user requirement to **continue displaying the candidate index recommendation** alongside the warning.
3. **Consolidation Paths**: Surfaces prefix overlap suggestions (`ExistingPrefixIdx` and `ReplaceSuggestion`) so developers can merge narrower existing indexes into the new composite index rather than continuously piling on new indexes.
