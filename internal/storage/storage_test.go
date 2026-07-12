package storage

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateCreatesSchemaVersionAndColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "modelmux.db")
	store, err := New(path)
	if err != nil {
		t.Fatalf("new storage failed: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close storage failed: %v", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open migrated db failed: %v", err)
	}
	defer db.Close()

	var version int
	if err := db.QueryRow("SELECT version FROM schema_migrations WHERE version = 1").Scan(&version); err != nil {
		t.Fatalf("expected schema migration version 1: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected schema migration version 1, got %d", version)
	}

	for _, tt := range []struct {
		table  string
		column string
	}{
		{table: "keys_runtime", column: "daily_request_limit"},
		{table: "eval_results", column: "fail_reason"},
		{table: "request_logs", column: "estimated_cost"},
	} {
		exists, err := dbColumnExists(db, tt.table, tt.column)
		if err != nil {
			t.Fatalf("check column %s.%s failed: %v", tt.table, tt.column, err)
		}
		if !exists {
			t.Fatalf("expected column %s.%s to exist", tt.table, tt.column)
		}
	}
}

func dbColumnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
