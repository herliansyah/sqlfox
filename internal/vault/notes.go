package vault

import (
	"database/sql"
	"errors"
	"fmt"
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
	_, err := v.db.Exec(query, n.ID, n.ConnectionID, n.DatabaseName, n.TableName, n.Content, n.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save table note: %w", err)
	}
	return nil
}

// ListTableNotesForConnection returns all notes saved for a specific connection.
func (v *Vault) ListTableNotesForConnection(connectionID string) ([]*TableNote, error) {
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
