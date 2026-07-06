package eval

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"gopkg.in/yaml.v3"
)

type Suite struct {
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Targets     []Target `yaml:"targets"`
	Cases       []Case   `yaml:"cases"`
}

type Target struct {
	Model string `yaml:"model"`
	Group string `yaml:"group"`
}

type Case struct {
	Name           string   `yaml:"name"`
	Input          string   `yaml:"input"`
	ExpectedMode   string   `yaml:"expected_mode"`
	TimeoutSeconds int      `yaml:"timeout_seconds"`
	Tags           []string `yaml:"tags"`
}

type RunResult struct {
	RunID      string        `yaml:"run_id"`
	SuiteName  string        `yaml:"suite_name"`
	StartedAt  time.Time     `yaml:"started_at"`
	FinishedAt time.Time     `yaml:"finished_at"`
	Results    []CaseResult  `yaml:"results"`
}

type CaseResult struct {
	CaseName     string `yaml:"case_name"`
	TargetModel  string `yaml:"target_model"`
	TargetGroup  string `yaml:"target_group"`
	StatusCode   int    `yaml:"status_code"`
	LatencyMs    int64  `yaml:"latency_ms"`
	ResponseHash string `yaml:"response_hash"`
	Error        string `yaml:"error,omitempty"`
}

func LoadSuite(path string) (*Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var suite Suite
	if err := yaml.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("eval: parse suite %s: %w", path, err)
	}
	if suite.Name == "" {
		return nil, fmt.Errorf("eval: suite name is required")
	}
	if len(suite.Cases) == 0 {
		return nil, fmt.Errorf("eval: suite must have at least one case")
	}
	if len(suite.Targets) == 0 {
		return nil, fmt.Errorf("eval: suite must have at least one target")
	}
	return &suite, nil
}

func RunSuite(ctx context.Context, suite *Suite, cfg *config.Config) (*RunResult, error) {
	router, err := service.NewRouterService(cfg)
	if err != nil {
		return nil, err
	}

	run := &RunResult{
		RunID:     fmt.Sprintf("run-%d", time.Now().UnixNano()),
		SuiteName: suite.Name,
		StartedAt: time.Now(),
	}

	for _, target := range suite.Targets {
		targetID := target.Model
		if target.Group != "" {
			targetID = target.Group
		}
		for _, c := range suite.Cases {
			timeout := time.Duration(c.TimeoutSeconds) * time.Second
			if timeout <= 0 {
				timeout = 30 * time.Second
			}
			caseCtx, cancel := context.WithTimeout(ctx, timeout)

			result := runCase(caseCtx, router, c, targetID)
			cancel()
			result.TargetModel = target.Model
			result.TargetGroup = target.Group
			run.Results = append(run.Results, result)
		}
	}

	run.FinishedAt = time.Now()
	return run, nil
}

func runCase(ctx context.Context, router *service.RouterService, c Case, targetID string) CaseResult {
	result := CaseResult{CaseName: c.Name}

	modelBytes, _ := json.Marshal(targetID)
	inputBytes, _ := json.Marshal(c.Input)
	body := fmt.Sprintf(`{"model":%s,"messages":[{"role":"user","content":%s}]}`, string(modelBytes), string(inputBytes))
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "http://localhost/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	resp, err := router.HandleChatCompletion(ctx, req)
	result.LatencyMs = time.Since(started).Milliseconds()

	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = fmt.Sprintf("read body: %v", err)
		return result
	}

	hash := sha256.Sum256(respBody)
	result.ResponseHash = fmt.Sprintf("%x", hash[:8])

	if resp.StatusCode >= 400 {
		result.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(respBody[:minInt(len(respBody), 200)]))
	}

	return result
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
