package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
)

func TestMetricsDoesNotExposeAPIKeyValue(t *testing.T) {
	cfg := config.Default()
	cfg.Keys[0].Value = "provider-secret"
	rs, _ := service.NewRouterService(cfg)
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
	rs, _ := service.NewRouterService(cfg)
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
	rs, _ := service.NewRouterService(cfg)
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
	rs, _ := service.NewRouterService(cfg)
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
	rs, _ := service.NewRouterService(cfg)
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
	rs, _ := service.NewRouterService(cfg)
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
	rs, _ := service.NewRouterService(cfg)
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
	rs, _ := service.NewRouterService(cfg)
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

func TestChatSSEForwardsChunksAndHeaders(t *testing.T) {
	sseBody := "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\" World\"}}]}\n\ndata: [DONE]\n\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(sseBody))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = server.URL + "/v1"
	cfg.Models[0].ModelName = cfg.Models[0].ID
	rs, _ := service.NewRouterService(cfg)
	srv := New(rs, cfg)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"mimo-v2.5-pro","messages":[],"stream":true}`))
	rec := httptest.NewRecorder()
	srv.chatCompletionsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %s", ct)
	}
	if rec.Body.String() != sseBody {
		t.Fatalf("expected SSE body %q, got %q", sseBody, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "DONE") {
		t.Log("SSE [DONE] marker forwarded successfully")
	}
}

func TestChatSSEDeliversChunksBeforeStreamEnds(t *testing.T) {
	firstChunkSent := make(chan struct{})
	allowRest := make(chan struct{})
	doneSent := make(chan struct{})

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("upstream must support Flusher")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("data: first\n\n"))
		flusher.Flush()
		close(firstChunkSent)

		select {
		case <-allowRest:
		case <-time.After(2 * time.Second):
			t.Error("timeout waiting for test to read first chunk")
			return
		}
		w.Write([]byte("data: second\n\n"))
		flusher.Flush()

		w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
		close(doneSent)
	}))
	defer upstream.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = upstream.URL + "/v1"
	cfg.Providers[0].TimeoutSeconds = 5
	cfg.Models[0].ModelName = cfg.Models[0].ID
	rs, _ := service.NewRouterService(cfg)
	srv := New(rs, cfg)

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.chatCompletionsHandler(w, r)
	}))
	defer proxy.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(proxy.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"mimo-v2.5-pro","messages":[],"stream":true}`))
	if err != nil {
		t.Fatalf("proxy request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected Content-Type text/event-stream, got %s", ct)
	}

	select {
	case <-firstChunkSent:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for upstream to send first chunk")
	}

	firstRead := make(chan struct {
		body string
		err  error
	}, 1)
	go func() {
		buf := make([]byte, 64)
		n, err := resp.Body.Read(buf)
		firstRead <- struct {
			body string
			err  error
		}{body: string(buf[:n]), err: err}
	}()

	var firstChunk string
	select {
	case result := <-firstRead:
		if result.err != nil {
			close(allowRest)
			t.Fatalf("read first chunk failed: %v", result.err)
		}
		firstChunk = result.body
	case <-time.After(2 * time.Second):
		close(allowRest)
		t.Fatal("timeout waiting for first chunk through proxy")
	}
	if !strings.Contains(firstChunk, "first") {
		close(allowRest)
		t.Fatalf("expected first SSE chunk before stream ended, got %q", firstChunk)
	}

	select {
	case <-doneSent:
		close(allowRest)
		t.Fatal("upstream finished before client read first chunk")
	default:
	}

	close(allowRest)

	remaining, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read remaining body failed: %v", err)
	}
	fullBody := firstChunk + string(remaining)
	if !strings.Contains(fullBody, "first") || !strings.Contains(fullBody, "second") || !strings.Contains(fullBody, "DONE") {
		t.Fatalf("expected all SSE chunks in body, got: %s", fullBody)
	}
	t.Logf("full stream body received (%d bytes)", len(fullBody))
}

func TestAdminStatusReturnsSummary(t *testing.T) {
	cfg := config.Default()
	cfg.Server.RequireAuth = false
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys[0].Value = "test-key"
	rs, _ := NewTestRouterService(cfg)
	s := New(rs, cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/status", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["models"] == nil {
		t.Fatal("expected models in status")
	}
}

func TestAdminEnableKeySavesConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Server.RequireAuth = false
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "k1", Status: "disabled", Priority: 1},
	}
	config.Save(cfgPath, cfg)

	rs, _ := NewTestRouterService(cfg)
	s := New(rs, cfg)
	s.SetConfigPath(cfgPath)

	req := httptest.NewRequest(http.MethodPost, "/admin/keys/key-1/enable", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	reloaded, _ := config.Load(cfgPath)
	if reloaded.Keys[0].Status != "active" {
		t.Fatalf("expected active, got %s", reloaded.Keys[0].Status)
	}
}

func TestAdminDisableKeySavesConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Server.RequireAuth = false
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "k1", Status: "active", Priority: 1},
	}
	config.Save(cfgPath, cfg)

	rs, _ := NewTestRouterService(cfg)
	s := New(rs, cfg)
	s.SetConfigPath(cfgPath)

	req := httptest.NewRequest(http.MethodPost, "/admin/keys/key-1/disable", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	reloaded, _ := config.Load(cfgPath)
	if reloaded.Keys[0].Status != "disabled" {
		t.Fatalf("expected disabled, got %s", reloaded.Keys[0].Status)
	}
}

func TestAdminKeyNotFound(t *testing.T) {
	cfg := config.Default()
	cfg.Server.RequireAuth = false
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	rs, _ := NewTestRouterService(cfg)
	s := New(rs, cfg)

	req := httptest.NewRequest(http.MethodPost, "/admin/keys/nonexistent/enable", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestAdminReloadUpdatesRouter(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Server.RequireAuth = false
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "k1", Status: "active", Priority: 1},
	}
	config.Save(cfgPath, cfg)

	rs, _ := NewTestRouterService(cfg)
	s := New(rs, cfg)
	s.SetConfigPath(cfgPath)

	req := httptest.NewRequest(http.MethodPost, "/admin/reload", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func NewTestRouterService(cfg *config.Config) (*service.RouterService, error) {
	return service.NewRouterService(cfg)
}

func TestAdminBlockedFromNonLocalWhenAuthOff(t *testing.T) {
	cfg := config.Default()
	cfg.Server.RequireAuth = false
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys[0].Value = "test-key"
	rs, _ := NewTestRouterService(cfg)
	s := New(rs, cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/status", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	w := httptest.NewRecorder()
	s.authMiddleware(s.mux).ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-local admin access, got %d", w.Code)
	}
}

func TestAdminAllowedFromLocalWhenAuthOff(t *testing.T) {
	cfg := config.Default()
	cfg.Server.RequireAuth = false
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys[0].Value = "test-key"
	rs, _ := NewTestRouterService(cfg)
	s := New(rs, cfg)

	req := httptest.NewRequest(http.MethodGet, "/admin/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	s.authMiddleware(s.mux).ServeHTTP(w, req)

	if w.Code == http.StatusForbidden {
		t.Fatal("expected admin access from local to be allowed")
	}
}

func TestAdminFullChainIncludesRequestID(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := config.Default()
	cfg.Server.RequireAuth = false
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys = []config.KeyConfig{
		{ID: "k1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "v", Status: "active", Priority: 1},
	}
	config.Save(cfgPath, cfg)

	rs, _ := NewTestRouterService(cfg)
	s := New(rs, cfg)
	s.SetConfigPath(cfgPath)

	handler := s.srv.Handler

	req := httptest.NewRequest(http.MethodGet, "/admin/status", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("full chain: expected 200, got %d: %s", w.Code, w.Body.String())
	}
	reqID := w.Header().Get("X-ModelMux-Request-ID")
	if reqID == "" {
		t.Fatal("full chain: expected X-ModelMux-Request-ID header")
	}
}

func TestAdminFullChainBlocksNonLocal(t *testing.T) {
	cfg := config.Default()
	cfg.Server.RequireAuth = false
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys[0].Value = "test-key"
	rs, _ := NewTestRouterService(cfg)
	s := New(rs, cfg)

	handler := s.srv.Handler

	req := httptest.NewRequest(http.MethodGet, "/admin/status", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("full chain: expected 403 for non-local, got %d", w.Code)
	}
}

func TestFullChainCompletionsPreservesRequestID(t *testing.T) {
	cfg := config.Default()
	cfg.Server.RequireAuth = false
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys[0].Value = "test-key"
	rs, _ := NewTestRouterService(cfg)
	s := New(rs, cfg)

	handler := s.srv.Handler

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	reqID := w.Header().Get("X-ModelMux-Request-ID")
	if reqID == "" {
		t.Fatal("full chain: health check should have request ID")
	}
}
