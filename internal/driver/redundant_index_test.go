package driver

import (
	"testing"
)

func TestDetectRedundantIndexes(t *testing.T) {
	tables := []TableMetadata{
		{
			Name: "users",
			Columns: []ColumnMetadata{
				{Name: "id", IsPrimaryKey: true},
				{Name: "email"},
				{Name: "org_id"},
				{Name: "created_at"},
			},
			Indexes: []IndexMetadata{
				{Name: "PRIMARY", Columns: []string{"id"}, IsUnique: true},
				{Name: "idx_id_redundant", Columns: []string{"id"}, IsUnique: false},
				{Name: "idx_org_created", Columns: []string{"org_id", "created_at"}, IsUnique: false},
				{Name: "idx_org_prefix", Columns: []string{"org_id"}, IsUnique: false},
				{Name: "idx_email_dup1", Columns: []string{"email"}, IsUnique: false},
				{Name: "idx_email_dup2", Columns: []string{"email"}, IsUnique: false},
			},
		},
	}

	results := DetectRedundantIndexes("mysql", "testdb", tables)
	if len(results) != 3 {
		t.Fatalf("expected 3 redundant indexes, got %d: %+v", len(results), results)
	}

	foundPK := false
	foundPrefix := false
	foundDup := false

	for _, r := range results {
		if r.RedundantIndex == "idx_id_redundant" && r.ShadowedBy == "PRIMARY" {
			foundPK = true
		}
		if r.RedundantIndex == "idx_org_prefix" && r.ShadowedBy == "idx_org_created" {
			foundPrefix = true
		}
		if (r.RedundantIndex == "idx_email_dup2" || r.RedundantIndex == "idx_email_dup1") && r.Reason != "" {
			foundDup = true
		}
	}

	if !foundPK {
		t.Errorf("failed to detect redundant PK index idx_id_redundant")
	}
	if !foundPrefix {
		t.Errorf("failed to detect shadowed prefix index idx_org_prefix")
	}
	if !foundDup {
		t.Errorf("failed to detect duplicate index idx_email_dup")
	}

	// UNIQUE index must never be the one dropped, regardless of name order.
	uniq := DetectRedundantIndexes("mysql", "db", []TableMetadata{{Name: "t", Indexes: []IndexMetadata{
		{Name: "a_dup", Columns: []string{"email"}},
		{Name: "z_uniq", Columns: []string{"email"}, IsUnique: true},
	}}})
	if len(uniq) != 1 || uniq[0].RedundantIndex != "a_dup" {
		t.Errorf("expected non-unique a_dup to be redundant, got %+v", uniq)
	}
}
