package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/storage"
)

func TestRouterRejectsRequestsWhenStorageIsUnhealthy(t *testing.T) {
	store, err := storage.New(filepath.Join(t.TempDir(), "modelmux.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	router, err := NewRouterServiceWithStorage(config.Default(), store)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"mimo-v2.5-pro","messages":[]}`))
	_, err = router.HandleChatCompletion(context.Background(), req)
	var proxyErr *ProxyError
	if !errors.As(err, &proxyErr) {
		t.Fatalf("expected ProxyError, got %T: %v", err, err)
	}
	if proxyErr.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", proxyErr.HTTPStatus, http.StatusServiceUnavailable)
	}
	if proxyErr.Code != "persistent_storage_unavailable" {
		t.Fatalf("code = %q", proxyErr.Code)
	}
}
