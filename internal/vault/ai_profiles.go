package vault

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// AIProfile represents a configurable AI copilot profile.
type AIProfile struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Provider        string    `json:"provider"` // openai, anthropic, gemini, ollama, custom
	APIKey          string    `json:"api_key,omitempty"` // plain text in transit, encrypted in DB
	EncryptedAPIKey string    `json:"-"`
	ClearAPIKey     bool      `json:"-"`
	HasAPIKey       bool      `json:"has_api_key"`
	BaseURL         string    `json:"base_url"`
	ModelName       string    `json:"model_name"`
	Temperature     float32   `json:"temperature"`
	Language        string    `json:"language"`
	ResponseStyle   string    `json:"response_style"`
	CustomPrompt    string    `json:"custom_prompt"`
	IncludeSchema   bool      `json:"include_schema"`
	MaskIdentifiers bool      `json:"mask_identifiers"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func aiProfileFieldAAD(profileID, field string) []byte {
	return []byte("ai_profile:" + profileID + ":" + field)
}

func seedDefaultAIProfile(db *sql.DB) error {
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM ai_profiles").Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	now := time.Now()
	_, err := db.Exec(`
		INSERT INTO ai_profiles (
			id, name, provider, encrypted_api_key, base_url, model_name,
			temperature, language, response_style, custom_prompt,
			include_schema, mask_identifiers, is_active, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, "default-openai", "OpenAI (GPT-4o Mini)", "openai", "", "https://api.openai.com/v1", "gpt-4o-mini",
		0.3, "auto", "normal", "", 1, 1, 1, now, now)
	return err
}

// SaveAIProfile creates or updates an AI profile.
func (v *Vault) SaveAIProfile(p *AIProfile) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if p.ID == "" {
		p.ID = newUUID()
		p.CreatedAt = time.Now()
	} else if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	p.UpdatedAt = time.Now()

	var encKey string
	if p.ClearAPIKey {
		encKey = ""
	} else if p.APIKey != "" {
		enc, err := EncryptWithAAD([]byte(p.APIKey), v.key, aiProfileFieldAAD(p.ID, "api_key"))
		if err != nil {
			return fmt.Errorf("failed to encrypt AI api key: %w", err)
		}
		encKey = enc
	} else {
		encKey = p.EncryptedAPIKey
		if encKey == "" && p.ID != "" {
			var existingKey string
			if err := v.db.QueryRow(`SELECT encrypted_api_key FROM ai_profiles WHERE id = ?`, p.ID).Scan(&existingKey); err == nil {
				encKey = existingKey
			}
		}
	}
	p.EncryptedAPIKey = encKey
	p.HasAPIKey = encKey != ""

	// Check if this is the only profile or first profile being added; if so, make it active
	var totalProfiles int
	_ = v.db.QueryRow(`SELECT COUNT(*) FROM ai_profiles`).Scan(&totalProfiles)
	if totalProfiles == 0 {
		p.IsActive = true
	}

	// If this profile is marked active, deactivate others
	if p.IsActive {
		_, _ = v.db.Exec(`UPDATE ai_profiles SET is_active = 0 WHERE id != ?`, p.ID)
	}

	incSchemaInt := 0
	if p.IncludeSchema {
		incSchemaInt = 1
	}
	maskIdentInt := 0
	if p.MaskIdentifiers {
		maskIdentInt = 1
	}
	isActiveInt := 0
	if p.IsActive {
		isActiveInt = 1
	}

	query := `
	INSERT INTO ai_profiles (
		id, name, provider, encrypted_api_key, base_url, model_name,
		temperature, language, response_style, custom_prompt,
		include_schema, mask_identifiers, is_active, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		name = excluded.name,
		provider = excluded.provider,
		encrypted_api_key = excluded.encrypted_api_key,
		base_url = excluded.base_url,
		model_name = excluded.model_name,
		temperature = excluded.temperature,
		language = excluded.language,
		response_style = excluded.response_style,
		custom_prompt = excluded.custom_prompt,
		include_schema = excluded.include_schema,
		mask_identifiers = excluded.mask_identifiers,
		is_active = excluded.is_active,
		updated_at = excluded.updated_at
	`
	_, err := v.db.Exec(query,
		p.ID, p.Name, p.Provider, encKey, p.BaseURL, p.ModelName,
		p.Temperature, p.Language, p.ResponseStyle, p.CustomPrompt,
		incSchemaInt, maskIdentInt, isActiveInt, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

// ListAIProfiles returns all AI profiles. For listing, APIKey is stripped and HasAPIKey is populated.
func (v *Vault) ListAIProfiles() ([]*AIProfile, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	rows, err := v.db.Query(`
		SELECT id, name, provider, encrypted_api_key, base_url, model_name,
		       temperature, language, response_style, custom_prompt,
		       include_schema, mask_identifiers, is_active, created_at, updated_at
		FROM ai_profiles
		ORDER BY is_active DESC, updated_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []*AIProfile
	for rows.Next() {
		var p AIProfile
		var encKey string
		var incSchema, maskIdent, isActive int
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Provider, &encKey, &p.BaseURL, &p.ModelName,
			&p.Temperature, &p.Language, &p.ResponseStyle, &p.CustomPrompt,
			&incSchema, &maskIdent, &isActive, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		p.EncryptedAPIKey = encKey
		p.HasAPIKey = encKey != ""
		p.IncludeSchema = incSchema == 1
		p.MaskIdentifiers = maskIdent == 1
		p.IsActive = isActive == 1
		// Do not expose plaintext APIKey in list overview
		p.APIKey = ""
		list = append(list, &p)
	}
	return list, nil
}

// GetAIProfile returns an AI profile by ID, with the API key decrypted.
func (v *Vault) GetAIProfile(id string) (*AIProfile, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	row := v.db.QueryRow(`
		SELECT id, name, provider, encrypted_api_key, base_url, model_name,
		       temperature, language, response_style, custom_prompt,
		       include_schema, mask_identifiers, is_active, created_at, updated_at
		FROM ai_profiles
		WHERE id = ?
	`, id)

	var p AIProfile
	var encKey string
	var incSchema, maskIdent, isActive int
	if err := row.Scan(
		&p.ID, &p.Name, &p.Provider, &encKey, &p.BaseURL, &p.ModelName,
		&p.Temperature, &p.Language, &p.ResponseStyle, &p.CustomPrompt,
		&incSchema, &maskIdent, &isActive, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("ai profile not found: %s", id)
		}
		return nil, err
	}

	p.EncryptedAPIKey = encKey
	p.HasAPIKey = encKey != ""
	p.IncludeSchema = incSchema == 1
	p.MaskIdentifiers = maskIdent == 1
	p.IsActive = isActive == 1

	if encKey != "" {
		dec, err := DecryptWithAAD(encKey, v.key, aiProfileFieldAAD(p.ID, "api_key"))
		if err == nil {
			p.APIKey = string(dec)
		}
	}
	return &p, nil
}

// GetActiveAIProfile returns the currently active AI profile with its decrypted API key.
func (v *Vault) GetActiveAIProfile() (*AIProfile, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	row := v.db.QueryRow(`
		SELECT id, name, provider, encrypted_api_key, base_url, model_name,
		       temperature, language, response_style, custom_prompt,
		       include_schema, mask_identifiers, is_active, created_at, updated_at
		FROM ai_profiles
		WHERE is_active = 1
		LIMIT 1
	`)

	var p AIProfile
	var encKey string
	var incSchema, maskIdent, isActive int
	if err := row.Scan(
		&p.ID, &p.Name, &p.Provider, &encKey, &p.BaseURL, &p.ModelName,
		&p.Temperature, &p.Language, &p.ResponseStyle, &p.CustomPrompt,
		&incSchema, &maskIdent, &isActive, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Fallback: pick any first profile
			rowFirst := v.db.QueryRow(`
				SELECT id, name, provider, encrypted_api_key, base_url, model_name,
				       temperature, language, response_style, custom_prompt,
				       include_schema, mask_identifiers, is_active, created_at, updated_at
				FROM ai_profiles
				ORDER BY updated_at DESC
				LIMIT 1
			`)
			if err2 := rowFirst.Scan(
				&p.ID, &p.Name, &p.Provider, &encKey, &p.BaseURL, &p.ModelName,
				&p.Temperature, &p.Language, &p.ResponseStyle, &p.CustomPrompt,
				&incSchema, &maskIdent, &isActive, &p.CreatedAt, &p.UpdatedAt,
			); err2 != nil {
				return nil, errors.New("no active AI profile found")
			}
		} else {
			return nil, err
		}
	}

	p.EncryptedAPIKey = encKey
	p.HasAPIKey = encKey != ""
	p.IncludeSchema = incSchema == 1
	p.MaskIdentifiers = maskIdent == 1
	p.IsActive = true

	if encKey != "" {
		dec, err := DecryptWithAAD(encKey, v.key, aiProfileFieldAAD(p.ID, "api_key"))
		if err == nil {
			p.APIKey = string(dec)
		}
	}
	return &p, nil
}

// SetActiveAIProfile sets the profile with id as active, deactivating others.
func (v *Vault) SetActiveAIProfile(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	var exists int
	if err := v.db.QueryRow(`SELECT COUNT(*) FROM ai_profiles WHERE id = ?`, id).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("ai profile not found: %s", id)
	}

	tx, err := v.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE ai_profiles SET is_active = 0`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE ai_profiles SET is_active = 1, updated_at = ? WHERE id = ?`, time.Now(), id); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteAIProfile removes an AI profile. It prevents deleting the last remaining profile,
// and if deleting the active profile, automatically reassigns active status to another profile.
func (v *Vault) DeleteAIProfile(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	var total int
	if err := v.db.QueryRow(`SELECT COUNT(*) FROM ai_profiles`).Scan(&total); err != nil {
		return err
	}
	if total <= 1 {
		return errors.New("cannot delete the only remaining AI profile")
	}

	var isActive int
	if err := v.db.QueryRow(`SELECT is_active FROM ai_profiles WHERE id = ?`, id).Scan(&isActive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("ai profile not found: %s", id)
		}
		return err
	}

	tx, err := v.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM ai_profiles WHERE id = ?`, id); err != nil {
		return err
	}

	// If the deleted profile was active, elect a new active profile
	if isActive == 1 {
		var nextID string
		if err := tx.QueryRow(`SELECT id FROM ai_profiles ORDER BY updated_at DESC LIMIT 1`).Scan(&nextID); err == nil {
			if _, err := tx.Exec(`UPDATE ai_profiles SET is_active = 1 WHERE id = ?`, nextID); err != nil {
				return err
			}
		}
	}

	return tx.Commit()
}
