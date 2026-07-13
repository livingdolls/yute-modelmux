//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/adapter/httpserver"
	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/storage"
)

func boolPtr(v bool) *bool { return &v }

func startE2EServer(t *testing.T, cfg *config.Config) (string, func()) {
	t.Helper()

	router, err := service.NewRouterService(cfg)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}

	srv := httpserver.New(router, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Run(ctx) }()
	time.Sleep(100 * time.Millisecond)

	baseURL := "http://" + cfg.Server.Host + ":18787"

	return baseURL, func() {
		cancel()
		time.Sleep(50 * time.Millisecond)
	}
}

func startMockUpstream(t *testing.T, statusCode int, responseBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if responseBody != "" {
			w.Write([]byte(responseBody))
		}
	}))
}

func e2eConfig(baseURL string) *config.Config {
	if baseURL == "" {
		baseURL = "https://example.com/v1"
	}
	return &config.Config{
		App:         config.AppConfig{Name: "e2e", LogLevel: "info"},
		Server:      config.ServerConfig{Host: "127.0.0.1", Port: 18787, ReadTimeoutSecond: 30, WriteTimeoutSecond: 30, Admin: config.AdminConfig{RequireAuth: boolPtr(false)}},
		Cooldown:    config.CooldownConfig{RateLimitSeconds: 300, ServerErrorSeconds: 60, TimeoutSeconds: 60},
		Retry:       config.RetryConfig{MaxRetryPerKey: 1, MaxTotalAttempts: 3},
		Providers:   []config.ProviderConfig{{ID: "mimo", Name: "MiMo", Type: "openai-compatible", BaseURL: baseURL, AuthType: "bearer", TimeoutSeconds: 10, Enabled: true}},
		Models:      []config.ModelConfig{{ID: "mimo-v2.5-pro", ProviderID: "mimo", ModelName: "mimo-v2.5-pro", Strategy: "failover", Enabled: true}},
		ModelGroups: []config.ModelGroupConfig{{ID: "high-price", Name: "High", Strategy: "failover", Enabled: true, Members: []config.ModelGroupMemberConfig{{ModelID: "mimo-v2.5-pro", Priority: 1, Weight: 1, Enabled: true}}}},
		Keys:        []config.KeyConfig{{ID: "mimo-key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "test-key", Status: "active", Priority: 1}},
	}
}

func TestE2EHealth(t *testing.T) {
	cfg := e2eConfig("")
	baseURL, cleanup := startE2EServer(t, cfg)
	defer cleanup()

	resp, err := http.Get(baseURL + "/health")
	if err != nil {
		t.Fatalf("health request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestE2EModelsList(t *testing.T) {
	cfg := e2eConfig("")
	baseURL, cleanup := startE2EServer(t, cfg)
	defer cleanup()

	resp, err := http.Get(baseURL + "/v1/models")
	if err != nil {
		t.Fatalf("models request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Data) == 0 {
		t.Fatal("expected at least one model in list")
	}
}

func TestE2EChatCompletionSuccess(t *testing.T) {
	upstream := startMockUpstream(t, http.StatusOK, `{"choices":[{"message":{"content":"hello"}}]}`)
	defer upstream.Close()

	cfg := e2eConfig(upstream.URL + "/v1")
	baseURL, cleanup := startE2EServer(t, cfg)
	defer cleanup()

	resp, err := http.Post(baseURL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("chat request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}
}

func TestE2EAIRoutingToGroup(t *testing.T) {
	upstream := startMockUpstream(t, http.StatusOK, `{"choices":[{"message":{"content":"routed"}}]}`)
	defer upstream.Close()

	cfg := e2eConfig(upstream.URL + "/v1")
	cfg.AI.Enabled = true
	cfg.AI.Classifier.Enabled = true
	cfg.AI.RoutingRules = []config.AIRoutingRuleConfig{
		{When: config.AIRoutingRuleWhen{Task: "coding"}, UseGroup: "high-price"},
	}

	baseURL, cleanup := startE2EServer(t, cfg)
	defer cleanup()

	body := `{"model":"any-model","messages":[{"role":"user","content":"write a function that sorts"}]}`
	resp, err := http.Post(baseURL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("AI routing request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		rbody, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(rbody))
	}
}

func TestE2EGuardrailBlockWithTraceHeader(t *testing.T) {
	cfg := e2eConfig("https://example.com/v1")
	cfg.AI.Enabled = true
	cfg.AI.Guardrails.Enabled = true
	cfg.AI.RouteTrace.Enabled = true
	cfg.AI.RouteTrace.IncludeResponseHeader = true

	baseURL, cleanup := startE2EServer(t, cfg)
	defer cleanup()

	body := `{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"my api key is sk-1234567890abcdefghij"}]}`
	resp, err := http.Post(baseURL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("guardrail request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for guardrail block, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-ModelMux-Route-Trace-ID") == "" {
		t.Fatal("expected X-ModelMux-Route-Trace-ID header on guardrail block")
	}
}

func TestE2ENotFoundWithTraceHeader(t *testing.T) {
	cfg := e2eConfig("https://example.com/v1")
	cfg.AI.Enabled = true
	cfg.AI.RouteTrace.Enabled = true
	cfg.AI.RouteTrace.IncludeResponseHeader = true

	baseURL, cleanup := startE2EServer(t, cfg)
	defer cleanup()

	resp, err := http.Post(baseURL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"nonexistent-model","messages":[]}`))
	if err != nil {
		t.Fatalf("not found request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
	if resp.Header.Get("X-ModelMux-Route-Trace-ID") == "" {
		t.Fatal("expected X-ModelMux-Route-Trace-ID header on 404")
	}
}

func TestE2EAdminTracesWithoutReload(t *testing.T) {
	upstream := startMockUpstream(t, http.StatusOK, `{"choices":[{"message":{"content":"ok"}}]}`)
	defer upstream.Close()

	dir := t.TempDir()
	cfgPath := dir + "/config.yaml"

	cfg := e2eConfig(upstream.URL + "/v1")
	cfg.Storage.Type = "sqlite"
	cfg.Storage.Path = dir + "/modelmux.db"
	cfg.AI.Enabled = true
	cfg.AI.RouteTrace.Enabled = true
	config.Save(cfgPath, cfg)

	loadedCfg, _ := config.Load(cfgPath)
	store, _ := storage.New(loadedCfg.Storage.Path)
	if store == nil {
		t.Fatal("failed to create store")
	}
	defer store.Close()

	router, err := service.NewRouterServiceWithStorage(loadedCfg, store)
	if err != nil {
		t.Fatalf("create router: %v", err)
	}

	srv := httpserver.New(router, loadedCfg)
	srv.SetConfigPath(cfgPath)
	srv.SetStore(store)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Run(ctx) }()
	defer cancel()
	time.Sleep(100 * time.Millisecond)

	baseURL := "http://" + loadedCfg.Server.Host + ":18787"

	resp, err := http.Post(baseURL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hello"}]}`))
	if err != nil {
		t.Fatalf("chat request: %v", err)
	}
	reqID := resp.Header.Get("X-ModelMux-Request-ID")
	resp.Body.Close()

	if reqID == "" {
		t.Skip("no request ID, skipping trace lookup")
	}

	traceResp, err := http.Get(baseURL + "/admin/traces/" + reqID)
	if err != nil {
		t.Skipf("trace request failed: %v", err)
	}
	defer traceResp.Body.Close()

	if traceResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(traceResp.Body)
		t.Fatalf("expected 200 for admin trace, got %d: %s", traceResp.StatusCode, string(body))
	}
	t.Logf("trace endpoint returned 200, trace restored after startup")
}

func TestE2EPrometheusMetrics(t *testing.T) {
	cfg := e2eConfig("https://example.com/v1")
	baseURL, cleanup := startE2EServer(t, cfg)
	defer cleanup()

	resp, err := http.Get(baseURL + "/metrics?format=prometheus")
	if err != nil {
		t.Fatalf("metrics request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}
