package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

type openCodeGenerateOptions struct {
	Output       string
	BaseURL      string
	ProviderID   string
	ProviderName string
	DefaultModel string
	APIKeyEnv    string
	Force        bool
}

type openCodeConfig struct {
	Schema   string                      `json:"$schema"`
	Model    string                      `json:"model"`
	Provider map[string]openCodeProvider `json:"provider"`
}

type openCodeProvider struct {
	NPM     string                      `json:"npm"`
	Name    string                      `json:"name"`
	Options openCodeProviderOptions     `json:"options"`
	Models  map[string]openCodeModel    `json:"models"`
}

type openCodeProviderOptions struct {
	BaseURL string `json:"baseURL"`
	APIKey  string `json:"apiKey,omitempty"`
}

type openCodeModel struct {
	Name  string              `json:"name"`
	Limit *openCodeModelLimit `json:"limit,omitempty"`
}

type openCodeModelLimit struct {
	Context int `json:"context,omitempty"`
	Output  int `json:"output,omitempty"`
}

func newOpenCodeConfigCommand(configPath *string) *cobra.Command {
	opts := openCodeGenerateOptions{
		Output:       "opencode.json",
		ProviderID:   "modelmux",
		ProviderName: "ModelMux",
	}

	cmd := &cobra.Command{
		Use:     "generate-opencode",
		Aliases: []string{"opencode"},
		Short:   "Generate an OpenCode provider config from ModelMux config",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfigForOpenCode(*configPath)
			if err != nil {
				return err
			}

			data, selectedModel, err := buildOpenCodeConfig(cfg, opts)
			if err != nil {
				return err
			}

			if opts.Output == "-" {
				_, err = cmd.OutOrStdout().Write(data)
				return err
			}
			if err := writeOpenCodeConfig(opts.Output, data, opts.Force); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "created", opts.Output)
			fmt.Fprintf(cmd.OutOrStdout(), "default model: %s/%s\n", strings.TrimSpace(opts.ProviderID), selectedModel)
			return nil
		},
	}

	cmd.Flags().StringVarP(&opts.Output, "output", "o", opts.Output, "output path, or - for stdout")
	cmd.Flags().StringVar(&opts.BaseURL, "base-url", "", "ModelMux OpenAI-compatible base URL (default: derived from server.host and server.port)")
	cmd.Flags().StringVar(&opts.ProviderID, "provider-id", opts.ProviderID, "OpenCode provider ID")
	cmd.Flags().StringVar(&opts.ProviderName, "provider-name", opts.ProviderName, "OpenCode provider display name")
	cmd.Flags().StringVar(&opts.DefaultModel, "default-model", "", "default model or group ID (default: first enabled group, then first enabled model)")
	cmd.Flags().StringVar(&opts.APIKeyEnv, "api-key-env", "", "environment variable used by OpenCode for the ModelMux proxy token")
	cmd.Flags().BoolVarP(&opts.Force, "force", "f", false, "overwrite an existing output file")
	return cmd
}

func loadConfigForOpenCode(path string) (*config.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %w", err)
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	if validationErrs := cfg.ValidateAll(); len(validationErrs) > 0 {
		return nil, fmt.Errorf("invalid ModelMux config: %s", validationErrs.Error())
	}
	return &cfg, nil
}

func buildOpenCodeConfig(cfg *config.Config, opts openCodeGenerateOptions) ([]byte, string, error) {
	providerID := strings.TrimSpace(opts.ProviderID)
	if providerID == "" {
		return nil, "", fmt.Errorf("provider ID cannot be empty")
	}
	if strings.Contains(providerID, "/") {
		return nil, "", fmt.Errorf("provider ID %q cannot contain /", providerID)
	}
	providerName := strings.TrimSpace(opts.ProviderName)
	if providerName == "" {
		providerName = "ModelMux"
	}

	baseURL := strings.TrimRight(strings.TrimSpace(opts.BaseURL), "/")
	if baseURL == "" {
		baseURL = openCodeBaseURL(cfg.Server)
	}

	models := make(map[string]openCodeModel)
	firstGroup := ""
	firstPhysicalModel := ""
	for _, group := range cfg.ModelGroups {
		if !group.Enabled {
			continue
		}
		name := strings.TrimSpace(group.Name)
		if name == "" {
			name = group.ID
		}
		model := openCodeModel{Name: name}
		if group.ContextWindow > 0 || group.MaxOutputTokens > 0 {
			model.Limit = &openCodeModelLimit{
				Context: group.ContextWindow,
				Output:  group.MaxOutputTokens,
			}
		}
		models[group.ID] = model
		if firstGroup == "" {
			firstGroup = group.ID
		}
	}
	for _, modelCfg := range cfg.Models {
		if !modelCfg.Enabled {
			continue
		}
		models[modelCfg.ID] = openCodeModel{Name: modelCfg.ID}
		if firstPhysicalModel == "" {
			firstPhysicalModel = modelCfg.ID
		}
	}
	if len(models) == 0 {
		return nil, "", fmt.Errorf("config has no enabled models or model groups")
	}

	selectedModel := strings.TrimSpace(opts.DefaultModel)
	if selectedModel == "" {
		selectedModel = firstGroup
		if selectedModel == "" {
			selectedModel = firstPhysicalModel
		}
	}
	selectedModel = strings.TrimPrefix(selectedModel, providerID+"/")
	if _, ok := models[selectedModel]; !ok {
		available := make([]string, 0, len(models))
		for id := range models {
			available = append(available, id)
		}
		sort.Strings(available)
		return nil, "", fmt.Errorf("default model %q is not enabled; available models: %s", selectedModel, strings.Join(available, ", "))
	}

	apiKeyEnv := strings.TrimSpace(opts.APIKeyEnv)
	if apiKeyEnv == "" && cfg.Server.RequireAuth {
		apiKeyEnv = strings.TrimSpace(cfg.Server.AuthTokenEnv)
	}
	providerOptions := openCodeProviderOptions{BaseURL: baseURL}
	if apiKeyEnv != "" {
		providerOptions.APIKey = "{env:" + apiKeyEnv + "}"
	}

	out := openCodeConfig{
		Schema: "https://opencode.ai/config.json",
		Model:  providerID + "/" + selectedModel,
		Provider: map[string]openCodeProvider{
			providerID: {
				NPM:     "@ai-sdk/openai-compatible",
				Name:    providerName,
				Options: providerOptions,
				Models:  models,
			},
		},
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, "", fmt.Errorf("encode OpenCode config: %w", err)
	}
	return append(data, '\n'), selectedModel, nil
}

func openCodeBaseURL(server config.ServerConfig) string {
	host := strings.TrimSpace(server.Host)
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(server.Port)) + "/v1"
}

func writeOpenCodeConfig(path string, data []byte, force bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("output path cannot be empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if !force {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if errors.Is(err, fs.ErrExist) {
				return fmt.Errorf("output file %s already exists; use --force to overwrite", path)
			}
			return fmt.Errorf("create output file: %w", err)
		}
		if _, err := file.Write(data); err != nil {
			_ = file.Close()
			_ = os.Remove(path)
			return fmt.Errorf("write output file: %w", err)
		}
		if err := file.Close(); err != nil {
			return fmt.Errorf("close output file: %w", err)
		}
		return nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write output file: %w", err)
	}
	return nil
}
