package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
	"github.com/livingdolls/yute-modelmux/internal/core/port/inbound"
	"github.com/livingdolls/yute-modelmux/internal/secret"
	"github.com/livingdolls/yute-modelmux/internal/storage"
)

func TestSelectKeyPrefersLowestPriority(t *testing.T) {
	cfg := config.Default()
	cfg.Keys = []config.KeyConfig{
		{ID: "k1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Status: "active", Priority: 2, Value: "one"},
		{ID: "k2", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Status: "active", Priority: 1, Value: "two"},
	}
	rs, _ := NewRouterService(cfg)
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
	rs, _ := NewRouterService(cfg)
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
	rs, _ := NewRouterService(cfg)
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
	rs, _ := NewRouterService(cfg)
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
	rs, _ := NewRouterService(cfg)
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

func TestFinalizeStreamResultUsesFullStreamDuration(t *testing.T) {
	cfg := config.Default()
	rs, _ := NewRouterService(cfg)
	ctx := SetStreamResultContext(context.Background(), streamResultInfo{
		KeyID:      "mimo-key-1",
		ModelID:    "mimo-v2.5-pro",
		ProviderID: "mimo",
		StatusCode: http.StatusOK,
		StartedAt:  time.Now().Add(-25 * time.Millisecond),
	})

	rs.FinalizeStreamResult(ctx, nil)

	logs := rs.Logs()
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].LatencyMs <= 0 {
		t.Fatalf("expected stream latency to include elapsed body duration, got %d", logs[0].LatencyMs)
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
	rs, _ := NewRouterService(cfg)
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
	rs, _ := NewRouterService(cfg)
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
	rs, _ := NewRouterService(cfg)
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

	rs, _ := NewRouterService(cfg)
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

	rs, _ := NewRouterService(cfg)
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

func TestHandleChatCompletionRoutesWeightedGroup(t *testing.T) {
	t.Run("deterministic picks", func(t *testing.T) {
		members := []availableGroupMember{
			{member: domain.ModelGroupMember{ModelID: "a", Weight: 1}, model: domain.Model{ID: "a"}},
			{member: domain.ModelGroupMember{ModelID: "b", Weight: 2}, model: domain.Model{ID: "b"}},
			{member: domain.ModelGroupMember{ModelID: "c", Weight: 3}, model: domain.Model{ID: "c"}},
		}

		tests := []struct {
			pick   int
			expect string
		}{
			{0, "a"},
			{1, "b"},
			{2, "b"},
			{3, "c"},
			{5, "c"},
		}
		for _, tt := range tests {
			result := selectWeightedMemberByPick(members, tt.pick)
			if result.model.ID != tt.expect {
				t.Errorf("pick %d: expected %s, got %s", tt.pick, tt.expect, result.model.ID)
			}
		}
	})

	t.Run("zero weight normalized", func(t *testing.T) {
		members := []availableGroupMember{
			{member: domain.ModelGroupMember{ModelID: "x", Weight: 0}, model: domain.Model{ID: "x"}},
			{member: domain.ModelGroupMember{ModelID: "y", Weight: 0}, model: domain.Model{ID: "y"}},
		}
		result := selectWeightedMemberByPick(members, 0)
		if result.model.ID != "x" {
			t.Errorf("expected first member for zero weight, got %s", result.model.ID)
		}
	})

	t.Run("integration routing", func(t *testing.T) {
		var requests []string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload struct {
				Model string `json:"model"`
			}
			json.NewDecoder(r.Body).Decode(&payload)
			requests = append(requests, payload.Model)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		cfg := &config.Config{
			App:      config.AppConfig{Name: "test", LogLevel: "info"},
			Server:   config.ServerConfig{Host: "127.0.0.1", Port: 8787},
			Cooldown: config.CooldownConfig{RateLimitSeconds: 300, ServerErrorSeconds: 60},
			Retry:    config.RetryConfig{MaxRetryPerKey: 0, MaxTotalAttempts: 5},
			Providers: []config.ProviderConfig{
				{ID: "openai", Name: "OpenAI", Type: "openai-compatible", BaseURL: server.URL + "/v1", AuthType: "bearer", TimeoutSeconds: 5, Enabled: true},
			},
			Models: []config.ModelConfig{
				{ID: "model-a", ProviderID: "openai", ModelName: "model-a", Strategy: "failover", Enabled: true},
				{ID: "model-b", ProviderID: "openai", ModelName: "model-b", Strategy: "failover", Enabled: true},
			},
			ModelGroups: []config.ModelGroupConfig{
				{ID: "weighted-group", Name: "Weighted", Strategy: "weighted", Enabled: true, Members: []config.ModelGroupMemberConfig{
					{ModelID: "model-a", Weight: 1, Enabled: true},
					{ModelID: "model-b", Weight: 1, Enabled: true},
				}},
			},
			Keys: []config.KeyConfig{
				{ID: "key-a", ProviderID: "openai", ModelID: "model-a", Value: "a-key", Status: "active", Priority: 1},
				{ID: "key-b", ProviderID: "openai", ModelID: "model-b", Value: "b-key", Status: "active", Priority: 1},
			},
		}

		rs, _ := NewRouterService(cfg)
		for range 5 {
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"weighted-group","messages":[]}`))
			resp, err := rs.HandleChatCompletion(context.Background(), req)
			if err != nil {
				t.Fatalf("handle weighted group request failed: %v", err)
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}
			resp.Body.Close()
		}

		if len(requests) != 5 {
			t.Fatalf("expected 5 routed requests, got %d", len(requests))
		}
		for _, r := range requests {
			if r != "model-a" && r != "model-b" {
				t.Errorf("routed to unexpected model %q", r)
			}
		}
	})
}

func TestHandleChatCompletionPreservesCooldownWhenTotalAttemptsExhausted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := singleKeyRetryConfig(server.URL + "/v1")
	cfg.Retry.MaxRetryPerKey = 5
	cfg.Retry.MaxTotalAttempts = 2

	rs, _ := NewRouterService(cfg)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test-model","messages":[]}`))

	_, err := rs.HandleChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected error after total attempts exhausted")
	}

	keys := rs.ListKeys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Status != domain.KeyStatusCooldown {
		t.Fatalf("expected key to be cooldown after total attempts exhausted, got status %s", keys[0].Status)
	}
	if keys[0].CooldownEnd == nil {
		t.Fatal("expected key to have cooldown set")
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

func TestKeySuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" && r.Method == http.MethodGet {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := singleKeyRetryConfig(server.URL + "/v1")
	rs, _ := NewRouterService(cfg)

	if err := rs.TestKey(context.Background(), "test-key"); err != nil {
		t.Fatalf("TestKey failed: %v", err)
	}
}

func TestKeyFailsOn401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := singleKeyRetryConfig(server.URL + "/v1")
	rs, _ := NewRouterService(cfg)

	if err := rs.TestKey(context.Background(), "test-key"); err == nil {
		t.Fatal("TestKey should fail on 401")
	}
}

func TestKeyUnknownKeyReturnsError(t *testing.T) {
	cfg := singleKeyRetryConfig("https://example.com/v1")
	rs, _ := NewRouterService(cfg)

	if err := rs.TestKey(context.Background(), "nonexistent-key"); err == nil {
		t.Fatal("TestKey should fail for unknown key")
	}
}

func TestKeyBecomesLimitedOnDailyRequestLimit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	defer ts.Close()

	cfg := singleKeyRetryConfig(ts.URL + "/v1")
	cfg.Keys[0].DailyRequestLimit = 3
	rs, _ := NewRouterService(cfg)

	for i := 0; i < 4; i++ {
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", bytes.NewReader([]byte(`{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`)))
		resp, err := rs.HandleChatCompletion(context.Background(), req)
		if err != nil {
			if strings.Contains(err.Error(), "currently limited") {
				keys := rs.ListKeys()
				if len(keys) == 1 && keys[0].Status == domain.KeyStatusLimited {
					return
				}
				t.Fatalf("key should be limited but status is %s", keys[0].Status)
			}
			t.Fatalf("unexpected error: %v", err)
		}
		if resp != nil {
			resp.Body.Close()
		}
	}
	t.Fatalf("expected key to become limited after %d requests", cfg.Keys[0].DailyRequestLimit)
}

func TestLimitedKeyNotSelected(t *testing.T) {
	cfg := config.Default()
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys = append(cfg.Keys, config.KeyConfig{
		ID: "key-2", ProviderID: "mimo", ModelID: "mimo-v2.5-pro",
		Value: "key2", Status: "active", Priority: 2,
	})
	rs, _ := NewRouterService(cfg)

	keys := rs.ListKeys()
	for i := range keys {
		if keys[i].ID == "key-2" {
			keys[i].Status = domain.KeyStatusLimited
			break
		}
	}

	key, err := rs.SelectKey(context.Background(), "mimo-v2.5-pro")
	if err != nil {
		t.Fatalf("SelectKey should not return error when limited key exists: %v", err)
	}
	if key.ID == "key-2" {
		t.Fatal("SelectKey should not return limited key")
	}
	if key.ID != "mimo-key-1" {
		t.Fatalf("expected mimo-key-1, got %s", key.ID)
	}
}

func TestConfigStoragePathTildeExpansion(t *testing.T) {
	home, _ := os.UserHomeDir()
	if home == "" {
		t.Skip("no home directory")
	}
	cfg := config.Default()
	cfg.Storage.Type = "sqlite"
	cfg.Storage.Path = "~/.local/share/modelmux/modelmux.db"

	store, err := storage.New(expandHomeForTest(cfg.Storage.Path))
	if err != nil {
		t.Fatalf("storage.New should succeed with tilde path: %v", err)
	}
	store.Close()
}

func expandHomeForTest(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func TestSecretResolutionPriority(t *testing.T) {
	masterKey := "test-master-key-for-secret-resolution"
	t.Setenv("MODELMUX_MASTER_KEY", masterKey)
	t.Setenv("TEST_ENV_VAL", "env-value")

	dir := t.TempDir()
	storePath := filepath.Join(dir, "secrets.enc")
	secStore, err := secret.NewStore(storePath)
	if err != nil {
		t.Fatalf("failed to create secret store: %v", err)
	}
	secStore.Set("test-ref", "secret-value")

	cfg := config.Default()
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys = []config.KeyConfig{
		{
			ID: "key-all-three", ProviderID: "mimo", ModelID: "mimo-v2.5-pro",
			SecretRef: "test-ref", ValueEnv: "TEST_ENV_VAL", Value: "plain-value",
			Status: "active", Priority: 1,
		},
	}

	rs, err := NewRouterServiceWithSecret(cfg, nil, secStore)
	if err != nil {
		t.Fatalf("NewRouterServiceWithSecret failed: %v", err)
	}
	keys := rs.ListKeys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].Value != "secret-value" {
		t.Fatalf("secret_ref should have highest priority, got value=%q", keys[0].Value)
	}
}

func TestSecretStoreMissingKeyError(t *testing.T) {
	masterKey := "test-master-key-missing"
	t.Setenv("MODELMUX_MASTER_KEY", masterKey)

	dir := t.TempDir()
	storePath := filepath.Join(dir, "secrets.enc")
	secStore, err := secret.NewStore(storePath)
	if err != nil {
		t.Fatalf("failed to create secret store: %v", err)
	}

	cfg := config.Default()
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Keys = []config.KeyConfig{{
		ID: "key-only-secret", ProviderID: "mimo", ModelID: "mimo-v2.5-pro",
		SecretRef: "non-existent-ref",
		Status:    "active", Priority: 1,
	}}

	_, err = NewRouterServiceWithSecret(cfg, nil, secStore)
	if err == nil {
		t.Fatal("expected error for missing secret_ref")
	}
}

func TestDailyTokenLimitCountsTotalTokens(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":90,"completion_tokens":10}}`))
	}))
	defer ts.Close()

	cfg := singleKeyRetryConfig(ts.URL + "/v1")
	cfg.Keys[0].DailyTokenLimit = 100
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", bytes.NewReader([]byte(`{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`)))
	resp, err := rs.HandleChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("first request should not error with 100-token limit and 100-token usage: %v", err)
	}
	if resp != nil {
		resp.Body.Close()
	}

	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", bytes.NewReader([]byte(`{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`)))
	resp2, err := rs.HandleChatCompletion(context.Background(), req2)
	if err == nil {
		if resp2 != nil {
			resp2.Body.Close()
		}
		t.Fatal("second request should fail because total tokens (100) >= limit (100)")
	}
	if !strings.Contains(err.Error(), "currently limited") {
		t.Fatalf("expected limited error, got: %v", err)
	}
}

func TestStreamDailyTokenLimitCountsOpenAIUsageChunk(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[],\"usage\":{\"prompt_tokens\":90,\"completion_tokens\":10}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer ts.Close()

	cfg := singleKeyRetryConfig(ts.URL + "/v1")
	cfg.Keys[0].DailyTokenLimit = 100
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions", bytes.NewReader([]byte(`{"model":"test-model","messages":[{"role":"user","content":"hi"}],"stream":true}`)))
	resp, err := rs.HandleChatCompletion(req.Context(), req)
	if err != nil {
		t.Fatalf("stream request should succeed: %v", err)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read stream failed: %v", err)
	}
	resp.Body.Close()
	rs.FinalizeStreamResult(req.Context(), nil)

	keys := rs.ListKeys()
	if keys[0].DailyTokenCount != 100 {
		t.Fatalf("expected stream token count 100, got %d", keys[0].DailyTokenCount)
	}
}

func TestRequestsPerMinuteLimitsKeySelection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = ts.URL + "/v1"
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "k1", Status: "active", Priority: 1, RequestsPerMinute: 2},
		{ID: "key-2", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "k2", Status: "active", Priority: 2, RequestsPerMinute: 2},
	}
	rs, _ := NewRouterService(cfg)

	for i := 0; i < 2; i++ {
		req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}]}`)))
		resp, err := rs.HandleChatCompletion(context.Background(), req)
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
		resp.Body.Close()
	}

	keys := rs.ListKeys()
	if keys[0].MinuteRequestCount != 2 || keys[1].MinuteRequestCount != 0 {
		t.Fatalf("expected key-1 count=2, key-2 count=0, got %d/%d", keys[0].MinuteRequestCount, keys[1].MinuteRequestCount)
	}

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}]}`)))
	resp, err := rs.HandleChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("third request should use key-2: %v", err)
	}
	resp.Body.Close()

	keys = rs.ListKeys()
	if keys[1].MinuteRequestCount == 0 {
		t.Fatal("expected key-2 to be used after key-1 limited")
	}
}

func TestMaxConcurrentRequestsLimitsKeySelection(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = ts.URL + "/v1"
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "k1", Status: "active", Priority: 1, MaxConcurrentRequests: 1},
		{ID: "key-2", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "k2", Status: "active", Priority: 2, MaxConcurrentRequests: 1},
	}
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}]}`)))
	resp, err := rs.HandleChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	req2, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi2"}]}`)))
	resp2, err2 := rs.HandleChatCompletion(context.Background(), req2)
	if err2 != nil {
		t.Fatalf("second request should use key-2: %v", err2)
	}
	resp.Body.Close()
	resp2.Body.Close()

	keys := rs.ListKeys()
	if keys[0].ConcurrentCount != 0 || keys[1].ConcurrentCount != 0 {
		t.Fatalf("expected concurrency released, got key-1=%d key-2=%d", keys[0].ConcurrentCount, keys[1].ConcurrentCount)
	}
}

func TestModelConcurrencyLimitIsTracked(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = ts.URL + "/v1"
	cfg.Models[0].MaxConcurrentRequests = 5
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "k1", Status: "active", Priority: 1},
	}
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}]}`)))
	resp, err := rs.HandleChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	resp.Body.Close()

	models := rs.ListModels()
	if models[0].ConcurrentCount != 0 {
		t.Fatalf("expected model concurrency released, got %d", models[0].ConcurrentCount)
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

func TestCapabilityBlocksCompletionsForChatOnly(t *testing.T) {
	cfg := config.Default()
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Models[0].Capabilities = &config.ModelCapabilityConfig{}
	f := false
	cfg.Models[0].Capabilities.Completions = &f
	cfg.Keys[0].Value = "test-key"
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","prompt":"hello"}`)))
	_, err := rs.HandleCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected capability error for completions on chat-only model")
	}
}

func TestCapabilityBlocksToolCalling(t *testing.T) {
	cfg := config.Default()
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Models[0].Capabilities = &config.ModelCapabilityConfig{}
	f := false
	cfg.Models[0].Capabilities.Tools = &f
	cfg.Keys[0].Value = "test-key"
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"test"}}]}`)))
	_, err := rs.HandleChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected capability error for tools on non-tools model")
	}
}

func TestCapabilityBlocksStreaming(t *testing.T) {
	cfg := config.Default()
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Models[0].Capabilities = &config.ModelCapabilityConfig{}
	f := false
	cfg.Models[0].Capabilities.Streaming = &f
	cfg.Keys[0].Value = "test-key"
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}],"stream":true}`)))
	_, err := rs.HandleChatCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected capability error for streaming on non-streaming model")
	}
}

func TestCapabilityDefaultAllowsBasicForOpenAI(t *testing.T) {
	cfg := config.Default()
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Providers[0].Type = "openai-compatible"
	cfg.Keys[0].Value = "test-key"
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}],"stream":true}`)))
	_, err := rs.HandleChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected openai default to allow chat+stream: %v", err)
	}
}

func TestCapabilityOverrideAllowsTools(t *testing.T) {
	cfg := config.Default()
	cfg.Providers[0].BaseURL = "https://example.com/v1"
	cfg.Models[0].Capabilities = &config.ModelCapabilityConfig{}
	ft := true
	cfg.Models[0].Capabilities.Tools = &ft
	cfg.Keys[0].Value = "test-key"
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"test"}}]}`)))
	_, err := rs.HandleChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatalf("expected tools override to allow tools: %v", err)
	}
}

func TestCapabilityDefaultBlocksCompletionsForAnthropic(t *testing.T) {
	cfg := config.Default()
	cfg.Providers[0].BaseURL = "https://api.anthropic.com"
	cfg.Providers[0].Type = "anthropic"
	cfg.Keys[0].Value = "test-key"
	f := false
	cfg.Models[0].Capabilities = &config.ModelCapabilityConfig{Streaming: &f}
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, "/v1/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","prompt":"hello"}`)))
	_, err := rs.HandleCompletion(context.Background(), req)
	if err == nil {
		t.Fatal("expected anthropic default to block completions")
	}
}

func TestLeastUsedStrategyPicksLowestDailyCount(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = ts.URL + "/v1"
	cfg.Models[0].Strategy = "least_used"
	cfg.Keys = []config.KeyConfig{
		{ID: "ka", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "a", Status: "active", Priority: 1},
		{ID: "kb", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "b", Status: "active", Priority: 2},
		{ID: "kc", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "c", Status: "active", Priority: 3},
	}
	rs, _ := NewRouterService(cfg)

	req, _ := http.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"mimo-v2.5-pro","messages":[{"role":"user","content":"hi"}]}`)))
	resp, _ := rs.HandleChatCompletion(context.Background(), req)
	if resp != nil {
		resp.Body.Close()
	}

	keys := rs.ListKeys()
	if keys[0].DailyRequestCount != 1 {
		t.Fatalf("expected key ka (lowest priority) to be picked first, got counts: %d/%d/%d", keys[0].DailyRequestCount, keys[1].DailyRequestCount, keys[2].DailyRequestCount)
	}
}
