package domain

import "time"

type RequestLog struct {
	ID          string
	RequestID   string
	GroupID     string
	ModelID     string
	ProviderID  string
	KeyID       string
	StatusCode  int
	Error       string
	LatencyMs   int64
	TokenInput  int
	TokenOutput int
	CreatedAt   time.Time
}
