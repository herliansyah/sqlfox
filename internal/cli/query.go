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

	"github.com/herliansyah/sqlfox/internal/driver"
	"github.com/herliansyah/sqlfox/internal/safety"
)

// RunQuery executes SQL statements against the target database and writes formatted results to stdout.
func RunQuery(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	return RunQueryContext(context.Background(), args, stdout, stderr, stdin)
}

// RunQueryContext executes SQL statements using the caller's context (for timeouts & signals).
func RunQueryContext(ctx context.Context, args []string, stdout, stderr io.Writer, stdin io.Reader) int {
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

	// 2. Connect to target (--conn strictly resolves from vault so --driver never hijacks a profile name)
	if ctx == nil {
		ctx = context.Background()
	}
	var target *SessionTarget
	if *dsnFlag != "" {
		target, err = ResolveTarget(ctx, *dsnFlag, *driverFlag, *vaultDbFlag, *passFlag)
	} else {
		target, err = ResolveVaultTarget(ctx, *connFlag, *driverFlag, *vaultDbFlag, *passFlag)
	}
	if err != nil {
		fmt.Fprintf(stderr, "❌ Connection Error: %v\n", err)
		return 1
	}
	defer target.Cleanup()

	// 3. Split statements cleanly with dialect awareness
	statements := SplitStatements(strings.NewReader(sqlText), target.Driver)
	if len(statements) == 0 {
		fmt.Fprintln(stderr, "❌ Error: No executable SQL statements found in input.")
		return 1
	}

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
	var activeTx driver.TxSession
	if *txFlag && len(statements) > 1 {
		tx, err := target.Session.BeginTx(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "❌ Failed to begin transaction: %v\n", err)
			return 1
		}
		activeTx = tx
		defer func() {
			if activeTx != nil {
				_ = activeTx.Rollback()
			}
		}()
	}

	// 6. Execute statements
	for idx, stmt := range statements {
		isQuery := isRowReturningQuery(stmt)
		start := time.Now()

		if isQuery {
			// Stream query results directly (works for both single and multi-statement queries, inside or outside tx)
			var qErr error
			if activeTx != nil {
				qErr = executeAndFormatQueryStream(ctx, activeTx, stmt, limit, format, stdout, stderr)
			} else {
				qErr = executeAndFormatQuery(ctx, target.Session, stmt, limit, format, stdout, stderr)
			}
			if qErr != nil {
				if activeTx != nil {
					_ = activeTx.Rollback()
					activeTx = nil
				}
				fmt.Fprintf(stderr, "❌ Query Error: %v\n", qErr)
				return 1
			}
		} else {
			var res *driver.ExecResult
			var err error
			if activeTx != nil {
				res, err = activeTx.Exec(ctx, stmt)
			} else {
				res, err = target.Session.Exec(ctx, stmt)
			}
			elapsed := time.Since(start).Truncate(time.Millisecond)
			if err != nil {
				if activeTx != nil {
					_ = activeTx.Rollback()
					activeTx = nil
				}
				fmt.Fprintf(stderr, "❌ SQL Error (statement %d/%d): %v\n", idx+1, len(statements), err)
				return 1
			}
			if len(statements) > 1 || format == "table" {
				fmt.Fprintf(stderr, "Statement %d/%d OK: %d rows affected (%s)\n", idx+1, len(statements), res.RowsAffected, elapsed)
			}
		}
	}

	if activeTx != nil {
		if err := activeTx.Commit(); err != nil {
			fmt.Fprintf(stderr, "❌ Failed to commit transaction: %v\n", err)
			return 1
		}
		activeTx = nil
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
	return driver.IsQueryStatement(sql)
}

type queryStreamer interface {
	QueryStream(ctx context.Context, sql string, limit int) (driver.RowStream, error)
}

func executeAndFormatQuery(ctx context.Context, sess driver.Session, sql string, limit int, format string, stdout, stderr io.Writer) error {
	return executeAndFormatQueryStream(ctx, sess, sql, limit, format, stdout, stderr)
}

func executeAndFormatQueryStream(ctx context.Context, sess queryStreamer, sql string, limit int, format string, stdout, stderr io.Writer) error {
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

	// Stream csv, ndjson, and json directly to stdout so large exports never OOM.
	var csvWriter *csv.Writer
	jsonFirst := true
	if format == "csv" {
		csvWriter = csv.NewWriter(stdout)
		if err := csvWriter.Write(columns); err != nil {
			return err
		}
	} else if format == "json" {
		if _, err := io.WriteString(stdout, "[\n"); err != nil {
			return err
		}
	}

	for {
		chunk, err := stream.NextChunk(batchSize)
		if len(chunk) > 0 {
			switch format {
			case "ndjson":
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
			case "csv":
				for _, row := range chunk {
					record := make([]string, len(columns))
					for i := range columns {
						if i < len(row) && row[i] != nil {
							record[i] = fmt.Sprint(formatValue(row[i]))
						}
					}
					if err := csvWriter.Write(record); err != nil {
						return err
					}
				}
				csvWriter.Flush()
				if err := csvWriter.Error(); err != nil {
					return err
				}
			case "json":
				for _, row := range chunk {
					rowMap := make(map[string]any, len(columns))
					for i, col := range columns {
						if i < len(row) {
							rowMap[col] = formatValue(row[i])
						}
					}
					if !jsonFirst {
						if _, err := io.WriteString(stdout, ",\n"); err != nil {
							return err
						}
					}
					jsonFirst = false
					b, err := json.MarshalIndent(rowMap, "  ", "  ")
					if err != nil {
						return err
					}
					if _, err := io.WriteString(stdout, "  "); err != nil {
						return err
					}
					if _, err := stdout.Write(b); err != nil {
						return err
					}
				}
			case "table":
				allRows = append(allRows, chunk...)
			default:
				return fmt.Errorf("unsupported output format: %s (choose table, json, ndjson, csv)", format)
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
		if !jsonFirst {
			if _, err := io.WriteString(stdout, "\n"); err != nil {
				return err
			}
		}
		if _, err := io.WriteString(stdout, "]\n"); err != nil {
			return err
		}
	case "csv":
		csvWriter.Flush()
		return csvWriter.Error()
	case "ndjson":
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
	return driver.FormatValueForJSON(val)
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
