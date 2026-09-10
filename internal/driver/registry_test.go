package driver_test

import (
	"testing"

	"sqlfox/internal/driver"
	_ "sqlfox/internal/driver/mysql"
	_ "sqlfox/internal/driver/sqlite"
)

func TestRegistry(t *testing.T) {
	d, err := driver.GetDriver("sqlite")
	if err != nil {
		t.Fatalf("expected sqlite driver to be registered, got: %v", err)
	}
	if d.Name() != "sqlite" {
		t.Fatalf("expected sqlite name, got: %s", d.Name())
	}

	dMySQL, err := driver.GetDriver("mysql")
	if err != nil {
		t.Fatalf("expected mysql driver to be registered, got: %v", err)
	}
	if dMySQL.Name() != "mysql" {
		t.Fatalf("expected mysql name, got: %s", dMySQL.Name())
	}

	_, err = driver.GetDriver("unregistered-db")
	if err == nil {
		t.Fatal("expected error for unregistered driver, got nil")
	}
}
