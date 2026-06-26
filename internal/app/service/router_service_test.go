package service

import (
	"context"
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
