package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type KeyRuntimeRecord struct {
	KeyID             string
	Status            string
	UsedCount         int
	ErrorCount        int
	CooldownEnd       string
	LastUsedAt        string
	UpdatedAt         string
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

type LogFilter struct {
	ModelID    string
	KeyID      string
	ProviderID string
	GroupID    string
	StatusCode int
	Limit      int
	Offset     int
}

type RouteTraceRecord struct {
	ID            string
	RequestID     string
	OriginalModel string
	ReroutedModel string
	StepsJSON     string
	CreatedAt     string
}

type ChatSessionRecord struct {
	ID        int
	Name      string
	Target    string
	CreatedAt string
}

type ChatMessageRecord struct {
	ID        int
	SessionID int
	Role      string
	Content   string
	CreatedAt string
}

type EvalRunRecord struct {
	ID         string
	SuiteName  string
	StartedAt  string
	FinishedAt string
	TotalCases int
}

type EvalResultRecord struct {
	ID           int
	RunID        string
	CaseName     string
	TargetModel  string
	TargetGroup  string
	StatusCode   int
	LatencyMs    int64
	ResponseHash string
	Error        string
}

type Storage interface {
	SaveKeyRuntime(record KeyRuntimeRecord) error
	LoadKeyRuntime() ([]KeyRuntimeRecord, error)
	SaveRequestLog(record RequestLogRecord) error
	LoadRequestLogs() ([]RequestLogRecord, error)
	QueryRequestLogs(filter LogFilter) ([]RequestLogRecord, int, error)
	SaveRouteTrace(record RouteTraceRecord) error
	GetRouteTraceByRequestID(requestID string) (*RouteTraceRecord, error)
	SaveChatSession(name, target string) (int, error)
	SaveChatMessage(sessionID int, role, content string) error
	ListChatSessions() ([]ChatSessionRecord, error)
	GetChatMessages(sessionID int) ([]ChatMessageRecord, error)
	SaveEvalRun(record EvalRunRecord) error
	SaveEvalResult(record EvalResultRecord) error
	ListEvalRuns() ([]EvalRunRecord, error)
	GetEvalResults(runID string) ([]EvalResultRecord, error)
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

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS route_traces (
			id TEXT PRIMARY KEY,
			request_id TEXT NOT NULL,
			original_model TEXT NOT NULL DEFAULT '',
			rerouted_model TEXT NOT NULL DEFAULT '',
			steps_json TEXT NOT NULL DEFAULT '[]',
			created_at TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec("CREATE INDEX IF NOT EXISTS idx_route_traces_request_id ON route_traces(request_id)")
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS chat_sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			target TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS chat_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT ''
		)
	`)
		if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS eval_runs (
			id TEXT PRIMARY KEY,
			suite_name TEXT NOT NULL,
			started_at TEXT NOT NULL DEFAULT '',
			finished_at TEXT NOT NULL DEFAULT '',
			total_cases INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return err
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS eval_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id TEXT NOT NULL,
			case_name TEXT NOT NULL DEFAULT '',
			target_model TEXT NOT NULL DEFAULT '',
			target_group TEXT NOT NULL DEFAULT '',
			status_code INTEGER NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			response_hash TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT ''
		)
	`)
	if err != nil {
		return err
	}

	alterColumns(db,
		"ALTER TABLE keys_runtime ADD COLUMN last_used_at TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE keys_runtime ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE keys_runtime ADD COLUMN daily_request_count INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE keys_runtime ADD COLUMN daily_token_count INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE keys_runtime ADD COLUMN daily_date TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE keys_runtime ADD COLUMN daily_request_limit INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE keys_runtime ADD COLUMN daily_token_limit INTEGER NOT NULL DEFAULT 0",
	)

	return nil
}

func alterColumns(db *sql.DB, stmts ...string) {
	for _, stmt := range stmts {
		_, err := db.Exec(stmt)
		if err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			fmt.Fprintf(os.Stderr, "storage: migration warning: %v\n", err)
		}
	}
}

func (s *sqliteStore) SaveKeyRuntime(record KeyRuntimeRecord) error {
	_, err := s.db.Exec(`
		INSERT INTO keys_runtime (key_id, status, used_count, error_count, cooldown_end, last_used_at, updated_at, token_input, token_output, daily_request_count, daily_token_count, daily_date, daily_request_limit, daily_token_limit)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key_id) DO UPDATE SET
			status = excluded.status,
			used_count = excluded.used_count,
			error_count = excluded.error_count,
			cooldown_end = excluded.cooldown_end,
			last_used_at = excluded.last_used_at,
			updated_at = excluded.updated_at,
			token_input = excluded.token_input,
			token_output = excluded.token_output,
			daily_request_count = excluded.daily_request_count,
			daily_token_count = excluded.daily_token_count,
			daily_date = excluded.daily_date,
			daily_request_limit = excluded.daily_request_limit,
			daily_token_limit = excluded.daily_token_limit
	`, record.KeyID, record.Status, record.UsedCount, record.ErrorCount, record.CooldownEnd, record.LastUsedAt, record.UpdatedAt, record.TokenInput, record.TokenOutput, record.DailyRequestCount, record.DailyTokenCount, record.DailyDate, record.DailyRequestLimit, record.DailyTokenLimit)
	return err
}

func (s *sqliteStore) LoadKeyRuntime() ([]KeyRuntimeRecord, error) {
	rows, err := s.db.Query("SELECT key_id, status, used_count, error_count, cooldown_end, COALESCE(last_used_at,''), COALESCE(updated_at,''), token_input, token_output, COALESCE(daily_request_count,0), COALESCE(daily_token_count,0), COALESCE(daily_date,''), COALESCE(daily_request_limit,0), COALESCE(daily_token_limit,0) FROM keys_runtime")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []KeyRuntimeRecord
	for rows.Next() {
		var r KeyRuntimeRecord
		if err := rows.Scan(&r.KeyID, &r.Status, &r.UsedCount, &r.ErrorCount, &r.CooldownEnd, &r.LastUsedAt, &r.UpdatedAt, &r.TokenInput, &r.TokenOutput, &r.DailyRequestCount, &r.DailyTokenCount, &r.DailyDate, &r.DailyRequestLimit, &r.DailyTokenLimit); err != nil {
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

func (s *sqliteStore) QueryRequestLogs(filter LogFilter) ([]RequestLogRecord, int, error) {
	where := "WHERE 1=1"
	args := []any{}
	if filter.ModelID != "" {
		where += " AND model_id = ?"
		args = append(args, filter.ModelID)
	}
	if filter.KeyID != "" {
		where += " AND key_id = ?"
		args = append(args, filter.KeyID)
	}
	if filter.ProviderID != "" {
		where += " AND provider_id = ?"
		args = append(args, filter.ProviderID)
	}
	if filter.GroupID != "" {
		where += " AND group_id = ?"
		args = append(args, filter.GroupID)
	}
	if filter.StatusCode > 0 {
		where += " AND status_code = ?"
		args = append(args, filter.StatusCode)
	}

	var total int
	countQuery := "SELECT COUNT(*) FROM request_logs " + where
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > 50000 {
		limit = 50000
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	queryArgs := append(args, limit, offset)
	query := "SELECT id, group_id, model_id, provider_id, key_id, status_code, error, latency_ms, token_input, token_output, created_at FROM request_logs " + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"

	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []RequestLogRecord
	for rows.Next() {
		var r RequestLogRecord
		if err := rows.Scan(&r.ID, &r.GroupID, &r.ModelID, &r.ProviderID, &r.KeyID, &r.StatusCode, &r.Error, &r.LatencyMs, &r.TokenInput, &r.TokenOutput, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		records = append(records, r)
	}
	return records, total, rows.Err()
}

func (s *sqliteStore) SaveRouteTrace(record RouteTraceRecord) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO route_traces (id, request_id, original_model, rerouted_model, steps_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, record.ID, record.RequestID, record.OriginalModel, record.ReroutedModel, record.StepsJSON, record.CreatedAt)
	return err
}

func (s *sqliteStore) GetRouteTraceByRequestID(requestID string) (*RouteTraceRecord, error) {
	row := s.db.QueryRow("SELECT id, request_id, original_model, rerouted_model, steps_json, created_at FROM route_traces WHERE request_id = ? ORDER BY created_at DESC LIMIT 1", requestID)
	var r RouteTraceRecord
	err := row.Scan(&r.ID, &r.RequestID, &r.OriginalModel, &r.ReroutedModel, &r.StepsJSON, &r.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &r, nil
}

func (s *sqliteStore) SaveChatSession(name, target string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := s.db.Exec("INSERT INTO chat_sessions (name, target, created_at) VALUES (?, ?, ?)", name, target, now)
	if err != nil {
		return 0, err
	}
	id, err := result.LastInsertId()
	return int(id), err
}

func (s *sqliteStore) SaveChatMessage(sessionID int, role, content string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec("INSERT INTO chat_messages (session_id, role, content, created_at) VALUES (?, ?, ?, ?)", sessionID, role, content, now)
	return err
}

func (s *sqliteStore) ListChatSessions() ([]ChatSessionRecord, error) {
	rows, err := s.db.Query("SELECT id, name, target, created_at FROM chat_sessions ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []ChatSessionRecord
	for rows.Next() {
		var r ChatSessionRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.Target, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *sqliteStore) GetChatMessages(sessionID int) ([]ChatMessageRecord, error) {
	rows, err := s.db.Query("SELECT id, session_id, role, content, created_at FROM chat_messages WHERE session_id = ? ORDER BY id ASC", sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []ChatMessageRecord
	for rows.Next() {
		var r ChatMessageRecord
		if err := rows.Scan(&r.ID, &r.SessionID, &r.Role, &r.Content, &r.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
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

func (s *sqliteStore) SaveEvalRun(record EvalRunRecord) error {
	_, err := s.db.Exec("INSERT INTO eval_runs (id, suite_name, started_at, finished_at, total_cases) VALUES (?, ?, ?, ?, ?)",
		record.ID, record.SuiteName, record.StartedAt, record.FinishedAt, record.TotalCases)
	return err
}

func (s *sqliteStore) SaveEvalResult(record EvalResultRecord) error {
	_, err := s.db.Exec("INSERT INTO eval_results (run_id, case_name, target_model, target_group, status_code, latency_ms, response_hash, error) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		record.RunID, record.CaseName, record.TargetModel, record.TargetGroup, record.StatusCode, record.LatencyMs, record.ResponseHash, record.Error)
	return err
}

func (s *sqliteStore) ListEvalRuns() ([]EvalRunRecord, error) {
	rows, err := s.db.Query("SELECT id, suite_name, started_at, finished_at, total_cases FROM eval_runs ORDER BY id DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []EvalRunRecord
	for rows.Next() {
		var r EvalRunRecord
		if err := rows.Scan(&r.ID, &r.SuiteName, &r.StartedAt, &r.FinishedAt, &r.TotalCases); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

func (s *sqliteStore) GetEvalResults(runID string) ([]EvalResultRecord, error) {
	rows, err := s.db.Query("SELECT id, run_id, case_name, target_model, target_group, status_code, latency_ms, response_hash, error FROM eval_results WHERE run_id = ? ORDER BY id ASC", runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []EvalResultRecord
	for rows.Next() {
		var r EvalResultRecord
		if err := rows.Scan(&r.ID, &r.RunID, &r.CaseName, &r.TargetModel, &r.TargetGroup, &r.StatusCode, &r.LatencyMs, &r.ResponseHash, &r.Error); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
