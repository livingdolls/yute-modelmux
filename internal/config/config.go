package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App         AppConfig          `yaml:"app"`
	Server      ServerConfig       `yaml:"server"`
	Storage     StorageConfig      `yaml:"storage"`
	Cooldown    CooldownConfig     `yaml:"cooldown"`
	Retry       RetryConfig        `yaml:"retry"`
	HealthCheck HealthCheckConfig  `yaml:"health_check"`
	AI          AIConfig           `yaml:"ai"`
	Providers   []ProviderConfig   `yaml:"providers"`
	Models      []ModelConfig      `yaml:"models"`
	ModelGroups []ModelGroupConfig `yaml:"model_groups"`
	Keys        []KeyConfig        `yaml:"keys"`
}

type AppConfig struct {
	Name     string `yaml:"name"`
	LogLevel string `yaml:"log_level"`
}

type ServerConfig struct {
	Host               string      `yaml:"host"`
	Port               int         `yaml:"port"`
	ReadTimeoutSecond  int         `yaml:"read_timeout_seconds"`
	WriteTimeoutSecond int         `yaml:"write_timeout_seconds"`
	RequireAuth        bool        `yaml:"require_auth"`
	AuthTokenEnv       string      `yaml:"auth_token_env"`
	MaxRequestBodyMB   int         `yaml:"max_request_body_mb"`
	Admin              AdminConfig `yaml:"admin,omitempty"`
}

type AdminConfig struct {
	RequireAuth *bool `yaml:"require_auth,omitempty"`
}

type StorageConfig struct {
	Type          string `yaml:"type"`
	Path          string `yaml:"path"`
	RetentionDays int    `yaml:"retention_days"`
}

type CooldownConfig struct {
	RateLimitSeconds   int `yaml:"rate_limit_seconds"`
	ServerErrorSeconds int `yaml:"server_error_seconds"`
	TimeoutSeconds     int `yaml:"timeout_seconds"`
}

type RetryConfig struct {
	MaxRetryPerKey      int   `yaml:"max_retry_per_key"`
	MaxTotalAttempts    int   `yaml:"max_total_attempts"`
	BackoffMilliseconds []int `yaml:"backoff_milliseconds"`
}

type HealthCheckConfig struct {
	Enabled         bool `yaml:"enabled"`
	IntervalSeconds int  `yaml:"interval_seconds"`
	TimeoutSeconds  int  `yaml:"timeout_seconds"`
}

type AIConfig struct {
	Enabled      bool                  `yaml:"enabled"`
	Classifier   ClassifierConfig      `yaml:"classifier"`
	Guardrails   GuardrailConfig       `yaml:"guardrails"`
	RouteTrace   RouteTraceConfig      `yaml:"route_trace"`
	RoutingRules []AIRoutingRuleConfig `yaml:"routing_rules"`
	Prompts      PromptsConfig         `yaml:"prompts"`
	Memory       MemoryConfig          `yaml:"memory"`
}

type PromptsConfig struct {
	Dir string `yaml:"dir"`
}

type MemoryConfig struct {
	Enabled bool `yaml:"enabled"`
}

type ClassifierConfig struct {
	Enabled bool   `yaml:"enabled"`
	Mode    string `yaml:"mode"`
}

type GuardrailConfig struct {
	Enabled        bool `yaml:"enabled"`
	MaxPromptChars int  `yaml:"max_prompt_chars"`
}

type RouteTraceConfig struct {
	Enabled               bool `yaml:"enabled"`
	IncludeResponseHeader bool `yaml:"include_response_header"`
}

type AIRoutingRuleConfig struct {
	Description       string            `yaml:"description"`
	When              AIRoutingRuleWhen `yaml:"when"`
	UseModel          string            `yaml:"use_model"`
	UseGroup          string            `yaml:"use_group"`
	FallbackGroup     string            `yaml:"fallback_group"`
	RequireCapability []string          `yaml:"require_capability"`
}

type AIRoutingRuleWhen struct {
	Task         string `yaml:"task"`
	HasTools     *bool  `yaml:"has_tools"`
	HasVision    *bool  `yaml:"has_vision"`
	HasStreaming *bool  `yaml:"has_streaming"`
	IsChat       *bool  `yaml:"is_chat"`
	IsCompletion *bool  `yaml:"is_completion"`
}

type ProviderConfig struct {
	ID             string `yaml:"id"`
	Name           string `yaml:"name"`
	Type           string `yaml:"type"`
	BaseURL        string `yaml:"base_url"`
	AuthType       string `yaml:"auth_type"`
	AuthHeaderName string `yaml:"auth_header_name"`
	TimeoutSeconds int    `yaml:"timeout_seconds"`
	Enabled        bool   `yaml:"enabled"`
}

type ModelConfig struct {
	ID                    string                 `yaml:"id"`
	ProviderID            string                 `yaml:"provider_id"`
	ModelName             string                 `yaml:"model_name"`
	Strategy              string                 `yaml:"strategy"`
	Enabled               bool                   `yaml:"enabled"`
	RequestsPerMinute     int                    `yaml:"requests_per_minute"`
	MaxConcurrentRequests int                    `yaml:"max_concurrent_requests"`
	Capabilities          *ModelCapabilityConfig `yaml:"capabilities,omitempty"`
	Cost                  *CostConfig            `yaml:"cost,omitempty"`
}

type CostConfig struct {
	InputPer1M  float64 `yaml:"input_per_1m"`
	OutputPer1M float64 `yaml:"output_per_1m"`
}

type ModelCapabilityConfig struct {
	Chat        *bool `yaml:"chat,omitempty"`
	Completions *bool `yaml:"completions,omitempty"`
	Streaming   *bool `yaml:"streaming,omitempty"`
	Tools       *bool `yaml:"tools,omitempty"`
	Vision      *bool `yaml:"vision,omitempty"`
	JSONMode    *bool `yaml:"json_mode,omitempty"`
}

type KeyConfig struct {
	ID                    string `yaml:"id"`
	ProviderID            string `yaml:"provider_id"`
	ModelID               string `yaml:"model_id"`
	Name                  string `yaml:"name"`
	Value                 string `yaml:"value"`
	ValueEnv              string `yaml:"value_env"`
	SecretRef             string `yaml:"secret_ref"`
	Status                string `yaml:"status"`
	Priority              int    `yaml:"priority"`
	DailyRequestLimit     int    `yaml:"daily_request_limit"`
	DailyTokenLimit       int    `yaml:"daily_token_limit"`
	RequestsPerMinute     int    `yaml:"requests_per_minute"`
	TokensPerMinute       int    `yaml:"tokens_per_minute"`
	MaxConcurrentRequests int    `yaml:"max_concurrent_requests"`
}

type ModelGroupConfig struct {
	ID       string                   `yaml:"id"`
	Name     string                   `yaml:"name"`
	Strategy string                   `yaml:"strategy"`
	Enabled  bool                     `yaml:"enabled"`
	Members  []ModelGroupMemberConfig `yaml:"members"`
}

type ModelGroupMemberConfig struct {
	ModelID  string `yaml:"model_id"`
	KeyID    string `yaml:"key_id"`
	Priority int    `yaml:"priority"`
	Weight   int    `yaml:"weight"`
	Enabled  bool   `yaml:"enabled"`
}

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "./config.yaml"
	}
	return filepath.Join(home, ".config", "modelmux", "config.yaml")
}

func defaultStoragePath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "./modelmux.db"
	}
	return filepath.Join(home, ".local", "share", "modelmux", "modelmux.db")
}

func Default() *Config {
	return &Config{
		App:         AppConfig{Name: "modelmux", LogLevel: "info"},
		Server:      ServerConfig{Host: "127.0.0.1", Port: 8787, ReadTimeoutSecond: 60, WriteTimeoutSecond: 300, AuthTokenEnv: "MODELMUX_AUTH_TOKEN", MaxRequestBodyMB: 10},
		Storage:     StorageConfig{Type: "", Path: defaultStoragePath()},
		Cooldown:    CooldownConfig{RateLimitSeconds: 300, ServerErrorSeconds: 60, TimeoutSeconds: 60},
		Retry:       RetryConfig{MaxRetryPerKey: 1, MaxTotalAttempts: 5, BackoffMilliseconds: []int{300, 700, 1500}},
		HealthCheck: HealthCheckConfig{Enabled: false, IntervalSeconds: 300, TimeoutSeconds: 15},
		AI: AIConfig{
			Enabled:    false,
			Classifier: ClassifierConfig{Enabled: false, Mode: "heuristic"},
			Guardrails: GuardrailConfig{Enabled: false, MaxPromptChars: 0},
			RouteTrace: RouteTraceConfig{Enabled: false, IncludeResponseHeader: false},
			Prompts:    PromptsConfig{Dir: ""},
			Memory:     MemoryConfig{Enabled: false},
		},
		Providers:   []ProviderConfig{{ID: "mimo", Name: "Xiaomi MiMo", Type: "openai-compatible", BaseURL: "https://api.example.com/v1", AuthType: "bearer", TimeoutSeconds: 120, Enabled: true}},
		Models:      []ModelConfig{{ID: "mimo-v2.5-pro", ProviderID: "mimo", ModelName: "mimo-v2.5-pro", Strategy: "failover", Enabled: true}},
		ModelGroups: []ModelGroupConfig{{ID: "high-price", Name: "High Price Models", Strategy: "failover", Enabled: true, Members: []ModelGroupMemberConfig{{ModelID: "mimo-v2.5-pro", Priority: 1, Weight: 1, Enabled: true}}}},
		Keys:        []KeyConfig{{ID: "mimo-key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Name: "MiMo Personal 1", Value: "replace-with-api-key", Status: "active", Priority: 1}},
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := cfg.ResolveSecrets(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Save writes through a same-directory temp file and then renames it into place.
// Replacement atomicity follows the host OS filesystem semantics.
func Save(path string, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".modelmux-config-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTmp := true
	defer func() {
		if removeTmp {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	removeTmp = false
	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

func WriteDefault(path string) error {
	return Save(path, Default())
}

func (c *Config) ResolveSecrets() error {
	if c.Server.RequireAuth && c.Server.AuthTokenEnv != "" && os.Getenv(c.Server.AuthTokenEnv) == "" {
		return fmt.Errorf("server auth requires environment variable %q which is not set", c.Server.AuthTokenEnv)
	}
	if c.AdminRequireAuth() && c.Server.AuthTokenEnv != "" && os.Getenv(c.Server.AuthTokenEnv) == "" {
		return fmt.Errorf("server admin auth requires environment variable %q which is not set", c.Server.AuthTokenEnv)
	}
	for i := range c.Keys {
		if c.Keys[i].Value != "" || c.Keys[i].SecretRef != "" || c.Keys[i].ValueEnv == "" {
			continue
		}
		if os.Getenv(c.Keys[i].ValueEnv) == "" {
			return fmt.Errorf("key %s requires environment variable %q which is not set", c.Keys[i].ID, c.Keys[i].ValueEnv)
		}
	}
	return nil
}

type ValidationErrors []string

func (ve ValidationErrors) Error() string {
	return strings.Join(ve, "; ")
}

func (ve ValidationErrors) Errors() []string {
	return ve
}

func (c *Config) Validate() error {
	errs := c.collectValidationErrors()
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func (c *Config) ValidateAll() ValidationErrors {
	return c.collectValidationErrors()
}

func (c *Config) collectValidationErrors() ValidationErrors {
	var errs ValidationErrors
	if c.Server.Host == "" {
		errs = append(errs, "server.host is required")
	}
	if c.Server.Port <= 0 {
		errs = append(errs, "server.port must be greater than zero")
	}
	if c.Server.ReadTimeoutSecond < 0 {
		errs = append(errs, "server.read_timeout_seconds must be non-negative")
	}
	if c.Server.WriteTimeoutSecond < 0 {
		errs = append(errs, "server.write_timeout_seconds must be non-negative")
	}
	if c.Server.MaxRequestBodyMB < 0 {
		errs = append(errs, "server.max_request_body_mb must be non-negative")
	}
	if c.Server.RequireAuth && c.Server.AuthTokenEnv == "" {
		errs = append(errs, "server.auth_token_env is required when server.require_auth is true")
	}
	if c.AdminRequireAuth() && c.Server.AuthTokenEnv == "" {
		errs = append(errs, "server.auth_token_env is required when server.admin.require_auth is true")
	}
	if c.Storage.RetentionDays < 0 {
		errs = append(errs, "storage.retention_days must be non-negative")
	}
	if c.Cooldown.RateLimitSeconds < 0 {
		errs = append(errs, "cooldown.rate_limit_seconds must be non-negative")
	}
	if c.Cooldown.ServerErrorSeconds < 0 {
		errs = append(errs, "cooldown.server_error_seconds must be non-negative")
	}
	if c.Cooldown.TimeoutSeconds < 0 {
		errs = append(errs, "cooldown.timeout_seconds must be non-negative")
	}
	if c.Retry.MaxRetryPerKey < 0 {
		errs = append(errs, "retry.max_retry_per_key must be non-negative")
	}
	if c.Retry.MaxTotalAttempts < 0 {
		errs = append(errs, "retry.max_total_attempts must be non-negative")
	}
	for i, backoff := range c.Retry.BackoffMilliseconds {
		if backoff < 0 {
			errs = append(errs, fmt.Sprintf("retry.backoff_milliseconds[%d] must be non-negative", i))
		}
	}
	if c.HealthCheck.IntervalSeconds < 0 {
		errs = append(errs, "health_check.interval_seconds must be non-negative")
	}
	if c.HealthCheck.TimeoutSeconds < 0 {
		errs = append(errs, "health_check.timeout_seconds must be non-negative")
	}
	if c.AI.Guardrails.MaxPromptChars < 0 {
		errs = append(errs, "ai.guardrails.max_prompt_chars must be non-negative")
	}
	providerIDs := map[string]struct{}{}
	for _, p := range c.Providers {
		if p.ID == "" {
			errs = append(errs, "provider.id is required")
			continue
		}
		if _, exists := providerIDs[p.ID]; exists {
			errs = append(errs, "duplicate provider id "+p.ID)
		}
		providerIDs[p.ID] = struct{}{}
		if p.Enabled {
			if p.BaseURL == "" {
				errs = append(errs, "provider "+p.ID+" base_url is required")
			}
			if p.BaseURL != "" && !strings.HasPrefix(p.BaseURL, "http://") && !strings.HasPrefix(p.BaseURL, "https://") {
				errs = append(errs, "provider "+p.ID+" base_url must start with http:// or https://")
			}
			validTypes := map[string]bool{"openai-compatible": true, "anthropic": true, "gemini": true, "custom": true}
			if p.Type != "" && !validTypes[p.Type] {
				errs = append(errs, "provider "+p.ID+" type "+p.Type+" is not valid; must be one of: openai-compatible, anthropic, gemini, custom")
			}
			if p.AuthType != "" && p.AuthType != "bearer" && p.AuthType != "header" {
				errs = append(errs, "provider "+p.ID+" auth_type must be bearer or header")
			}
			if p.AuthType == "header" && p.AuthHeaderName == "" {
				errs = append(errs, "provider "+p.ID+" auth_header_name is required when auth_type is header")
			}
			if p.TimeoutSeconds <= 0 || p.TimeoutSeconds > 3600 {
				errs = append(errs, "provider "+p.ID+" timeout_seconds must be between 1 and 3600")
			}
		}
	}
	modelIDs := map[string]struct{}{}
	modelByProviderID := map[string]string{}
	for _, m := range c.Models {
		if m.ID == "" {
			errs = append(errs, "model.id is required")
			continue
		}
		if _, exists := modelIDs[m.ID]; exists {
			errs = append(errs, "duplicate model id "+m.ID)
		}
		modelIDs[m.ID] = struct{}{}
		modelByProviderID[m.ID] = m.ProviderID
		if _, ok := providerIDs[m.ProviderID]; !ok && m.ProviderID != "" {
			errs = append(errs, "model "+m.ID+" references unknown provider "+m.ProviderID)
		}
		if m.Enabled {
			if m.ModelName == "" {
				errs = append(errs, "model "+m.ID+" model_name is required")
			}
			validStrategies := map[string]bool{"failover": true, "round_robin": true, "least_error": true, "least_used": true}
			if m.Strategy != "" && !validStrategies[m.Strategy] {
				errs = append(errs, "model "+m.ID+" strategy "+m.Strategy+" is not valid; must be one of: failover, round_robin, least_error, least_used")
			}
		}
		if m.RequestsPerMinute < 0 {
			errs = append(errs, "model "+m.ID+" requests_per_minute must be non-negative")
		}
		if m.MaxConcurrentRequests < 0 {
			errs = append(errs, "model "+m.ID+" max_concurrent_requests must be non-negative")
		}
		if m.Cost != nil {
			if m.Cost.InputPer1M < 0 {
				errs = append(errs, "model "+m.ID+" cost.input_per_1m must be non-negative")
			}
			if m.Cost.OutputPer1M < 0 {
				errs = append(errs, "model "+m.ID+" cost.output_per_1m must be non-negative")
			}
		}
	}
	groupIDs := map[string]struct{}{}
	for _, g := range c.ModelGroups {
		if g.ID == "" {
			errs = append(errs, "model_group.id is required")
			continue
		}
		if _, exists := groupIDs[g.ID]; exists {
			errs = append(errs, "duplicate model group id "+g.ID)
		}
		if _, exists := modelIDs[g.ID]; exists {
			errs = append(errs, "model group id "+g.ID+" conflicts with model id")
		}
		groupIDs[g.ID] = struct{}{}
		if g.Enabled {
			if g.Name == "" {
				errs = append(errs, "model group "+g.ID+" name is required")
			}
			validStrategies := map[string]bool{"failover": true, "round_robin": true, "weighted": true}
			if g.Strategy != "" && !validStrategies[g.Strategy] {
				errs = append(errs, "model group "+g.ID+" strategy "+g.Strategy+" is not valid; must be one of: failover, round_robin, weighted")
			}
		}
		if len(g.Members) == 0 {
			if g.Enabled {
				errs = append(errs, "enabled model group "+g.ID+" must have at least one member")
			}
			continue
		}
	}
	keyIDs := map[string]struct{}{}
	for _, k := range c.Keys {
		if k.ID == "" {
			errs = append(errs, "key.id is required")
			continue
		}
		if _, exists := keyIDs[k.ID]; exists {
			errs = append(errs, "duplicate key id "+k.ID)
		}
		keyIDs[k.ID] = struct{}{}
		if _, ok := providerIDs[k.ProviderID]; !ok && k.ProviderID != "" {
			errs = append(errs, "key "+k.ID+" references unknown provider "+k.ProviderID)
		}
		if _, ok := modelIDs[k.ModelID]; !ok && k.ModelID != "" {
			errs = append(errs, "key "+k.ID+" references unknown model "+k.ModelID)
		}
		validStatuses := map[string]bool{"": true, "active": true, "cooldown": true, "invalid": true, "limited": true, "disabled": true}
		if !validStatuses[k.Status] {
			errs = append(errs, "key "+k.ID+" status "+k.Status+" is not valid; must be one of: active, cooldown, invalid, limited, disabled")
		}
		if k.Priority < 0 {
			errs = append(errs, "key "+k.ID+" priority must be non-negative")
		}
		if k.DailyRequestLimit < 0 {
			errs = append(errs, "key "+k.ID+" daily_request_limit must be non-negative")
		}
		if k.DailyTokenLimit < 0 {
			errs = append(errs, "key "+k.ID+" daily_token_limit must be non-negative")
		}
		if k.RequestsPerMinute < 0 {
			errs = append(errs, "key "+k.ID+" requests_per_minute must be non-negative")
		}
		if k.TokensPerMinute < 0 {
			errs = append(errs, "key "+k.ID+" tokens_per_minute must be non-negative")
		}
		if k.MaxConcurrentRequests < 0 {
			errs = append(errs, "key "+k.ID+" max_concurrent_requests must be non-negative")
		}
		if k.ProviderID != "" && k.ModelID != "" {
			if modelProviderID := modelByProviderID[k.ModelID]; modelProviderID != k.ProviderID {
				errs = append(errs, "key "+k.ID+" provider "+k.ProviderID+" does not match model "+k.ModelID+" provider "+modelProviderID)
			}
		}
		if k.Value == "" && k.ValueEnv == "" && k.SecretRef == "" {
			errs = append(errs, "key "+k.ID+" has no value; set keys[].value or keys[].value_env or keys[].secret_ref in config")
		}
	}
	for _, g := range c.ModelGroups {
		for _, member := range g.Members {
			if member.Priority < 0 {
				errs = append(errs, "model group "+g.ID+" member priority must be non-negative")
			}
			if member.Weight < 0 {
				errs = append(errs, "model group "+g.ID+" member weight must be non-negative")
			}
			hasModel := member.ModelID != ""
			hasKey := member.KeyID != ""
			if hasModel == hasKey {
				errs = append(errs, "model group "+g.ID+" member must set exactly one of model_id or key_id")
				continue
			}
			if hasModel {
				if _, ok := modelIDs[member.ModelID]; !ok {
					errs = append(errs, "model group "+g.ID+" references unknown model "+member.ModelID)
				}
				continue
			}
			if _, ok := keyIDs[member.KeyID]; !ok {
				errs = append(errs, "model group "+g.ID+" references unknown key "+member.KeyID)
			}
		}
	}
	for i, rule := range c.AI.RoutingRules {
		prefix := fmt.Sprintf("ai.routing_rules[%d]", i)
		if rule.UseModel != "" && rule.UseGroup != "" {
			errs = append(errs, prefix+" must set at most one of use_model or use_group")
		}
		if rule.UseModel != "" {
			if _, ok := modelIDs[rule.UseModel]; !ok {
				errs = append(errs, prefix+" use_model references unknown model "+rule.UseModel)
			}
		}
		if rule.UseGroup != "" {
			if _, ok := groupIDs[rule.UseGroup]; !ok {
				errs = append(errs, prefix+" use_group references unknown group "+rule.UseGroup)
			}
		}
		if rule.FallbackGroup != "" {
			if _, ok := groupIDs[rule.FallbackGroup]; !ok {
				errs = append(errs, prefix+" fallback_group references unknown group "+rule.FallbackGroup)
			}
		}
		validCaps := map[string]bool{"tools": true, "json_mode": true, "vision": true, "streaming": true, "chat": true, "completions": true}
		for _, cap := range rule.RequireCapability {
			if !validCaps[cap] {
				errs = append(errs, prefix+" require_capability "+cap+" is not valid; must be one of: tools, json_mode, vision, streaming, chat, completions")
			}
		}
	}
	return errs
}

func (c *Config) AuthToken() string {
	if c.Server.AuthTokenEnv == "" {
		return ""
	}
	return os.Getenv(c.Server.AuthTokenEnv)
}

func (c *Config) AdminRequireAuth() bool {
	if c.Server.Admin.RequireAuth == nil {
		return true
	}
	return *c.Server.Admin.RequireAuth
}

func NormalizeBaseURL(base string) string {
	return strings.TrimRight(base, "/")
}
