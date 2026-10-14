# 0002: Deterministic AST Parser for Safety Guard

To prevent catastrophic data loss with sub-millisecond overhead and zero network dependency, Safety Guard enforces destructive query detection via deterministic SQL AST parsing rather than regex heuristics or LLM inference.

## Considered Options

- **Regex Pattern Matching**: Fast and dependency-free, but vulnerable to false negatives via comments, subqueries, or multiline statements.
- **LLM / AI Inference**: Context-aware, but introduces 500ms–2s network latency, failure on offline connections, and non-deterministic probabilistic risk.
- **Deterministic AST Parser with Lexical Masking & Multi-Statement Gate**: Strictly parses the query syntax tree locally (<1ms) to verify the presence and validity of targeting predicates (e.g. `WHERE` clauses). For queries that bypass standard parser grammars (such as Common Table Expressions), statements are preprocessed (`stripLeadingCTE`) before evaluation. Lexical fallback masks comments and string literals (`maskCommentsAndLiterals`) to prevent keywords hidden in comments or strings from confusing regex checks. Compound multi-statement batches are decomposed and individually evaluated (`inspectAllStatements`), rejecting inputs containing multiple challenge-requiring commands and ensuring trailing destructive commands cannot bypass validation.

## Consequences

Safety Guard provides zero-false-negative structural enforcement for critical destructive statements (`DELETE`, `UPDATE`, `DROP`, `TRUNCATE`, `ALTER`). AI is strictly relegated to non-blocking explanatory roles. Multi-statement batches execute only when all statements conform to the active session safety mode, preventing destructive command smuggling.
