package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
)

func TestModelsExposeModelGroupMetadata(t *testing.T) {
	cfg := config.Default()
	cfg.ModelGroups[0].Description = "Flagship coding models"
	cfg.ModelGroups[0].Strategy = "consistent_hash"
	cfg.ModelGroups[0].RequiredCapabilities = []string{"chat", "streaming", "tools"}
	cfg.ModelGroups[0].ContextWindow = 128000
	cfg.ModelGroups[0].MaxOutputTokens = 32000
	router, err := service.NewRouterService(cfg)
	if err != nil {
		t.Fatal(err)
	}
	server := New(router, cfg)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	server.modelsHandler(rec, req)

	var payload struct {
		Data []modelListItem `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, item := range payload.Data {
		if item.ID != "high-price" {
			continue
		}
		if item.ModelMuxType != "group" || item.Strategy != "consistent_hash" || item.Description != "Flagship coding models" {
			t.Fatalf("unexpected group metadata: %+v", item)
		}
		if !item.Capabilities.Tools || item.ContextWindow != 128000 || item.MaxOutputTokens != 32000 {
			t.Fatalf("unexpected group capabilities or limits: %+v", item)
		}
		return
	}
	t.Fatal("group high-price not found")
}
