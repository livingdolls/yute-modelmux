package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/config"
)

func groupFeatureBool(value bool) *bool { return &value }

func groupFeatureConfig(baseURL string) *config.Config {
	return &config.Config{
		App:      config.AppConfig{Name: "test", LogLevel: "info"},
		Server:   config.ServerConfig{Host: "127.0.0.1", Port: 8787},
		Cooldown: config.CooldownConfig{RateLimitSeconds: 30, ServerErrorSeconds: 30, TimeoutSeconds: 30},
		Retry:    config.RetryConfig{MaxRetryPerKey: 0, MaxTotalAttempts: 4},
		Providers: []config.ProviderConfig{
			{ID: "provider", Name: "Provider", Type: "openai-compatible", BaseURL: baseURL, AuthType: "bearer", TimeoutSeconds: 5, Enabled: true},
		},
		Models: []config.ModelConfig{
			{ID: "economy", ProviderID: "provider", ModelName: "economy-upstream", Strategy: "failover", Enabled: true, Capabilities: &config.ModelCapabilityConfig{Chat: groupFeatureBool(true), Streaming: groupFeatureBool(true), Tools: groupFeatureBool(false)}},
			{ID: "flagship", ProviderID: "provider", ModelName: "flagship-upstream", Strategy: "failover", Enabled: true, Capabilities: &config.ModelCapabilityConfig{Chat: groupFeatureBool(true), Streaming: groupFeatureBool(true), Tools: groupFeatureBool(true)}},
		},
		ModelGroups: []config.ModelGroupConfig{
			{ID: "coding-balanced", Name: "Coding Balanced", Description: "Agent coding group", Strategy: "failover", Enabled: true, ContextWindow: 128000, MaxOutputTokens: 32000, Members: []config.ModelGroupMemberConfig{
				{ModelID: "economy", Priority: 1, Weight: 1, Enabled: true},
				{ModelID: "flagship", Priority: 2, Weight: 1, Enabled: true},
			}},
		},
		Keys: []config.KeyConfig{
			{ID: "economy-key", ProviderID: "provider", ModelID: "economy", Value: "economy-secret", Status: "active", Priority: 1},
			{ID: "flagship-key", ProviderID: "provider", ModelID: "flagship", Value: "flagship-secret", Status: "active", Priority: 1},
		},
	}
}

func TestGroupRoutingFiltersIncompatibleModelsAndSetsHeaders(t *testing.T) {
	var selectedModel string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		selectedModel = payload.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer upstream.Close()

	router, err := NewRouterService(groupFeatureConfig(upstream.URL + "/v1"))
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coding-balanced","messages":[],"tools":[{"type":"function","function":{"name":"read_file"}}]}`))
	resp, err := router.HandleChatCompletion(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if selectedModel != "flagship-upstream" {
		t.Fatalf("expected compatible flagship model, got %q", selectedModel)
	}
	for header, want := range map[string]string{
		"X-ModelMux-Requested-Model":   "coding-balanced",
		"X-ModelMux-Selected-Model":    "flagship",
		"X-ModelMux-Selected-Provider": "provider",
		"X-ModelMux-Group":             "coding-balanced",
	} {
		if got := resp.Header.Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
		}
	}
}

func TestGroupCapabilityMismatchReturnsBadRequest(t *testing.T) {
	cfg := groupFeatureConfig("https://example.test/v1")
	cfg.Models[1].Capabilities.Tools = groupFeatureBool(false)
	router, err := NewRouterService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"coding-balanced","messages":[],"tools":[{"type":"function","function":{"name":"read_file"}}]}`))
	_, err = router.HandleChatCompletion(context.Background(), req)
	var proxyErr *ProxyError
	if !errors.As(err, &proxyErr) {
		t.Fatalf("expected ProxyError, got %T: %v", err, err)
	}
	if proxyErr.HTTPStatus != http.StatusBadRequest || proxyErr.Code != "group_capability_mismatch" {
		t.Fatalf("unexpected error: %+v", proxyErr)
	}
}

func TestGroupRequiredCapabilitiesRejectHeterogeneousMembers(t *testing.T) {
	cfg := groupFeatureConfig("https://example.test/v1")
	cfg.ModelGroups[0].RequiredCapabilities = []string{"tools"}
	_, err := NewRouterService(cfg)
	if err == nil || !strings.Contains(err.Error(), "requires capability tools") {
		t.Fatalf("expected required-capability validation error, got %v", err)
	}
}

func TestConsistentHashSelectionIsStable(t *testing.T) {
	cfg := groupFeatureConfig("https://example.test/v1")
	cfg.ModelGroups[0].Strategy = "consistent_hash"
	router, err := NewRouterService(cfg)
	if err != nil {
		t.Fatal(err)
	}

	var selected string
	for i := 0; i < 10; i++ {
		_, model, ok := router.selectGroupMemberForRequest("coding-balanced", map[string]struct{}{}, "/chat/completions", []byte(`{"messages":[]}`), "conversation-42")
		if !ok {
			t.Fatal("expected a selected group member")
		}
		if i == 0 {
			selected = model.ID
		} else if model.ID != selected {
			t.Fatalf("selection changed from %s to %s", selected, model.ID)
		}
	}

	_, fallback, ok := router.selectGroupMemberForRequest("coding-balanced", map[string]struct{}{}, "/chat/completions", []byte(`{"messages":[]}`), "")
	if !ok || fallback.ID != "economy" {
		t.Fatalf("expected no-session fallback to priority member economy, got %+v", fallback)
	}
}
