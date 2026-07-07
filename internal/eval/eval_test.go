package eval

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
)

func TestLoadSuite(t *testing.T) {
	content := `name: test-suite
targets:
  - model: test-model
cases:
  - name: hello
    input: Say hello
`
	dir := t.TempDir()
	path := filepath.Join(dir, "suite.yaml")
	os.WriteFile(path, []byte(content), 0o644)

	suite, err := LoadSuite(path)
	if err != nil {
		t.Fatalf("LoadSuite failed: %v", err)
	}
	if suite.Name != "test-suite" {
		t.Fatalf("expected test-suite, got %s", suite.Name)
	}
	if len(suite.Targets) != 1 || suite.Targets[0].Model != "test-model" {
		t.Fatalf("expected 1 target test-model, got %+v", suite.Targets)
	}
	if len(suite.Cases) != 1 || suite.Cases[0].Name != "hello" {
		t.Fatalf("expected 1 case hello, got %+v", suite.Cases)
	}
}

func TestRunSuite(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"choices":[{"message":{"content":"Hello!"}}]}`))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.Providers[0].BaseURL = server.URL + "/v1"
	cfg.Models[0].ModelName = cfg.Models[0].ID
	router, _ := service.NewRouterService(cfg)

	suite := &Suite{
		Name: "quick-test",
		Targets: []Target{
			{Model: "mimo-v2.5-pro"},
		},
		Cases: []Case{
			{Name: "greeting", Input: "Hello", TimeoutSeconds: 5},
		},
	}

	result, err := RunSuite(context.Background(), suite, router)
	if err != nil {
		t.Fatalf("RunSuite failed: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	r := result.Results[0]
	if r.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", r.StatusCode, r.Error)
	}
	if r.ResponseHash == "" {
		t.Fatal("expected response hash")
	}
	if r.LatencyMs < 0 {
		t.Fatalf("expected non-negative latency, got %d", r.LatencyMs)
	}
	if result.StartedAt.After(result.FinishedAt) {
		t.Fatal("started_at must be before finished_at")
	}
}

func TestRunSuiteHandlesError(t *testing.T) {
	cfg := config.Default()
	cfg.Providers[0].BaseURL = "https://localhost:1/v1"
	router, _ := service.NewRouterService(cfg)

	suite := &Suite{
		Name: "error-test",
		Targets: []Target{
			{Model: "mimo-v2.5-pro"},
		},
		Cases: []Case{
			{Name: "will-fail", Input: "Hi", TimeoutSeconds: 1},
		},
	}

	result, err := RunSuite(context.Background(), suite, router)
	if err != nil {
		t.Fatalf("RunSuite should not fail: %v", err)
	}
	r := result.Results[0]
	if r.Error == "" {
		t.Fatal("expected error for unreachable upstream")
	}
}

func TestLoadSuiteValidationErrors(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		content string
	}{
		{"no name", "targets: [{model: x}]\ncases: [{name: a, input: hi}]"},
		{"no targets", "name: x\ncases: [{name: a, input: hi}]"},
		{"no cases", "name: x\ntargets: [{model: y}]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name+".yaml")
			os.WriteFile(path, []byte(tt.content), 0o644)
			_, err := LoadSuite(path)
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
