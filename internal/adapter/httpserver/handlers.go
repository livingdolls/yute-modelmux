package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/livingdolls/yute-modelmux/internal/app/service"
	"github.com/livingdolls/yute-modelmux/internal/core/domain"
)

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) modelsHandler(w http.ResponseWriter, r *http.Request) {
	type modelItem struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	}
	models := s.rs.ListModels()
	groups := s.rs.ListModelGroups()
	items := make([]modelItem, 0, len(models)+len(groups))
	for _, m := range models {
		if !m.Enabled {
			continue
		}
		items = append(items, modelItem{ID: m.ID, Object: "model"})
	}
	for _, g := range groups {
		if !g.Enabled {
			continue
		}
		items = append(items, modelItem{ID: g.ID, Object: "model"})
	}
	writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": items})
}

func (s *Server) chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	maxBytes := int64(s.cfg.Server.MaxRequestBodyMB) * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
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
	_ = copyWithFlush(w, resp.Body)
}

func copyWithFlush(dst io.Writer, src io.Reader) error {
	flusher, canFlush := dst.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, werr := dst.Write(buf[:n]); werr != nil {
				return werr
			}
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
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
	type groupMetric struct {
		ID                string `json:"id"`
		Requests          int    `json:"requests"`
		Errors            int    `json:"errors"`
		ActiveModels      int    `json:"active_models"`
		UnavailableModels int    `json:"unavailable_models"`
	}
	keys := s.rs.ListKeys()
	logs := s.rs.Logs()
	models := s.rs.ListModels()
	groups := s.rs.ListModelGroups()
	metrics := make([]modelMetric, 0, len(models))
	for _, model := range models {
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
	groupMetrics := make([]groupMetric, 0, len(groups))
	for _, group := range groups {
		metric := groupMetric{ID: group.ID}
		for _, member := range group.Members {
			active := false
			for _, model := range models {
				if model.ID == member.ModelID && model.Enabled && member.Enabled {
					active = modelHasActiveKey(keys, model.ID)
					break
				}
			}
			if active {
				metric.ActiveModels++
			} else {
				metric.UnavailableModels++
			}
		}
		for _, log := range logs {
			if log.GroupID != group.ID {
				continue
			}
			metric.Requests++
			if log.StatusCode >= 400 || log.Error != "" {
				metric.Errors++
			}
		}
		groupMetrics = append(groupMetrics, metric)
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": metrics, "groups": groupMetrics})
}

func modelHasActiveKey(keys []domain.APIKey, modelID string) bool {
	now := time.Now()
	for _, key := range keys {
		if key.ModelID != modelID {
			continue
		}
		if key.Status == domain.KeyStatusDisabled || key.Status == domain.KeyStatusInvalid {
			continue
		}
		if key.Status == domain.KeyStatusCooldown && key.CooldownEnd != nil && key.CooldownEnd.After(now) {
			continue
		}
		return true
	}
	return false
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
