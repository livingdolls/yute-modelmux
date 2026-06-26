package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
)

type Server struct {
	rs  *service.RouterService
	cfg *config.Config
	mux *http.ServeMux
	srv *http.Server
}

func New(rs *service.RouterService, cfg *config.Config) *Server {
	mux := http.NewServeMux()
	s := &Server{rs: rs, cfg: cfg, mux: mux}
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/v1/models", s.modelsHandler)
	mux.HandleFunc("/v1/chat/completions", s.chatCompletionsHandler)
	mux.HandleFunc("/metrics", s.metricsHandler)
	s.srv = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      s.authMiddleware(mux),
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSecond) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSecond) * time.Second,
	}
	return s
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}
