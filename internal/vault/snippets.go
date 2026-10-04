package vault

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrSnippetNotFound = errors.New("snippet not found")
)

// Snippet represents a reusable parameterized SQL template.
type Snippet struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	SQLContent string    `json:"sql_content"`
	Tags       string    `json:"tags"` // e.g. "mysql", "sqlite", "general"
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ListSnippets returns all saved query snippets ordered by title.
func (v *Vault) ListSnippets() ([]*Snippet, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	rows, err := v.db.Query(`
		SELECT id, title, sql_content, tags, created_at, updated_at
		FROM snippets
		ORDER BY title ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query snippets: %w", err)
	}
	defer rows.Close()

	var result []*Snippet
	for rows.Next() {
		var s Snippet
		var tags sql.NullString
		if err := rows.Scan(&s.ID, &s.Title, &s.SQLContent, &tags, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan snippet: %w", err)
		}
		s.Tags = tags.String
		result = append(result, &s)
	}
	return result, rows.Err()
}

// SaveSnippet inserts or updates a query snippet.
func (v *Vault) SaveSnippet(s *Snippet) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := time.Now()
	if s.ID == "" {
		s.ID = newUUID()
		s.CreatedAt = now
	}
	s.UpdatedAt = now

	query := `
	INSERT INTO snippets (id, title, sql_content, tags, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		title = excluded.title,
		sql_content = excluded.sql_content,
		tags = excluded.tags,
		updated_at = excluded.updated_at
	`
	_, err := v.db.Exec(query, s.ID, s.Title, s.SQLContent, s.Tags, s.CreatedAt, s.UpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save snippet: %w", err)
	}
	return nil
}

// DeleteSnippet removes a snippet by ID.
func (v *Vault) DeleteSnippet(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	res, err := v.db.Exec("DELETE FROM snippets WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete snippet: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrSnippetNotFound
	}
	return nil
}
