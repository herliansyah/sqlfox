package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"sqlfox/internal/driver"
	"sqlfox/internal/safety"
)

// RunQuery executes SQL statements against the target database and writes formatted results to stdout.
func RunQuery(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	fs := flag.NewFlagSet("query", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		dsnFlag        = fs.String("dsn", "", "Direct connection string or SQLite file path")
		connFlag       = fs.String("conn", "", "Vault connection profile name or ID")
		driverFlag     = fs.String("driver", "", "Explicit driver override (mysql, sqlite)")
		fileFlag       = fs.String("file", "", "Path to .sql script file to execute")
		formatFlag     = fs.String("format", "table", "Output format: table, json, ndjson, csv")
		limitFlag      = fs.Int("limit", -1, "Row limit (default 100 for table, 0/unlimited for json/ndjson/csv)")
		forceFlag      = fs.Bool("force", false, "Bypass Safety Guard for destructive operations in automation")
		skipSafetyFlag = fs.Bool("dangerously-skip-safety", false, "Alias for --force")
		txFlag         = fs.Bool("tx", false, "Wrap multi-statement execution in a transaction")
		vaultDbFlag    = fs.String("vault-db", "", "Path to config.db vault file")
		passFlag       = fs.String("pass", "", "Master password for vault encryption")
	)

	if err := fs.Parse(args); err != nil {
		return 1
	}

	force := *forceFlag || *skipSafetyFlag
	format := strings.ToLower(strings.TrimSpace(*formatFlag))
	limit := *limitFlag
	if limit < 0 {
		if format == "table" {
			limit = 100
		} else {
			limit = 0 // unlimited for data streams
		}
	}

	targetParam := *dsnFlag
	if targetParam == "" {
		targetParam = *connFlag
	}
	if targetParam == "" {
		fmt.Fprintln(stderr, "❌ Error: Either --dsn or --conn must be specified.")
		return 1
	}

	// 1. Read SQL queries from file, argument, or stdin
	sqlText, err := resolveSQLInput(fs.Args(), *fileFlag, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "❌ Error reading SQL input: %v\n", err)
		return 1
	}
	if strings.TrimSpace(sqlText) == "" {
		fmt.Fprintln(stderr, "❌ Error: No SQL statement provided (use argument, --file, or pipe via STDIN).")
		return 1
	}

	// 2. Split statements cleanly
	statements := SplitStatements(strings.NewReader(sqlText))
	if len(statements) == 0 {
		fmt.Fprintln(stderr, "❌ Error: No executable SQL statements found in input.")
		return 1
	}

	// 3. Connect to target
	ctx := context.Background()
	target, err := ResolveTarget(ctx, targetParam, *driverFlag, *vaultDbFlag, *passFlag)
	if err != nil {
		fmt.Fprintf(stderr, "❌ Connection Error: %v\n", err)
		return 1
	}
	defer target.Cleanup()

	// 4. Pre-execution Safety Guard inspection (Fail-closed)
	for idx, stmt := range statements {
		insp := safety.Inspect(stmt, target.Environment)
		if insp.IsDestructive || insp.RequiresChallenge || insp.RiskLevel == safety.RiskDangerous || insp.RiskLevel == safety.RiskCritical {
			if !force {
				fmt.Fprintf(stderr, "❌ Safety Guard Block (Statement %d/%d): %s\n", idx+1, len(statements), insp.Reason)
				if insp.TargetTable != "" {
					fmt.Fprintf(stderr, "   Target Table: %s | Operation: %s\n", insp.TargetTable, insp.Operation)
				}
				fmt.Fprintln(stderr, "   Execution aborted in non-interactive headless mode.")
				fmt.Fprintln(stderr, "   Hint: Supply --force or --dangerously-skip-safety to execute intentionally.")
				return 2
			}
		}
	}

	// 5. Transaction wrapper if requested
	if *txFlag && len(statements) > 1 {
		beginSQL := "BEGIN TRANSACTION"
		if target.Driver == "mysql" {
			beginSQL = "START TRANSACTION"
		}
		if _, err := target.Session.Exec(ctx, beginSQL); err != nil {
			fmt.Fprintf(stderr, "❌ Failed to begin transaction: %v\n", err)
			return 1
		}
	}

	// 6. Execute statements
	for idx, stmt := range statements {
		isQuery := isRowReturningQuery(stmt)
		start := time.Now()

		if isQuery && len(statements) == 1 {
			// Stream query results directly
			if err := executeAndFormatQuery(ctx, target.Session, stmt, limit, format, stdout, stderr); err != nil {
				if *txFlag && len(statements) > 1 {
					_, _ = target.Session.Exec(ctx, "ROLLBACK")
				}
				fmt.Fprintf(stderr, "❌ Query Error: %v\n", err)
				return 1
			}
		} else {
			res, err := target.Session.Exec(ctx, stmt)
			elapsed := time.Since(start).Truncate(time.Millisecond)
			if err != nil {
				if *txFlag && len(statements) > 1 {
					_, _ = target.Session.Exec(ctx, "ROLLBACK")
				}
				fmt.Fprintf(stderr, "❌ SQL Error (statement %d/%d): %v\n", idx+1, len(statements), err)
				return 1
			}
			if len(statements) > 1 || format == "table" {
				fmt.Fprintf(stderr, "Statement %d/%d OK: %d rows affected (%s)\n", idx+1, len(statements), res.RowsAffected, elapsed)
			}
		}
	}

	if *txFlag && len(statements) > 1 {
		if _, err := target.Session.Exec(ctx, "COMMIT"); err != nil {
			fmt.Fprintf(stderr, "❌ Failed to commit transaction: %v\n", err)
			return 1
		}
	}

	return 0
}

func resolveSQLInput(positional []string, file string, stdin io.Reader) (string, error) {
	if len(positional) > 0 {
		return strings.Join(positional, " "), nil
	}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if stdin != nil {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	return "", nil
}

func isRowReturningQuery(sql string) bool {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	return strings.HasPrefix(upper, "SELECT") ||
		strings.HasPrefix(upper, "SHOW") ||
		strings.HasPrefix(upper, "EXPLAIN") ||
		strings.HasPrefix(upper, "PRAGMA") ||
		strings.HasPrefix(upper, "DESC") ||
		strings.HasPrefix(upper, "WITH")
}

func executeAndFormatQuery(ctx context.Context, sess driver.Session, sql string, limit int, format string, stdout, stderr io.Writer) error {
	stream, err := sess.QueryStream(ctx, sql, limit)
	if err != nil {
		return err
	}
	defer stream.Close()

	colDefs := stream.Columns()
	columns := make([]string, len(colDefs))
	for i, c := range colDefs {
		columns[i] = c.Name
	}

	var allRows [][]any
	batchSize := 100
	if limit > 0 && limit < batchSize {
		batchSize = limit
	}

	for {
		chunk, err := stream.NextChunk(batchSize)
		if len(chunk) > 0 {
			if format == "ndjson" {
				// Stream directly row-by-row for zero memory accumulation
				for _, row := range chunk {
					rowMap := make(map[string]any, len(columns))
					for i, col := range columns {
						if i < len(row) {
							rowMap[col] = formatValue(row[i])
						}
					}
					if err := json.NewEncoder(stdout).Encode(rowMap); err != nil {
						return err
					}
				}
			} else {
				allRows = append(allRows, chunk...)
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	switch format {
	case "table":
		renderTable(stdout, columns, allRows)
		if limit > 0 && len(allRows) >= limit {
			fmt.Fprintf(stderr, "(Showing first %d rows. Use --limit 0 for all rows)\n", limit)
		}
	case "json":
		jsonList := make([]map[string]any, 0, len(allRows))
		for _, row := range allRows {
			rowMap := make(map[string]any, len(columns))
			for i, col := range columns {
				if i < len(row) {
					rowMap[col] = formatValue(row[i])
				}
			}
			jsonList = append(jsonList, rowMap)
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(jsonList)
	case "csv":
		w := csv.NewWriter(stdout)
		if err := w.Write(columns); err != nil {
			return err
		}
		for _, row := range allRows {
			record := make([]string, len(columns))
			for i := range columns {
				if i < len(row) {
					record[i] = fmt.Sprint(formatValue(row[i]))
				}
			}
			if err := w.Write(record); err != nil {
				return err
			}
		}
		w.Flush()
		return w.Error()
	case "ndjson":
		// already streamed
		return nil
	default:
		return fmt.Errorf("unsupported output format: %s (choose table, json, ndjson, csv)", format)
	}

	return nil
}

func formatValue(val any) any {
	if val == nil {
		return nil
	}
	switch v := val.(type) {
	case []byte:
		return string(v)
	default:
		return v
	}
}

func renderTable(w io.Writer, columns []string, rows [][]any) {
	tw := tabwriter.NewWriter(w, 2, 4, 2, ' ', 0)
	// Header
	fmt.Fprintln(tw, strings.Join(columns, "\t"))

	// Separator
	dividers := make([]string, len(columns))
	for i, c := range columns {
		divLen := len(c)
		if divLen < 4 {
			divLen = 4
		}
		dividers[i] = strings.Repeat("-", divLen)
	}
	fmt.Fprintln(tw, strings.Join(dividers, "\t"))

	// Data
	for _, row := range rows {
		fields := make([]string, len(columns))
		for i := range columns {
			if i < len(row) {
				if row[i] == nil {
					fields[i] = "NULL"
				} else {
					fields[i] = fmt.Sprint(formatValue(row[i]))
				}
			} else {
				fields[i] = "NULL"
			}
		}
		fmt.Fprintln(tw, strings.Join(fields, "\t"))
	}
	tw.Flush()
}
