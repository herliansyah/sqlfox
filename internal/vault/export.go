package vault

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// VaultArchive represents the portable encrypted export envelope.
type VaultArchive struct {
	Version int    `json:"version"`
	Salt    string `json:"salt"`
	Data    string `json:"data"` // Base64 encrypted JSON
}

// ExportVault packages all stored connections into an encrypted archive using the provided export passphrase.
func ExportVault(v *Vault, exportPassphrase string) ([]byte, error) {
	if exportPassphrase == "" {
		return nil, errors.New("export passphrase cannot be empty")
	}

	// 1. Fetch all connections with decrypted secrets
	list, err := v.ListConnections()
	if err != nil {
		return nil, err
	}

	var fullConnections []*ConnectionRecord
	for _, c := range list {
		full, err := v.GetConnection(c.ID)
		if err != nil {
			return nil, err
		}
		fullConnections = append(fullConnections, full)
	}

	plainJSON, err := json.Marshal(fullConnections)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal connections: %w", err)
	}

	// 2. Derive ephemeral export key
	salt, err := GenerateSalt()
	if err != nil {
		return nil, err
	}
	exportKey := DeriveKey(exportPassphrase, salt)

	// 3. Encrypt payload
	encryptedData, err := Encrypt(plainJSON, exportKey)
	if err != nil {
		return nil, err
	}

	archive := VaultArchive{
		Version: 1,
		Salt:    base64.StdEncoding.EncodeToString(salt),
		Data:    encryptedData,
	}

	return json.MarshalIndent(archive, "", "  ")
}

// ImportVault imports connections from an encrypted archive into the target vault.
func ImportVault(v *Vault, archiveJSON []byte, exportPassphrase string) (int, error) {
	if exportPassphrase == "" {
		return 0, errors.New("export passphrase cannot be empty")
	}

	var archive VaultArchive
	if err := json.Unmarshal(archiveJSON, &archive); err != nil {
		return 0, fmt.Errorf("invalid vault archive format: %w", err)
	}

	salt, err := base64.StdEncoding.DecodeString(archive.Salt)
	if err != nil {
		return 0, fmt.Errorf("invalid archive salt: %w", err)
	}

	exportKey := DeriveKey(exportPassphrase, salt)
	plainJSON, err := Decrypt(archive.Data, exportKey)
	if err != nil {
		return 0, fmt.Errorf("archive decryption failed: %w", err)
	}

	var connections []*ConnectionRecord
	if err := json.Unmarshal(plainJSON, &connections); err != nil {
		return 0, fmt.Errorf("corrupted archive payload: %w", err)
	}

	count := 0
	for _, conn := range connections {
		if err := v.SaveConnection(conn); err != nil {
			return count, fmt.Errorf("failed to import connection '%s': %w", conn.Name, err)
		}
		count++
	}

	return count, nil
}
