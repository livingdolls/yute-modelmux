package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/config"
)

func TestHealthCheckerMarksFailedKeyInvalid(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = ts.URL + "/v1"
	cfg.HealthCheck = config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 1,
		TimeoutSeconds:  5,
	}
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "bad-key", Status: "active", Priority: 1},
	}

	rs, _ := NewRouterService(cfg)
	hc := NewHealthChecker(rs, cfg.HealthCheck)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	hc.Start(ctx)

	time.Sleep(1500 * time.Millisecond)
	hc.Stop()

	keys := rs.ListKeys()
	if keys[0].Status != "invalid" {
		t.Fatalf("expected key invalid after health check failure, got %s", keys[0].Status)
	}
}

func TestHealthCheckerReenablesRecoveredKey(t *testing.T) {
	callCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount <= 2 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = ts.URL + "/v1"
	cfg.HealthCheck = config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 1,
		TimeoutSeconds:  5,
	}
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "key", Status: "active", Priority: 1},
	}

	rs, _ := NewRouterService(cfg)
	hc := NewHealthChecker(rs, cfg.HealthCheck)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	hc.Start(ctx)

	time.Sleep(3500 * time.Millisecond)
	hc.Stop()

	keys := rs.ListKeys()
	if keys[0].Status != "active" {
		t.Fatalf("expected key active after recovery, got %s", keys[0].Status)
	}
}

func TestHealthCheckerSkipsDisabledKey(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = ts.URL + "/v1"
	cfg.HealthCheck = config.HealthCheckConfig{
		Enabled:         true,
		IntervalSeconds: 1,
		TimeoutSeconds:  5,
	}
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "ok", Status: "disabled", Priority: 1},
	}

	rs, _ := NewRouterService(cfg)
	hc := NewHealthChecker(rs, cfg.HealthCheck)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	hc.Start(ctx)

	time.Sleep(1500 * time.Millisecond)
	hc.Stop()

	keys := rs.ListKeys()
	if keys[0].Status != "disabled" {
		t.Fatalf("expected disabled key to stay disabled, got %s", keys[0].Status)
	}
}

func TestHealthCheckerDoesNotStartWhenDisabled(t *testing.T) {
	cfg := config.Default()
	cfg.HealthCheck = config.HealthCheckConfig{
		Enabled:         false,
		IntervalSeconds: 1,
		TimeoutSeconds:  5,
	}
	cfg.Keys = []config.KeyConfig{
		{ID: "key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Value: "ok", Status: "active", Priority: 1},
	}

	rs, _ := NewRouterService(cfg)
	hc := NewHealthChecker(rs, cfg.HealthCheck)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	hc.Start(ctx)

	if hc.cancel != nil {
		t.Fatal("expected no cancel func when disabled")
	}
}
