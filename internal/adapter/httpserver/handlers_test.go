package httpserver

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
)

func TestMetricsDoesNotExposeAPIKeyValue(t *testing.T) {
	cfg := config.Default()
	cfg.Keys[0].Value = "provider-secret"
	rs := service.NewRouterService(cfg)
	srv := New(rs, cfg)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	srv.metricsHandler(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "provider-secret") {
		t.Fatalf("metrics leaked API key: %s", body)
	}
	if !strings.Contains(body, "active_keys") {
		t.Fatalf("metrics missing model summary: %s", body)
	}
	if !strings.Contains(body, "groups") || !strings.Contains(body, "active_models") {
		t.Fatalf("metrics missing group summary: %s", body)
	}
}

func TestModelsIncludesModelGroups(t *testing.T) {
	cfg := config.Default()
	rs := service.NewRouterService(cfg)
	srv := New(rs, cfg)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	srv.modelsHandler(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "high-price") {
		t.Fatalf("models endpoint missing group id: %s", body)
	}
}
