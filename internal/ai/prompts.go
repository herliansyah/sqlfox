package ai

import (
	"fmt"
)

// BaseSystemPrompt returns the safety and role context for the SQL Copilot.
func BaseSystemPrompt(dialect string, schemaContext string) string {
	return fmt.Sprintf(`You are SqlFox Copilot, an elite database performance and SQL engineering assistant.
Target Dialect: %s.

Rules:
1. Provide concise, direct answers without unnecessary fluff.
2. When outputting SQL, wrap it in a single markdown `+"```sql ... ```"+` block.
3. Optimize for database performance, correct indexing, and avoiding full-table scans.
4. Strictly respect the schema provided below. Never hallucinate tables or columns not present in the schema.

%s
`, dialect, schemaContext)
}

func BuildExplainQueryPrompt(query string) string {
	return fmt.Sprintf("Explain the following SQL query in clear, human-understandable terms. Highlight tables accessed, filter logic, and potential performance bottlenecks:\n\n```sql\n%s\n```", query)
}

func BuildExplainPlanPrompt(query string, planText string) string {
	return fmt.Sprintf("Analyze this query execution plan (EXPLAIN). Explain what the database engine is doing, identify any full table scans or temporary filesort steps, and suggest specific indexes to improve execution speed:\n\nQuery:\n```sql\n%s\n```\n\nExecution Plan:\n%s", query, planText)
}

func BuildOptimizeQueryPrompt(query string, planText string) string {
	return fmt.Sprintf("Optimize the following query for performance. Provide: (1) An improved SQL query rewrite if applicable, and (2) Specific CREATE INDEX statements that will accelerate this query.\n\nQuery:\n```sql\n%s\n```\n\nPlan:\n%s", query, planText)
}

func BuildNLToSQLPrompt(instruction string) string {
	return fmt.Sprintf("Generate a syntactically correct, highly optimal SQL query for the following natural language request:\n\n\"%s\"\n\nReturn ONLY the SQL block with a 1-sentence explanation.", instruction)
}

func BuildDebugErrorPrompt(query string, errorMsg string) string {
	return fmt.Sprintf("The following SQL query produced a database error. Explain the root cause of the error and provide the corrected SQL query.\n\nQuery:\n```sql\n%s\n```\n\nDatabase Error:\n%s", query, errorMsg)
}
