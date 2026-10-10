package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"sqlfox/internal/driver"
)

// RunTransfer executes cross-connection streaming data transfer between databases.
func RunTransfer(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	fs := flag.NewFlagSet("transfer", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		fromFlag            = fs.String("from", "", "Source connection DSN or Vault connection name")
		toFlag              = fs.String("to", "", "Target connection DSN or Vault connection name")
		fromDriverFlag      = fs.String("driver-from", "", "Explicit driver override for source")
		toDriverFlag        = fs.String("driver-to", "", "Explicit driver override for target")
		tablesFlag          = fs.String("tables", "", "Comma-separated list of tables to transfer")
		allTablesFlag       = fs.Bool("all-tables", false, "Transfer all tables found in source database")
		createMissingFlag   = fs.Bool("create-missing", false, "Create target table schema if it does not exist")
		truncateFlag        = fs.Bool("truncate", false, "Truncate target table before inserting data")
		batchSizeFlag       = fs.Int("batch-size", 100, "Number of rows per batch insert")
		continueOnErrorFlag = fs.Bool("continue-on-error", false, "Continue with next table if an error occurs")
		vaultDbFlag         = fs.String("vault-db", "", "Path to config.db vault file")
		passFlag            = fs.String("pass", "", "Master password for vault encryption")
	)

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if *fromFlag == "" || *toFlag == "" {
		fmt.Fprintln(stderr, "❌ Error: Both --from and --to endpoints must be specified.")
		return 1
	}

	if *tablesFlag == "" && !*allTablesFlag {
		fmt.Fprintln(stderr, "❌ Error: Specify either --tables \"tbl1,tbl2\" or --all-tables.")
		return 1
	}

	ctx := context.Background()

	// 1. Resolve source and target
	fromTarget, err := ResolveTarget(ctx, *fromFlag, *fromDriverFlag, *vaultDbFlag, *passFlag)
	if err != nil {
		fmt.Fprintf(stderr, "❌ Source Connection Error: %v\n", err)
		return 1
	}
	defer fromTarget.Cleanup()

	toTarget, err := ResolveTarget(ctx, *toFlag, *toDriverFlag, *vaultDbFlag, *passFlag)
	if err != nil {
		fmt.Fprintf(stderr, "❌ Target Connection Error: %v\n", err)
		return 1
	}
	defer toTarget.Cleanup()

	// 2. Determine tables list
	var tableList []string
	if *allTablesFlag {
		schema, err := fromTarget.Session.InspectSchema(ctx, fromTarget.DBName)
		if err != nil {
			fmt.Fprintf(stderr, "❌ Failed to inspect source schema: %v\n", err)
			return 1
		}
		for _, t := range schema.Tables {
			tableList = append(tableList, t.Name)
		}
	} else {
		for _, part := range strings.Split(*tablesFlag, ",") {
			tbl := strings.TrimSpace(part)
			if tbl != "" {
				tableList = append(tableList, tbl)
			}
		}
	}

	if len(tableList) == 0 {
		fmt.Fprintln(stderr, "❌ Error: No tables selected for transfer.")
		return 1
	}

	fmt.Fprintf(stderr, "🚚 Starting transfer: %d table(s) from [%s] to [%s]\n", len(tableList), fromTarget.Driver, toTarget.Driver)

	hasError := false

	// 3. Process tables sequentially with per-table atomicity
	for idx, tbl := range tableList {
		tblStart := time.Now()
		fmt.Fprintf(stderr, "[%d/%d] Transferring table '%s'...\n", idx+1, len(tableList), tbl)

		// A. Check target schema / create missing
		if *createMissingFlag {
			toSchema, err := toTarget.Session.InspectSchema(ctx, toTarget.DBName)
			tableExists := false
			if err == nil && toSchema != nil {
				for _, t := range toSchema.Tables {
					if strings.EqualFold(t.Name, tbl) {
						tableExists = true
						break
					}
				}
			}

			if !tableExists {
				ddl, err := fromTarget.Session.GetTableDDL(ctx, tbl)
				if err != nil {
					fmt.Fprintf(stderr, "  ❌ Failed to get source DDL for '%s': %v\n", tbl, err)
					hasError = true
					if !*continueOnErrorFlag {
						return 1
					}
					continue
				}
				if _, err := toTarget.Session.Exec(ctx, ddl); err != nil {
					fmt.Fprintf(stderr, "  ❌ Failed to create table '%s' on target: %v\n", tbl, err)
					hasError = true
					if !*continueOnErrorFlag {
						return 1
					}
					continue
				}
				fmt.Fprintf(stderr, "  ✨ Created missing target table '%s'\n", tbl)
			}
		}

		// B. Truncate if requested
		if *truncateFlag {
			truncSQL := fmt.Sprintf("DELETE FROM %s", driver.QuoteIdentifier(tbl, toTarget.Driver))
			if _, err := toTarget.Session.Exec(ctx, truncSQL); err != nil {
				fmt.Fprintf(stderr, "  ⚠️ Truncate failed on '%s': %v\n", tbl, err)
			} else {
				fmt.Fprintf(stderr, "  🧹 Truncated target table '%s'\n", tbl)
			}
		}

		// C. Stream data from source
		selectSQL := fmt.Sprintf("SELECT * FROM %s", driver.QuoteIdentifier(tbl, fromTarget.Driver))
		stream, err := fromTarget.Session.QueryStream(ctx, selectSQL, 0)
		if err != nil {
			fmt.Fprintf(stderr, "  ❌ Failed to read from source table '%s': %v\n", tbl, err)
			hasError = true
			if !*continueOnErrorFlag {
				return 1
			}
			continue
		}

		// D. Target transaction for per-table atomicity pinned to dedicated connection
		tx, err := toTarget.Session.BeginTx(ctx)
		if err != nil {
			stream.Close()
			fmt.Fprintf(stderr, "  ❌ Failed to start target transaction for '%s': %v\n", tbl, err)
			hasError = true
			if !*continueOnErrorFlag {
				return 1
			}
			continue
		}

		colDefs := stream.Columns()
		columns := make([]string, len(colDefs))
		for i, c := range colDefs {
			columns[i] = c.Name
		}

		totalRows := 0
		var transferErr error

		for {
			chunk, err := stream.NextChunk(*batchSizeFlag)
			if len(chunk) > 0 {
				if err := flushInsertBatch(ctx, tx, tbl, columns, chunk, toTarget.Driver); err != nil {
					transferErr = err
					break
				}
				totalRows += len(chunk)
				fmt.Fprintf(stderr, "  -> %s: %d rows written...\r", tbl, totalRows)
			}
			if err != nil {
				if err == io.EOF {
					break
				}
				transferErr = err
				break
			}
		}

		stream.Close()

		if transferErr != nil {
			_ = tx.Rollback()
			fmt.Fprintf(stderr, "\n  ❌ Transfer failed for '%s' (rolled back): %v\n", tbl, transferErr)
			hasError = true
			if !*continueOnErrorFlag {
				return 1
			}
			continue
		}

		if err := tx.Commit(); err != nil {
			fmt.Fprintf(stderr, "\n  ❌ Failed to commit target transaction for '%s': %v\n", tbl, err)
			hasError = true
			if !*continueOnErrorFlag {
				return 1
			}
			continue
		}

		elapsed := time.Since(tblStart).Truncate(time.Millisecond)
		fmt.Fprintf(stderr, "  ✅ Transferred %d rows into '%s' in %s\n", totalRows, tbl, elapsed)
	}

	if hasError {
		fmt.Fprintln(stderr, "⚠️ Transfer completed with errors.")
		return 1
	}

	fmt.Fprintln(stderr, "🎉 Cross-connection transfer finished successfully.")
	return 0
}

type queryExecutor interface {
	Exec(ctx context.Context, sql string, args ...any) (*driver.ExecResult, error)
}

func flushInsertBatch(ctx context.Context, sess queryExecutor, table string, columns []string, batch [][]any, drv string) error {
	if len(batch) == 0 || len(columns) == 0 {
		return nil
	}

	var sb strings.Builder
	sb.WriteString("INSERT INTO ")
	sb.WriteString(driver.QuoteIdentifier(table, drv))
	sb.WriteString(" (")
	for i, c := range columns {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(driver.QuoteIdentifier(c, drv))
	}
	sb.WriteString(") VALUES ")

	for rIdx, row := range batch {
		if rIdx > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("(")
		for cIdx := range columns {
			if cIdx > 0 {
				sb.WriteString(", ")
			}
			if cIdx < len(row) {
				sb.WriteString(driver.FormatSQLValue(row[cIdx]))
			} else {
				sb.WriteString("NULL")
			}
		}
		sb.WriteString(")")
	}

	_, err := sess.Exec(ctx, sb.String())
	return err
}

