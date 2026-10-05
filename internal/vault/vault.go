package vault

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Environment          string    `json:"environment"` // Production, Staging, Development
	Driver               string    `json:"driver"`      // mysql, sqlite
	Host                 string    `json:"host"`
	Port                 int       `json:"port"`
	DatabaseName         string    `json:"database_name"`
	User                 string    `json:"user"`
	Password             string    `json:"password,omitempty"` // Plaintext when in-memory
	EncryptedPassword    string    `json:"-"`
	UseSSL            bool      `json:"use_ssl"`
	SSLCA             string    `json:"ssl_ca,omitempty"`
	SSLCert           string    `json:"ssl_cert,omitempty"`
	SSLKey            string    `json:"ssl_key,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Vault manages encrypted credential and profile storage.
type Vault struct {
	db  *sql.DB
	key []byte
	mu  sync.RWMutex
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

	return &Vault{
		db:  db,
		key: key,
	}, nil
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
	`
	if _, err := db.Exec(query); err != nil {
		return err
	}
	// ponytail: Idempotent column addition for existing vault databases
	_, _ = db.Exec("ALTER TABLE audit_logs ADD COLUMN source TEXT NOT NULL DEFAULT 'editor'")
	_, _ = db.Exec("CREATE INDEX IF NOT EXISTS idx_audit_logs_source ON audit_logs(session_id, source)")
	return seedDefaultSnippets(db)
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
	defer v.mu.RUnlock()

	salt, checkCipher, err := getMeta(v.db)
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

// SaveConnection inserts or updates a connection record.
func (v *Vault) SaveConnection(conn *ConnectionRecord) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if conn.ID == "" {
		conn.ID = newUUID()
		conn.CreatedAt = time.Now()
	}
	conn.UpdatedAt = time.Now()

	var encPass string
	var err error

	if conn.Password != "" {
		encPass, err = Encrypt([]byte(conn.Password), v.key)
		if err != nil {
			return fmt.Errorf("failed to encrypt password: %w", err)
		}
	} else {
		encPass = conn.EncryptedPassword
		if encPass == "" && conn.ID != "" {
			var existingPass string
			if err := v.db.QueryRow(`SELECT encrypted_password FROM connections WHERE id = ?`, conn.ID).Scan(&existingPass); err == nil {
				encPass = existingPass
			}
		}
	}

	useSSLInt := 0
	if conn.UseSSL {
		useSSLInt = 1
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

	_, err = v.db.Exec(query,
		conn.ID, conn.Name, conn.Environment, conn.Driver, conn.Host, conn.Port, conn.DatabaseName, conn.User,
		encPass, useSSLInt, conn.SSLCA, conn.SSLCert, conn.SSLKey,
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

	return &c, nil
}

func (v *Vault) decryptConnection(c *ConnectionRecord) error {
	if c.EncryptedPassword != "" {
		dec, err := Decrypt(c.EncryptedPassword, v.key)
		if err != nil {
			return fmt.Errorf("failed to decrypt password: %w", err)
		}
		c.Password = string(dec)
	}
	return nil
}


