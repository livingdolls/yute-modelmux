package httpserver

import (
	"io"
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

func TestChatReturns400ForInvalidJSON(t *testing.T) {
	cfg := config.Default()
	rs := service.NewRouterService(cfg)
	srv := New(rs, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`not json`))
	rec := httptest.NewRecorder()
	srv.chatCompletionsHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid JSON, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatReturns404ForUnknownModel(t *testing.T) {
	cfg := config.Default()
	rs := service.NewRouterService(cfg)
	srv := New(rs, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"not-exists","messages":[]}`))
	rec := httptest.NewRecorder()
	srv.chatCompletionsHandler(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown model, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatReturns403ForDisabledModel(t *testing.T) {
	cfg := config.Default()
	cfg.Models[0].Enabled = false
	rs := service.NewRouterService(cfg)
	srv := New(rs, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"mimo-v2.5-pro","messages":[]}`))
	rec := httptest.NewRecorder()
	srv.chatCompletionsHandler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for disabled model, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestChatReturnsErrorForLargeRequestBody(t *testing.T) {
	cfg := config.Default()
	cfg.Server.MaxRequestBodyMB = 1
	rs := service.NewRouterService(cfg)
	srv := New(rs, cfg)

	largeBody := strings.Repeat("x", 2*1024*1024)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(largeBody))
	rec := httptest.NewRecorder()
	srv.chatCompletionsHandler(rec, req)

	if rec.Code < 400 {
		t.Fatalf("expected error for oversized body, got %d", rec.Code)
	}
}

func TestCopyWithFlushForwardsChunks(t *testing.T) {
	body := "data: hello\n\ndata: world\n\n"
	rec := httptest.NewRecorder()
	err := copyWithFlush(rec, io.NopCloser(strings.NewReader(body)))
	if err != nil {
		t.Fatalf("copyWithFlush failed: %v", err)
	}
	if rec.Body.String() != body {
		t.Fatalf("expected body %q, got %q", body, rec.Body.String())
	}
}

func TestCopyWithFlushHandlesEmptyStream(t *testing.T) {
	rec := httptest.NewRecorder()
	err := copyWithFlush(rec, io.NopCloser(strings.NewReader("")))
	if err != nil {
		t.Fatalf("copyWithFlush failed: %v", err)
	}
	if rec.Body.String() != "" {
		t.Fatalf("expected empty body, got %q", rec.Body.String())
	}
}

func TestCompletionsReturns400ForInvalidJSON(t *testing.T) {
	cfg := config.Default()
	rs := service.NewRouterService(cfg)
	srv := New(rs, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(`not json`))
	rec := httptest.NewRecorder()
	srv.completionsHandler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCompletionsRoutesToCorrectEndpoint(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = server.URL + "/v1"
	cfg.Models[0].ModelName = cfg.Models[0].ID
	rs := service.NewRouterService(cfg)
	srv := New(rs, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(`{"model":"mimo-v2.5-pro","prompt":"hello"}`))
	rec := httptest.NewRecorder()
	srv.completionsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if gotPath != "/v1/completions" {
		t.Fatalf("expected upstream path /v1/completions, got %s", gotPath)
	}
}
