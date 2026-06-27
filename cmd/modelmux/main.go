package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/adapter/httpserver"
	"github.com/livingdolls/yute-modelmux/internal/adapter/tui"
	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/core/port/inbound"
	"github.com/livingdolls/yute-modelmux/internal/storage"
	"github.com/spf13/cobra"
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
			router := service.NewRouterService(cfg)
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
	rootCmd.AddCommand(keyCmd)

	var jsonOutput bool
	var logLimit int
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

			router := newRouterServiceWithStorage(cfg, store)
			logs := router.Logs()
			sort.SliceStable(logs, func(i, j int) bool {
				return logs[i].CreatedAt.After(logs[j].CreatedAt)
			})

			if logLimit > 0 && logLimit < len(logs) {
				logs = logs[:logLimit]
			}

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
					members[j] = member{ModelID: m.ModelID, Priority: m.Priority, Weight: m.Weight, Enabled: m.Enabled}
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
				memberStrs[j] = fmt.Sprintf("%s%s", m.ModelID, status)
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

			router := newRouterServiceWithStorage(cfg, store)
			srv := httpserver.New(router, cfg)
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

			router := newRouterServiceWithStorage(cfg, store)
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
					return newRouterServiceWithStorage(next, store), nil
				},
			})
		},
	})

	return rootCmd
}

func createStorage(cfg *config.Config) (storage.Storage, error) {
	if cfg.Storage.Type != "sqlite" {
		return nil, nil
	}
	path := cfg.Storage.Path
	if path == "" {
		path = config.Default().Storage.Path
	}
	return storage.New(path)
}

func newRouterServiceWithStorage(cfg *config.Config, store storage.Storage) *service.RouterService {
	if store != nil {
		return service.NewRouterServiceWithStorage(cfg, store)
	}
	return service.NewRouterService(cfg)
}
