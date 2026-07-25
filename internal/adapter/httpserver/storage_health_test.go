package httpserver

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/storage"
)

func TestUnhealthyStorageDegradesHealthAndBlocksTraffic(t *testing.T) {
	cfg := config.Default()
	router, err := service.NewRouterService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.New(filepath.Join(t.TempDir(), "modelmux.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveKeyRuntime(storage.KeyRuntimeRecord{KeyID: "key-1"}); err == nil {
		t.Fatal("expected storage failure")
	}

	server := New(router, cfg)
	server.SetStore(store)

	healthReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	healthRec := httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("health status = %d, want %d", healthRec.Code, http.StatusServiceUnavailable)
	}

	modelsReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	modelsRec := httptest.NewRecorder()
	server.srv.Handler.ServeHTTP(modelsRec, modelsReq)
	if modelsRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("models status = %d, want %d", modelsRec.Code, http.StatusServiceUnavailable)
	}
}
