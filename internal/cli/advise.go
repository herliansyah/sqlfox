package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"sqlfox/internal/advisor"
	"sqlfox/internal/driver"
)

// RunAdvise executes the 'sqlfox advise' command.
func RunAdvise(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	fs := flag.NewFlagSet("advise", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var dsn string
	var connName string
	var sqlQuery string
	var filePath string
	var format string
	var vaultPath string
	var masterPass string
	var driverOverride string

	fs.StringVar(&dsn, "dsn", "", "Direct connection DSN (e.g. mysql://user:pass@host:3306/db)")
	fs.StringVar(&connName, "conn", "", "Named connection profile in the vault")
	fs.StringVar(&sqlQuery, "sql", "", "SQL query statement to analyze")
	fs.StringVar(&filePath, "file", "", "Path to .sql file to analyze")
	fs.StringVar(&format, "format", "text", "Output format: text, json")
	fs.StringVar(&vaultPath, "db", "", "Path to config vault database (default: ~/.sqlfox/config.db)")
	fs.StringVar(&masterPass, "pass", "", "Master password for vault encryption")
	fs.StringVar(&driverOverride, "driver", "", "Database driver override (mysql, postgres, sqlite)")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// 1. Resolve SQL Statement
	sqlStmt := strings.TrimSpace(sqlQuery)
	if sqlStmt == "" && filePath != "" {
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(stderr, "Error reading file '%s': %v\n", filePath, err)
			return 1
		}
		sqlStmt = strings.TrimSpace(string(content))
	} else if sqlStmt == "" && stdin != nil {
		// Read piped stdin; skip an interactive terminal (or a closed fd) so the command never blocks/panics.
		if f, ok := stdin.(*os.File); ok {
			if stat, err := f.Stat(); err != nil || stat.Mode()&os.ModeCharDevice != 0 {
				stdin = nil
			}
		}
		if stdin != nil {
			b, _ := io.ReadAll(stdin)
			sqlStmt = strings.TrimSpace(string(b))
		}
	}

	if sqlStmt == "" {
		fmt.Fprintln(stderr, "Error: missing SQL statement. Provide --sql \"...\", --file <path>, or pipe via stdin.")
		return 1
	}

	// 2. Resolve Connection Target (Optional: if no DSN or conn, can still give static AST advice!)
	targetStr := dsn
	if targetStr == "" {
		targetStr = connName
	}

	var sessTarget *SessionTarget
	var tablesMap = make(map[string]driver.TableMetadata)
	var explainPlan *driver.ExplainPlan
	dialect := driverOverride
	if dialect == "" {
		dialect = "mysql" // default dialect
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if targetStr != "" {
		var err error
		sessTarget, err = ResolveTarget(ctx, targetStr, driverOverride, vaultPath, masterPass)
		if err != nil {
			fmt.Fprintf(stderr, "Connection failed: %v\n", err)
			return 1
		}
		defer sessTarget.Cleanup()

		dialect = sessTarget.Driver
		if sInfo, sErr := sessTarget.Session.ServerInfo(ctx); sErr == nil && sInfo != nil && sInfo.Version != "" {
			dialect = fmt.Sprintf("%s %s", sessTarget.Driver, sInfo.Version)
		}
		if sc, sErr := sessTarget.Session.InspectSchema(ctx, sessTarget.DBName); sErr == nil && sc != nil {
			for _, t := range sc.Tables {
				tablesMap[t.Name] = t
			}
		}
		if explainSQL, tErr := driver.ExplainTarget(SplitStatements(strings.NewReader(sqlStmt), sessTarget.Driver)); tErr == nil {
			explainPlan, _ = sessTarget.Session.Explain(ctx, explainSQL)
		} else {
			fmt.Fprintf(stderr, "Skipping live EXPLAIN: %v\n", tErr)
		}
	}

	// 3. Formulate Advice
	result := advisor.AdviseWithExplain(sqlStmt, dialect, tablesMap, explainPlan)

	// 4. Output Results
	if strings.EqualFold(format, "json") {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(result); err != nil {
			fmt.Fprintf(stderr, "JSON encode error: %v\n", err)
			return 1
		}
		return 0
	}

	// Text Output format
	fmt.Fprintf(stdout, "SqlFox Index Advisor & Query Tuner\n")
	fmt.Fprintf(stdout, "Target Dialect: %s\n", strings.ToUpper(dialect))
	fmt.Fprintf(stdout, "Query: %s\n\n", result.Query)

	// Print Bloat Warnings first if any
	if len(result.BloatWarnings) > 0 {
		for _, b := range result.BloatWarnings {
			fmt.Fprintf(stdout, "⚠️  INDEX BLOAT WARNING [%s]:\n", b.Table)
			fmt.Fprintf(stdout, "   %s\n\n", b.Warning)
		}
	}

	if len(result.Recommendations) == 0 {
		fmt.Fprintln(stdout, "✓ No new index recommendations needed. Query predicates are either covered or non-indexable.")
		return 0
	}

	for i, rec := range result.Recommendations {
		fmt.Fprintf(stdout, "Recommendation #%d (Table: %s)\n", i+1, rec.Table)
		fmt.Fprintf(stdout, "  Suggested Index:  %s\n", rec.SuggestedIndex)
		fmt.Fprintf(stdout, "  Columns (ESR):    %s\n", strings.Join(rec.Columns, ", "))
		if len(rec.EqualityColumns) > 0 {
			fmt.Fprintf(stdout, "    • Equality [E]: %s\n", strings.Join(rec.EqualityColumns, ", "))
		}
		if len(rec.SortColumns) > 0 {
			fmt.Fprintf(stdout, "    • Sort     [S]: %s\n", strings.Join(rec.SortColumns, ", "))
		}
		if len(rec.RangeColumns) > 0 {
			fmt.Fprintf(stdout, "    • Range    [R]: %s\n", strings.Join(rec.RangeColumns, ", "))
		}
		fmt.Fprintf(stdout, "  Reason:           %s\n", rec.Reason)
		fmt.Fprintf(stdout, "  Benefit:          %s\n", rec.EstimatedBenefit)

		if rec.IsRedundant {
			fmt.Fprintf(stdout, "  Status:           REDUNDANT (%s)\n", rec.RedundantReason)
		} else if rec.ReplaceSuggestion != "" {
			fmt.Fprintf(stdout, "  Consolidation:    %s\n", rec.ReplaceSuggestion)
		}

		fmt.Fprintf(stdout, "  Safe DDL:\n    %s\n\n", rec.SafeDDL)
	}

	return 0
}
