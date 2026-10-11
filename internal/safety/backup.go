package safety

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sqlfox/internal/driver"
)

const maxDataBackupBytes = 50 * 1024 * 1024 // 50 MB threshold

// BackupTable performs a smart tiered backup: DDL always, and data if under 50MB.
// ponytail: Pure stateless function replacing single-method BackupManager wrapper.
func BackupTable(ctx context.Context, sess driver.Session, backupDir, tableName string, driverName ...string) (string, bool, error) {
	if backupDir == "" {
		backupDir = filepath.Join(".scratch", "backups")
	}

	drv := "mysql"
	if len(driverName) > 0 && driverName[0] != "" {
		drv = driverName[0]
	}

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return "", false, fmt.Errorf("failed to create backup dir: %w", err)
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.sql", timestamp, tableName)
	filePath := filepath.Join(backupDir, filename)

	f, err := os.Create(filePath)
	if err != nil {
		return "", false, fmt.Errorf("failed to create backup file: %w", err)
	}
	defer f.Close()

	// 1. Write metadata & DDL header
	ddl, err := sess.GetTableDDL(ctx, tableName)
	if err != nil {
		return "", false, fmt.Errorf("failed to fetch table DDL: %w", err)
	}

	header := fmt.Sprintf("-- SqlFox Automated Safety Snapshot\n-- Table: %s\n-- Created: %s\n\n%s;\n\n",
		tableName, time.Now().UTC().Format(time.RFC3339), ddl)
	if _, err := f.WriteString(header); err != nil {
		return "", false, err
	}

	// 2. Check table estimate
	estimate, err := sess.GetTableEstimate(ctx, tableName)
	dataIncluded := false
	if err == nil && estimate.SizeBytes < maxDataBackupBytes && estimate.RowCount < 100000 {
		// Dump rows
		stream, err := sess.QueryStream(ctx, fmt.Sprintf("SELECT * FROM %s", driver.QuoteIdentifier(tableName, drv)), 100000)
		if err == nil {
			defer stream.Close()
			cols := stream.Columns()
			colNames := make([]string, len(cols))
			for i, c := range cols {
				colNames[i] = driver.QuoteIdentifier(c.Name, drv)
			}
			colsStr := strings.Join(colNames, ", ")

			for {
				chunk, err := stream.NextChunk(200)
				if len(chunk) > 0 {
					dataIncluded = true
					for _, row := range chunk {
						vals := make([]string, len(row))
						for i, v := range row {
							vals[i] = driver.FormatSQLValue(v)
						}
						insertSQL := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s);\n",
							driver.QuoteIdentifier(tableName, drv), colsStr, strings.Join(vals, ", "))
						f.WriteString(insertSQL)
					}
				}
				if err != nil {
					break
				}
			}
		}
	}

	return filePath, dataIncluded, nil
}
