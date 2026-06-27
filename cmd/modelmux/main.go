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
