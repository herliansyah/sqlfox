package vault

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrOperationalNoteNotFound = errors.New("operational note not found")
)

// OperationalNote represents a unified, Vault-persisted note record
// with raw snapshot content, developer side-note annotations, object tags, and follow-up reminders.
type OperationalNote struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Content    string     `json:"content"`              // Raw snapshot (query, AI answer, DDL, error log)
	SideNote   string     `json:"side_note"`            // Developer annotations & action items
	Status     string     `json:"status"`               // "inbox", "resolved", "archived"
	IsPinned   bool       `json:"is_pinned"`
	CapturedAt time.Time  `json:"captured_at"`          // Timestamp when the incident/snapshot was taken
	ReminderAt *time.Time `json:"reminder_at,omitempty"` // Scheduled follow-up alarm timestamp (nullable)
	ObjectTags []string   `json:"object_tags"`          // JSON array: ["db://conn/db/table", "@note:uuid"]
	UserTags   []string   `json:"user_tags"`            // JSON array: ["#incident", "#slow-query"]
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// OperationalNoteFilter provides query criteria for listing notes in the inbox.
type OperationalNoteFilter struct {
	Status    string
	IsPinned  *bool
	ObjectTag string
	UserTag   string
	Search    string
}

// CreateOperationalNote inserts a new operational note into the vault.
func (v *Vault) CreateOperationalNote(n *OperationalNote) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if n.ID == "" {
		n.ID = newUUID()
	}
	now := time.Now()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	if n.UpdatedAt.IsZero() {
		n.UpdatedAt = now
	}
	if n.CapturedAt.IsZero() {
		n.CapturedAt = now
	}
	if n.Status == "" {
		n.Status = "inbox"
	}
	if n.ObjectTags == nil {
		n.ObjectTags = []string{}
	}
	if n.UserTags == nil {
		n.UserTags = []string{}
	}

	objTagsJSON, err := json.Marshal(n.ObjectTags)
	if err != nil {
		return fmt.Errorf("failed to marshal object tags: %w", err)
	}
	userTagsJSON, err := json.Marshal(n.UserTags)
	if err != nil {
		return fmt.Errorf("failed to marshal user tags: %w", err)
	}

	isPinnedInt := 0
	if n.IsPinned {
		isPinnedInt = 1
	}

	query := `
	INSERT INTO operational_notes (
		id, title, content, side_note, status, is_pinned,
		captured_at, reminder_at, object_tags, user_tags, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		title = excluded.title,
		content = excluded.content,
		side_note = excluded.side_note,
		status = excluded.status,
		is_pinned = excluded.is_pinned,
		captured_at = excluded.captured_at,
		reminder_at = excluded.reminder_at,
		object_tags = excluded.object_tags,
		user_tags = excluded.user_tags,
		updated_at = excluded.updated_at
	`

	_, err = v.db.Exec(query,
		n.ID, n.Title, n.Content, n.SideNote, n.Status, isPinnedInt,
		n.CapturedAt, n.ReminderAt, string(objTagsJSON), string(userTagsJSON), n.CreatedAt, n.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert operational note: %w", err)
	}
	return nil
}

// UpdateOperationalNote updates an existing operational note in the vault.
func (v *Vault) UpdateOperationalNote(n *OperationalNote) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	n.UpdatedAt = time.Now()
	if n.ObjectTags == nil {
		n.ObjectTags = []string{}
	}
	if n.UserTags == nil {
		n.UserTags = []string{}
	}

	objTagsJSON, err := json.Marshal(n.ObjectTags)
	if err != nil {
		return fmt.Errorf("failed to marshal object tags: %w", err)
	}
	userTagsJSON, err := json.Marshal(n.UserTags)
	if err != nil {
		return fmt.Errorf("failed to marshal user tags: %w", err)
	}

	isPinnedInt := 0
	if n.IsPinned {
		isPinnedInt = 1
	}

	query := `
	UPDATE operational_notes SET
		title = ?, content = ?, side_note = ?, status = ?, is_pinned = ?,
		captured_at = ?, reminder_at = ?, object_tags = ?, user_tags = ?, updated_at = ?
	WHERE id = ?
	`

	res, err := v.db.Exec(query,
		n.Title, n.Content, n.SideNote, n.Status, isPinnedInt,
		n.CapturedAt, n.ReminderAt, string(objTagsJSON), string(userTagsJSON), n.UpdatedAt,
		n.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update operational note: %w", err)
	}
	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrOperationalNoteNotFound
	}
	return nil
}

// GetOperationalNote retrieves a single operational note by ID.
func (v *Vault) GetOperationalNote(id string) (*OperationalNote, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	row := v.db.QueryRow(`
		SELECT id, title, content, side_note, status, is_pinned,
		       captured_at, reminder_at, object_tags, user_tags, created_at, updated_at
		FROM operational_notes
		WHERE id = ?
	`, id)

	return scanOperationalNote(row)
}

// ListOperationalNotes retrieves notes filtered by criteria and sorted with pinned notes first, then updated_at descending.
func (v *Vault) ListOperationalNotes(filter OperationalNoteFilter) ([]*OperationalNote, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	var conditions []string
	var args []any

	if filter.Status != "" {
		conditions = append(conditions, "status = ?")
		args = append(args, filter.Status)
	}

	if filter.IsPinned != nil {
		isPinnedInt := 0
		if *filter.IsPinned {
			isPinnedInt = 1
		}
		conditions = append(conditions, "is_pinned = ?")
		args = append(args, isPinnedInt)
	}

	if filter.ObjectTag != "" {
		conditions = append(conditions, "object_tags LIKE ?")
		args = append(args, "%"+filter.ObjectTag+"%")
	}

	if filter.UserTag != "" {
		conditions = append(conditions, "user_tags LIKE ?")
		args = append(args, "%"+filter.UserTag+"%")
	}

	if filter.Search != "" {
		searchPattern := "%" + strings.ToLower(filter.Search) + "%"
		conditions = append(conditions, "(LOWER(title) LIKE ? OR LOWER(content) LIKE ? OR LOWER(side_note) LIKE ?)")
		args = append(args, searchPattern, searchPattern, searchPattern)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT id, title, content, side_note, status, is_pinned,
		       captured_at, reminder_at, object_tags, user_tags, created_at, updated_at
		FROM operational_notes
		%s
		ORDER BY is_pinned DESC, updated_at DESC
	`, whereClause)

	rows, err := v.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query operational notes: %w", err)
	}
	defer rows.Close()

	var result []*OperationalNote
	for rows.Next() {
		n, err := scanOperationalNote(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row error reading operational notes: %w", err)
	}
	return result, nil
}

// ListDueReminders lists all active notes whose reminder_at is on or before asOf and not yet resolved.
func (v *Vault) ListDueReminders(asOf time.Time) ([]*OperationalNote, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	query := `
		SELECT id, title, content, side_note, status, is_pinned,
		       captured_at, reminder_at, object_tags, user_tags, created_at, updated_at
		FROM operational_notes
		WHERE status != 'resolved' AND reminder_at IS NOT NULL AND reminder_at <= ?
		ORDER BY reminder_at ASC
	`

	rows, err := v.db.Query(query, asOf)
	if err != nil {
		return nil, fmt.Errorf("failed to query due reminders: %w", err)
	}
	defer rows.Close()

	var result []*OperationalNote
	for rows.Next() {
		n, err := scanOperationalNote(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, n)
	}
	return result, nil
}

// SnoozeReminder updates the note's reminder_at to time.Now() + duration.
func (v *Vault) SnoozeReminder(id string, duration time.Duration) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	newReminder := time.Now().Add(duration)
	query := `UPDATE operational_notes SET reminder_at = ?, updated_at = ? WHERE id = ?`
	res, err := v.db.Exec(query, newReminder, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to snooze reminder: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrOperationalNoteNotFound
	}
	return nil
}

// ResolveOperationalNote marks an operational note as resolved and clears its reminder.
func (v *Vault) ResolveOperationalNote(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	query := `UPDATE operational_notes SET status = 'resolved', updated_at = ? WHERE id = ?`
	res, err := v.db.Exec(query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to resolve note: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrOperationalNoteNotFound
	}
	return nil
}

// DeleteOperationalNote permanently deletes an operational note by ID.
func (v *Vault) DeleteOperationalNote(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	res, err := v.db.Exec(`DELETE FROM operational_notes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete operational note: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrOperationalNoteNotFound
	}
	return nil
}

// ListAllOperationalNotes returns all operational notes (used for vault archive export).
func (v *Vault) ListAllOperationalNotes() ([]*OperationalNote, error) {
	return v.ListOperationalNotes(OperationalNoteFilter{})
}

// scanner interface abstraction to scan both *sql.Row and *sql.Rows
type noteScanner interface {
	Scan(dest ...any) error
}

func scanOperationalNote(s noteScanner) (*OperationalNote, error) {
	var n OperationalNote
	var isPinnedInt int
	var reminderAt sql.NullTime
	var objTagsRaw, userTagsRaw string

	err := s.Scan(
		&n.ID, &n.Title, &n.Content, &n.SideNote, &n.Status, &isPinnedInt,
		&n.CapturedAt, &reminderAt, &objTagsRaw, &userTagsRaw, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOperationalNoteNotFound
		}
		return nil, fmt.Errorf("failed to scan operational note: %w", err)
	}

	n.IsPinned = (isPinnedInt == 1)
	if reminderAt.Valid {
		n.ReminderAt = &reminderAt.Time
	}

	n.ObjectTags = []string{}
	if objTagsRaw != "" {
		_ = json.Unmarshal([]byte(objTagsRaw), &n.ObjectTags)
	}

	n.UserTags = []string{}
	if userTagsRaw != "" {
		_ = json.Unmarshal([]byte(userTagsRaw), &n.UserTags)
	}

	return &n, nil
}
