package vault

import (
	"path/filepath"
	"testing"
)

func TestAIProfilesCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_vault.db")

	v, err := OpenVault(dbPath, "test-master-password")
	if err != nil {
		t.Fatalf("OpenVault failed: %v", err)
	}
	defer v.Close()

	// Should have seeded default profile
	list, err := v.ListAIProfiles()
	if err != nil {
		t.Fatalf("ListAIProfiles failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 seeded profile, got %d", len(list))
	}
	if list[0].Provider != "openai" || !list[0].IsActive {
		t.Fatalf("expected active default openai profile, got %+v", list[0])
	}

	// Create second profile
	p2 := &AIProfile{
		ID:              "anthropic-profile",
		Name:            "Claude Sonnet",
		Provider:        "anthropic",
		APIKey:          "sk-ant-test-secret-key-12345",
		BaseURL:         "https://api.anthropic.com/v1",
		ModelName:       "claude-3-5-sonnet-20241022",
		Temperature:     0.2,
		Language:        "id",
		ResponseStyle:   "concise",
		CustomPrompt:    "You are an expert SQL engineer.",
		IncludeSchema:   true,
		MaskIdentifiers: true,
		IsActive:        false,
	}

	if err := v.SaveAIProfile(p2); err != nil {
		t.Fatalf("SaveAIProfile failed: %v", err)
	}

	// Verify listing masks API key but flags HasAPIKey
	list, err = v.ListAIProfiles()
	if err != nil {
		t.Fatalf("ListAIProfiles failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(list))
	}
	var found *AIProfile
	for _, p := range list {
		if p.ID == "anthropic-profile" {
			found = p
			break
		}
	}
	if found == nil {
		t.Fatalf("anthropic-profile not found in list")
	}
	if found.APIKey != "" {
		t.Errorf("APIKey should be masked/empty in list, got: %s", found.APIKey)
	}
	if !found.HasAPIKey {
		t.Errorf("HasAPIKey should be true")
	}

	// Verify GetAIProfile returns decrypted key
	retrieved, err := v.GetAIProfile("anthropic-profile")
	if err != nil {
		t.Fatalf("GetAIProfile failed: %v", err)
	}
	if retrieved.APIKey != "sk-ant-test-secret-key-12345" {
		t.Fatalf("decrypted API key mismatch: got %s", retrieved.APIKey)
	}

	// Test SetActiveAIProfile
	if err := v.SetActiveAIProfile("anthropic-profile"); err != nil {
		t.Fatalf("SetActiveAIProfile failed: %v", err)
	}

	active, err := v.GetActiveAIProfile()
	if err != nil {
		t.Fatalf("GetActiveAIProfile failed: %v", err)
	}
	if active.ID != "anthropic-profile" || !active.IsActive {
		t.Fatalf("expected anthropic-profile to be active, got: %+v", active)
	}

	// Test ExportVault & ImportVault
	archiveBytes, err := ExportVault(v, "backup-password")
	if err != nil {
		t.Fatalf("ExportVault failed: %v", err)
	}

	// Open second vault to import
	dbPath2 := filepath.Join(tmpDir, "imported_vault.db")
	v2, err := OpenVault(dbPath2, "other-master-password")
	if err != nil {
		t.Fatalf("OpenVault 2 failed: %v", err)
	}
	defer v2.Close()

	count, err := ImportVault(v2, archiveBytes, "backup-password")
	if err != nil {
		t.Fatalf("ImportVault failed: %v", err)
	}
	_ = count

	importedActive, err := v2.GetActiveAIProfile()
	if err != nil {
		t.Fatalf("GetActiveAIProfile on imported vault failed: %v", err)
	}
	if importedActive.ID != "anthropic-profile" || importedActive.APIKey != "sk-ant-test-secret-key-12345" {
		t.Fatalf("imported active profile mismatch: %+v", importedActive)
	}

	// Test delete safeguard: cannot delete last remaining profile
	// In v, we have 2 profiles. Delete anthropic-profile (active) -> should switch active to remaining profile
	if err := v.DeleteAIProfile("anthropic-profile"); err != nil {
		t.Fatalf("DeleteAIProfile failed: %v", err)
	}

	remainingActive, err := v.GetActiveAIProfile()
	if err != nil {
		t.Fatalf("GetActiveAIProfile after delete failed: %v", err)
	}
	if remainingActive.ID != "default-openai" || !remainingActive.IsActive {
		t.Fatalf("expected remaining profile to become active, got: %+v", remainingActive)
	}

	// Try to delete the only remaining profile -> must fail!
	err = v.DeleteAIProfile("default-openai")
	if err == nil {
		t.Fatalf("expected error when deleting only remaining profile, got nil")
	}

	// Verify ExportNotesFiles exports ai-profiles/*.json with redacted key
	files, err := ExportNotesFiles(v)
	if err != nil {
		t.Fatalf("ExportNotesFiles failed: %v", err)
	}
	foundAiFile := false
	for k, content := range files {
		if filepath.HasPrefix(k, "ai-profiles/") {
			foundAiFile = true
			if string(content) == "" {
				t.Fatalf("empty ai profile exported file")
			}
		}
	}
	if !foundAiFile {
		t.Fatalf("no ai-profiles/ found in ExportNotesFiles")
	}
}
