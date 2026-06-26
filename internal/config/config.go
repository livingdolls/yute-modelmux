package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App         AppConfig          `yaml:"app"`
	Server      ServerConfig       `yaml:"server"`
	Cooldown    CooldownConfig     `yaml:"cooldown"`
	Retry       RetryConfig        `yaml:"retry"`
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
	Host               string `yaml:"host"`
	Port               int    `yaml:"port"`
	ReadTimeoutSecond  int    `yaml:"read_timeout_seconds"`
	WriteTimeoutSecond int    `yaml:"write_timeout_seconds"`
	RequireAuth        bool   `yaml:"require_auth"`
	AuthTokenEnv       string `yaml:"auth_token_env"`
	MaxRequestBodyMB   int    `yaml:"max_request_body_mb"`
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
	ID         string `yaml:"id"`
	ProviderID string `yaml:"provider_id"`
	ModelName  string `yaml:"model_name"`
	Strategy   string `yaml:"strategy"`
	Enabled    bool   `yaml:"enabled"`
}

type KeyConfig struct {
	ID         string `yaml:"id"`
	ProviderID string `yaml:"provider_id"`
	ModelID    string `yaml:"model_id"`
	Name       string `yaml:"name"`
	Value      string `yaml:"value"`
	ValueEnv   string `yaml:"value_env"`
	Status     string `yaml:"status"`
	Priority   int    `yaml:"priority"`
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

func Default() *Config {
	return &Config{
		App:         AppConfig{Name: "modelmux", LogLevel: "info"},
		Server:      ServerConfig{Host: "127.0.0.1", Port: 8787, ReadTimeoutSecond: 60, WriteTimeoutSecond: 300, MaxRequestBodyMB: 10},
		Cooldown:    CooldownConfig{RateLimitSeconds: 300, ServerErrorSeconds: 60, TimeoutSeconds: 60},
		Retry:       RetryConfig{MaxRetryPerKey: 1, MaxTotalAttempts: 5, BackoffMilliseconds: []int{300, 700, 1500}},
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
	if err := cfg.ResolveSecrets(); err != nil {
		return nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func Save(path string, cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func WriteDefault(path string) error {
	return Save(path, Default())
}

func (c *Config) ResolveSecrets() error {
	for i := range c.Keys {
		if c.Keys[i].Value != "" || c.Keys[i].ValueEnv == "" {
			continue
		}
		if os.Getenv(c.Keys[i].ValueEnv) == "" {
			return fmt.Errorf("key %s requires environment variable %q which is not set", c.Keys[i].ID, c.Keys[i].ValueEnv)
		}
	}
	return nil
}

func (c *Config) Validate() error {
	if c.Server.Host == "" {
		return errors.New("server.host is required")
	}
	if c.Server.Port <= 0 {
		return errors.New("server.port must be greater than zero")
	}
	if c.Server.RequireAuth && c.Server.AuthTokenEnv == "" {
		return errors.New("server.auth_token_env is required when server.require_auth is true")
	}
	providerIDs := map[string]struct{}{}
	for _, p := range c.Providers {
		if p.ID == "" {
			return errors.New("provider.id is required")
		}
		if _, exists := providerIDs[p.ID]; exists {
			return fmt.Errorf("duplicate provider id %s", p.ID)
		}
		providerIDs[p.ID] = struct{}{}
	}
	modelIDs := map[string]struct{}{}
	modelByProviderID := map[string]string{}
	for _, m := range c.Models {
		if m.ID == "" {
			return errors.New("model.id is required")
		}
		if _, exists := modelIDs[m.ID]; exists {
			return fmt.Errorf("duplicate model id %s", m.ID)
		}
		if _, ok := providerIDs[m.ProviderID]; !ok {
			return fmt.Errorf("model %s references unknown provider %s", m.ID, m.ProviderID)
		}
		modelIDs[m.ID] = struct{}{}
		modelByProviderID[m.ID] = m.ProviderID
	}
	groupIDs := map[string]struct{}{}
	for _, g := range c.ModelGroups {
		if g.ID == "" {
			return errors.New("model_group.id is required")
		}
		if _, exists := groupIDs[g.ID]; exists {
			return fmt.Errorf("duplicate model group id %s", g.ID)
		}
		if _, exists := modelIDs[g.ID]; exists {
			return fmt.Errorf("model group id %s conflicts with model id", g.ID)
		}
		if len(g.Members) == 0 {
			if g.Enabled {
				return fmt.Errorf("enabled model group %s must have at least one member", g.ID)
			}
			groupIDs[g.ID] = struct{}{}
			continue
		}
		for _, member := range g.Members {
			if member.ModelID == "" {
				return fmt.Errorf("model group %s has member without model_id", g.ID)
			}
			if _, ok := modelIDs[member.ModelID]; !ok {
				return fmt.Errorf("model group %s references unknown model %s", g.ID, member.ModelID)
			}
		}
		groupIDs[g.ID] = struct{}{}
	}
	keyIDs := map[string]struct{}{}
	for _, k := range c.Keys {
		if k.ID == "" {
			return errors.New("key.id is required")
		}
		if _, exists := keyIDs[k.ID]; exists {
			return fmt.Errorf("duplicate key id %s", k.ID)
		}
		if _, ok := providerIDs[k.ProviderID]; !ok {
			return fmt.Errorf("key %s references unknown provider %s", k.ID, k.ProviderID)
		}
		if _, ok := modelIDs[k.ModelID]; !ok {
			return fmt.Errorf("key %s references unknown model %s", k.ID, k.ModelID)
		}
		if modelProviderID := modelByProviderID[k.ModelID]; modelProviderID != k.ProviderID {
			return fmt.Errorf("key %s provider %s does not match model %s provider %s", k.ID, k.ProviderID, k.ModelID, modelProviderID)
		}
		if k.Value == "" {
			if k.ValueEnv != "" {
				continue
			}
			return fmt.Errorf("key %s has no value; set keys[].value in config", k.ID)
		}
		keyIDs[k.ID] = struct{}{}
	}
	return nil
}

func (c *Config) AuthToken() string {
	if c.Server.AuthTokenEnv == "" {
		return ""
	}
	return os.Getenv(c.Server.AuthTokenEnv)
}

func NormalizeBaseURL(base string) string {
	return strings.TrimRight(base, "/")
}
