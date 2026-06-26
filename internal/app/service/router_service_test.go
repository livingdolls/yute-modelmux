package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
	"github.com/livingdolls/yute-modelmux/internal/core/port/inbound"
)

func TestSelectKeyPrefersLowestPriority(t *testing.T) {
	cfg := config.Default()
	cfg.Keys = []config.KeyConfig{
		{ID: "k1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Status: "active", Priority: 2, Value: "one"},
		{ID: "k2", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Status: "active", Priority: 1, Value: "two"},
	}
	rs := NewRouterService(cfg)
	key, err := rs.SelectKey(context.Background(), "mimo-v2.5-pro")
	if err != nil {
		t.Fatalf("select key failed: %v", err)
	}
	if key.ID != "k2" {
		t.Fatalf("expected k2, got %s", key.ID)
	}
}

func TestSelectKeySkipsCooldown(t *testing.T) {
	now := time.Now().Add(5 * time.Minute)
	cfg := config.Default()
	cfg.Keys = []config.KeyConfig{
		{ID: "k1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Status: "cooldown", Priority: 1, Value: "one"},
		{ID: "k2", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Status: "active", Priority: 2, Value: "two"},
	}
	rs := NewRouterService(cfg)
	rs.keys[0].CooldownEnd = &now
	key, err := rs.SelectKey(context.Background(), "mimo-v2.5-pro")
	if err != nil {
		t.Fatalf("select key failed: %v", err)
	}
	if key.ID != "k2" {
		t.Fatalf("expected k2, got %s", key.ID)
	}
}

func TestMarkKeyResultCooldown(t *testing.T) {
	cfg := config.Default()
	rs := NewRouterService(cfg)
	if err := rs.MarkKeyResult(context.Background(), "mimo-key-1", inbound.KeyResult{StatusCode: 429, ShouldRotateKey: true, CooldownSeconds: 300}); err != nil {
		t.Fatalf("mark key result failed: %v", err)
	}
	if rs.keys[0].Status != domain.KeyStatusCooldown {
		t.Fatalf("expected cooldown status, got %s", rs.keys[0].Status)
	}
}

func TestExtractModelFromBodyUsesJSONField(t *testing.T) {
	modelID, err := extractModelFromBody([]byte(`{"messages":[{"role":"user","content":"please use \"model\":\"wrong\""}],"model":"right"}`))
	if err != nil {
		t.Fatalf("extract model failed: %v", err)
	}
	if modelID != "right" {
		t.Fatalf("expected right, got %s", modelID)
	}
}

func TestClassifyServerErrorSetsCooldown(t *testing.T) {
	cfg := config.Default()
	result := classifyResult(&http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable"}, nil, cfg)
	if !result.ShouldRotateKey {
		t.Fatal("expected server error to rotate key")
	}
	if result.CooldownSeconds != cfg.Cooldown.ServerErrorSeconds {
		t.Fatalf("expected server error cooldown %d, got %d", cfg.Cooldown.ServerErrorSeconds, result.CooldownSeconds)
	}
}

func TestHandleChatCompletionReturnsAllKeysUnavailableError(t *testing.T) {
	cfg := config.Default()
	cfg.Keys[0].Status = "cooldown"
	rs := NewRouterService(cfg)
	cooldownEnd := time.Now().Add(time.Minute)
	rs.keys[0].CooldownEnd = &cooldownEnd
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"mimo-v2.5-pro","messages":[]}`))

	_, err := rs.HandleChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected all keys unavailable error")
	}
	var proxyErr *ProxyError
	if !errors.As(err, &proxyErr) {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if proxyErr.HTTPStatus != http.StatusTooManyRequests || proxyErr.Code != "all_keys_limited" {
		t.Fatalf("unexpected proxy error: %+v", proxyErr)
	}
}

func TestMarkKeyResultLogsSuccess(t *testing.T) {
	cfg := config.Default()
	rs := NewRouterService(cfg)
	if err := rs.MarkKeyResult(context.Background(), "mimo-key-1", inbound.KeyResult{Success: true, ModelID: "mimo-v2.5-pro", ProviderID: "mimo", StatusCode: 200, LatencyMs: 12}); err != nil {
		t.Fatalf("mark key result failed: %v", err)
	}
	logs := rs.Logs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].StatusCode != 200 || logs[0].ModelID != "mimo-v2.5-pro" || logs[0].LatencyMs != 12 {
		t.Fatalf("unexpected log entry: %+v", logs[0])
	}
}

func TestHandleChatCompletionRoutesModelGroup(t *testing.T) {
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		gotModel = payload.Model
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := groupRoutingConfig(server.URL + "/v1")
	rs := NewRouterService(cfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"high-price","messages":[]}`))

	resp, err := rs.HandleChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("handle group request failed: %v", err)
	}
	defer resp.Body.Close()

	if gotModel != "gpt-5.5-xHigh" {
		t.Fatalf("expected provider model gpt-5.5-xHigh, got %s", gotModel)
	}
	logs := rs.Logs()
	if len(logs) != 1 || logs[0].GroupID != "high-price" || logs[0].ModelID != "gpt-5.5-xhigh" {
		t.Fatalf("unexpected group log: %+v", logs)
	}
}

func TestHandleChatCompletionGroupFallsBackToNextMember(t *testing.T) {
	var gotModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		gotModel = payload.Model
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := groupRoutingConfig(server.URL + "/v1")
	rs := NewRouterService(cfg)
	cooldownEnd := time.Now().Add(time.Minute)
	rs.keys[0].Status = domain.KeyStatusCooldown
	rs.keys[0].CooldownEnd = &cooldownEnd
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"high-price","messages":[]}`))

	resp, err := rs.HandleChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("handle group fallback failed: %v", err)
	}
	defer resp.Body.Close()

	if gotModel != "gpt-5.5-fast" {
		t.Fatalf("expected fallback provider model gpt-5.5-fast, got %s", gotModel)
	}
}

func TestHandleChatCompletionGroupUnavailable(t *testing.T) {
	cfg := groupRoutingConfig("https://example.test/v1")
	rs := NewRouterService(cfg)
	cooldownEnd := time.Now().Add(time.Minute)
	for i := range rs.keys {
		rs.keys[i].Status = domain.KeyStatusCooldown
		rs.keys[i].CooldownEnd = &cooldownEnd
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"high-price","messages":[]}`))

	_, err := rs.HandleChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected group unavailable error")
	}
	var proxyErr *ProxyError
	if !errors.As(err, &proxyErr) {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if proxyErr.HTTPStatus != http.StatusTooManyRequests || proxyErr.Code != "all_group_models_unavailable" {
		t.Fatalf("unexpected proxy error: %+v", proxyErr)
	}
}

func TestHandleChatCompletionRetriesPerKeyWithBackoff(t *testing.T) {
	var reqCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&reqCount, 1)
		var payload struct {
			Model string `json:"model"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		if count == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()

	cfg := singleKeyRetryConfig(server.URL + "/v1")
	cfg.Retry.MaxRetryPerKey = 3
	cfg.Retry.BackoffMilliseconds = []int{10, 20}

	rs := NewRouterService(cfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-model","messages":[]}`))

	started := time.Now()
	resp, err := rs.HandleChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected retry to succeed after transient error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	finalCount := atomic.LoadInt32(&reqCount)
	if finalCount < 2 {
		t.Fatalf("expected at least 2 requests (1 failed + 1 success), got %d", finalCount)
	}
	if time.Since(started) < 10*time.Millisecond {
		t.Fatal("expected backoff delay")
	}
}

func TestHandleChatCompletionRetryRespectsMaxTotalAttempts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := singleKeyRetryConfig(server.URL + "/v1")
	cfg.Retry.MaxRetryPerKey = 5
	cfg.Retry.MaxTotalAttempts = 2

	rs := NewRouterService(cfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-model","messages":[]}`))

	_, err := rs.HandleChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected all keys unavailable error after exhausting total attempts")
	}
	var proxyErr *ProxyError
	if !errors.As(err, &proxyErr) {
		t.Fatalf("expected ProxyError, got %T", err)
	}
	if proxyErr.Code != "all_keys_limited" {
		t.Fatalf("expected all_keys_limited, got %s", proxyErr.Code)
	}
}

func singleKeyRetryConfig(baseURL string) *config.Config {
	return &config.Config{
		App:      config.AppConfig{Name: "test", LogLevel: "info"},
		Server:   config.ServerConfig{Host: "127.0.0.1", Port: 8787, ReadTimeoutSecond: 60, WriteTimeoutSecond: 300},
		Cooldown: config.CooldownConfig{RateLimitSeconds: 300, ServerErrorSeconds: 60, TimeoutSeconds: 60},
		Retry:    config.RetryConfig{MaxRetryPerKey: 1, MaxTotalAttempts: 3, BackoffMilliseconds: []int{10}},
		Providers: []config.ProviderConfig{
			{ID: "test-provider", Name: "Test", Type: "openai-compatible", BaseURL: baseURL, AuthType: "bearer", TimeoutSeconds: 5, Enabled: true},
		},
		Models: []config.ModelConfig{
			{ID: "test-model", ProviderID: "test-provider", ModelName: "test-model", Strategy: "failover", Enabled: true},
		},
		Keys: []config.KeyConfig{
			{ID: "test-key", ProviderID: "test-provider", ModelID: "test-model", Value: "test-secret", Status: "active", Priority: 1},
		},
	}
}

func groupRoutingConfig(baseURL string) *config.Config {
	return &config.Config{
		App:      config.AppConfig{Name: "modelmux", LogLevel: "info"},
		Server:   config.ServerConfig{Host: "127.0.0.1", Port: 8787, ReadTimeoutSecond: 60, WriteTimeoutSecond: 300},
		Cooldown: config.CooldownConfig{RateLimitSeconds: 300, ServerErrorSeconds: 60, TimeoutSeconds: 60},
		Retry:    config.RetryConfig{MaxRetryPerKey: 1, MaxTotalAttempts: 5},
		Providers: []config.ProviderConfig{
			{ID: "openai", Name: "OpenAI", Type: "openai-compatible", BaseURL: baseURL, AuthType: "bearer", TimeoutSeconds: 5, Enabled: true},
		},
		Models: []config.ModelConfig{
			{ID: "gpt-5.5-xhigh", ProviderID: "openai", ModelName: "gpt-5.5-xHigh", Strategy: "failover", Enabled: true},
			{ID: "gpt-5.5-fast", ProviderID: "openai", ModelName: "gpt-5.5-fast", Strategy: "failover", Enabled: true},
		},
		ModelGroups: []config.ModelGroupConfig{
			{ID: "high-price", Name: "High Price", Strategy: "failover", Enabled: true, Members: []config.ModelGroupMemberConfig{
				{ModelID: "gpt-5.5-xhigh", Priority: 1, Weight: 1, Enabled: true},
				{ModelID: "gpt-5.5-fast", Priority: 2, Weight: 1, Enabled: true},
			}},
		},
		Keys: []config.KeyConfig{
			{ID: "openai-xhigh-key-1", ProviderID: "openai", ModelID: "gpt-5.5-xhigh", Value: "xhigh-key", Status: "active", Priority: 1},
			{ID: "openai-fast-key-1", ProviderID: "openai", ModelID: "gpt-5.5-fast", Value: "fast-key", Status: "active", Priority: 1},
		},
	}
}
