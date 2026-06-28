package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
)

func generateRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

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
	mux.HandleFunc("/v1/completions", s.completionsHandler)
	mux.HandleFunc("/metrics", s.metricsHandler)
	mux.HandleFunc("/logs", s.logsHandler)
	s.srv = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      s.requestIDMiddleware(s.authMiddleware(mux)),
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSecond) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSecond) * time.Second,
	}
	return s
}

func (s *Server) requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := generateRequestID()
		ctx := service.SetRequestID(r.Context(), reqID)
		r = r.WithContext(ctx)
		w.Header().Set("X-ModelMux-Request-ID", reqID)

		startedAt := time.Now()
		next.ServeHTTP(w, r)
		elapsed := time.Since(startedAt)

		if slog.Default().Enabled(ctx, slog.LevelInfo) {
			slog.InfoContext(ctx, "request",
				"req_id", reqID,
				"method", r.Method,
				"path", r.URL.Path,
				"remote_addr", r.RemoteAddr,
				"elapsed_ms", elapsed.Milliseconds(),
			)
		}
	})
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
