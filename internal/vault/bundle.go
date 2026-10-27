package vault

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"
)

// ExportNotesFiles gathers human-readable Markdown notes, table documentation,
// and SQL snippets into a map of relative file paths to file contents.
func ExportNotesFiles(v *Vault) (map[string][]byte, error) {
	files := make(map[string][]byte)

	// 1. Gather Operational Notes
	opNotes, err := v.ListAllOperationalNotes()
	if err != nil {
		return nil, fmt.Errorf("failed to list operational notes: %w", err)
	}
	for _, n := range opNotes {
		slug := slugify(n.Title)
		filename := fmt.Sprintf("notes/%s-%s.md", n.ID, slug)
		files[filename] = formatOperationalNoteMarkdown(n)
	}

	// 2. Gather Table Notes
	tableNotes, err := v.ListAllTableNotes()
	if err != nil {
		return nil, fmt.Errorf("failed to list table notes: %w", err)
	}
	for _, tn := range tableNotes {
		filename := fmt.Sprintf("table-notes/%s/%s/%s.md", encodePathSegment(tn.ConnectionID), encodePathSegment(tn.DatabaseName), encodePathSegment(tn.TableName))
		files[filename] = []byte(tn.Content)
	}

	// 3. Gather Snippets
	snippets, err := v.ListSnippets()
	if err != nil {
		return nil, fmt.Errorf("failed to list snippets: %w", err)
	}
	for _, s := range snippets {
		slug := slugify(s.Title)
		filename := fmt.Sprintf("snippets/%s-%s.sql", s.ID, slug)
		files[filename] = formatSnippetSQL(s)
	}

	return files, nil
}

// ExportNotesBundle builds an unencrypted ZIP archive containing all Markdown notes,
// table documentation, and SQL snippets for offline reading and cross-tool portability.
func ExportNotesBundle(v *Vault) ([]byte, error) {
	files, err := ExportNotesFiles(v)
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	zw := zip.NewWriter(buf)

	for filename, content := range files {
		f, err := zw.Create(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to create zip entry %s: %w", filename, err)
		}
		if _, err := f.Write(content); err != nil {
			return nil, fmt.Errorf("failed to write zip entry %s: %w", filename, err)
		}
	}

	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize zip archive: %w", err)
	}

	return buf.Bytes(), nil
}

// ImportNotesBundle extracts an unencrypted ZIP archive and restores all notes,
// table notes, and snippets into the vault. Returns the total count of imported artifacts.
func ImportNotesBundle(v *Vault, zipData []byte) (int, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return 0, fmt.Errorf("invalid zip archive: %w", err)
	}

	importedCount := 0

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}

		cleanPath := path.Clean(f.Name)
		rc, err := f.Open()
		if err != nil {
			return importedCount, fmt.Errorf("failed to read %s: %w", cleanPath, err)
		}
		contentBytes, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return importedCount, fmt.Errorf("failed to read content of %s: %w", cleanPath, err)
		}

		parts := strings.Split(cleanPath, "/")

		// Case 1: Operational Note: "notes/<id>-<slug>.md"
		if len(parts) >= 2 && parts[0] == "notes" && strings.HasSuffix(cleanPath, ".md") {
			note := parseOperationalNoteMarkdown(string(contentBytes), parts[len(parts)-1])
			if err := v.CreateOperationalNote(note); err == nil {
				importedCount++
			}
			continue
		}

		// Case 2: Table Note: "table-notes/<conn>/<db>/<table>.md"
		if len(parts) == 4 && parts[0] == "table-notes" && strings.HasSuffix(cleanPath, ".md") {
			tn := &TableNote{
				ConnectionID: decodePathSegment(parts[1]),
				DatabaseName: decodePathSegment(parts[2]),
				TableName:    decodePathSegment(strings.TrimSuffix(parts[3], ".md")),
				Content:      string(contentBytes),
			}
			if err := v.SaveTableNote(tn); err == nil {
				importedCount++
			}
			continue
		}

		// Case 3: Snippet: "snippets/<id>-<slug>.sql"
		if len(parts) >= 2 && parts[0] == "snippets" && strings.HasSuffix(cleanPath, ".sql") {
			snippet := parseSnippetSQL(string(contentBytes), parts[len(parts)-1])
			if err := v.SaveSnippet(snippet); err == nil {
				importedCount++
			}
			continue
		}
	}

	return importedCount, nil
}

func formatOperationalNoteMarkdown(n *OperationalNote) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("id: %s\n", n.ID))
	b.WriteString(fmt.Sprintf("title: %s\n", sanitizeYAMLString(n.Title)))
	b.WriteString(fmt.Sprintf("status: %s\n", n.Status))
	b.WriteString(fmt.Sprintf("is_pinned: %t\n", n.IsPinned))
	b.WriteString(fmt.Sprintf("captured_at: %s\n", n.CapturedAt.Format(time.RFC3339)))
	if n.ReminderAt != nil {
		b.WriteString(fmt.Sprintf("reminder_at: %s\n", n.ReminderAt.Format(time.RFC3339)))
	}
	if len(n.ObjectTags) > 0 {
		b.WriteString(fmt.Sprintf("object_tags: %s\n", strings.Join(n.ObjectTags, ", ")))
	}
	if len(n.UserTags) > 0 {
		b.WriteString(fmt.Sprintf("user_tags: %s\n", strings.Join(n.UserTags, ", ")))
	}
	if n.SideNote != "" {
		b.WriteString(fmt.Sprintf("side_note: %s\n", sanitizeYAMLString(n.SideNote)))
	}
	b.WriteString("---\n\n")
	b.WriteString(n.Content)
	return []byte(b.String())
}

func parseOperationalNoteMarkdown(raw string, filename string) *OperationalNote {
	note := &OperationalNote{
		Title:      strings.TrimSuffix(filename, ".md"),
		Status:     "inbox",
		CapturedAt: time.Now(),
		ObjectTags: []string{},
		UserTags:   []string{},
	}

	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if strings.HasPrefix(raw, "---\n") {
		idx := strings.Index(raw[4:], "\n---\n")
		if idx != -1 {
			frontmatter := raw[4 : 4+idx]
			note.Content = strings.TrimPrefix(raw[4+idx+5:], "\n")
			lines := strings.Split(frontmatter, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				parts := strings.SplitN(line, ":", 2)
				if len(parts) != 2 {
					continue
				}
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				v = strings.Trim(v, `"'`)

				switch k {
				case "id":
					note.ID = v
				case "title":
					note.Title = v
				case "status":
					note.Status = v
				case "is_pinned":
					note.IsPinned = (v == "true" || v == "1")
				case "side_note":
					note.SideNote = v
				case "captured_at":
					if t, err := time.Parse(time.RFC3339, v); err == nil {
						note.CapturedAt = t
					}
				case "reminder_at":
					if t, err := time.Parse(time.RFC3339, v); err == nil {
						note.ReminderAt = &t
					}
				case "object_tags":
					tags := strings.Split(v, ",")
					for _, tag := range tags {
						if t := strings.TrimSpace(tag); t != "" {
							note.ObjectTags = append(note.ObjectTags, t)
						}
					}
				case "user_tags":
					tags := strings.Split(v, ",")
					for _, tag := range tags {
						if t := strings.TrimSpace(tag); t != "" {
							note.UserTags = append(note.UserTags, t)
						}
					}
				}
			}
			return note
		}
	}

	note.Content = raw
	return note
}

func formatSnippetSQL(s *Snippet) []byte {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("-- id: %s\n", s.ID))
	b.WriteString(fmt.Sprintf("-- title: %s\n", s.Title))
	if s.Tags != "" {
		b.WriteString(fmt.Sprintf("-- tags: %s\n", s.Tags))
	}
	b.WriteString("\n")
	b.WriteString(s.SQLContent)
	return []byte(b.String())
}

func parseSnippetSQL(raw string, filename string) *Snippet {
	snippet := &Snippet{
		Title: strings.TrimSuffix(filename, ".sql"),
	}

	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	var sqlLines []string

	inHeader := true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if inHeader && strings.HasPrefix(trimmed, "--") {
			headerContent := strings.TrimSpace(strings.TrimPrefix(trimmed, "--"))
			parts := strings.SplitN(headerContent, ":", 2)
			if len(parts) == 2 {
				k := strings.TrimSpace(parts[0])
				v := strings.TrimSpace(parts[1])
				switch k {
				case "id":
					snippet.ID = v
				case "title":
					snippet.Title = v
				case "tags":
					snippet.Tags = v
				}
				continue
			}
		}
		if trimmed != "" {
			inHeader = false
		}
		if !inHeader {
			sqlLines = append(sqlLines, line)
		}
	}

	snippet.SQLContent = strings.TrimSpace(strings.Join(sqlLines, "\n"))
	if snippet.SQLContent == "" {
		snippet.SQLContent = raw
	}
	return snippet
}

func sanitizeYAMLString(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

// encodePathSegment makes an identifier a lossless, traversal-safe path segment (unlike slugify, which
// is only for display names): percent-encoding round-trips any name, a leading '.' is escaped so ".."
// cannot form, and the empty name becomes "%" (a token PathEscape never emits).
func encodePathSegment(s string) string {
	if s == "" {
		return "%"
	}
	enc := url.PathEscape(s)
	if strings.HasPrefix(enc, ".") {
		enc = "%2E" + enc[1:]
	}
	return enc
}

// decodePathSegment reverses encodePathSegment; legacy slug paths decode to themselves.
func decodePathSegment(s string) string {
	if s == "%" {
		return ""
	}
	if dec, err := url.PathUnescape(s); err == nil {
		return dec
	}
	return s
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteRune('-')
			}
		}
	}
	res := strings.Trim(b.String(), "-")
	if res == "" {
		return "untitled"
	}
	if len(res) > 40 {
		return res[:40]
	}
	return res
}
