package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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
