package dataimport

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sqlfox/internal/driver"
	_ "sqlfox/internal/driver/sqlite"
)

func TestSniffDelimiter(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected rune
	}{
		{"comma", "id,name,email\n1,Alice,alice@example.com\n2,Bob,bob@example.com", ','},
		{"semicolon", "id;name;email\n1;Alice;alice@example.com\n2;Bob;bob@example.com", ';'},
		{"tab", "id\tname\temail\n1\tAlice\talice@example.com\n2\tBob\tbob@example.com", '\t'},
		{"pipe", "id|name|email\n1|Alice|alice@example.com\n2|Bob|bob@example.com", '|'},
		{"default fallback", "singleline_no_delimiters", ','},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := SniffDelimiter([]byte(tc.input))
			if got != tc.expected {
				t.Fatalf("expected delimiter %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestCSVImporter_Import(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-import-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test.db")
	sess, err := driver.Connect(context.Background(), driver.Config{
		Driver:       "sqlite",
		DatabaseName: dbPath,
	})
	if err != nil {
		t.Fatalf("driver.Connect failed: %v", err)
	}
	defer sess.Close()

	ctx := context.Background()

	// 1. Create target table with UNIQUE constraint on phone
	_, err = sess.Exec(ctx, `CREATE TABLE contacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		phone TEXT UNIQUE,
		notes TEXT
	);`)
	if err != nil {
		t.Fatalf("CREATE TABLE failed: %v", err)
	}

	// 2. CSV content with semicolon delimiter, UTF-8 BOM, leading zeros in phone, empty notes
	csvData := "\xef\xbb\xbfNama Lengkap;Nomor Telepon;Catatan Tambahan;IgnoreMe\n" +
		"Ahmad;08123456789;;ignore1\n" +
		"Budi;08987654321;VIP Customer;ignore2\n" +
		"Cici;08123456789;Duplicate Phone;ignore3\n" // duplicate phone to test SkipErrors fallback

	mapping := map[string]string{
		"Nama Lengkap":     "name",
		"Nomor Telepon":    "phone",
		"Catatan Tambahan": "notes",
		"IgnoreMe":         "SKIP",
	}

	opts := ImportOptions{
		EmptyAsNull: true,
		SkipErrors:  true,
		Mapping:     mapping,
	}

	res, err := Import(ctx, sess, "contacts", strings.NewReader(csvData), opts)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	if !res.Success {
		t.Fatalf("expected import success")
	}
	if res.Delimiter != ";" {
		t.Fatalf("expected auto-sniffed delimiter ';', got %q", res.Delimiter)
	}
	if res.TotalRows != 3 {
		t.Fatalf("expected 3 total rows, got %d", res.TotalRows)
	}
	if res.ImportedRows != 2 {
		t.Fatalf("expected 2 imported rows (1 skipped on error), got %d", res.ImportedRows)
	}
	if res.SkippedRows != 1 {
		t.Fatalf("expected 1 skipped row, got %d", res.SkippedRows)
	}
	if len(res.Errors) == 0 {
		t.Fatalf("expected error message recorded for duplicate row")
	}

	// 3. Verify data in SQLite: verify leading zero preserved and empty notes is NULL
	stream, err := sess.QueryStream(ctx, "SELECT name, phone, notes FROM contacts WHERE name = 'Ahmad'", 10)
	if err != nil {
		t.Fatalf("SELECT failed: %v", err)
	}
	defer stream.Close()

	chunk, err := stream.NextChunk(10)
	if err != nil && len(chunk) == 0 {
		t.Fatalf("NextChunk failed: %v", err)
	}
	if len(chunk) != 1 {
		t.Fatalf("expected 1 row for Ahmad, got %d", len(chunk))
	}

	row := chunk[0]
	nameVal, phoneVal, notesVal := row[0], row[1], row[2]

	if nameVal != "Ahmad" {
		t.Fatalf("expected name 'Ahmad', got %v", nameVal)
	}
	if phoneVal != "08123456789" {
		t.Fatalf("expected phone to preserve leading zero '08123456789', got %v", phoneVal)
	}
	if notesVal != nil {
		t.Fatalf("expected notes to be NULL, got %v", notesVal)
	}
}
