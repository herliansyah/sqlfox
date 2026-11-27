package vault

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrTableNoteNotFound = errors.New("table note not found")
)

// TableNote represents a local Markdown documentation record bound to a specific table.
type TableNote struct {
	ID           string    `json:"id"`
	ConnectionID string    `json:"connection_id"`
	DatabaseName string    `json:"database_name"`
	TableName    string    `json:"table_name"`
	Content      string    `json:"content"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// GetTableNote retrieves documentation notes for a specific table in a connection database.
func (v *Vault) GetTableNote(connectionID, databaseName, tableName string) (*TableNote, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	row := v.db.QueryRow(`
		SELECT id, connection_id, database_name, table_name, content, updated_at
		FROM table_notes
		WHERE connection_id = ? AND database_name = ? AND table_name = ?
	`, connectionID, databaseName, tableName)

	var note TableNote
	if err := row.Scan(&note.ID, &note.ConnectionID, &note.DatabaseName, &note.TableName, &note.Content, &note.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrTableNoteNotFound
		}
		return nil, fmt.Errorf("failed to scan table note: %w", err)
	}
	return &note, nil
}

// SaveTableNote creates or updates a table's documentation notes.
func (v *Vault) SaveTableNote(n *TableNote) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.saveTableNote(v.db, n)
}

func (v *Vault) saveTableNote(exec sqlExecutor, n *TableNote) error {

	if n.ID == "" {
		n.ID = newUUID()
	}
	n.UpdatedAt = time.Now()

	query := `
	INSERT INTO table_notes (id, connection_id, database_name, table_name, content, updated_at)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(connection_id, database_name, table_name) DO UPDATE SET
		content = excluded.content,
		updated_at = excluded.updated_at
	`
	_, err := exec.Exec(query, n.ID, n.ConnectionID, n.DatabaseName, n.TableName, n.Content, n.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save table note: %w", err)
	}
	return nil
}

// ListTableNotesForConnection returns table notes for a connection. Runbook
// operational notes tagged db://<conn>/<db>/<table> are the source of truth for
// notes written from the UI; legacy table_notes rows fill gaps for tables without
// a runbook note (bundle imports).
func (v *Vault) ListTableNotesForConnection(connectionID string) ([]*TableNote, error) {
	opNotes, err := v.ListOperationalNotes(OperationalNoteFilter{ObjectTag: "db://" + connectionID + "/"})
	if err != nil {
		return nil, err
	}

	legacy, err := v.listLegacyTableNotesForConnection(connectionID)
	if err != nil {
		return nil, err
	}

	byTable := make(map[string]*TableNote, len(opNotes)+len(legacy))
	for _, n := range legacy {
		byTable[n.DatabaseName+"\x00"+n.TableName] = n
	}
	for _, on := range opNotes {
		dbName, tableName, ok := parseTableTag(on.ObjectTags, connectionID)
		if !ok {
			continue
		}
		content := on.Content
		if on.SideNote != "" {
			content += "\n\nDeveloper Notes:\n" + on.SideNote
		}
		byTable[dbName+"\x00"+tableName] = &TableNote{
			ID:           on.ID,
			ConnectionID: connectionID,
			DatabaseName: dbName,
			TableName:    tableName,
			Content:      content,
			UpdatedAt:    on.UpdatedAt,
		}
	}

	result := make([]*TableNote, 0, len(byTable))
	for _, n := range byTable {
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].DatabaseName != result[j].DatabaseName {
			return result[i].DatabaseName < result[j].DatabaseName
		}
		return result[i].TableName < result[j].TableName
	})
	return result, nil
}

// parseTableTag extracts the database and table name from a db:// tag for the given connection.
func parseTableTag(tags []string, connectionID string) (databaseName, tableName string, ok bool) {
	prefix := "db://" + connectionID + "/"
	for _, tag := range tags {
		rest, found := strings.CutPrefix(tag, prefix)
		if !found {
			continue
		}
		if dbName, table, found := strings.Cut(rest, "/"); found && table != "" {
			return dbName, table, true
		}
	}
	return "", "", false
}

// listLegacyTableNotesForConnection returns notes from the legacy table_notes table.
func (v *Vault) listLegacyTableNotesForConnection(connectionID string) ([]*TableNote, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	rows, err := v.db.Query(`
		SELECT id, connection_id, database_name, table_name, content, updated_at
		FROM table_notes
		WHERE connection_id = ?
		ORDER BY database_name, table_name
	`, connectionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query table notes: %w", err)
	}
	defer rows.Close()

	var result []*TableNote
	for rows.Next() {
		var n TableNote
		if err := rows.Scan(&n.ID, &n.ConnectionID, &n.DatabaseName, &n.TableName, &n.Content, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan table note: %w", err)
		}
		result = append(result, &n)
	}
	return result, rows.Err()
}

// ListAllTableNotes returns all table notes in the vault across all connections.
func (v *Vault) ListAllTableNotes() ([]*TableNote, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	rows, err := v.db.Query(`
		SELECT id, connection_id, database_name, table_name, content, updated_at
		FROM table_notes
		ORDER BY connection_id, database_name, table_name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query all table notes: %w", err)
	}
	defer rows.Close()

	var result []*TableNote
	for rows.Next() {
		var n TableNote
		if err := rows.Scan(&n.ID, &n.ConnectionID, &n.DatabaseName, &n.TableName, &n.Content, &n.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan table note: %w", err)
		}
		result = append(result, &n)
	}
	return result, rows.Err()
}
