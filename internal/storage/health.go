package storage

import (
	"fmt"
	"sync"
)

type guardedStorage struct {
	Storage

	mu        sync.RWMutex
	healthErr error
}

func newGuardedStorage(inner Storage) Storage {
	return &guardedStorage{Storage: inner}
}

func (s *guardedStorage) mark(err error) error {
	if err == nil {
		return nil
	}
	s.mu.Lock()
	if s.healthErr == nil {
		s.healthErr = err
	}
	s.mu.Unlock()
	return err
}

func (s *guardedStorage) HealthError() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.healthErr == nil {
		return nil
	}
	return fmt.Errorf("persistent storage is unhealthy: %w", s.healthErr)
}

func HealthError(store Storage) error {
	if store == nil {
		return nil
	}
	if reporter, ok := store.(interface{ HealthError() error }); ok {
		return reporter.HealthError()
	}
	return nil
}

func (s *guardedStorage) SaveKeyRuntime(record KeyRuntimeRecord) error {
	return s.mark(s.Storage.SaveKeyRuntime(record))
}

func (s *guardedStorage) LoadKeyRuntime() ([]KeyRuntimeRecord, error) {
	records, err := s.Storage.LoadKeyRuntime()
	return records, s.mark(err)
}

func (s *guardedStorage) SaveRequestLog(record RequestLogRecord) error {
	return s.mark(s.Storage.SaveRequestLog(record))
}

func (s *guardedStorage) LoadRequestLogs() ([]RequestLogRecord, error) {
	records, err := s.Storage.LoadRequestLogs()
	return records, s.mark(err)
}

func (s *guardedStorage) QueryRequestLogs(filter LogFilter) ([]RequestLogRecord, int, error) {
	records, total, err := s.Storage.QueryRequestLogs(filter)
	return records, total, s.mark(err)
}

func (s *guardedStorage) SaveRouteTrace(record RouteTraceRecord) error {
	return s.mark(s.Storage.SaveRouteTrace(record))
}

func (s *guardedStorage) GetRouteTraceByRequestID(requestID string) (*RouteTraceRecord, error) {
	record, err := s.Storage.GetRouteTraceByRequestID(requestID)
	return record, s.mark(err)
}

func (s *guardedStorage) SaveChatSession(name, target string) (int, error) {
	id, err := s.Storage.SaveChatSession(name, target)
	return id, s.mark(err)
}

func (s *guardedStorage) SaveChatMessage(sessionID int, role, content string) error {
	return s.mark(s.Storage.SaveChatMessage(sessionID, role, content))
}

func (s *guardedStorage) ListChatSessions() ([]ChatSessionRecord, error) {
	records, err := s.Storage.ListChatSessions()
	return records, s.mark(err)
}

func (s *guardedStorage) GetChatMessages(sessionID int) ([]ChatMessageRecord, error) {
	records, err := s.Storage.GetChatMessages(sessionID)
	return records, s.mark(err)
}

func (s *guardedStorage) SaveEvalRun(record EvalRunRecord) error {
	return s.mark(s.Storage.SaveEvalRun(record))
}

func (s *guardedStorage) SaveEvalResult(record EvalResultRecord) error {
	return s.mark(s.Storage.SaveEvalResult(record))
}

func (s *guardedStorage) ListEvalRuns() ([]EvalRunRecord, error) {
	records, err := s.Storage.ListEvalRuns()
	return records, s.mark(err)
}

func (s *guardedStorage) GetEvalResults(runID string) ([]EvalResultRecord, error) {
	records, err := s.Storage.GetEvalResults(runID)
	return records, s.mark(err)
}

func (s *guardedStorage) PruneBefore(before string) (int, error) {
	count, err := s.Storage.PruneBefore(before)
	return count, s.mark(err)
}

func (s *guardedStorage) Stats() (map[string]int, error) {
	stats, err := s.Storage.Stats()
	return stats, s.mark(err)
}

func (s *guardedStorage) Vacuum() error {
	return s.mark(s.Storage.Vacuum())
}
