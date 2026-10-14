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

// ExportPayload encapsulates all vault artifacts exported into an archive.
type ExportPayload struct {
	Connections      []*ConnectionRecord `json:"connections"`
	Snippets         []*Snippet          `json:"snippets,omitempty"`
	TableNotes       []*TableNote        `json:"table_notes,omitempty"`
	OperationalNotes []*OperationalNote  `json:"operational_notes,omitempty"`
}

// ExportVault packages all stored connections, snippets, and table notes into an encrypted archive using the provided export passphrase.
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

	// 2. Fetch all snippets
	snippets, err := v.ListSnippets()
	if err != nil {
		return nil, err
	}

	// 3. Fetch all table notes
	tableNotes, err := v.ListAllTableNotes()
	if err != nil {
		return nil, err
	}

	// 4. Fetch all operational notes
	opNotes, err := v.ListAllOperationalNotes()
	if err != nil {
		return nil, err
	}

	payload := ExportPayload{
		Connections:      fullConnections,
		Snippets:         snippets,
		TableNotes:       tableNotes,
		OperationalNotes: opNotes,
	}

	plainJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal export payload: %w", err)
	}

	// 4. Derive ephemeral export key
	salt, err := GenerateSalt()
	if err != nil {
		return nil, err
	}
	exportKey := DeriveKey(exportPassphrase, salt)

	// 5. Encrypt payload
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

// ImportVault imports connections, snippets, and table notes from an encrypted archive into the target vault.
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

	var payload ExportPayload
	if err := json.Unmarshal(plainJSON, &payload); err == nil && (len(payload.Connections) > 0 || len(payload.Snippets) > 0 || len(payload.TableNotes) > 0 || len(payload.OperationalNotes) > 0) {
		// New structured envelope
		count := 0
		for _, conn := range payload.Connections {
			if err := v.SaveConnection(conn); err != nil {
				return count, fmt.Errorf("failed to import connection '%s': %w", conn.Name, err)
			}
			count++
		}
		for _, snip := range payload.Snippets {
			if err := v.SaveSnippet(snip); err != nil {
				return count, fmt.Errorf("failed to import snippet '%s': %w", snip.Title, err)
			}
		}
		for _, note := range payload.TableNotes {
			if err := v.SaveTableNote(note); err != nil {
				return count, fmt.Errorf("failed to import table note for '%s': %w", note.TableName, err)
			}
		}
		for _, opNote := range payload.OperationalNotes {
			if err := v.CreateOperationalNote(opNote); err != nil {
				return count, fmt.Errorf("failed to import operational note '%s': %w", opNote.Title, err)
			}
		}
		return count, nil
	}

	// Fallback to legacy raw connections list
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
