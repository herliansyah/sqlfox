# 0002: Deterministic AST Parser for Safety Guard

To prevent catastrophic data loss with sub-millisecond overhead and zero network dependency, Safety Guard enforces destructive query detection via deterministic SQL AST parsing rather than regex heuristics or LLM inference.

## Considered Options

- **Regex Pattern Matching**: Fast and dependency-free, but vulnerable to false negatives via comments, subqueries, or multiline statements.
- **LLM / AI Inference**: Context-aware, but introduces 500ms–2s network latency, failure on offline connections, and non-deterministic probabilistic risk.
- **Deterministic AST Parser**: Strictly parses the query syntax tree locally (<1ms) to verify the presence and validity of targeting predicates (e.g. `WHERE` clauses).

## Consequences

Safety Guard provides zero-false-negative structural enforcement for critical destructive statements (`DELETE`, `UPDATE`, `DROP`, `TRUNCATE`, `ALTER`). AI is strictly relegated to non-blocking explanatory roles.
