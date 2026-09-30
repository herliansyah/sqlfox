package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sqlfox/internal/driver"
	"sqlfox/internal/vault"
)

func TestServer_API_Lifecycle(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-server-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	vaultDB := filepath.Join(tempDir, "config.db")
	targetDB := filepath.Join(tempDir, "target.db")

	v, err := vault.OpenVault(vaultDB, "test-master")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "test-master")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := ts.Client()

	// 1. Create a Connection
	createPayload := vault.ConnectionRecord{
		Name:         "Local Test SQLite",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: targetDB,
	}
	body, _ := json.Marshal(createPayload)

	resp, err := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/connections failed: %v, status: %d", err, resp.StatusCode)
	}

	var savedConn vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&savedConn)
	resp.Body.Close()

	if savedConn.ID == "" {
		t.Fatal("expected saved connection ID")
	}

	// 2. List Connections
	resp, err = client.Get(ts.URL + "/api/connections")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/connections failed: %v", err)
	}
	var list []vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()

	if len(list) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(list))
	}

	// 3. Connect Session
	resp, err = client.Post(fmt.Sprintf("%s/api/session/connect/%s", ts.URL, savedConn.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/session/connect failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Execute CREATE TABLE
	createTableSQL := map[string]any{
		"sql": "CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, price REAL);",
	}
	sqlBody, _ := json.Marshal(createTableSQL)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, savedConn.ID), "application/json", bytes.NewReader(sqlBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Execute CREATE TABLE failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Test Safety Guard Inspection on dangerous query
	dangerousSQL := map[string]any{
		"sql": "DELETE FROM products;",
	}
	dangBody, _ := json.Marshal(dangerousSQL)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/inspect", ts.URL, savedConn.ID), "application/json", bytes.NewReader(dangBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Inspect failed: %v", err)
	}
	var inspRes struct {
		RiskLevel         string `json:"risk_level"`
		RequiresChallenge bool   `json:"requires_challenge"`
		TargetTable       string `json:"target_table"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&inspRes)
	resp.Body.Close()

	if inspRes.RiskLevel != "CRITICAL" || !inspRes.RequiresChallenge || inspRes.TargetTable != "products" {
		t.Fatalf("unexpected inspection outcome: %+v", inspRes)
	}

	// 6. Attempt executing dangerous query without confirmation -> Expect 422 Unprocessable
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, savedConn.ID), "application/json", bytes.NewReader(dangBody))
	if err != nil || resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected HTTP 422 for blocked query, got status: %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 7. Insert and Query Rows
	insertSQL := map[string]any{
		"sql": "INSERT INTO products (name, price) VALUES ('Widget A', 19.99), ('Widget B', 29.99);",
	}
	insBody, _ := json.Marshal(insertSQL)
	resp, _ = client.Post(fmt.Sprintf("%s/api/session/%s/execute", ts.URL, savedConn.ID), "application/json", bytes.NewReader(insBody))
	resp.Body.Close()

	// Fetch table rows via keyset/viewer endpoint
	resp, err = client.Get(fmt.Sprintf("%s/api/session/%s/table/products/rows", ts.URL, savedConn.ID))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET table rows failed: %v", err)
	}
	var rowsRes struct {
		Rows [][]any `json:"rows"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&rowsRes)
	resp.Body.Close()

	if len(rowsRes.Rows) != 2 {
		t.Fatalf("expected 2 rows in products table, got %d", len(rowsRes.Rows))
	}

	// 7b. Test Explain query with Database parameter
	explainSQL := map[string]any{
		"sql":      "SELECT * FROM products WHERE price > 20;",
		"database": targetDB,
	}
	expBody, _ := json.Marshal(explainSQL)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/explain", ts.URL, savedConn.ID), "application/json", bytes.NewReader(expBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Explain query failed: %v", err)
	}
	resp.Body.Close()

	// 7c. Test Stateless schema introspection with ?db=
	resp, err = client.Get(fmt.Sprintf("%s/api/session/%s/schema?db=%s", ts.URL, savedConn.ID, targetDB))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET schema with db parameter failed: %v", err)
	}
	resp.Body.Close()

	// 8. Toggle ReadOnly
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/toggle-readonly", ts.URL, savedConn.ID), "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("Toggle read-only failed: %v", err)
	}
	resp.Body.Close()

	// 9. Test Transient Connection Test endpoint (SQLite)
	testTransientPayload := vault.ConnectionRecord{
		Driver:       "sqlite",
		DatabaseName: targetDB,
	}
	testBody, _ := json.Marshal(testTransientPayload)
	resp, err = client.Post(ts.URL+"/api/connections/test-transient/test", "application/json", bytes.NewReader(testBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/connections/test-transient/test failed: %v, status: %d", err, resp.StatusCode)
	}
	var testResult struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&testResult)
	resp.Body.Close()
	if !testResult.Success {
		t.Fatalf("expected transient connection test to succeed")
	}

	// 9b. Test Transient Connection Test endpoint (MySQL driver recognized)
	testMySQLPayload := vault.ConnectionRecord{
		Driver: "mysql",
		Host:   "127.0.0.1",
		Port:   59999, // Unused port to test driver lookup without real db
	}
	mySQLBody, _ := json.Marshal(testMySQLPayload)
	resp, err = client.Post(ts.URL+"/api/connections/test-transient/test", "application/json", bytes.NewReader(mySQLBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST transient mysql test failed: %v", err)
	}
	_ = json.NewDecoder(resp.Body).Decode(&testResult)
	resp.Body.Close()
	// Driver should be recognized (i.e. error must NOT be "unsupported database driver")
	if strings.Contains(testResult.Error, "unsupported database driver") {
		t.Fatalf("unexpected unsupported driver error for mysql: %s", testResult.Error)
	}

	// 10. Update Connection (editing connection name)
	savedConn.Name = "Updated SQLite Database"
	updateBody, _ := json.Marshal(savedConn)
	resp, err = client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(updateBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST update connection failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 11. Test Select Database endpoint
	selectDBPayload := map[string]string{"database": "main"}
	sdbBody, _ := json.Marshal(selectDBPayload)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/select-db", ts.URL, savedConn.ID), "application/json", bytes.NewReader(sdbBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/session/select-db failed: %v, status: %d", err, resp.StatusCode)
	}
	var schemaAfterSelect struct {
		DatabaseName string   `json:"database_name"`
		Databases    []string `json:"databases"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&schemaAfterSelect)
	resp.Body.Close()
	if schemaAfterSelect.DatabaseName != "main" || len(schemaAfterSelect.Databases) == 0 {
		t.Fatalf("unexpected schema after select-db: %+v", schemaAfterSelect)
	}

	// 12. Test Explain Query Plan endpoint
	explainPayload := map[string]string{
		"sql":      "SELECT * FROM products WHERE id = 1;",
		"database": targetDB,
	}
	expBody, _ = json.Marshal(explainPayload)
	resp, err = client.Post(fmt.Sprintf("%s/api/session/%s/explain", ts.URL, savedConn.ID), "application/json", bytes.NewReader(expBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/session/explain failed: %v, status: %d", err, resp.StatusCode)
	}
	var explainResult struct {
		RawJSON string               `json:"raw_json"`
		Rows    []driver.ExplainRow `json:"rows"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&explainResult); err != nil {
		t.Fatalf("failed to decode explain response: %v", err)
	}
	resp.Body.Close()
	if explainResult.Rows == nil {
		t.Fatalf("expected non-nil Rows in explainResult, got nil")
	}
	if len(explainResult.Rows) == 0 {
		t.Fatalf("expected at least 1 plan row, got 0")
	}

	// 13. Delete Connection
	reqDelete, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/connections/%s", ts.URL, savedConn.ID), nil)
	resp, err = client.Do(reqDelete)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE connection failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()
}

func TestServer_API_Snippets(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-snip-api-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	vaultDB := filepath.Join(tempDir, "config.db")
	v, err := vault.OpenVault(vaultDB, "test-master")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "test-master")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := ts.Client()

	// 1. GET /api/snippets -> should have default seeded snippets
	resp, err := client.Get(ts.URL + "/api/snippets")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/snippets failed: %v, status: %d", err, resp.StatusCode)
	}
	var snippets []vault.Snippet
	_ = json.NewDecoder(resp.Body).Decode(&snippets)
	resp.Body.Close()

	if len(snippets) < 3 {
		t.Fatalf("expected seeded snippets, got %d", len(snippets))
	}

	// 2. POST /api/snippets -> create new snippet
	newSnip := vault.Snippet{
		Title:      "Active Tenant Check",
		SQLContent: "SELECT * FROM tenants WHERE id = {{tenant_id:1}} AND status = '{{status:active}}';",
		Tags:       "general,multi-tenant",
	}
	snipBody, _ := json.Marshal(newSnip)
	resp, err = client.Post(ts.URL+"/api/snippets", "application/json", bytes.NewReader(snipBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/snippets failed: %v, status: %d", err, resp.StatusCode)
	}
	var savedSnip vault.Snippet
	_ = json.NewDecoder(resp.Body).Decode(&savedSnip)
	resp.Body.Close()

	if savedSnip.ID == "" || savedSnip.Title != "Active Tenant Check" {
		t.Fatalf("unexpected saved snippet: %+v", savedSnip)
	}

	// 3. DELETE /api/snippets/{id}
	reqDel, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/api/snippets/%s", ts.URL, savedSnip.ID), nil)
	resp, err = client.Do(reqDel)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/snippets failed: %v, status: %d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Verify deletion
	resp, err = client.Get(ts.URL + "/api/snippets")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/snippets failed: %v", err)
	}
	var remaining []vault.Snippet
	_ = json.NewDecoder(resp.Body).Decode(&remaining)
	resp.Body.Close()

	for _, s := range remaining {
		if s.ID == savedSnip.ID {
			t.Fatalf("expected snippet %s to be deleted", savedSnip.ID)
		}
	}
}

func TestServer_API_TableNotes(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sqlfox-notes-api-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	vaultDB := filepath.Join(tempDir, "config.db")
	v, err := vault.OpenVault(vaultDB, "test-master")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "test-master")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := ts.Client()

	// 1. GET note when none exists -> returns empty content
	resp, err := client.Get(ts.URL + "/api/session/conn-123/notes/orders?db=sales")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET table note failed: %v, status: %d", err, resp.StatusCode)
	}
	var emptyNote struct {
		Content string `json:"content"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&emptyNote)
	resp.Body.Close()
	if emptyNote.Content != "" {
		t.Fatalf("expected empty note, got: %s", emptyNote.Content)
	}

	// 2. POST save note
	payload := map[string]string{
		"content": "### Orders Rules\n- Must verify user_id.",
	}
	body, _ := json.Marshal(payload)
	resp, err = client.Post(ts.URL+"/api/session/conn-123/notes/orders?db=sales", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("POST table note failed: %v, status: %d", err, resp.StatusCode)
	}
	var savedNote vault.TableNote
	_ = json.NewDecoder(resp.Body).Decode(&savedNote)
	resp.Body.Close()

	if savedNote.Content != payload["content"] {
		t.Fatalf("saved note content mismatch: %+v", savedNote)
	}

	// 3. GET verify saved note
	resp, err = client.Get(ts.URL + "/api/session/conn-123/notes/orders?db=sales")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET table note after save failed: %v", err)
	}
	var fetchedNote vault.TableNote
	_ = json.NewDecoder(resp.Body).Decode(&fetchedNote)
	resp.Body.Close()

	if fetchedNote.Content != payload["content"] {
		t.Fatalf("fetched note content mismatch: %+v", fetchedNote)
	}
}

func TestServer_API_ImportCSV(t *testing.T) {
	// 1. Verify SniffCSVDelimiter
	if SniffCSVDelimiter([]byte("a,b,c\n1,2,3")) != ',' {
		t.Fatal("expected comma delimiter")
	}
	if SniffCSVDelimiter([]byte("name;phone;age\nJohn;0812;30")) != ';' {
		t.Fatal("expected semicolon delimiter")
	}
	if SniffCSVDelimiter([]byte("col1\tcol2\nval1\tval2")) != '\t' {
		t.Fatal("expected tab delimiter")
	}

	// 2. Integration test with SQLite session
	tempDir, err := os.MkdirTemp("", "sqlfox-import-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	vaultDB := filepath.Join(tempDir, "config.db")
	targetDB := filepath.Join(tempDir, "target.db")

	v, err := vault.OpenVault(vaultDB, "test-master")
	if err != nil {
		t.Fatal(err)
	}
	defer v.Close()

	srv := NewServer(v, nil, filepath.Join(tempDir, "backups"), "test-master")
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	client := ts.Client()

	// Create and connect SQLite connection
	connPayload := vault.ConnectionRecord{
		Name:         "Import Test DB",
		Environment:  "Development",
		Driver:       "sqlite",
		DatabaseName: targetDB,
	}
	body, _ := json.Marshal(connPayload)
	resp, err := client.Post(ts.URL+"/api/connections", "application/json", bytes.NewReader(body))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to create connection: %v", err)
	}
	var savedConn vault.ConnectionRecord
	_ = json.NewDecoder(resp.Body).Decode(&savedConn)
	resp.Body.Close()

	// Connect session
	resp, err = client.Post(ts.URL+"/api/session/connect/"+savedConn.ID, "application/json", nil)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to connect session: %v", err)
	}
	resp.Body.Close()

	// Create test table
	createTableSQL := `
	CREATE TABLE customers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		phone TEXT,
		email TEXT
	);
	`
	execBody, _ := json.Marshal(map[string]any{"sql": createTableSQL})
	resp, err = client.Post(ts.URL+"/api/session/"+savedConn.ID+"/execute", "application/json", bytes.NewReader(execBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to create table: %v", err)
	}
	resp.Body.Close()

	// Prepare CSV with UTF-8 BOM, semicolon delimiter, and leading zero phone number
	csvContent := "\xef\xbb\xbfnama_lengkap;nomor_hp;email;unwanted\n" +
		"Budi Santoso;081234567890;budi@example.com;foo\n" +
		"Siti Rahma;085678901234;;bar\n"

	// Multipart upload
	var reqBody bytes.Buffer
	writer := multipart.NewWriter(&reqBody)
	part, _ := writer.CreateFormFile("file", "customers.csv")
	_, _ = part.Write([]byte(csvContent))
	_ = writer.WriteField("delimiter", "auto")
	_ = writer.WriteField("empty_as_null", "true")
	_ = writer.WriteField("skip_errors", "false")
	mapping := map[string]string{
		"0": "name",
		"1": "phone",
		"2": "email",
		"3": "SKIP",
	}
	mappingBytes, _ := json.Marshal(mapping)
	_ = writer.WriteField("mapping", string(mappingBytes))
	_ = writer.Close()

	req, err := http.NewRequest("POST", ts.URL+"/api/session/"+savedConn.ID+"/table/customers/import-csv", &reqBody)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("CSV import failed: %v, status: %d", err, resp.StatusCode)
	}

	var importRes struct {
		Success      bool     `json:"success"`
		Delimiter    string   `json:"delimiter"`
		TotalRows    int      `json:"total_rows"`
		ImportedRows int      `json:"imported_rows"`
		SkippedRows  int      `json:"skipped_rows"`
		Errors       []string `json:"errors"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&importRes)
	resp.Body.Close()

	if !importRes.Success || importRes.ImportedRows != 2 || importRes.Delimiter != ";" {
		t.Fatalf("unexpected import result: %+v", importRes)
	}

	// Verify data in table: check leading zero preservation and null email
	queryBody, _ := json.Marshal(map[string]any{"sql": "SELECT name, phone, email FROM customers ORDER BY id ASC;"})
	resp, err = client.Post(ts.URL+"/api/session/"+savedConn.ID+"/execute", "application/json", bytes.NewReader(queryBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("failed to query imported table: %v", err)
	}
	var queryRes struct {
		Rows [][]any `json:"rows"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&queryRes)
	resp.Body.Close()

	if len(queryRes.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(queryRes.Rows))
	}

	// Verify Budi's phone has leading zero
	budiPhone := fmt.Sprintf("%v", queryRes.Rows[0][1])
	if budiPhone != "081234567890" {
		t.Fatalf("expected leading zero preserved '081234567890', got '%s'", budiPhone)
	}

	// Verify Siti's email is nil (NULL)
	sitiEmail := queryRes.Rows[1][2]
	if sitiEmail != nil {
		t.Fatalf("expected NULL email for Siti, got: %v", sitiEmail)
	}
}



