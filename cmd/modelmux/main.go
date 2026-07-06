package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/adapter/httpserver"
	"github.com/livingdolls/yute-modelmux/internal/adapter/tui"
	"github.com/livingdolls/yute-modelmux/internal/ai"
	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/port/inbound"
	elib "github.com/livingdolls/yute-modelmux/internal/eval"
	plib "github.com/livingdolls/yute-modelmux/internal/prompt"
	"github.com/livingdolls/yute-modelmux/internal/secret"
	"github.com/livingdolls/yute-modelmux/internal/storage"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := newRootCommand().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	var configPath string

	rootCmd := &cobra.Command{
		Use:   "modelmux",
		Short: "ModelMux routes LLM requests across multiple API keys",
	}

	rootCmd.PersistentFlags().StringVar(&configPath, "config", config.DefaultConfigPath(), "config file path")

	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "modelmux %s\ncommit: %s\nbuilt: %s\n", version, commit, date)
		},
	})

	configCmd := &cobra.Command{Use: "config", Short: "Config helpers"}
	configCmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: "Create an example config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.WriteDefault(configPath); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "created", configPath)
			return nil
		},
	})

	var validateJSON bool
	var validateCheckProvider bool
	configValidateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate config file for correctness",
		Long: `Validate the configuration file for syntax, referential integrity, and
environment variable availability. Displays all errors found.

Exit code is non-zero if any validation errors are found.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(configPath)
			if err != nil {
				return fmt.Errorf("cannot read config file: %w", err)
			}
			var cfg config.Config
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return fmt.Errorf("invalid YAML: %w", err)
			}

			var allErrs []string
			resolveErrs := cfg.ResolveSecrets()
			if resolveErrs != nil {
				allErrs = append(allErrs, resolveErrs.Error())
			}
			valErrs := cfg.ValidateAll()
			allErrs = append(allErrs, valErrs...)

			if validateCheckProvider {
				for _, p := range cfg.Providers {
					if !p.Enabled || p.BaseURL == "" || p.ID == "" {
						continue
					}
					client := &http.Client{Timeout: 10 * time.Second}
					resp, err := client.Get(strings.TrimRight(p.BaseURL, "/") + "/health")
					if err != nil {
						allErrs = append(allErrs, fmt.Sprintf("provider %s unreachable: %s", p.ID, err))
					} else {
						resp.Body.Close()
						if resp.StatusCode >= 500 {
							allErrs = append(allErrs, fmt.Sprintf("provider %s returned %d", p.ID, resp.StatusCode))
						}
					}
				}
			}

			if len(allErrs) == 0 {
				if validateJSON {
					b, _ := json.MarshalIndent(map[string]any{"valid": true, "errors": []string{}}, "", "  ")
					fmt.Fprintln(cmd.OutOrStdout(), string(b))
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "config is valid")
				}
				return nil
			}

			if validateJSON {
				type jsonOutput struct {
					Valid  bool     `json:"valid"`
					Errors []string `json:"errors"`
				}
				out := jsonOutput{Valid: false, Errors: allErrs}
				b, _ := json.MarshalIndent(out, "", "  ")
				fmt.Fprintln(cmd.OutOrStdout(), string(b))
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), "config has", len(allErrs), "error(s):")
				for _, e := range allErrs {
					fmt.Fprintln(cmd.ErrOrStderr(), "  -", e)
				}
			}
			return fmt.Errorf("config validation failed with %d error(s)", len(allErrs))
		},
	}
	configValidateCmd.Flags().BoolVar(&validateJSON, "json", false, "output as JSON")
	configValidateCmd.Flags().BoolVar(&validateCheckProvider, "check-provider", false, "also check provider reachability")
	configCmd.AddCommand(configValidateCmd)
	rootCmd.AddCommand(configCmd)

	var keyTestID string
	keyCmd := &cobra.Command{Use: "key", Short: "Key management"}
	keyTestCmd := &cobra.Command{
		Use:   "test",
		Short: "Test an API key against its provider",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			router, err := service.NewRouterService(cfg)
			if err != nil {
				return err
			}
			if err := router.TestKey(cmd.Context(), keyTestID); err != nil {
				fmt.Fprintln(cmd.ErrOrStderr(), "FAIL:", err)
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "OK:", keyTestID)
			return nil
		},
	}
	keyTestCmd.Flags().StringVar(&keyTestID, "id", "", "key id to test")
	keyTestCmd.MarkFlagRequired("id")
	keyCmd.AddCommand(keyTestCmd)

	var keyID string
	keyEnableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable an API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateKeyStatus(configPath, keyID, "active", cmd.OutOrStdout())
		},
	}
	keyEnableCmd.Flags().StringVar(&keyID, "id", "", "key id to enable")
	keyEnableCmd.MarkFlagRequired("id")
	keyCmd.AddCommand(keyEnableCmd)

	keyDisableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable an API key",
		RunE: func(cmd *cobra.Command, args []string) error {
			return mutateKeyStatus(configPath, keyID, "disabled", cmd.OutOrStdout())
		},
	}
	keyDisableCmd.Flags().StringVar(&keyID, "id", "", "key id to disable")
	keyDisableCmd.MarkFlagRequired("id")
	keyCmd.AddCommand(keyDisableCmd)

	var (
		keyAddID         string
		keyAddProviderID string
		keyAddModelID    string
		keyAddName       string
		keyAddValueEnv   string
		keyAddPriority   int
	)
	keyAddCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new API key to config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if keyAddPriority <= 0 {
				keyAddPriority = 1
			}
			cfg.Keys = append(cfg.Keys, config.KeyConfig{
				ID:         keyAddID,
				ProviderID: keyAddProviderID,
				ModelID:    keyAddModelID,
				Name:       keyAddName,
				ValueEnv:   keyAddValueEnv,
				Status:     "active",
				Priority:   keyAddPriority,
			})
			if err := config.Save(configPath, cfg); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "added key", keyAddID)
			return nil
		},
	}
	keyAddCmd.Flags().StringVar(&keyAddID, "id", "", "key id")
	keyAddCmd.Flags().StringVar(&keyAddProviderID, "provider-id", "", "provider id")
	keyAddCmd.Flags().StringVar(&keyAddModelID, "model-id", "", "model id")
	keyAddCmd.Flags().StringVar(&keyAddName, "name", "", "display name")
	keyAddCmd.Flags().StringVar(&keyAddValueEnv, "value-env", "", "environment variable holding the API key value")
	keyAddCmd.Flags().IntVar(&keyAddPriority, "priority", 1, "priority (lower = higher)")
	keyAddCmd.MarkFlagRequired("id")
	keyAddCmd.MarkFlagRequired("provider-id")
	keyAddCmd.MarkFlagRequired("model-id")
	keyAddCmd.MarkFlagRequired("value-env")
	keyCmd.AddCommand(keyAddCmd)
	rootCmd.AddCommand(keyCmd)

	var (
		providerAddID         string
		providerAddName       string
		providerAddType       string
		providerAddBaseURL    string
		providerAddAuthType   string
		providerAddAuthHeader string
		providerAddTimeout    int
	)
	providerCmd := &cobra.Command{Use: "provider", Short: "Provider management"}
	providerAddCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new provider to config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if providerAddTimeout <= 0 {
				providerAddTimeout = 120
			}
			if providerAddType == "" {
				providerAddType = "openai-compatible"
			}
			if providerAddAuthType == "" {
				providerAddAuthType = "bearer"
			}
			cfg.Providers = append(cfg.Providers, config.ProviderConfig{
				ID:             providerAddID,
				Name:           providerAddName,
				Type:           providerAddType,
				BaseURL:        providerAddBaseURL,
				AuthType:       providerAddAuthType,
				AuthHeaderName: providerAddAuthHeader,
				TimeoutSeconds: providerAddTimeout,
				Enabled:        true,
			})
			if err := config.Save(configPath, cfg); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "added provider", providerAddID)
			return nil
		},
	}
	providerAddCmd.Flags().StringVar(&providerAddID, "id", "", "provider id")
	providerAddCmd.Flags().StringVar(&providerAddName, "name", "", "display name")
	providerAddCmd.Flags().StringVar(&providerAddType, "type", "openai-compatible", "provider type (openai-compatible, anthropic, gemini, custom)")
	providerAddCmd.Flags().StringVar(&providerAddBaseURL, "base-url", "", "provider base URL")
	providerAddCmd.Flags().StringVar(&providerAddAuthType, "auth-type", "bearer", "auth type (bearer, header)")
	providerAddCmd.Flags().StringVar(&providerAddAuthHeader, "auth-header", "", "custom auth header name")
	providerAddCmd.Flags().IntVar(&providerAddTimeout, "timeout", 120, "request timeout in seconds")
	providerAddCmd.MarkFlagRequired("id")
	providerAddCmd.MarkFlagRequired("base-url")
	providerCmd.AddCommand(providerAddCmd)
	rootCmd.AddCommand(providerCmd)

	var (
		modelAddID         string
		modelAddProviderID string
		modelAddModelName  string
		modelAddStrategy   string
	)
	modelCmd := &cobra.Command{Use: "model", Short: "Model management"}
	modelAddCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new model to config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if modelAddStrategy == "" {
				modelAddStrategy = "failover"
			}
			if modelAddModelName == "" {
				modelAddModelName = modelAddID
			}
			cfg.Models = append(cfg.Models, config.ModelConfig{
				ID:         modelAddID,
				ProviderID: modelAddProviderID,
				ModelName:  modelAddModelName,
				Strategy:   modelAddStrategy,
				Enabled:    true,
			})
			if err := config.Save(configPath, cfg); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "added model", modelAddID)
			return nil
		},
	}
	modelAddCmd.Flags().StringVar(&modelAddID, "id", "", "model id")
	modelAddCmd.Flags().StringVar(&modelAddProviderID, "provider-id", "", "provider id")
	modelAddCmd.Flags().StringVar(&modelAddModelName, "model-name", "", "upstream model name (defaults to id)")
	modelAddCmd.Flags().StringVar(&modelAddStrategy, "strategy", "failover", "rotation strategy (failover, round_robin, least_error)")
	modelAddCmd.MarkFlagRequired("id")
	modelAddCmd.MarkFlagRequired("provider-id")
	modelCmd.AddCommand(modelAddCmd)
	rootCmd.AddCommand(modelCmd)

	var (
		groupAddID       string
		groupAddName     string
		groupAddStrategy string
		groupAddMembers  []string
	)
	groupCmd := &cobra.Command{Use: "group", Short: "Model group management"}
	groupAddCmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new model group to config",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			if groupAddStrategy == "" {
				groupAddStrategy = "failover"
			}
			members := make([]config.ModelGroupMemberConfig, len(groupAddMembers))
			for i, m := range groupAddMembers {
				members[i] = config.ModelGroupMemberConfig{ModelID: m, Priority: i + 1, Weight: 1, Enabled: true}
			}
			cfg.ModelGroups = append(cfg.ModelGroups, config.ModelGroupConfig{
				ID:       groupAddID,
				Name:     groupAddName,
				Strategy: groupAddStrategy,
				Enabled:  true,
				Members:  members,
			})
			if err := config.Save(configPath, cfg); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "added group", groupAddID)
			return nil
		},
	}
	groupAddCmd.Flags().StringVar(&groupAddID, "id", "", "group id")
	groupAddCmd.Flags().StringVar(&groupAddName, "name", "", "display name")
	groupAddCmd.Flags().StringVar(&groupAddStrategy, "strategy", "failover", "group strategy (failover, round_robin, weighted)")
	groupAddCmd.Flags().StringSliceVar(&groupAddMembers, "members", nil, "model IDs (comma-separated)")
	groupAddCmd.MarkFlagRequired("id")
	groupAddCmd.MarkFlagRequired("members")
	groupCmd.AddCommand(groupAddCmd)
	rootCmd.AddCommand(groupCmd)

	var (
		secretRef   string
		secretValue string
	)
	secretCmd := &cobra.Command{Use: "secret", Short: "Secret store management"}
	secretSetCmd := &cobra.Command{
		Use:   "set",
		Short: "Store an API key value in the encrypted secret store",
		Long: `Store a secret value. If --value is not provided, you will be prompted
interactively with hidden input (safer than shell history).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			storePath := secretPath(cfg)
			s, err := secret.NewStore(storePath)
			if err != nil {
				return err
			}
			val := secretValue
			if val == "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Enter value for %s: ", secretRef)
				input, err := readPassword()
				if err != nil {
					return fmt.Errorf("failed to read input: %w", err)
				}
				val = strings.TrimSpace(input)
				if val == "" {
					return fmt.Errorf("secret value cannot be empty")
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
			if err := s.Set(secretRef, val); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "stored secret", secretRef)
			return nil
		},
	}
	secretSetCmd.Flags().StringVar(&secretRef, "ref", "", "secret reference name (used as keys[].secret_ref)")
	secretSetCmd.Flags().StringVar(&secretValue, "value", "", "API key value to store (omit for hidden prompt)")
	secretSetCmd.MarkFlagRequired("ref")
	secretCmd.AddCommand(secretSetCmd)

	secretListCmd := &cobra.Command{
		Use:   "list",
		Short: "List stored secret references",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			storePath := secretPath(cfg)
			s, err := secret.NewStore(storePath)
			if err != nil {
				return err
			}
			for _, ref := range s.List() {
				fmt.Fprintln(cmd.OutOrStdout(), ref)
			}
			return nil
		},
	}
	secretCmd.AddCommand(secretListCmd)

	var (
		secretDeleteRef string
	)
	secretDeleteCmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a secret from the store",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			storePath := secretPath(cfg)
			s, err := secret.NewStore(storePath)
			if err != nil {
				return err
			}
			if err := s.Delete(secretDeleteRef); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "deleted secret", secretDeleteRef)
			return nil
		},
	}
	secretDeleteCmd.Flags().StringVar(&secretDeleteRef, "ref", "", "secret reference to delete")
	secretDeleteCmd.MarkFlagRequired("ref")
	secretCmd.AddCommand(secretDeleteCmd)

	secretVerifyCmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify the secret store file can be decrypted",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			storePath := secretPath(cfg)
			if err := secret.VerifyFile(storePath); err != nil {
				return fmt.Errorf("secret store verification failed: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "secret store is valid")
			return nil
		},
	}
	secretCmd.AddCommand(secretVerifyCmd)

	var secretExportOutput string
	secretExportCmd := &cobra.Command{
		Use:   "export",
		Short: "Export encrypted secret store data (for backup)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			storePath := secretPath(cfg)
			s, err := secret.NewStore(storePath)
			if err != nil {
				return err
			}
			data, err := s.ExportData()
			if err != nil {
				return err
			}
			if secretExportOutput != "" {
				return os.WriteFile(secretExportOutput, data, 0o600)
			}
			_, err = cmd.OutOrStdout().Write(data)
			return err
		},
	}
	secretExportCmd.Flags().StringVar(&secretExportOutput, "output", "", "output file path (default: stdout)")
	secretCmd.AddCommand(secretExportCmd)

	var secretImportPath string
	secretImportCmd := &cobra.Command{
		Use:   "import",
		Short: "Import encrypted secret store data from backup",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			storePath := secretPath(cfg)
			data, err := io.ReadAll(cmd.InOrStdin())
			if err != nil {
				return err
			}
			if secretImportPath != "" {
				data, err = os.ReadFile(secretImportPath)
				if err != nil {
					return err
				}
			}
			if len(data) == 0 {
				return fmt.Errorf("no data to import")
			}
			if err := secret.ImportData(storePath, data); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "imported secret store")
			return nil
		},
	}
	secretImportCmd.Flags().StringVar(&secretImportPath, "input", "", "input file path (default: stdin)")
	secretCmd.AddCommand(secretImportCmd)

	secretRotateCmd := &cobra.Command{
		Use:   "rotate-master-key",
		Short: "Rotate the master encryption key",
		Long: `Re-encrypt the secret store with a new master key.
Set MODELMUX_MASTER_KEY to the current key and MODELMUX_NEW_MASTER_KEY
to the new key before running this command.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			newKey := os.Getenv("MODELMUX_NEW_MASTER_KEY")
			if newKey == "" {
				return fmt.Errorf("MODELMUX_NEW_MASTER_KEY environment variable is not set")
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}
			storePath := secretPath(cfg)
			s, err := secret.NewStore(storePath)
			if err != nil {
				return err
			}
			if err := s.RotateKey(newKey); err != nil {
				return fmt.Errorf("key rotation failed: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "master key rotated successfully")
			return nil
		},
	}
	secretCmd.AddCommand(secretRotateCmd)

	rootCmd.AddCommand(secretCmd)

	var jsonOutput bool
	var logLimit int
	var logModelID, logProviderID, logKeyID, logGroupID string
	var logStatusCode int
	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent request logs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			store, err := createStorage(cfg)
			if err != nil {
				return err
			}
			if store != nil {
				defer store.Close()
			}

			router, rerr := newRouterServiceWithSecret(cfg, store, nil)
			if rerr != nil {
				return rerr
			}

			filter := storage.LogFilter{
				ModelID:    logModelID,
				ProviderID: logProviderID,
				KeyID:      logKeyID,
				GroupID:    logGroupID,
				StatusCode: logStatusCode,
				Limit:      logLimit,
			}
			if filter.Limit <= 0 {
				filter.Limit = 20
			}
			logs, _ := router.QueryLogs(filter)

			if jsonOutput {
				type logItem struct {
					ID          string `json:"id"`
					ModelID     string `json:"model_id"`
					ProviderID  string `json:"provider_id"`
					KeyID       string `json:"key_id"`
					StatusCode  int    `json:"status_code"`
					Error       string `json:"error,omitempty"`
					LatencyMs   int64  `json:"latency_ms"`
					TokenInput  int    `json:"token_input"`
					TokenOutput int    `json:"token_output"`
					CreatedAt   string `json:"created_at"`
				}
				items := make([]logItem, len(logs))
				for i, l := range logs {
					createdAt := ""
					if !l.CreatedAt.IsZero() {
						createdAt = l.CreatedAt.Format(time.RFC3339)
					}
					items[i] = logItem{
						ID:          l.ID,
						ModelID:     l.ModelID,
						ProviderID:  l.ProviderID,
						KeyID:       l.KeyID,
						StatusCode:  l.StatusCode,
						Error:       l.Error,
						LatencyMs:   l.LatencyMs,
						TokenInput:  l.TokenInput,
						TokenOutput: l.TokenOutput,
						CreatedAt:   createdAt,
					}
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(items)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-24s %-12s %-6s %-10s %s\n", "STATUS", "MODEL", "PROVIDER", "CODE", "LATENCY", "ERROR")
			fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 80))
			for _, l := range logs {
				status := "OK"
				if l.Error != "" || l.StatusCode >= 400 {
					status = "ERROR"
				}
				if l.StatusCode == http.StatusTooManyRequests {
					status = "RATE429"
				}
				latency := fmt.Sprintf("%dms", l.LatencyMs)
				errStr := l.Error
				if len(errStr) > 40 {
					errStr = errStr[:40]
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-24s %-12s %-6d %-10s %s\n", status, l.ModelID, l.ProviderID, l.StatusCode, latency, errStr)
			}
			return nil
		},
	}
	logsCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
	logsCmd.Flags().IntVar(&logLimit, "limit", 20, "Limit number of logs")
	logsCmd.Flags().StringVar(&logModelID, "model-id", "", "Filter by model ID")
	logsCmd.Flags().StringVar(&logProviderID, "provider-id", "", "Filter by provider ID")
	logsCmd.Flags().StringVar(&logKeyID, "key-id", "", "Filter by key ID")
	logsCmd.Flags().StringVar(&logGroupID, "group-id", "", "Filter by group ID")
	logsCmd.Flags().IntVar(&logStatusCode, "status-code", 0, "Filter by status code")
	rootCmd.AddCommand(logsCmd)

	var readOnlyJSON bool
	makeReadOnlyCmd := func(use, short string, runFn func(cmd *cobra.Command, cfg *config.Config)) *cobra.Command {
		cmd := &cobra.Command{
			Use:   use,
			Short: short,
			RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
				if err != nil {
					return err
				}
				runFn(cmd, cfg)
				return nil
			},
		}
		cmd.Flags().BoolVar(&readOnlyJSON, "json", false, "Output in JSON format")
		return cmd
	}

	rootCmd.AddCommand(makeReadOnlyCmd("providers", "List configured providers", func(cmd *cobra.Command, cfg *config.Config) {
		if readOnlyJSON {
			type item struct {
				ID             string `json:"id"`
				Name           string `json:"name"`
				Type           string `json:"type"`
				BaseURL        string `json:"base_url"`
				AuthType       string `json:"auth_type"`
				AuthHeaderName string `json:"auth_header_name,omitempty"`
				TimeoutSeconds int    `json:"timeout_seconds"`
				Enabled        bool   `json:"enabled"`
			}
			items := make([]item, len(cfg.Providers))
			for i, p := range cfg.Providers {
				items[i] = item{ID: p.ID, Name: p.Name, Type: p.Type, BaseURL: p.BaseURL, AuthType: p.AuthType, AuthHeaderName: p.AuthHeaderName, TimeoutSeconds: p.TimeoutSeconds, Enabled: p.Enabled}
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(items)
			return
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %-20s %-12s %-8s %s\n", "ID", "NAME", "TYPE", "AUTH", "ENABLED", "BASE URL")
		fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 120))
		for _, p := range cfg.Providers {
			enabled := "no"
			if p.Enabled {
				enabled = "yes"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %-20s %-12s %-8s %s\n", p.ID, p.Name, p.Type, p.AuthType, enabled, p.BaseURL)
		}
	}))

	rootCmd.AddCommand(makeReadOnlyCmd("models", "List configured models", func(cmd *cobra.Command, cfg *config.Config) {
		if readOnlyJSON {
			type item struct {
				ID         string `json:"id"`
				ProviderID string `json:"provider_id"`
				ModelName  string `json:"model_name"`
				Strategy   string `json:"strategy"`
				Enabled    bool   `json:"enabled"`
			}
			items := make([]item, len(cfg.Models))
			for i, m := range cfg.Models {
				items[i] = item{ID: m.ID, ProviderID: m.ProviderID, ModelName: m.ModelName, Strategy: m.Strategy, Enabled: m.Enabled}
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(items)
			return
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %-20s %-14s %-8s\n", "ID", "PROVIDER", "MODEL NAME", "STRATEGY", "ENABLED")
		fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 90))
		for _, m := range cfg.Models {
			enabled := "no"
			if m.Enabled {
				enabled = "yes"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %-20s %-14s %-8s\n", m.ID, m.ProviderID, m.ModelName, m.Strategy, enabled)
		}
	}))

	rootCmd.AddCommand(makeReadOnlyCmd("groups", "List model groups", func(cmd *cobra.Command, cfg *config.Config) {
		if readOnlyJSON {
			type member struct {
				ModelID  string `json:"model_id"`
				KeyID    string `json:"key_id,omitempty"`
				Priority int    `json:"priority"`
				Weight   int    `json:"weight"`
				Enabled  bool   `json:"enabled"`
			}
			type item struct {
				ID       string   `json:"id"`
				Name     string   `json:"name"`
				Strategy string   `json:"strategy"`
				Enabled  bool     `json:"enabled"`
				Members  []member `json:"members"`
			}
			items := make([]item, len(cfg.ModelGroups))
			for i, g := range cfg.ModelGroups {
				members := make([]member, len(g.Members))
				for j, m := range g.Members {
					members[j] = member{ModelID: m.ModelID, KeyID: m.KeyID, Priority: m.Priority, Weight: m.Weight, Enabled: m.Enabled}
				}
				items[i] = item{ID: g.ID, Name: g.Name, Strategy: g.Strategy, Enabled: g.Enabled, Members: members}
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(items)
			return
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %-14s %-8s %s\n", "ID", "NAME", "STRATEGY", "ENABLED", "MEMBERS")
		fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 90))
		for _, g := range cfg.ModelGroups {
			enabled := "no"
			if g.Enabled {
				enabled = "yes"
			}
			memberStrs := make([]string, len(g.Members))
			for j, m := range g.Members {
				status := ""
				if !m.Enabled {
					status = " (disabled)"
				}
				ref := "model:" + m.ModelID
				if m.KeyID != "" {
					ref = "key:" + m.KeyID
				}
				memberStrs[j] = fmt.Sprintf("%s%s", ref, status)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %-14s %-8s %s\n", g.ID, g.Name, g.Strategy, enabled, strings.Join(memberStrs, ", "))
		}
	}))

	rootCmd.AddCommand(makeReadOnlyCmd("keys", "List configured API keys", func(cmd *cobra.Command, cfg *config.Config) {
		if readOnlyJSON {
			type item struct {
				ID         string `json:"id"`
				ProviderID string `json:"provider_id"`
				ModelID    string `json:"model_id"`
				Name       string `json:"name"`
				ValueEnv   string `json:"value_env,omitempty"`
				Status     string `json:"status"`
				Priority   int    `json:"priority"`
			}
			items := make([]item, len(cfg.Keys))
			for i, k := range cfg.Keys {
				items[i] = item{ID: k.ID, ProviderID: k.ProviderID, ModelID: k.ModelID, Name: k.Name, ValueEnv: k.ValueEnv, Status: k.Status, Priority: k.Priority}
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(items)
			return
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %-15s %-10s %-8s %s\n", "ID", "PROVIDER", "MODEL", "STATUS", "PRI", "NAME")
		fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 90))
		for _, k := range cfg.Keys {
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-20s %-15s %-10s %-8d %s\n", k.ID, k.ProviderID, k.ModelID, k.Status, k.Priority, k.Name)
		}
	}))
	rootCmd.AddCommand(aiCommands(&configPath))
	rootCmd.AddCommand(promptCommands(&configPath))
	rootCmd.AddCommand(chatCommand(&configPath))
	rootCmd.AddCommand(evalCommands(&configPath))

	rootCmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the local proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			if cfg.Server.Host == "0.0.0.0" && !cfg.Server.RequireAuth {
				fmt.Fprintln(cmd.ErrOrStderr(), "WARNING: server bound to 0.0.0.0 without authentication enabled.")
				fmt.Fprintln(cmd.ErrOrStderr(), "Anyone on the network can use your API keys. Set server.require_auth=true and server.auth_token_env.")
			}

			store, err := createStorage(cfg)
			if err != nil {
				return err
			}
			if store != nil {
				defer store.Close()
			}

			secStore, err := createSecretStore(cfg)
			if err != nil {
				return err
			}

			router, rerr := newRouterServiceWithSecret(cfg, store, secStore)
			if rerr != nil {
				return rerr
			}
			srv := httpserver.New(router, cfg)
			srv.SetConfigPath(configPath)

			healthChecker := service.NewHealthChecker(router, cfg.HealthCheck)
			srv.SetHealthChecker(healthChecker)
			healthChecker.Start(cmd.Context())
			defer healthChecker.Stop()

			return srv.Run(cmd.Context())
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "tui",
		Short: "Open the terminal dashboard",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			store, err := createStorage(cfg)
			if err != nil {
				return err
			}
			if store != nil {
				defer store.Close()
			}

			secStore, err := createSecretStore(cfg)
			if err != nil {
				return err
			}

			router, rerr := newRouterServiceWithSecret(cfg, store, secStore)
			if rerr != nil {
				return rerr
			}
			return tui.Run(tui.Options{
				ConfigPath: configPath,
				Config:     cfg,
				Router:     router,
				SaveConfig: func(next *config.Config) error {
					return config.Save(configPath, next)
				},
				ReloadRouter: func(next *config.Config) (inbound.RouterService, error) {
					if err := next.ResolveSecrets(); err != nil {
						return nil, err
					}
					if err := next.Validate(); err != nil {
						return nil, err
					}
					r, rerr := newRouterServiceWithSecret(next, store, secStore)
					if rerr != nil {
						return nil, rerr
					}
					return r, nil
				},
			})
		},
	})

	return rootCmd
}

func mutateKeyStatus(configPath, keyID, status string, w io.Writer) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	found := false
	for i := range cfg.Keys {
		if cfg.Keys[i].ID == keyID {
			cfg.Keys[i].Status = status
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("key %s not found", keyID)
	}
	if err := config.Save(configPath, cfg); err != nil {
		return err
	}
	fmt.Fprintf(w, "key %s set to %s\n", keyID, status)
	return nil
}

func secretPath(cfg *config.Config) string {
	path := cfg.Storage.Path
	if path == "" {
		path = config.Default().Storage.Path
	}
	path = expandHome(path)
	dir := strings.TrimSuffix(path, "modelmux.db")
	if dir == path {
		dir = path + ".d"
	}
	return dir + "secrets.enc"
}

func createSecretStore(cfg *config.Config) (*secret.Store, error) {
	if os.Getenv("MODELMUX_MASTER_KEY") == "" {
		return nil, nil
	}
	return secret.NewStore(secretPath(cfg))
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func createStorage(cfg *config.Config) (storage.Storage, error) {
	if cfg.Storage.Type != "sqlite" {
		return nil, nil
	}
	path := cfg.Storage.Path
	if path == "" {
		path = config.Default().Storage.Path
	}
	return storage.New(expandHome(path))
}

func newRouterServiceWithSecret(cfg *config.Config, store storage.Storage, secStore *secret.Store) (*service.RouterService, error) {
	if store != nil || secStore != nil {
		return service.NewRouterServiceWithSecret(cfg, store, secStore)
	}
	return service.NewRouterService(cfg)
}

func createFullRouter(cfg *config.Config) (*service.RouterService, storage.Storage, *secret.Store, error) {
	store, err := createStorage(cfg)
	if err != nil {
		return nil, nil, nil, err
	}
	secStore, err := createSecretStore(cfg)
	if err != nil {
		if store != nil {
			store.Close()
		}
		return nil, nil, nil, err
	}
	router, err := service.NewRouterServiceWithSecret(cfg, store, secStore)
	if err != nil {
		if store != nil {
			store.Close()
		}
		return nil, nil, nil, err
	}
	return router, store, secStore, nil
}

func readPassword() (string, error) {
	fd := int(syscall.Stdin)
	bytePassword, err := term.ReadPassword(fd)
	if err != nil {
		return "", err
	}
	return string(bytePassword), nil
}

func aiCommands(configPath *string) *cobra.Command {
	var classifyFile string
	var explainRequestID string

	aiCmd := &cobra.Command{Use: "ai", Short: "AI diagnostics commands"}

	classifyCmd := &cobra.Command{
		Use:   "classify",
		Short: "Classify a request file using the heuristic classifier",
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(classifyFile)
			if err != nil {
				return err
			}
			c := ai.NewClassifier()
			profile := c.Classify(data)
			fmt.Fprintf(cmd.OutOrStdout(), "Task: %s\n", profile.TaskClass)
			fmt.Fprintf(cmd.OutOrStdout(), "Prompt size: %d\n", profile.PromptSize)
			fmt.Fprintf(cmd.OutOrStdout(), "System prompt: %v\n", profile.HasSystemPrompt)
			fmt.Fprintf(cmd.OutOrStdout(), "Tools: %v\n", profile.HasToolDefinition)
			fmt.Fprintf(cmd.OutOrStdout(), "Vision: %v\n", profile.HasImageContent)
			fmt.Fprintf(cmd.OutOrStdout(), "Streaming: %v\n", profile.IsStreaming)
			if len(profile.DetectedCaps) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Capabilities: %s\n", strings.Join(profile.DetectedCaps, ", "))
			}
			return nil
		},
	}
	classifyCmd.Flags().StringVar(&classifyFile, "file", "", "path to request JSON file")
	classifyCmd.MarkFlagRequired("file")
	aiCmd.AddCommand(classifyCmd)

	explainCmd := &cobra.Command{
		Use:   "explain",
		Short: "Explain the route trace for a request ID",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			store, err := createStorage(cfg)
			if err != nil {
				return err
			}
			if store == nil {
				return fmt.Errorf("storage must be enabled (set storage.type: sqlite)")
			}
			defer store.Close()

			trace, err := store.GetRouteTraceByRequestID(explainRequestID)
			if err != nil {
				return err
			}
			if trace == nil {
				return fmt.Errorf("no trace found for request %s", explainRequestID)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Trace ID: %s\n", trace.ID)
			fmt.Fprintf(cmd.OutOrStdout(), "Request ID: %s\n", trace.RequestID)
			fmt.Fprintf(cmd.OutOrStdout(), "Original model: %s\n", trace.OriginalModel)
			fmt.Fprintf(cmd.OutOrStdout(), "Rerouted model: %s\n", trace.ReroutedModel)
			fmt.Fprintf(cmd.OutOrStdout(), "Created: %s\n", trace.CreatedAt)
			if trace.StepsJSON != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Steps: %s\n", trace.StepsJSON)
			}
			return nil
		},
	}
	explainCmd.Flags().StringVar(&explainRequestID, "request-id", "", "request ID to explain")
	explainCmd.MarkFlagRequired("request-id")
	aiCmd.AddCommand(explainCmd)

	doctorCmd := &cobra.Command{
		Use:   "doctor-config",
		Short: "Validate AI configuration and show diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			aiCfg := cfg.AI
			fmt.Fprintf(cmd.OutOrStdout(), "AI enabled: %v\n", aiCfg.Enabled)
			fmt.Fprintf(cmd.OutOrStdout(), "Classifier enabled: %v\n", aiCfg.Classifier.Enabled)
			fmt.Fprintf(cmd.OutOrStdout(), "Classifier mode: %s\n", aiCfg.Classifier.Mode)
			fmt.Fprintf(cmd.OutOrStdout(), "Guardrails enabled: %v\n", aiCfg.Guardrails.Enabled)
			fmt.Fprintf(cmd.OutOrStdout(), "Max prompt chars: %d\n", aiCfg.Guardrails.MaxPromptChars)
			fmt.Fprintf(cmd.OutOrStdout(), "Route trace enabled: %v\n", aiCfg.RouteTrace.Enabled)
			fmt.Fprintf(cmd.OutOrStdout(), "Trace response header: %v\n", aiCfg.RouteTrace.IncludeResponseHeader)
			if !aiCfg.Enabled {
				fmt.Fprintln(cmd.OutOrStdout(), "\nAI features are disabled. Set ai.enabled: true to enable.")
			}
			return nil
		},
	}
	aiCmd.AddCommand(doctorCmd)

	return aiCmd
}

func promptCommands(configPath *string) *cobra.Command {
	var promptModel string

	cmd := &cobra.Command{Use: "prompt", Short: "Prompt template management"}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available prompt templates",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			dir := cfg.AI.Prompts.Dir
			if dir == "" {
				return fmt.Errorf("ai.prompts.dir is not configured")
			}
			loader := plib.NewLoader(dir)
			templates, err := loader.LoadAll()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %s\n", "NAME", "MODEL", "DESCRIPTION")
			fmt.Fprintln(cmd.OutOrStdout(), strings.Repeat("-", 80))
			for _, tmpl := range templates {
				model := tmpl.Model
				if model == "" {
					model = "(default)"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-20s %-15s %s\n", tmpl.Name, model, tmpl.Description)
			}
			return nil
		},
	}
	cmd.AddCommand(listCmd)

	showCmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show a prompt template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			dir := cfg.AI.Prompts.Dir
			if dir == "" {
				return fmt.Errorf("ai.prompts.dir is not configured")
			}
			loader := plib.NewLoader(dir)
			tmpl, err := loader.FindByName(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\n", tmpl.Name)
			fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", tmpl.Description)
			if tmpl.Model != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Model: %s\n", tmpl.Model)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "System: %s\n", tmpl.System)
			fmt.Fprintf(cmd.OutOrStdout(), "Template: %s\n", tmpl.Template)
			return nil
		},
	}
	cmd.AddCommand(showCmd)

	runCmd := &cobra.Command{
		Use:   "run <name>",
		Short: "Run a prompt template through the router",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			dir := cfg.AI.Prompts.Dir
			if dir == "" {
				return fmt.Errorf("ai.prompts.dir is not configured")
			}
			loader := plib.NewLoader(dir)
			tmpl, err := loader.FindByName(args[0])
			if err != nil {
				return err
			}

			input, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			if len(input) == 0 {
				return fmt.Errorf("no input data; pipe content to stdin")
			}

			rendered, err := tmpl.Render(string(input))
			if err != nil {
				return err
			}

			model := rendered.Model
			if model == "" {
				model = promptModel
			}
			if model == "" {
				return fmt.Errorf("no model specified; use --model flag or set model in template")
			}

			router, store, _, err := createFullRouter(cfg)
			if err != nil {
				return err
			}
			if store != nil {
				defer store.Close()
			}
			body := buildChatBody(model, rendered.System, rendered.User)
			req, _ := http.NewRequestWithContext(cmd.Context(), http.MethodPost, "http://localhost/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := router.HandleChatCompletion(cmd.Context(), req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			io.Copy(cmd.OutOrStdout(), resp.Body)
			return nil
		},
	}
	runCmd.Flags().StringVar(&promptModel, "model", "", "model ID or group to route request through")
	cmd.AddCommand(runCmd)

	return cmd
}

func buildChatBody(model, system, user string) string {
	if system != "" {
		return fmt.Sprintf(`{"model":%s,"messages":[{"role":"system","content":%s},{"role":"user","content":%s}]}`,
			jsonString(model), jsonString(system), jsonString(user))
	}
	return fmt.Sprintf(`{"model":%s,"messages":[{"role":"user","content":%s}]}`, jsonString(model), jsonString(user))
}

func jsonString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}

func chatCommand(configPath *string) *cobra.Command {
	var sessionName string
	var chatModel string

	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Chat with a model through the router",
		RunE: func(cmd *cobra.Command, args []string) error {
			if chatModel == "" {
				return fmt.Errorf("--model is required")
			}
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}

			input, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			if len(bytes.TrimSpace(input)) == 0 {
				return fmt.Errorf("no input; pipe a message to stdin")
			}

			body := fmt.Sprintf(`{"model":%s,"messages":[{"role":"user","content":%s}]}`, jsonString(chatModel), jsonString(string(bytes.TrimSpace(input))))
			router, store, _, err := createFullRouter(cfg)
			if err != nil {
				return err
			}
			if store != nil {
				defer store.Close()
			}

			req, _ := http.NewRequestWithContext(cmd.Context(), http.MethodPost, "http://localhost/v1/chat/completions", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := router.HandleChatCompletion(cmd.Context(), req)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}

			if sessionName != "" && store != nil {
				id, err := store.SaveChatSession(sessionName, chatModel)
				if err == nil {
					_ = store.SaveChatMessage(id, "user", string(bytes.TrimSpace(input)))
					_ = store.SaveChatMessage(id, "assistant", string(respBody))
				}
			}

			fmt.Fprintln(cmd.OutOrStdout(), string(respBody))
			return nil
		},
	}
	cmd.Flags().StringVar(&sessionName, "session", "", "session name for persistence (requires SQLite)")
	cmd.Flags().StringVar(&chatModel, "model", "", "model ID or group to use")
	cmd.MarkFlagRequired("model")
	return cmd
}

func evalCommands(configPath *string) *cobra.Command {
	var runOutput string
	var reportJSON bool

	cmd := &cobra.Command{Use: "eval", Short: "Evaluation harness commands"}

	runCmd := &cobra.Command{
		Use:   "run <suite.yaml>",
		Short: "Run an eval suite against configured models",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			router, store, _, err := createFullRouter(cfg)
			if err != nil {
				return err
			}
			if store != nil {
				defer store.Close()
			}
			suite, err := elib.LoadSuite(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Running suite %q (%d targets x %d cases)...\n",
				suite.Name, len(suite.Targets), len(suite.Cases))

			result, err := elib.RunSuite(c.Context(), suite, router)
			if err != nil {
				return err
			}

			passed := 0
			failed := 0
			for _, r := range result.Results {
				if r.Error == "" && r.StatusCode >= 200 && r.StatusCode < 400 {
					passed++
				} else {
					failed++
				}
			}
			fmt.Fprintf(c.OutOrStdout(), "Done: %d passed, %d failed (%.2fs)\n",
				passed, failed, result.FinishedAt.Sub(result.StartedAt).Seconds())

			if runOutput != "" {
				data, _ := yaml.Marshal(result)
				os.WriteFile(runOutput, data, 0o644)
				fmt.Fprintf(c.OutOrStdout(), "Saved to %s\n", runOutput)
			}

			if store != nil {
				_ = store.SaveEvalRun(storage.EvalRunRecord{
					ID: result.RunID, SuiteName: result.SuiteName,
					StartedAt: result.StartedAt.Format(time.RFC3339),
					FinishedAt: result.FinishedAt.Format(time.RFC3339),
					TotalCases: len(result.Results),
				})
				for _, r := range result.Results {
					_ = store.SaveEvalResult(storage.EvalResultRecord{
						RunID: result.RunID, CaseName: r.CaseName,
						TargetModel: r.TargetModel, TargetGroup: r.TargetGroup,
						StatusCode: r.StatusCode, LatencyMs: r.LatencyMs,
						ResponseHash: r.ResponseHash, Error: r.Error,
					})
				}
			}
			return nil
		},
	}
	runCmd.Flags().StringVar(&runOutput, "output", "", "save report to YAML file")
	cmd.AddCommand(runCmd)

	reportCmd := &cobra.Command{
		Use:   "report <run-id>",
		Short: "Show eval run report",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			cfg, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			store, err := createStorage(cfg)
			if err != nil {
				return err
			}
			if store == nil {
				return fmt.Errorf("storage required for eval reports")
			}
			defer store.Close()

			results, err := store.GetEvalResults(args[0])
			if err != nil {
				return err
			}
			if len(results) == 0 {
				return fmt.Errorf("no results for run %s", args[0])
			}

			if reportJSON {
				enc := json.NewEncoder(c.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(results)
			}

			fmt.Fprintf(c.OutOrStdout(), "%-12s %-20s %-6s %-10s %s\n", "CASE", "TARGET", "STATUS", "LATENCY", "ERROR")
			fmt.Fprintln(c.OutOrStdout(), strings.Repeat("-", 100))
			for _, r := range results {
				target := r.TargetModel
				if target == "" {
					target = r.TargetGroup
				}
				latency := fmt.Sprintf("%dms", r.LatencyMs)
				errMsg := r.Error
				if len(errMsg) > 50 {
					errMsg = errMsg[:50] + "..."
				}
				fmt.Fprintf(c.OutOrStdout(), "%-12s %-20s %-6d %-10s %s\n", r.CaseName, target, r.StatusCode, latency, errMsg)
			}
			return nil
		},
	}
	reportCmd.Flags().BoolVar(&reportJSON, "json", false, "output as JSON")
	cmd.AddCommand(reportCmd)

	return cmd
}
