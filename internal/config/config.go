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
	App       AppConfig        `yaml:"app"`
	Server    ServerConfig     `yaml:"server"`
	Cooldown  CooldownConfig   `yaml:"cooldown"`
	Retry     RetryConfig      `yaml:"retry"`
	Providers []ProviderConfig `yaml:"providers"`
	Models    []ModelConfig    `yaml:"models"`
	Keys      []KeyConfig      `yaml:"keys"`
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

func DefaultConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "./config.yaml"
	}
	return filepath.Join(home, ".config", "modelmux", "config.yaml")
}

func Default() *Config {
	return &Config{
		App:       AppConfig{Name: "modelmux", LogLevel: "info"},
		Server:    ServerConfig{Host: "127.0.0.1", Port: 8787, ReadTimeoutSecond: 60, WriteTimeoutSecond: 300},
		Cooldown:  CooldownConfig{RateLimitSeconds: 300, ServerErrorSeconds: 60, TimeoutSeconds: 60},
		Retry:     RetryConfig{MaxRetryPerKey: 1, MaxTotalAttempts: 5, BackoffMilliseconds: []int{300, 700, 1500}},
		Providers: []ProviderConfig{{ID: "mimo", Name: "Xiaomi MiMo", Type: "openai-compatible", BaseURL: "https://api.example.com/v1", AuthType: "bearer", TimeoutSeconds: 120, Enabled: true}},
		Models:    []ModelConfig{{ID: "mimo-v2.5-pro", ProviderID: "mimo", ModelName: "mimo-v2.5-pro", Strategy: "failover", Enabled: true}},
		Keys:      []KeyConfig{{ID: "mimo-key-1", ProviderID: "mimo", ModelID: "mimo-v2.5-pro", Name: "MiMo Personal 1", ValueEnv: "MIMO_KEY_1", Status: "active", Priority: 1}},
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
		value := os.Getenv(c.Keys[i].ValueEnv)
		if value == "" {
			return fmt.Errorf("environment variable %s is empty for key %s", c.Keys[i].ValueEnv, c.Keys[i].ID)
		}
		c.Keys[i].Value = value
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
	providerIDs := map[string]struct{}{}
	for _, p := range c.Providers {
		if p.ID == "" {
			return errors.New("provider.id is required")
		}
		providerIDs[p.ID] = struct{}{}
	}
	modelIDs := map[string]struct{}{}
	for _, m := range c.Models {
		if m.ID == "" {
			return errors.New("model.id is required")
		}
		if _, ok := providerIDs[m.ProviderID]; !ok {
			return fmt.Errorf("model %s references unknown provider %s", m.ID, m.ProviderID)
		}
		modelIDs[m.ID] = struct{}{}
	}
	for _, k := range c.Keys {
		if k.ID == "" {
			return errors.New("key.id is required")
		}
		if _, ok := providerIDs[k.ProviderID]; !ok {
			return fmt.Errorf("key %s references unknown provider %s", k.ID, k.ProviderID)
		}
		if _, ok := modelIDs[k.ModelID]; !ok {
			return fmt.Errorf("key %s references unknown model %s", k.ID, k.ModelID)
		}
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
