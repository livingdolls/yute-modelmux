package main

import (
	"fmt"
	"os"

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
