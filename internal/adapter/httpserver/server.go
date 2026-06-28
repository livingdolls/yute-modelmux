package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/config"
	"github.com/livingdolls/yute-modelmux/internal/secret"
	"github.com/livingdolls/yute-modelmux/internal/storage"
)

func generateRequestID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type Server struct {
	rs           *service.RouterService
	rsMu         sync.RWMutex
	cfg          *config.Config
	cfgMu        sync.RWMutex
	configPath   string
	store        storage.Storage
	retiredStores []storage.Storage
	retiredMu    sync.Mutex
	mux          *http.ServeMux
	srv          *http.Server
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
	mux.HandleFunc("POST /admin/reload", s.adminReloadHandler)
	mux.HandleFunc("POST /admin/keys/{id}/enable", s.adminEnableKeyHandler)
	mux.HandleFunc("POST /admin/keys/{id}/disable", s.adminDisableKeyHandler)
	mux.HandleFunc("POST /admin/keys/{id}/test", s.adminTestKeyHandler)
	mux.HandleFunc("GET /admin/status", s.adminStatusHandler)
	s.srv = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      s.requestIDMiddleware(s.authMiddleware(mux)),
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSecond) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSecond) * time.Second,
	}
	return s
}

func (s *Server) SetConfigPath(path string) { s.configPath = path }

func (s *Server) loadRouterService() *service.RouterService {
	s.rsMu.RLock()
	defer s.rsMu.RUnlock()
	return s.rs
}

func (s *Server) loadConfig() *config.Config {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.cfg
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

func resolveSecretPath(cfg *config.Config) string {
	dbPath := cfg.Storage.Path
	if dbPath == "" {
		dbPath = config.Default().Storage.Path
	}
	dbPath = expandHome(dbPath)
	dir := strings.TrimSuffix(dbPath, "modelmux.db")
	if dir == dbPath {
		dir = dbPath + ".d"
	}
	return dir + "secrets.enc"
}

func (s *Server) adminReloadHandler(w http.ResponseWriter, r *http.Request) {
	path := s.configPath
	if path == "" {
		path = config.DefaultConfigPath()
	}
	cfg, err := config.Load(path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to load config: " + err.Error()})
		return
	}

	var newStore storage.Storage
	if cfg.Storage.Type == "sqlite" {
		storePath := cfg.Storage.Path
		if storePath == "" {
			storePath = config.Default().Storage.Path
		}
		storePath = expandHome(storePath)
		newStore, err = storage.New(storePath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to open storage: " + err.Error()})
			return
		}
	}

	var secStore *secret.Store
	if os.Getenv("MODELMUX_MASTER_KEY") != "" {
		secStore, err = secret.NewStore(resolveSecretPath(cfg))
		if err != nil {
			if newStore != nil {
				newStore.Close()
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to open secret store: " + err.Error()})
			return
		}
	}

	newRS, err := service.NewRouterServiceWithSecret(cfg, newStore, secStore)
	if err != nil {
		if newStore != nil {
			newStore.Close()
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to create router: " + err.Error()})
		return
	}

	s.rsMu.Lock()
	oldStore := s.store
	s.rs = newRS
	s.store = newStore
	s.rsMu.Unlock()

	s.cfgMu.Lock()
	s.cfg = cfg
	s.cfgMu.Unlock()

	if oldStore != nil {
		s.retiredMu.Lock()
		s.retiredStores = append(s.retiredStores, oldStore)
		s.retiredMu.Unlock()
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": "reloaded"})
}

func (s *Server) adminEnableKeyHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing key id"})
		return
	}
	path := s.configPath
	if path == "" {
		path = config.DefaultConfigPath()
	}
	cfg, err := config.Load(path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to load config: " + err.Error()})
		return
	}
	found := false
	for i := range cfg.Keys {
		if cfg.Keys[i].ID == id {
			cfg.Keys[i].Status = "active"
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "key not found"})
		return
	}
	if err := config.Save(path, cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save config: " + err.Error()})
		return
	}
	rs := s.loadRouterService()
	rs.SetKeyStatus(id, "active")
	writeJSON(w, http.StatusOK, map[string]any{"status": "enabled", "id": id})
}

func (s *Server) adminDisableKeyHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing key id"})
		return
	}
	path := s.configPath
	if path == "" {
		path = config.DefaultConfigPath()
	}
	cfg, err := config.Load(path)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to load config: " + err.Error()})
		return
	}
	found := false
	for i := range cfg.Keys {
		if cfg.Keys[i].ID == id {
			cfg.Keys[i].Status = "disabled"
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "key not found"})
		return
	}
	if err := config.Save(path, cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to save config: " + err.Error()})
		return
	}
	rs := s.loadRouterService()
	rs.SetKeyStatus(id, "disabled")
	writeJSON(w, http.StatusOK, map[string]any{"status": "disabled", "id": id})
}

func (s *Server) adminTestKeyHandler(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "missing key id"})
		return
	}
	rs := s.loadRouterService()
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := rs.TestKey(ctx, id); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"status": "failed", "id": id, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "success", "id": id})
}

func (s *Server) adminStatusHandler(w http.ResponseWriter, r *http.Request) {
	rs := s.loadRouterService()
	providers := rs.ListProviders()
	models := rs.ListModels()
	groups := rs.ListModelGroups()
	keys := rs.ListKeys()

	cooldownCount := 0
	invalidCount := 0
	limitedCount := 0
	disabledCount := 0
	for _, k := range keys {
		switch k.Status {
		case "cooldown":
			cooldownCount++
		case "invalid":
			invalidCount++
		case "limited":
			limitedCount++
		case "disabled":
			disabledCount++
		}
	}

	type keySummary struct {
		ID         string `json:"id"`
		ModelID    string `json:"model_id"`
		Status     string `json:"status"`
		Priority   int    `json:"priority"`
		UsedToday  int    `json:"used_today"`
	}

	var keySummaries []keySummary
	for _, k := range keys {
		keySummaries = append(keySummaries, keySummary{
			ID:        k.ID,
			ModelID:   k.ModelID,
			Status:    string(k.Status),
			Priority:  k.Priority,
			UsedToday: k.DailyRequestCount,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"providers":   len(providers),
		"models":      len(models),
		"groups":      len(groups),
		"keys_total":  len(keys),
		"keys_active": len(keys) - cooldownCount - invalidCount - limitedCount - disabledCount,
		"keys_cooldown": cooldownCount,
		"keys_invalid":  invalidCount,
		"keys_limited":  limitedCount,
		"keys_disabled": disabledCount,
		"keys":          keySummaries,
	})
}

func isLocalAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	switch host {
	case "127.0.0.1", "::1", "localhost":
		return true
	}
	return false
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
		shutdownErr := s.srv.Shutdown(shutdownCtx)
		s.closeRetired()
		return shutdownErr
	case err := <-errCh:
		if err == http.ErrServerClosed {
			s.closeRetired()
			return nil
		}
		return err
	}
}

func (s *Server) closeRetired() {
	s.retiredMu.Lock()
	defer s.retiredMu.Unlock()
	for _, store := range s.retiredStores {
		store.Close()
	}
	s.retiredStores = nil
}
