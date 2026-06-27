package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type KeyRuntimeRecord struct {
	KeyID             string
	Status            string
	UsedCount         int
	ErrorCount        int
	CooldownEnd       string
	TokenInput        int
	TokenOutput       int
	DailyRequestCount int
	DailyTokenCount   int
	DailyDate         string
	DailyRequestLimit int
	DailyTokenLimit   int
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

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS request_logs (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL DEFAULT '',
			model_id TEXT NOT NULL DEFAULT '',
			provider_id TEXT NOT NULL DEFAULT '',
			key_id TEXT NOT NULL DEFAULT '',
			status_code INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			latency_ms INTEGER NOT NULL DEFAULT 0,
			token_input INTEGER NOT NULL DEFAULT 0,
			token_output INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec("CREATE INDEX IF NOT EXISTS idx_request_logs_created_at ON request_logs(created_at)")
	if err != nil {
		return err
	}

	_, err = db.Exec("CREATE INDEX IF NOT EXISTS idx_request_logs_model_id ON request_logs(model_id)")
	if err != nil {
		return err
	}

	db.Exec("ALTER TABLE keys_runtime ADD COLUMN daily_request_count INTEGER NOT NULL DEFAULT 0")
	db.Exec("ALTER TABLE keys_runtime ADD COLUMN daily_token_count INTEGER NOT NULL DEFAULT 0")
	db.Exec("ALTER TABLE keys_runtime ADD COLUMN daily_date TEXT NOT NULL DEFAULT ''")
	db.Exec("ALTER TABLE keys_runtime ADD COLUMN daily_request_limit INTEGER NOT NULL DEFAULT 0")
	db.Exec("ALTER TABLE keys_runtime ADD COLUMN daily_token_limit INTEGER NOT NULL DEFAULT 0")

	return nil
}

func (s *sqliteStore) SaveKeyRuntime(record KeyRuntimeRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO keys_runtime (key_id, status, used_count, error_count, cooldown_end, token_input, token_output, daily_request_count, daily_token_count, daily_date, daily_request_limit, daily_token_limit)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key_id) DO UPDATE SET
			status = excluded.status,
			used_count = excluded.used_count,
			error_count = excluded.error_count,
			cooldown_end = excluded.cooldown_end,
			token_input = excluded.token_input,
			token_output = excluded.token_output,
			daily_request_count = excluded.daily_request_count,
			daily_token_count = excluded.daily_token_count,
			daily_date = excluded.daily_date,
			daily_request_limit = excluded.daily_request_limit,
			daily_token_limit = excluded.daily_token_limit
	`, record.KeyID, record.Status, record.UsedCount, record.ErrorCount, record.CooldownEnd, record.TokenInput, record.TokenOutput, record.DailyRequestCount, record.DailyTokenCount, record.DailyDate, record.DailyRequestLimit, record.DailyTokenLimit)
	return err
}

func (s *sqliteStore) LoadKeyRuntime() ([]KeyRuntimeRecord, error) {
	rows, err := s.db.Query("SELECT key_id, status, used_count, error_count, cooldown_end, token_input, token_output, COALESCE(daily_request_count,0), COALESCE(daily_token_count,0), COALESCE(daily_date,''), COALESCE(daily_request_limit,0), COALESCE(daily_token_limit,0) FROM keys_runtime")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []KeyRuntimeRecord
	for rows.Next() {
		var r KeyRuntimeRecord
		if err := rows.Scan(&r.KeyID, &r.Status, &r.UsedCount, &r.ErrorCount, &r.CooldownEnd, &r.TokenInput, &r.TokenOutput, &r.DailyRequestCount, &r.DailyTokenCount, &r.DailyDate, &r.DailyRequestLimit, &r.DailyTokenLimit); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *sqliteStore) SaveRequestLog(record RequestLogRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO request_logs (id, group_id, model_id, provider_id, key_id, status_code, error, latency_ms, token_input, token_output, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, record.ID, record.GroupID, record.ModelID, record.ProviderID, record.KeyID, record.StatusCode, record.Error, record.LatencyMs, record.TokenInput, record.TokenOutput, record.CreatedAt)
	return err
}

func (s *sqliteStore) LoadRequestLogs() ([]RequestLogRecord, error) {
	rows, err := s.db.Query("SELECT id, group_id, model_id, provider_id, key_id, status_code, error, latency_ms, token_input, token_output, created_at FROM request_logs ORDER BY created_at DESC LIMIT 200")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []RequestLogRecord
	for rows.Next() {
		var r RequestLogRecord
		if err := rows.Scan(&r.ID, &r.GroupID, &r.ModelID, &r.ProviderID, &r.KeyID, &r.StatusCode, &r.Error, &r.LatencyMs, &r.TokenInput, &r.TokenOutput, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
