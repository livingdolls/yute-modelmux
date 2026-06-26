package main

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/livingdolls/yute-modelmux/internal/adapter/httpserver"
	"github.com/livingdolls/yute-modelmux/internal/adapter/tui"
	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/spf13/cobra"
)

func main() {
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

	rootCmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start the local proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			router := service.NewRouterService(cfg)
			srv := httpserver.New(router, cfg)

			ctx, cancel := context.WithCancel(cmd.Context())
			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = srv.Run(ctx)
			}()

			if err := tui.Run(cfg, router); err != nil {
				cancel()
				wg.Wait()
				return err
			}

			cancel()
			wg.Wait()
			return nil
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
			router := service.NewRouterService(cfg)
			return tui.Run(cfg, router)
		},
	})

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
