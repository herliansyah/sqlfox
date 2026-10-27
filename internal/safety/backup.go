package safety

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"sqlfox/internal/driver"
)

const (
	maxDataBackupBytes = 50 * 1024 * 1024 // 50 MB threshold
	maxDataBackupRows  = 100000
)

// ErrBackupIncomplete marks a backup whose DDL was written but whose data dump failed or was cut short.
var ErrBackupIncomplete = errors.New("backup data incomplete")

var reUnsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

// backupFileName builds a filename that always stays inside backupDir: table names may contain
// path separators or ".." (quoted identifiers), which must never reach filepath.Join.
func backupFileName(timestamp, tableName string) string {
	safe := strings.TrimLeft(reUnsafeFileChars.ReplaceAllString(tableName, "_"), ".")
	if safe == "" {
		safe = "table"
	}
	if len(safe) > 100 {
		safe = safe[:100]
	}
	return fmt.Sprintf("%s-%s.sql", timestamp, safe)
}

// BackupTable performs a smart tiered backup: DDL always, and data if under 50MB / 100k rows.
// It returns the file path even with an error wrapping ErrBackupIncomplete, so callers can still
// point the user at the (DDL-only or partial) snapshot while warning that it is not complete.
// ponytail: Pure stateless function replacing single-method BackupManager wrapper.
func BackupTable(ctx context.Context, sess driver.Session, backupDir, tableName string, driverName ...string) (string, bool, error) {
	if backupDir == "" {
		backupDir = filepath.Join(".scratch", "backups")
	}

	drv := "mysql"
	if len(driverName) > 0 && driverName[0] != "" {
		drv = driverName[0]
	}

	// Fetch the DDL before creating any file: a failure leaves nothing behind.
	ddl, err := sess.GetTableDDL(ctx, tableName)
	if err != nil {
		return "", false, fmt.Errorf("failed to fetch table DDL: %w", err)
	}

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", false, fmt.Errorf("failed to create backup dir: %w", err)
	}

	filePath := filepath.Join(backupDir, backupFileName(time.Now().Format("20060102-150405"), tableName))
	f, err := os.Create(filePath)
	if err != nil {
		return "", false, fmt.Errorf("failed to create backup file: %w", err)
	}

	var writeErr error
	write := func(s string) {
		if writeErr == nil {
			_, writeErr = f.WriteString(s)
		}
	}
	finish := func(dataIncluded bool, dataErr error) (string, bool, error) {
		closeErr := f.Close()
		if writeErr == nil {
			writeErr = closeErr
		}
		if writeErr != nil {
			return filePath, false, fmt.Errorf("%w: write failed: %v", ErrBackupIncomplete, writeErr)
		}
		if dataErr != nil {
			return filePath, false, fmt.Errorf("%w: %v", ErrBackupIncomplete, dataErr)
		}
		return filePath, dataIncluded, nil
	}

	// 1. Write metadata & DDL header
	write(fmt.Sprintf("-- SqlFox Automated Safety Snapshot\n-- Table: %s\n-- Created: %s\n\n%s;\n\n",
		tableName, time.Now().UTC().Format(time.RFC3339), ddl))

	// 2. Check table estimate; large tables get a DDL-only snapshot by design.
	estimate, err := sess.GetTableEstimate(ctx, tableName)
	if err != nil || estimate == nil || estimate.SizeBytes >= maxDataBackupBytes || estimate.RowCount >= maxDataBackupRows {
		return finish(false, nil)
	}

	// Ask for one row past the cap so a stale row estimate cannot silently truncate the snapshot.
	stream, err := sess.QueryStream(ctx, fmt.Sprintf("SELECT * FROM %s", driver.QuoteIdentifier(tableName, drv)), maxDataBackupRows+1)
	if err != nil {
		return finish(false, fmt.Errorf("read rows: %w", err))
	}
	defer stream.Close()
	cols := stream.Columns()
	colNames := make([]string, len(cols))
	for i, c := range cols {
		colNames[i] = driver.QuoteIdentifier(c.Name, drv)
	}
	colsStr := strings.Join(colNames, ", ")

	rows := 0
	for writeErr == nil {
		chunk, err := stream.NextChunk(200)
		for _, row := range chunk {
			if rows >= maxDataBackupRows {
				return finish(false, fmt.Errorf("table has more than %d rows; data snapshot truncated", maxDataBackupRows))
			}
			vals := make([]string, len(row))
			for i, v := range row {
				vals[i] = driver.FormatSQLValueForDriver(v, drv)
			}
			write(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);\n",
				driver.QuoteIdentifier(tableName, drv), colsStr, strings.Join(vals, ", ")))
			rows++
		}
		if err == io.EOF || (err == nil && len(chunk) == 0) {
			break
		}
		if err != nil {
			return finish(false, fmt.Errorf("read rows: %w", err))
		}
	}
	return finish(true, nil)
}
