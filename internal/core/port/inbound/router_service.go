package inbound

import (
	"context"
	"net/http"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

type RouterService interface {
	HandleChatCompletion(ctx context.Context, req *http.Request) (*http.Response, error)
	HandleCompletion(ctx context.Context, req *http.Request) (*http.Response, error)
	SelectKey(ctx context.Context, modelID string) (*domain.APIKey, error)
	MarkKeyResult(ctx context.Context, keyID string, result KeyResult) error
	TestKey(ctx context.Context, keyID string) error
	ListRouteTraces() []domain.RouteTrace
	ListProviders() []domain.Provider
	ListModels() []domain.Model
	ListModelGroups() []domain.ModelGroup
	ListKeys() []domain.APIKey
	Logs() []domain.RequestLog
}

type KeyResult struct {
	Success         bool
	ShouldRotateKey bool
	ModelID         string
	GroupID         string
	ProviderID      string
	StatusCode      int
	Error           string
	LatencyMs       int64
	CooldownSeconds int
	TokenInput      int
	TokenOutput     int
	EstimatedCost   float64
}
