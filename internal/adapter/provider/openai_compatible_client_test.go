package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

func TestForwardChatCompletionPreservesBasePathAndRewritesModel(t *testing.T) {
	var gotPath string
	var gotAuth string
	var gotModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
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

	provider := domain.Provider{BaseURL: server.URL + "/v1", AuthType: domain.AuthTypeBearer, TimeoutSeconds: 5}
	model := domain.Model{ID: "deepseek-chat", ModelName: "deepseek/deepseek-chat"}
	key := domain.APIKey{Value: "provider-secret"}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"deepseek-chat","messages":[]}`))
	req.Header.Set("Authorization", "Bearer local-token")

	resp, err := New().ForwardChatCompletion(context.Background(), provider, model, key, req)
	if err != nil {
		t.Fatalf("forward failed: %v", err)
	}
	defer resp.Body.Close()

	if gotPath != "/v1/chat/completions" {
		t.Fatalf("expected /v1/chat/completions, got %s", gotPath)
	}
	if gotAuth != "Bearer provider-secret" {
		t.Fatalf("expected provider auth header, got %q", gotAuth)
	}
	if gotModel != "deepseek/deepseek-chat" {
		t.Fatalf("expected provider model name, got %s", gotModel)
	}
}

func TestForwardChatCompletionCustomAuthDoesNotLeakLocalAuthorization(t *testing.T) {
	var gotAuth string
	var gotAPIKey string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotAPIKey = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := domain.Provider{BaseURL: server.URL + "/v1", AuthType: domain.AuthTypeHeader, AuthHeaderName: "X-API-Key", TimeoutSeconds: 5}
	model := domain.Model{ID: "custom", ModelName: "custom"}
	key := domain.APIKey{Value: "provider-secret"}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"custom","messages":[]}`))
	req.Header.Set("Authorization", "Bearer local-token")

	resp, err := New().ForwardChatCompletion(context.Background(), provider, model, key, req)
	if err != nil {
		t.Fatalf("forward failed: %v", err)
	}
	defer resp.Body.Close()

	if gotAuth != "" {
		t.Fatalf("local authorization header leaked upstream: %q", gotAuth)
	}
	if gotAPIKey != "provider-secret" {
		t.Fatalf("expected custom provider key, got %q", gotAPIKey)
	}
}
