package inbound

import (
	"context"
	"net/http"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type RouterService interface {
	HandleChatCompletion(ctx context.Context, req *http.Request) (*http.Response, error)
	SelectKey(ctx context.Context, modelID string) (*domain.APIKey, error)
	MarkKeyResult(ctx context.Context, keyID string, result KeyResult) error
	ListProviders() []domain.Provider
	ListModels() []domain.Model
	ListModelGroups() []domain.ModelGroup
	ListChatSessions() []domain.ChatSession
	ListKeys() []domain.APIKey
	Logs() []domain.RequestLog
}

type KeyResult struct {
	Success         bool
	ShouldRotateKey bool
	ModelID         string
	SessionID       string
	GroupID         string
	ProviderID      string
	StatusCode      int
	Error           string
	LatencyMs       int64
	CooldownSeconds int
}
