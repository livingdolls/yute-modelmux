package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
)

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) modelsHandler(w http.ResponseWriter, r *http.Request) {
	type modelItem struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	}
	items := make([]modelItem, 0, len(s.rs.ListModels()))
	for _, m := range s.rs.ListModels() {
		if !m.Enabled {
			continue
		}
		items = append(items, modelItem{ID: m.ID, Object: "model"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": items})
}

func (s *Server) chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	resp, err := s.rs.HandleChatCompletion(r.Context(), r)
	if err != nil {
		writeProxyError(w, err)
		return
	}
	defer resp.Body.Close()
	for k, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	type modelMetric struct {
		ID           string `json:"id"`
		Requests     int    `json:"requests"`
		Errors       int    `json:"errors"`
		ActiveKeys   int    `json:"active_keys"`
		CooldownKeys int    `json:"cooldown_keys"`
		InvalidKeys  int    `json:"invalid_keys"`
	}

	keys := s.rs.ListKeys()
	logs := s.rs.Logs()
	metrics := make([]modelMetric, 0, len(s.rs.ListModels()))
	for _, model := range s.rs.ListModels() {
		metric := modelMetric{ID: model.ID}
		for _, key := range keys {
			if key.ModelID != model.ID {
				continue
			}
			switch key.Status {
			case "active":
				metric.ActiveKeys++
			case "cooldown":
				metric.CooldownKeys++
			case "invalid":
				metric.InvalidKeys++
			}
		}
		for _, log := range logs {
			if log.ModelID != model.ID {
				continue
			}
			metric.Requests++
			if log.StatusCode >= 400 || log.Error != "" {
				metric.Errors++
			}
		}
		metrics = append(metrics, metric)
	}

	writeJSON(w, http.StatusOK, map[string]any{"models": metrics})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeProxyError(w http.ResponseWriter, err error) {
	var proxyErr *service.ProxyError
	if errors.As(err, &proxyErr) {
		writeJSON(w, proxyErr.HTTPStatus, map[string]any{"error": map[string]any{"message": proxyErr.Message, "type": proxyErr.Type, "code": proxyErr.Code}})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]any{"message": err.Error(), "type": "modelmux_error"}})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Server.RequireAuth {
			next.ServeHTTP(w, r)
			return
		}
		expected := s.cfg.AuthToken()
		if expected == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "server auth token is not configured"})
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+expected {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
