package vault

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// ponytail: RFC 4122 v4 UUID generator using stdlib crypto/rand; eliminates external uuid module.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

var (
	ErrIncorrectMasterPassword = errors.New("incorrect master password")
	ErrConnectionNotFound      = errors.New("connection not found")
)

const validationToken = "sqlfox_master_valid_token"

// ConnectionRecord represents a stored database connection configuration.
type ConnectionRecord struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Environment       string    `json:"environment"` // Production, Staging, Development
	Driver            string    `json:"driver"`      // mysql, sqlite
	Host              string    `json:"host"`
	Port              int       `json:"port"`
	DatabaseName      string    `json:"database_name"`
	User              string    `json:"user"`
	Password          string    `json:"password,omitempty"` // Plaintext when in-memory
	EncryptedPassword string    `json:"-"`
	UseSSL            bool      `json:"use_ssl"`
	SSLCA             string    `json:"ssl_ca,omitempty"`
	SSLCert           string    `json:"ssl_cert,omitempty"`
	SSLKey            string    `json:"ssl_key,omitempty"`
	HasSSLKey         bool      `json:"has_ssl_key,omitempty"`
	ClearSSL          bool      `json:"clear_ssl,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Vault manages encrypted credential and profile storage.
type Vault struct {
	db     *sql.DB
	dbPath string
	key    []byte
	mu     sync.RWMutex
}

// Path returns the filesystem path of the SQLite vault database.
func (v *Vault) Path() string {
	if v == nil {
		return ""
	}
	return v.dbPath
}

// OpenVault opens or creates the SQLite vault at the specified path and initializes the master key.
func OpenVault(dbPath string, masterPassword string) (*Vault, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create vault directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open vault db: %w", err)
	}
	// SQLite allows only one writer at a time; capping pool size prevents SQLITE_BUSY under concurrent reads/writes.
	db.SetMaxOpenConns(1)

	_, _ = db.Exec("PRAGMA journal_mode = WAL; PRAGMA synchronous = NORMAL; PRAGMA busy_timeout = 5000;")

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	salt, checkCipher, err := getMeta(db)
	if err != nil {
		db.Close()
		return nil, err
	}

	var key []byte
	if salt == nil {
		// First-time setup: generate salt and save check token
		salt, err = GenerateSalt()
		if err != nil {
			db.Close()
			return nil, err
		}
		key = DeriveKey(masterPassword, salt)
		checkCipher, err = Encrypt([]byte(validationToken), key)
		if err != nil {
			db.Close()
			return nil, err
		}

		if err := saveMeta(db, salt, checkCipher); err != nil {
			db.Close()
			return nil, err
		}
	} else {
		// Existing vault: derive key and verify check token
		key = DeriveKey(masterPassword, salt)
		decrypted, err := Decrypt(checkCipher, key)
		if err != nil || string(decrypted) != validationToken {
			db.Close()
			return nil, ErrIncorrectMasterPassword
		}
	}

	v := &Vault{
		db:     db,
		dbPath: dbPath,
		key:    key,
	}
	// Tighten vault file permissions so local multi-user hosts cannot read config.db.
	if dbPath != "" && dbPath != ":memory:" && !strings.HasPrefix(dbPath, "file:") {
		_ = os.Chmod(dbPath, 0600)
	}
	// ponytail: Re-seal any legacy pre-ADR-0060 secrets with bound AAD so vault cannot be swapped
	v.migrateLegacyAAD()
	// ponytail: Purge stale audit logs on vault open to prevent unbounded disk growth
	_, _ = v.PurgeAuditLogs(30, 50000)
	return v, nil
}

func initSchema(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS connections (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		environment TEXT NOT NULL,
		driver TEXT NOT NULL,
		host TEXT,
		port INTEGER,
		database_name TEXT,
		user TEXT,
		encrypted_password TEXT,
		use_ssl INTEGER DEFAULT 0,
		ssl_ca TEXT,
		ssl_cert TEXT,
		ssl_key TEXT,
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS snippets (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		sql_content TEXT NOT NULL,
		tags TEXT DEFAULT '',
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS table_notes (
		id TEXT PRIMARY KEY,
		connection_id TEXT NOT NULL,
		database_name TEXT NOT NULL,
		table_name TEXT NOT NULL,
		content TEXT NOT NULL,
		updated_at TIMESTAMP,
		UNIQUE(connection_id, database_name, table_name)
	);

	CREATE TABLE IF NOT EXISTS operational_notes (
		id TEXT PRIMARY KEY,
		title TEXT NOT NULL,
		content TEXT NOT NULL,
		side_note TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'inbox',
		is_pinned INTEGER NOT NULL DEFAULT 0,
		captured_at TIMESTAMP NOT NULL,
		reminder_at TIMESTAMP,
		object_tags TEXT NOT NULL DEFAULT '[]',
		user_tags TEXT NOT NULL DEFAULT '[]',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_operational_notes_status ON operational_notes(status);
	CREATE INDEX IF NOT EXISTS idx_operational_notes_reminder ON operational_notes(reminder_at);
	CREATE INDEX IF NOT EXISTS idx_operational_notes_pinned ON operational_notes(is_pinned DESC, updated_at DESC);

	CREATE TABLE IF NOT EXISTS audit_logs (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		connection_id TEXT NOT NULL,
		query_sql TEXT NOT NULL,
		duration_ms INTEGER NOT NULL,
		rows_affected INTEGER NOT NULL,
		error TEXT DEFAULT '',
		source TEXT NOT NULL DEFAULT 'editor',
		executed_at TIMESTAMP NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_audit_logs_session ON audit_logs(session_id, executed_at DESC);
	CREATE INDEX IF NOT EXISTS idx_audit_logs_executed ON audit_logs(executed_at DESC);

	CREATE TABLE IF NOT EXISTS flight_recorder_sessions (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		connection_id TEXT NOT NULL,
		name TEXT NOT NULL,
		duration_seconds INTEGER DEFAULT 0,
		sample_count INTEGER DEFAULT 0,
		started_at TIMESTAMP NOT NULL,
		stopped_at TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_flight_recorder_sessions ON flight_recorder_sessions(session_id, started_at DESC);

	CREATE TABLE IF NOT EXISTS flight_recorder_ticks (
		id TEXT PRIMARY KEY,
		recording_id TEXT NOT NULL,
		tick_index INTEGER NOT NULL,
		sampled_at TIMESTAMP NOT NULL,
		qps INTEGER DEFAULT 0,
		threads_running INTEGER DEFAULT 0,
		threads_connected INTEGER DEFAULT 0,
		row_lock_waits INTEGER DEFAULT 0,
		buffer_pool_wait_free INTEGER DEFAULT 0,
		top_queries_json TEXT DEFAULT '[]'
	);

	CREATE INDEX IF NOT EXISTS idx_flight_recorder_ticks ON flight_recorder_ticks(recording_id, tick_index ASC);

	CREATE TABLE IF NOT EXISTS ai_profiles (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		provider TEXT NOT NULL,
		encrypted_api_key TEXT DEFAULT '',
		base_url TEXT DEFAULT '',
		model_name TEXT NOT NULL,
		temperature REAL DEFAULT 0.3,
		language TEXT DEFAULT 'auto',
		response_style TEXT DEFAULT 'normal',
		custom_prompt TEXT DEFAULT '',
		include_schema INTEGER DEFAULT 1,
		mask_identifiers INTEGER DEFAULT 1,
		is_active INTEGER DEFAULT 0,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_ai_profiles_active ON ai_profiles(is_active DESC, updated_at DESC);
	`
	if _, err := db.Exec(query); err != nil {
		return err
	}
	// ponytail: Idempotent column addition for existing vault databases
	_, _ = db.Exec("ALTER TABLE audit_logs ADD COLUMN source TEXT NOT NULL DEFAULT 'editor'")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_audit_logs_source ON audit_logs(session_id, source)")
	if err := seedDefaultSnippets(db); err != nil {
		return err
	}
	return seedDefaultAIProfile(db)
}

func seedDefaultSnippets(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM snippets").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	now := time.Now()
	seeds := []Snippet{
		{
			ID:         newUUID(),
			Title:      "Find Duplicate Rows",
			SQLContent: "SELECT {{column_name:email}}, COUNT(*) AS duplicate_count\nFROM {{table_name:users}}\nGROUP BY {{column_name:email}}\nHAVING COUNT(*) > 1\nORDER BY duplicate_count DESC\nLIMIT {{limit:50}};",
			Tags:       "general",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ID:         newUUID(),
			Title:      "Table Row Counts & Sizes (MySQL)",
			SQLContent: "SELECT table_name, table_rows, ROUND(data_length / 1024 / 1024, 2) AS data_mb, ROUND(index_length / 1024 / 1024, 2) AS index_mb\nFROM information_schema.tables\nWHERE table_schema = {{database_name:DATABASE()}}\nORDER BY data_length DESC\nLIMIT {{limit:20}};",
			Tags:       "mysql",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ID:         newUUID(),
			Title:      "Active Running Queries (MySQL)",
			SQLContent: "SELECT id, user, host, db, command, time, state, info\nFROM information_schema.processlist\nWHERE command != 'Sleep'\nORDER BY time DESC;",
			Tags:       "mysql",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			ID:         newUUID(),
			Title:      "Table Columns & Info (SQLite)",
			SQLContent: "PRAGMA table_info({{table_name:users}});",
			Tags:       "sqlite",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}

	for _, s := range seeds {
		_, err := db.Exec(`
			INSERT INTO snippets (id, title, sql_content, tags, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, s.ID, s.Title, s.SQLContent, s.Tags, s.CreatedAt, s.UpdatedAt)
		if err != nil {
			return err
		}
	}
	return nil
}

func getMeta(db *sql.DB) ([]byte, string, error) {
	var saltB64, checkCipher string
	row := db.QueryRow(`SELECT value FROM meta WHERE key = 'salt'`)
	if err := row.Scan(&saltB64); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", nil
		}
		return nil, "", err
	}

	row = db.QueryRow(`SELECT value FROM meta WHERE key = 'check'`)
	if err := row.Scan(&checkCipher); err != nil {
		return nil, "", err
	}

	salt, err := base64.StdEncoding.DecodeString(saltB64)
	if err != nil {
		return nil, "", err
	}

	return salt, checkCipher, nil
}

func saveMeta(db *sql.DB, salt []byte, checkCipher string) error {
	saltB64 := base64.StdEncoding.EncodeToString(salt)
	_, err := db.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES ('salt', ?), ('check', ?)`, saltB64, checkCipher)
	return err
}

// Close closes the underlying SQLite database.
func (v *Vault) Close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.db.Close()
}

// ValidateMasterPassword verifies if a candidate master password is correct.
func (v *Vault) ValidateMasterPassword(password string) bool {
	v.mu.RLock()
	salt, checkCipher, err := getMeta(v.db)
	v.mu.RUnlock()
	if err != nil || salt == nil || checkCipher == "" {
		return false
	}

	testKey := DeriveKey(password, salt)
	decrypted, err := Decrypt(checkCipher, testKey)
	if err != nil {
		return false
	}
	return string(decrypted) == validationToken
}

type sqlExecutor interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) *sql.Row
}

// SaveConnection inserts or updates a connection record.
func (v *Vault) SaveConnection(conn *ConnectionRecord) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	return v.saveConnection(v.db, conn)
}

func (v *Vault) saveConnection(exec sqlExecutor, conn *ConnectionRecord) error {

	if conn.ID == "" {
		conn.ID = newUUID()
		conn.CreatedAt = time.Now()
	}
	conn.UpdatedAt = time.Now()

	var encPass string
	var err error

	if conn.Password != "" {
		encPass, err = EncryptWithAAD([]byte(conn.Password), v.key, connFieldAAD(conn.ID, "password"))
		if err != nil {
			return fmt.Errorf("failed to encrypt password: %w", err)
		}
	} else {
		encPass = conn.EncryptedPassword
		if encPass == "" && conn.ID != "" {
			var existingPass, exHost, exDriver string
			var exPort int
			if err := exec.QueryRow(`SELECT encrypted_password, host, port, driver FROM connections WHERE id = ?`, conn.ID).
				Scan(&existingPass, &exHost, &exPort, &exDriver); err == nil {
				// Prevent password exfiltration by only preserving the stored password if endpoint matches.
				if strings.EqualFold(conn.Host, exHost) && conn.Port == exPort && strings.EqualFold(conn.Driver, exDriver) {
					encPass = existingPass
				}
			}
		}
	}

	useSSLInt := 0
	if conn.UseSSL {
		useSSLInt = 1
	}
	sslCA := conn.SSLCA
	sslCert := conn.SSLCert
	sslKey := conn.SSLKey

	if conn.ClearSSL {
		useSSLInt = 0
		conn.UseSSL = false
		sslCA = ""
		sslCert = ""
		sslKey = ""
	} else if conn.ID != "" && conn.UseSSL && sslKey == "" {
		// Preserve stored client private key when editing an SSL connection without re-entering the key,
		// only if host/port/driver still match (prevents key exfiltration to an attacker-controlled endpoint).
		var exHost, exDriver string
		var exPort int
		var exKey sql.NullString
		if err := exec.QueryRow(`SELECT host, port, driver, ssl_key FROM connections WHERE id = ?`, conn.ID).
			Scan(&exHost, &exPort, &exDriver, &exKey); err == nil {
			if strings.EqualFold(conn.Host, exHost) && conn.Port == exPort && strings.EqualFold(conn.Driver, exDriver) {
				sslKey = exKey.String
			}
		}
	} else if conn.ID != "" && !conn.UseSSL && sslCA == "" && sslCert == "" && sslKey == "" {
		// Preserve existing SSL settings when caller omits them on partial update,
		// only if host/port/driver still match (prevents SSL credential exfiltration to a new endpoint).
		var exHost, exDriver string
		var exPort, exUseSSL int
		var exCA, exCert, exKey sql.NullString
		if err := exec.QueryRow(`SELECT host, port, driver, use_ssl, ssl_ca, ssl_cert, ssl_key FROM connections WHERE id = ?`, conn.ID).
			Scan(&exHost, &exPort, &exDriver, &exUseSSL, &exCA, &exCert, &exKey); err == nil {
			if strings.EqualFold(conn.Host, exHost) && conn.Port == exPort && strings.EqualFold(conn.Driver, exDriver) {
				useSSLInt = exUseSSL
				conn.UseSSL = exUseSSL == 1
				sslCA = exCA.String
				sslCert = exCert.String
				sslKey = exKey.String
			}
		}
	}

	conn.HasSSLKey = sslKey != ""

	// The client private key is a secret like the password; keep values already sealed by this vault.
	if sslKey != "" && !strings.HasPrefix(sslKey, sealedPrefix) {
		enc, err := EncryptWithAAD([]byte(sslKey), v.key, connFieldAAD(conn.ID, "ssl_key"))
		if err != nil {
			return fmt.Errorf("failed to encrypt ssl key: %w", err)
		}
		sslKey = sealedPrefix + enc
	}

	query := `
	INSERT INTO connections (
		id, name, environment, driver, host, port, database_name, user,
		encrypted_password, use_ssl, ssl_ca, ssl_cert, ssl_key,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		name = excluded.name,
		environment = excluded.environment,
		driver = excluded.driver,
		host = excluded.host,
		port = excluded.port,
		database_name = excluded.database_name,
		user = excluded.user,
		encrypted_password = excluded.encrypted_password,
		use_ssl = excluded.use_ssl,
		ssl_ca = excluded.ssl_ca,
		ssl_cert = excluded.ssl_cert,
		ssl_key = excluded.ssl_key,
		updated_at = excluded.updated_at;
	`

	_, err = exec.Exec(query,
		conn.ID, conn.Name, conn.Environment, conn.Driver, conn.Host, conn.Port, conn.DatabaseName, conn.User,
		encPass, useSSLInt, sslCA, sslCert, sslKey,
		conn.CreatedAt, conn.UpdatedAt,
	)
	return err
}

// GetConnection retrieves and decrypts a connection by ID.
func (v *Vault) GetConnection(id string) (*ConnectionRecord, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	row := v.db.QueryRow(`SELECT 
		id, name, environment, driver, host, port, database_name, user,
		encrypted_password, use_ssl, ssl_ca, ssl_cert, ssl_key,
		created_at, updated_at
	FROM connections WHERE id = ?`, id)

	conn, err := scanConnection(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConnectionNotFound
		}
		return nil, err
	}

	if err := v.decryptConnection(conn); err != nil {
		return nil, err
	}
	return conn, nil
}

// GetConnectionByName retrieves and decrypts a connection by name (case-insensitive) or ID.
func (v *Vault) GetConnectionByName(nameOrID string) (*ConnectionRecord, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	row := v.db.QueryRow(`SELECT 
		id, name, environment, driver, host, port, database_name, user,
		encrypted_password, use_ssl, ssl_ca, ssl_cert, ssl_key,
		created_at, updated_at
	FROM connections WHERE id = ? OR LOWER(name) = LOWER(?) LIMIT 1`, nameOrID, nameOrID)

	conn, err := scanConnection(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrConnectionNotFound
		}
		return nil, err
	}

	if err := v.decryptConnection(conn); err != nil {
		return nil, err
	}
	return conn, nil
}

// ListConnections returns all connections without plaintext secrets.
func (v *Vault) ListConnections() ([]*ConnectionRecord, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	rows, err := v.db.Query(`SELECT 
		id, name, environment, driver, host, port, database_name, user,
		encrypted_password, use_ssl, ssl_ca, ssl_cert, ssl_key,
		created_at, updated_at
	FROM connections ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*ConnectionRecord
	for rows.Next() {
		conn, err := scanConnection(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, conn)
	}
	return list, nil
}

// DeleteConnection removes a connection by ID.
func (v *Vault) DeleteConnection(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	res, err := v.db.Exec(`DELETE FROM connections WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrConnectionNotFound
	}
	return nil
}

type scannable interface {
	Scan(dest ...any) error
}

func scanConnection(s scannable) (*ConnectionRecord, error) {
	var c ConnectionRecord
	var useSSL int
	var encPass, sslCA, sslCert, sslKey sql.NullString
	var host, dbName, user sql.NullString
	var port sql.NullInt64

	err := s.Scan(
		&c.ID, &c.Name, &c.Environment, &c.Driver, &host, &port, &dbName, &user,
		&encPass, &useSSL, &sslCA, &sslCert, &sslKey,
		&c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	c.Host = host.String
	c.Port = int(port.Int64)
	c.DatabaseName = dbName.String
	c.User = user.String
	c.EncryptedPassword = encPass.String
	c.UseSSL = useSSL == 1
	c.SSLCA = sslCA.String
	c.SSLCert = sslCert.String
	c.SSLKey = sslKey.String
	c.HasSSLKey = sslKey.String != ""

	return &c, nil
}

func (v *Vault) decryptConnection(c *ConnectionRecord) error {
	if c.EncryptedPassword != "" {
		dec, err := DecryptWithAAD(c.EncryptedPassword, v.key, connFieldAAD(c.ID, "password"))
		if err != nil {
			return fmt.Errorf("failed to decrypt password: %w", err)
		}
		c.Password = string(dec)
	}
	if strings.HasPrefix(c.SSLKey, sealedPrefix) {
		dec, err := DecryptWithAAD(strings.TrimPrefix(c.SSLKey, sealedPrefix), v.key, connFieldAAD(c.ID, "ssl_key"))
		if err != nil {
			return fmt.Errorf("failed to decrypt ssl key: %w", err)
		}
		c.SSLKey = string(dec)
	}
	return nil
}

// sealedPrefix marks ssl_key values encrypted at rest; legacy plaintext rows still load unchanged.
const sealedPrefix = "enc:v1:"

// connFieldAAD binds a connection secret's ciphertext to its (connection ID, field) identity so it
// cannot be swapped into another field or connection by an actor with raw vault write access.
// DecryptWithAAD falls back to no-AAD for values sealed before this binding existed.
func connFieldAAD(connID, field string) []byte {
	return []byte("sqlfox:connection:" + connID + ":" + field)
}

// migrateLegacyAAD transparently re-seals any connection secrets written before per-field AAD
// binding existed (ADR 0060). Values sealed with nil AAD are decrypted, re-encrypted with their
// bound AAD ("sqlfox:connection:<id>:<field>"), and updated in-place so no connection remains in
// the swap-vulnerable nil-AAD format.
func (v *Vault) migrateLegacyAAD() {
	rows, err := v.db.Query(`SELECT id, encrypted_password, ssl_key FROM connections WHERE (encrypted_password != '' AND encrypted_password IS NOT NULL) OR (ssl_key != '' AND ssl_key IS NOT NULL)`)
	if err != nil {
		return
	}
	defer rows.Close()

	type secretToMigrate struct {
		id        string
		newPass   string
		newSSLKey string
	}
	var updates []secretToMigrate

	for rows.Next() {
		var id string
		var encPass, sslKey sql.NullString
		if err := rows.Scan(&id, &encPass, &sslKey); err != nil {
			continue
		}
		var item secretToMigrate
		item.id = id

		if encPass.Valid && encPass.String != "" {
			passAAD := connFieldAAD(id, "password")
			// Try with bound AAD first; if it already verifies, no migration needed.
			if _, err := decrypt(encPass.String, v.key, passAAD); err != nil {
				// Failed with AAD; try legacy nil-AAD
				if plain, err := decrypt(encPass.String, v.key, nil); err == nil {
					if sealed, err := EncryptWithAAD(plain, v.key, passAAD); err == nil {
						item.newPass = sealed
					}
				}
			}
		}

		if sslKey.Valid && sslKey.String != "" {
			keyAAD := connFieldAAD(id, "ssl_key")
			if strings.HasPrefix(sslKey.String, sealedPrefix) {
				rawCipher := strings.TrimPrefix(sslKey.String, sealedPrefix)
				if _, err := decrypt(rawCipher, v.key, keyAAD); err != nil {
					if plain, err := decrypt(rawCipher, v.key, nil); err == nil {
						if sealed, err := EncryptWithAAD(plain, v.key, keyAAD); err == nil {
							item.newSSLKey = sealedPrefix + sealed
						}
					}
				}
			} else {
				// Legacy plaintext ssl_key stored prior to at-rest sealing: encrypt in-place with bound AAD.
				if sealed, err := EncryptWithAAD([]byte(sslKey.String), v.key, keyAAD); err == nil {
					item.newSSLKey = sealedPrefix + sealed
				}
			}
		}

		if item.newPass != "" || item.newSSLKey != "" {
			updates = append(updates, item)
		}
	}

	for _, u := range updates {
		if u.newPass != "" && u.newSSLKey != "" {
			_, _ = v.db.Exec(`UPDATE connections SET encrypted_password = ?, ssl_key = ? WHERE id = ?`, u.newPass, u.newSSLKey, u.id)
		} else if u.newPass != "" {
			_, _ = v.db.Exec(`UPDATE connections SET encrypted_password = ? WHERE id = ?`, u.newPass, u.id)
		} else if u.newSSLKey != "" {
			_, _ = v.db.Exec(`UPDATE connections SET ssl_key = ? WHERE id = ?`, u.newSSLKey, u.id)
		}
	}
}
