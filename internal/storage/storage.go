package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type KeyRuntimeRecord struct {
	KeyID        string
	Status       string
	UsedCount    int
	ErrorCount   int
	CooldownEnd  string
	TokenInput   int
	TokenOutput  int
}

type RequestLogRecord struct {
	ID          string
	GroupID     string
	ModelID     string
	ProviderID  string
	KeyID       string
	StatusCode  int
	Error       string
	LatencyMs   int64
	TokenInput  int
	TokenOutput int
	CreatedAt   string
}

type Storage interface {
	SaveKeyRuntime(record KeyRuntimeRecord) error
	LoadKeyRuntime() ([]KeyRuntimeRecord, error)
	SaveRequestLog(record RequestLogRecord) error
	LoadRequestLogs() ([]RequestLogRecord, error)
	Close() error
}

func New(path string) (Storage, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("storage: create directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("storage: open database: %w", err)
	}

	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: enable WAL: %w", err)
	}

	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: enable foreign keys: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("storage: migrate: %w", err)
	}

	return &sqliteStore{db: db}, nil
}

type sqliteStore struct {
	db *sql.DB
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS keys_runtime (
			key_id TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'active',
			used_count INTEGER NOT NULL DEFAULT 0,
			error_count INTEGER NOT NULL DEFAULT 0,
			cooldown_end TEXT NOT NULL DEFAULT '',
			token_input INTEGER NOT NULL DEFAULT 0,
			token_output INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return err
	}

	return nil
}

func (s *sqliteStore) SaveKeyRuntime(record KeyRuntimeRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO keys_runtime (key_id, status, used_count, error_count, cooldown_end, token_input, token_output)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key_id) DO UPDATE SET
			status = excluded.status,
			used_count = excluded.used_count,
			error_count = excluded.error_count,
			cooldown_end = excluded.cooldown_end,
			token_input = excluded.token_input,
			token_output = excluded.token_output
	`, record.KeyID, record.Status, record.UsedCount, record.ErrorCount, record.CooldownEnd, record.TokenInput, record.TokenOutput)
	return err
}

func (s *sqliteStore) LoadKeyRuntime() ([]KeyRuntimeRecord, error) {
	rows, err := s.db.Query("SELECT key_id, status, used_count, error_count, cooldown_end, token_input, token_output FROM keys_runtime")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []KeyRuntimeRecord
	for rows.Next() {
		var r KeyRuntimeRecord
		if err := rows.Scan(&r.KeyID, &r.Status, &r.UsedCount, &r.ErrorCount, &r.CooldownEnd, &r.TokenInput, &r.TokenOutput); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *sqliteStore) SaveRequestLog(record RequestLogRecord) error {
	return nil
}

func (s *sqliteStore) LoadRequestLogs() ([]RequestLogRecord, error) {
	return nil, nil
}
