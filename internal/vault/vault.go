package vault

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

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
	UseSSL               bool      `json:"use_ssl"`
	SSLCA                string    `json:"ssl_ca,omitempty"`
	SSLCert              string    `json:"ssl_cert,omitempty"`
	SSLKey               string    `json:"ssl_key,omitempty"`
	UseSSH               bool      `json:"use_ssh"`
	SSHHost              string    `json:"ssh_host,omitempty"`
	SSHPort              int       `json:"ssh_port,omitempty"`
	SSHUser              string    `json:"ssh_user,omitempty"`
	SSHPassword          string    `json:"ssh_password,omitempty"`
	EncryptedSSHPassword string    `json:"-"`
	SSHKey               string    `json:"ssh_key,omitempty"`
	EncryptedSSHKey      string    `json:"-"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
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
		use_ssh INTEGER DEFAULT 0,
		ssh_host TEXT,
		ssh_port INTEGER,
		ssh_user TEXT,
		encrypted_ssh_password TEXT,
		encrypted_ssh_key TEXT,
		created_at TIMESTAMP,
		updated_at TIMESTAMP
	);
	`
	_, err := db.Exec(query)
	return err
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

// SaveConnection inserts or updates a connection record.
func (v *Vault) SaveConnection(conn *ConnectionRecord) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if conn.ID == "" {
		conn.ID = uuid.NewString()
		conn.CreatedAt = time.Now()
	}
	conn.UpdatedAt = time.Now()

	var encPass, encSSHPass, encSSHKey string
	var err error

	if conn.Password != "" {
		encPass, err = Encrypt([]byte(conn.Password), v.key)
		if err != nil {
			return fmt.Errorf("failed to encrypt password: %w", err)
		}
	} else {
		encPass = conn.EncryptedPassword
	}

	if conn.SSHPassword != "" {
		encSSHPass, err = Encrypt([]byte(conn.SSHPassword), v.key)
		if err != nil {
			return fmt.Errorf("failed to encrypt ssh password: %w", err)
		}
	} else {
		encSSHPass = conn.EncryptedSSHPassword
	}

	if conn.SSHKey != "" {
		encSSHKey, err = Encrypt([]byte(conn.SSHKey), v.key)
		if err != nil {
			return fmt.Errorf("failed to encrypt ssh key: %w", err)
		}
	} else {
		encSSHKey = conn.EncryptedSSHKey
	}

	useSSLInt := 0
	if conn.UseSSL {
		useSSLInt = 1
	}
	useSSHInt := 0
	if conn.UseSSH {
		useSSHInt = 1
	}

	query := `
	INSERT INTO connections (
		id, name, environment, driver, host, port, database_name, user,
		encrypted_password, use_ssl, ssl_ca, ssl_cert, ssl_key,
		use_ssh, ssh_host, ssh_port, ssh_user, encrypted_ssh_password, encrypted_ssh_key,
		created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		use_ssh = excluded.use_ssh,
		ssh_host = excluded.ssh_host,
		ssh_port = excluded.ssh_port,
		ssh_user = excluded.ssh_user,
		encrypted_ssh_password = excluded.encrypted_ssh_password,
		encrypted_ssh_key = excluded.encrypted_ssh_key,
		updated_at = excluded.updated_at;
	`

	_, err = v.db.Exec(query,
		conn.ID, conn.Name, conn.Environment, conn.Driver, conn.Host, conn.Port, conn.DatabaseName, conn.User,
		encPass, useSSLInt, conn.SSLCA, conn.SSLCert, conn.SSLKey,
		useSSHInt, conn.SSHHost, conn.SSHPort, conn.SSHUser, encSSHPass, encSSHKey,
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
		use_ssh, ssh_host, ssh_port, ssh_user, encrypted_ssh_password, encrypted_ssh_key,
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

// ListConnections returns all connections without plaintext secrets.
func (v *Vault) ListConnections() ([]*ConnectionRecord, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	rows, err := v.db.Query(`SELECT 
		id, name, environment, driver, host, port, database_name, user,
		encrypted_password, use_ssl, ssl_ca, ssl_cert, ssl_key,
		use_ssh, ssh_host, ssh_port, ssh_user, encrypted_ssh_password, encrypted_ssh_key,
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
	var useSSL, useSSH int
	var encPass, sslCA, sslCert, sslKey, encSSHPass, encSSHKey sql.NullString
	var host, dbName, user, sshHost, sshUser sql.NullString
	var port, sshPort sql.NullInt64

	err := s.Scan(
		&c.ID, &c.Name, &c.Environment, &c.Driver, &host, &port, &dbName, &user,
		&encPass, &useSSL, &sslCA, &sslCert, &sslKey,
		&useSSH, &sshHost, &sshPort, &sshUser, &encSSHPass, &encSSHKey,
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
	c.UseSSH = useSSH == 1
	c.SSHHost = sshHost.String
	c.SSHPort = int(sshPort.Int64)
	c.SSHUser = sshUser.String
	c.EncryptedSSHPassword = encSSHPass.String
	c.EncryptedSSHKey = encSSHKey.String

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
	if c.EncryptedSSHPassword != "" {
		dec, err := Decrypt(c.EncryptedSSHPassword, v.key)
		if err != nil {
			return fmt.Errorf("failed to decrypt ssh password: %w", err)
		}
		c.SSHPassword = string(dec)
	}
	if c.EncryptedSSHKey != "" {
		dec, err := Decrypt(c.EncryptedSSHKey, v.key)
		if err != nil {
			return fmt.Errorf("failed to decrypt ssh key: %w", err)
		}
		c.SSHKey = string(dec)
	}
	return nil
}
